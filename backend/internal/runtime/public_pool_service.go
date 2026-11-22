package runtime

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/charger"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/persistence"
)

var (
	ErrRefreshQuotaExhausted     = errors.New("free refresh quota exhausted")
	ErrPublicPoolUnavailable     = errors.New("public cookie pool unavailable")
	ErrPersonalCookieRequired    = errors.New("personal cookie required")
	ErrDailyProbeLimitReached    = errors.New("daily pile probe limit reached")
	ErrPublicPoolCookieDuplicate = persistence.ErrPublicPoolCookieDuplicate
)

var shanghaiLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}()

func quotaTodayString(now time.Time) string {
	return now.In(shanghaiLoc).Format("2006-01-02")
}

func nextQuotaResetAt(now time.Time) time.Time {
	local := now.In(shanghaiLoc)
	y, mo, d := local.Date()
	return time.Date(y, mo, d+1, 0, 0, 0, 0, shanghaiLoc).UTC()
}

func (r *UserRuntime) isPersonalCookieExpired() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.personalCookieExpired
}

func (r *UserRuntime) setPersonalCookieExpired(expired bool) {
	r.mu.Lock()
	r.personalCookieExpired = expired
	r.mu.Unlock()
}

func (m *Manager) hasValidPersonalCookie(runtime *UserRuntime) bool {
	if runtime == nil {
		return false
	}
	return runtime.client.HasPersonalCookie() && !runtime.isPersonalCookieExpired()
}

func (m *Manager) userJoinedHealthyPool(userID string) bool {
	item, err := m.repository.GetUserPublicPoolCookie(userID)
	if err != nil || item == nil {
		return false
	}
	return item.Enabled && item.Status == "healthy"
}

func (m *Manager) hasUnlimitedPrivilege(userID string, runtime *UserRuntime) bool {
	if !m.hasValidPersonalCookie(runtime) {
		return false
	}
	if m.Settings().RequirePoolShareForUnlimited {
		return m.userJoinedHealthyPool(userID)
	}
	return true
}

func (m *Manager) CanUseWatchReminders(userID string) bool {
	if m.Settings().FreeUserWatchAllowed {
		return true
	}
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return false
	}
	return m.hasUnlimitedPrivilege(userID, runtime)
}

func (m *Manager) ListPublicPoolCookies() ([]model.PublicPoolCookie, error) {
	return m.repository.ListPublicPoolCookies()
}

func (m *Manager) CreatePublicPoolCookie(req model.PublicPoolCookieCreateRequest) (model.PublicPoolCookie, error) {
	label := strings.TrimSpace(req.Label)
	if label == "" {
		return model.PublicPoolCookie{}, fmt.Errorf("请填写公共 Cookie 备注名")
	}
	if len(label) > 64 {
		return model.PublicPoolCookie{}, fmt.Errorf("备注名不能超过 64 个字符")
	}
	cookie, err := charger.ExtractCookie(req.Cookie)
	if err != nil {
		return model.PublicPoolCookie{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	id := randomID("pool")
	return m.repository.CreatePublicPoolCookie(persistence.PublicPoolCookieRecord{
		PublicPoolCookie: model.PublicPoolCookie{
			ID:      id,
			Label:   label,
			Enabled: enabled,
			Status:  "healthy",
		},
		Cookie: cookie,
	})
}

func (m *Manager) UpdatePublicPoolCookie(id string, req model.PublicPoolCookieUpdateRequest) (model.PublicPoolCookie, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.PublicPoolCookie{}, fmt.Errorf("cookie id is required")
	}
	if req.Label != nil {
		trimmed := strings.TrimSpace(*req.Label)
		if trimmed == "" || len(trimmed) > 64 {
			return model.PublicPoolCookie{}, fmt.Errorf("备注名长度需要在 1-64 个字符之间")
		}
		req.Label = &trimmed
	}
	if req.Cookie != nil && strings.TrimSpace(*req.Cookie) != "" {
		existing, err := m.repository.GetPublicPoolCookie(id)
		if err != nil {
			return model.PublicPoolCookie{}, err
		}
		if existing != nil && existing.OwnerUserID != "" {
			return model.PublicPoolCookie{}, fmt.Errorf("用户互助 Cookie 仅可由用户本人在账号页更新，管理员仅可修改备注或启停状态")
		}
		extracted, err := charger.ExtractCookie(*req.Cookie)
		if err != nil {
			return model.PublicPoolCookie{}, err
		}
		req.Cookie = &extracted
	}
	return m.repository.UpdatePublicPoolCookie(id, req.Label, req.Cookie, req.Enabled)
}

func (m *Manager) DeletePublicPoolCookie(id string) error {
	return m.repository.DeletePublicPoolCookie(strings.TrimSpace(id))
}

func (m *Manager) markPersonalCookieExpired(userID string, runtime *UserRuntime, now time.Time) error {
	if runtime != nil {
		runtime.setPersonalCookieExpired(true)
		runtime.recordFailure(true)
	}
	m.recordMetric(userID, "cookie_error")
	_ = m.repository.MarkUserPublicPoolCookieExpired(userID, "个人 Cookie 远端鉴权失效")
	return m.notifyCredentialExpired(userID, now)
}

func (m *Manager) handlePoolCookieExpired(rec persistence.PublicPoolCookieRecord, errMsg string, now time.Time) {
	_ = m.repository.MarkPublicPoolCookieResult(rec.ID, true, errMsg)
	if rec.OwnerUserID != "" {
		if ownerRT, rtErr := m.runtimeFor(rec.OwnerUserID); rtErr == nil {
			ownerRT.setPersonalCookieExpired(true)
			ownerRT.recordFailure(true)
			m.recordMetric(rec.OwnerUserID, "cookie_error")
			_ = m.notifyCredentialExpired(rec.OwnerUserID, now)
			_ = m.Save()
		}
	}
}

func (m *Manager) TestPublicPoolCookie(id string) (model.PublicPoolCookie, error) {
	rec, err := m.repository.GetPublicPoolCookie(strings.TrimSpace(id))
	if err != nil {
		return model.PublicPoolCookie{}, err
	}
	probeDeviceID := m.pickProbeDeviceID("")
	if probeDeviceID == "" {
		return model.PublicPoolCookie{}, fmt.Errorf("暂无可用于测试的充电桩，请先收录至少 1 台充电桩")
	}
	client := m.poolClient()
	_, fetchErr := client.FetchPileWithCookie(probeDeviceID, rec.Cookie)
	if fetchErr == nil {
		_ = m.repository.MarkPublicPoolCookieResult(rec.ID, false, "")
	} else if charger.IsAuthExpired(fetchErr) {
		m.handlePoolCookieExpired(*rec, "实测返回鉴权失效", time.Now().UTC())
		if avail := m.PublicPoolAvailableCount(); avail <= 1 {
			m.notifyAdminsPublicPoolDegraded(avail)
		}
	} else {
		_ = m.repository.MarkPublicPoolCookieResult(rec.ID, false, fetchErr.Error())
		return model.PublicPoolCookie{}, fetchErr
	}
	updated, err := m.repository.GetPublicPoolCookie(rec.ID)
	if err != nil {
		return model.PublicPoolCookie{}, err
	}
	return updated.PublicPoolCookie, nil
}

func (m *Manager) TestPersonalCookie(userID string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	if !runtime.client.HasPersonalCookie() {
		return model.DashboardSnapshot{}, ErrPersonalCookieRequired
	}
	probeDeviceID := m.pickProbeDeviceID(userID)
	if probeDeviceID == "" {
		return model.DashboardSnapshot{}, fmt.Errorf("暂无可用于测试的充电桩，请先添加至少 1 台充电桩")
	}
	now := time.Now().UTC()
	pile, fetchErr := runtime.client.FetchPileWithCookie(probeDeviceID, runtime.client.Cookie())
	if fetchErr != nil {
		if charger.IsAuthExpired(fetchErr) {
			_ = m.markPersonalCookieExpired(userID, runtime, now)
			_ = m.Save()
			return m.attachQuota(userID, runtime.store.Snapshot()), charger.ErrAuthExpired
		}
		return model.DashboardSnapshot{}, fetchErr
	}
	runtime.setPersonalCookieExpired(false)
	runtime.recordRemoteOK(now)
	if runtimeOwnsPile(runtime, pile.ID) {
		runtime.store.MergeCapturePiles([]model.Pile{pile})
	}
	m.recordSharedPiles(userID, []model.Pile{pile})
	_ = m.resolveCredentialExpired(userID, now)
	if err := m.Save(); err != nil {
		return model.DashboardSnapshot{}, err
	}
	return m.attachQuota(userID, runtime.store.Snapshot()), nil
}

func (m *Manager) pickProbeDeviceID(userID string) string {
	if userID != "" {
		if rt, err := m.runtimeFor(userID); err == nil {
			if ids := rt.client.DeviceIDs(); len(ids) > 0 {
				return ids[0]
			}
		}
	}
	if shared, err := m.repository.SearchSharedPiles("", 1); err == nil && len(shared) > 0 {
		return shared[0].DeviceID
	}
	return ""
}

func (m *Manager) PublicPoolAvailableCount() int {
	count, err := m.repository.CountHealthyPublicPoolCookies()
	if err != nil {
		return 0
	}
	return count
}

func (m *Manager) poolClient() *charger.Client {
	return charger.NewClientTemplateOnly(m.requests)
}

func (m *Manager) nextPoolStart(n int) int {
	if n <= 1 {
		return 0
	}
	m.poolMu.Lock()
	idx := int(m.poolCursor % uint64(n))
	m.poolCursor++
	m.poolMu.Unlock()
	return idx
}

func (m *Manager) fetchPileViaPublicPool(deviceID string) (model.Pile, error) {
	cookies, err := m.repository.ListHealthyPublicPoolCookies()
	if err != nil {
		return model.Pile{}, err
	}
	if len(cookies) == 0 {
		m.notifyAdminsPublicPoolDegraded(0)
		return model.Pile{}, ErrPublicPoolUnavailable
	}
	client := m.poolClient()
	start := m.nextPoolStart(len(cookies))
	remainingHealthy := len(cookies)
	var lastErr error
	for i := 0; i < len(cookies); i++ {
		rec := cookies[(start+i)%len(cookies)]
		pile, fetchErr := client.FetchPileWithCookie(deviceID, rec.Cookie)
		if fetchErr == nil {
			_ = m.repository.MarkPublicPoolCookieResult(rec.ID, false, "")
			return pile, nil
		}
		if charger.IsAuthExpired(fetchErr) {
			m.handlePoolCookieExpired(rec, "远端返回鉴权失效", time.Now().UTC())
			remainingHealthy--
			if remainingHealthy <= 1 {
				m.notifyAdminsPublicPoolDegraded(remainingHealthy)
			}
			lastErr = fetchErr
			continue
		}
		_ = m.repository.MarkPublicPoolCookieResult(rec.ID, false, fetchErr.Error())
		return model.Pile{}, fetchErr
	}
	if lastErr != nil {
		return model.Pile{}, ErrPublicPoolUnavailable
	}
	return model.Pile{}, ErrPublicPoolUnavailable
}

func (m *Manager) resolveDeviceIDViaPublicPool(number string) (string, error) {
	cookies, err := m.repository.ListHealthyPublicPoolCookies()
	if err != nil {
		return "", err
	}
	if len(cookies) == 0 {
		m.notifyAdminsPublicPoolDegraded(0)
		return "", ErrPublicPoolUnavailable
	}
	client := m.poolClient()
	start := m.nextPoolStart(len(cookies))
	remainingHealthy := len(cookies)
	var lastErr error
	for i := 0; i < len(cookies); i++ {
		rec := cookies[(start+i)%len(cookies)]
		id, resolveErr := client.ResolveDeviceIDWithCookie(number, rec.Cookie)
		if resolveErr == nil {
			_ = m.repository.MarkPublicPoolCookieResult(rec.ID, false, "")
			return id, nil
		}
		if charger.IsAuthExpired(resolveErr) {
			m.handlePoolCookieExpired(rec, "远端返回鉴权失效", time.Now().UTC())
			remainingHealthy--
			if remainingHealthy <= 1 {
				m.notifyAdminsPublicPoolDegraded(remainingHealthy)
			}
			lastErr = resolveErr
			continue
		}
		return "", resolveErr
	}
	if lastErr != nil {
		return "", ErrPublicPoolUnavailable
	}
	return "", ErrPublicPoolUnavailable
}

// notifyAdminsPublicPoolDegraded sends an in-app + WxPusher notification to admin users
// at most once every 30 minutes when the public cookie pool drops to <= 1 healthy entry.
func (m *Manager) notifyAdminsPublicPoolDegraded(remaining int) {
	now := time.Now().UTC()
	m.poolMu.Lock()
	if !m.poolAlertSentAt.IsZero() && now.Sub(m.poolAlertSentAt) < 30*time.Minute {
		m.poolMu.Unlock()
		return
	}
	m.poolAlertSentAt = now
	m.poolMu.Unlock()

	m.mu.RLock()
	adminIDs := make([]string, 0, 2)
	for id, u := range m.users {
		if u.Role == "admin" && u.Enabled {
			adminIDs = append(adminIDs, id)
		}
	}
	m.mu.RUnlock()

	msg := fmt.Sprintf("公共 Cookie 池当前仅剩 %d 个可用凭据，请尽快在管理后台补充或更新，以免影响免费用户刷新。", remaining)
	if remaining == 0 {
		msg = "公共 Cookie 池已全部失效（剩余 0 个可用），免费用户当前无法实时刷新，请尽快在管理后台更新公共 Cookie。"
	}
	for _, adminID := range adminIDs {
		_, _ = m.RecordNotification(model.Notification{
			UserID:    adminID,
			Type:      model.NotificationCredentialExpired,
			Severity:  "critical",
			Title:     "公共 Cookie 池告警",
			Message:   msg,
			DedupeKey: "public_cookie_pool_degraded",
			CreatedAt: now,
		})
	}
}

func (m *Manager) UserRefreshQuota(userID string) (model.UserRefreshQuota, error) {
	return m.userRefreshQuotaWithPoolCount(userID, m.PublicPoolAvailableCount())
}

func (m *Manager) userRefreshQuotaWithPoolCount(userID string, poolAvailable int) (model.UserRefreshQuota, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.UserRefreshQuota{}, err
	}
	settings := m.Settings()
	now := time.Now()
	today := quotaTodayString(now)
	resetAt := nextQuotaResetAt(now)
	cacheTTLSec := settings.SharedPileCacheTTLSeconds
	if cacheTTLSec <= 0 {
		cacheTTLSec = defaultSharedPileCacheTTL
	}
	rec, err := m.repository.GetUserQuota(userID, today)
	if err != nil {
		return model.UserRefreshQuota{}, err
	}
	dailyRem := settings.FreeDailyRefreshQuota - rec.DailyUsed
	if dailyRem < 0 {
		dailyRem = 0
	}
	hasPersonal := runtime.client.HasPersonalCookie()
	status := "none"
	if hasPersonal {
		if runtime.isPersonalCookieExpired() {
			status = "expired"
		} else {
			status = "valid"
		}
	}
	joinedPool := m.userJoinedHealthyPool(userID)
	unlimitedActive := status == "valid" && (!settings.RequirePoolShareForUnlimited || joinedPool)
	return model.UserRefreshQuota{
		HasPersonalCookie:            hasPersonal,
		PersonalCookieStatus:         status,
		JoinedPublicPool:             joinedPool,
		RequirePoolShareForUnlimited: settings.RequirePoolShareForUnlimited,
		UnlimitedActive:              unlimitedActive,
		FreeDailyLimit:               settings.FreeDailyRefreshQuota,
		FreeDailyUsed:                rec.DailyUsed,
		FreeDailyRemaining:           dailyRem,
		BonusRemaining:               rec.BonusRemaining,
		TotalFreeRemaining:           dailyRem + rec.BonusRemaining,
		DailyResetAt:                 &resetAt,
		SharedPileCacheTTLSeconds:    cacheTTLSec,
		ContributedPileCount:         rec.ContributedPileCount,
		PileContributionBonusQuota:   settings.PileContributionBonusQuota,
		FreeUserWatchAllowed:         settings.FreeUserWatchAllowed,
		CookieGuideURL:               settings.CookieGuideURL,
		PublicPoolAvailable:          poolAvailable,
	}, nil
}

func (m *Manager) attachQuota(userID string, snapshot model.DashboardSnapshot) model.DashboardSnapshot {
	if quota, err := m.UserRefreshQuota(userID); err == nil {
		snapshot.Quota = &quota
	}
	return snapshot
}

func (m *Manager) SearchSharedPiles(query string, limit int) ([]model.SharedPile, error) {
	return m.repository.SearchSharedPiles(query, limit)
}

func (m *Manager) AdminUpdateSharedPile(deviceID, name, address string) (*model.SharedPile, error) {
	trimmedName := strings.TrimSpace(name)
	trimmedAddr := strings.TrimSpace(address)
	if trimmedName == "" {
		return nil, fmt.Errorf("充电桩名称不能为空")
	}
	if len(trimmedName) > 128 || len(trimmedAddr) > 256 {
		return nil, fmt.Errorf("名称或地址长度超出限制")
	}
	return m.repository.UpdateSharedPileMeta(deviceID, trimmedName, trimmedAddr)
}

func (m *Manager) AdminRefreshSharedPile(deviceID string) (*model.SharedPile, error) {
	deviceID = strings.TrimSpace(deviceID)
	existing, err := m.repository.GetSharedPile(deviceID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, sql.ErrNoRows
	}
	pile, err := m.fetchPileViaPublicPool(deviceID)
	if err != nil {
		return nil, err
	}
	if existing.Name != "" && pile.Name == "" {
		pile.Name = existing.Name
	}
	if existing.Address != "" && pile.Address == "" {
		pile.Address = existing.Address
	}
	if _, err := m.repository.UpsertSharedPile(pile, existing.ContributedByUserID, time.Now().UTC()); err != nil {
		return nil, err
	}
	return m.repository.GetSharedPile(deviceID)
}

func (m *Manager) AdminDeleteSharedPile(deviceID string) error {
	return m.repository.DeleteSharedPile(deviceID)
}

func (m *Manager) AdminSetUserBonusQuota(userID string, bonus int) (model.UserRefreshQuota, error) {
	if bonus < 0 || bonus > 10000 {
		return model.UserRefreshQuota{}, fmt.Errorf("奖励刷新次数需要在 0 到 10000 之间")
	}
	if _, err := m.runtimeFor(userID); err != nil {
		return model.UserRefreshQuota{}, err
	}
	today := quotaTodayString(time.Now())
	if err := m.repository.SetUserBonusQuota(userID, today, bonus); err != nil {
		return model.UserRefreshQuota{}, err
	}
	return m.UserRefreshQuota(userID)
}

func (m *Manager) ClearPersonalCookie(userID string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	if err := runtime.client.UpdateCookie(""); err != nil {
		return model.DashboardSnapshot{}, err
	}
	_ = m.repository.DeleteUserPublicPoolCookie(userID)
	runtime.setPersonalCookieExpired(false)
	m.invalidateBackgroundCredentialValidation(userID)
	if err := m.Save(); err != nil {
		return model.DashboardSnapshot{}, err
	}
	m.wakeReminderScheduler()
	return m.attachQuota(userID, runtime.store.Snapshot()), nil
}

func (m *Manager) JoinPublicPoolWithExistingCookie(userID string) (model.DashboardSnapshot, error) {
	snapshot, err := m.TestPersonalCookie(userID)
	if err != nil {
		return snapshot, err
	}
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	if !m.hasValidPersonalCookie(runtime) {
		return model.DashboardSnapshot{}, ErrPersonalCookieRequired
	}
	cookie := runtime.client.Cookie()
	user, _ := m.User(userID)
	label := "用户互助 · " + user.Username
	if user.Username == "" {
		label = "用户互助 · " + userID
	}
	if _, err := m.repository.UpsertUserPublicPoolCookie(userID, label, cookie); err != nil {
		return model.DashboardSnapshot{}, err
	}
	_ = m.resolveCredentialExpired(userID, time.Now().UTC())
	return m.attachQuota(userID, runtime.store.Snapshot()), nil
}

func (m *Manager) LeavePublicPool(userID string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	if err := m.repository.DeleteUserPublicPoolCookie(userID); err != nil {
		return model.DashboardSnapshot{}, err
	}
	m.wakeReminderScheduler()
	return m.attachQuota(userID, runtime.store.Snapshot()), nil
}

func (m *Manager) sharedPileCacheTTL() time.Duration {
	sec := m.Settings().SharedPileCacheTTLSeconds
	if sec <= 0 {
		sec = defaultSharedPileCacheTTL
	}
	return time.Duration(sec) * time.Second
}

func (m *Manager) freshSharedPileSnapshot(deviceID string, now time.Time) (*model.Pile, bool) {
	pile, _, ok := m.freshSharedPileSnapshotWithTime(deviceID, now)
	return pile, ok
}

func (m *Manager) freshSharedPileSnapshotWithTime(deviceID string, now time.Time) (*model.Pile, time.Time, bool) {
	shared, err := m.repository.GetSharedPile(deviceID)
	if err != nil || shared == nil || shared.LastSnapshot == nil || shared.LastFetchedAt == nil {
		return nil, time.Time{}, false
	}
	if now.Sub(*shared.LastFetchedAt) >= m.sharedPileCacheTTL() {
		return nil, time.Time{}, false
	}
	clone := *shared.LastSnapshot
	return &clone, *shared.LastFetchedAt, true
}

func (m *Manager) recordSharedPiles(userID string, piles []model.Pile) {
	now := time.Now().UTC()
	for _, pile := range piles {
		if strings.TrimSpace(pile.ID) == "" {
			continue
		}
		_, _ = m.repository.UpsertSharedPile(pile, userID, now)
	}
}

// RefreshPile refreshes a single owned pile for the user.
// - Personal cookie users refresh directly (0 quota); if personal cookie expired,
//   automatically falls back to the public pool (1 credit if cache expired) and notifies user.
// - Free users hit the 60s global shared cache for free (0 credits); on cache miss,
//   consumes 1 credit and fetches via the public cookie pool.
func (m *Manager) RefreshPile(userID, deviceID string, force bool) (model.DashboardSnapshot, error) {
	deviceID = strings.TrimSpace(deviceID)
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	runtime.recordRequest()
	m.recordMetric(userID, "request")

	if !runtimeOwnsPile(runtime, deviceID) {
		return model.DashboardSnapshot{}, fmt.Errorf("未找到对应的充电桩")
	}
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		m.recordMetric(userID, "cache")
		snapshot := runtime.store.Snapshot()
		snapshot.Refresh.Cached = true
		snapshot.Refresh.Message = "管理员已暂停此账户的远端刷新，当前展示缓存数据"
		return m.attachQuota(userID, snapshot), nil
	}

	now := time.Now().UTC()
	fallbackFromExpiredPersonal := false

	// Path A: Active unlimited personal cookie privilege
	if m.hasUnlimitedPrivilege(userID, runtime) {
		result := runtime.client.FetchPile(deviceID, force)
		if len(result.Piles) > 0 {
			runtime.setPersonalCookieExpired(false)
			runtime.store.MergeCapturePiles(result.Piles)
			m.recordSharedPiles(userID, result.Piles)
			info := runtime.store.Snapshot().Refresh
			info.LastRemoteAt = &now
			info.AttemptedDevices = result.Attempted
			info.SuccessfulDevices = len(result.Piles)
			info.FailedDevices = len(result.Failures)
			info.Cached = false
			info.Partial = false
			info.UsedPool = false
			info.PersonalCookieFallback = false
			info.Message = "已使用个人 Cookie 刷新该充电桩"
			runtime.store.SetRefreshInfo(info)
			m.recordRefreshMetrics(userID, info)
			if err := m.saveAndRecordRemotePiles(userID, result.Piles); err != nil {
				return model.DashboardSnapshot{}, err
			}
			return m.attachQuota(userID, runtime.store.Snapshot()), nil
		}
		if !result.AuthExpired() {
			fetchErr := result.FirstError()
			if fetchErr == nil {
				fetchErr = fmt.Errorf("远端未返回该充电桩数据")
			}
			runtime.recordFailure(false)
			_ = m.Save()
			return model.DashboardSnapshot{}, fetchErr
		}
		// Personal cookie expired -> record notification & smoothly fall back to public pool!
		_ = m.markPersonalCookieExpired(userID, runtime, now)
		fallbackFromExpiredPersonal = true
	}

	// Path B: Public pool (free user or smooth fallback from expired personal cookie)
	if cachedPile, ok := m.freshSharedPileSnapshot(deviceID, now); ok {
		runtime.store.MergeCapturePiles([]model.Pile{*cachedPile})
		info := runtime.store.Snapshot().Refresh
		info.Cached = true
		info.UsedPool = true
		info.PersonalCookieFallback = fallbackFromExpiredPersonal
		if fallbackFromExpiredPersonal {
			info.Message = "个人 Cookie 已失效，已命中全站最近缓存（未消耗额度），请尽快更新个人 Cookie"
		} else {
			info.Message = "已同步全站最近更新的充电桩状态（未消耗刷新次数）"
		}
		runtime.store.SetRefreshInfo(info)
		m.recordMetric(userID, "cache")
		_ = m.Save()
		return m.attachQuota(userID, runtime.store.Snapshot()), nil
	}

	settings := m.Settings()
	today := quotaTodayString(now)
	receipt, ok, err := m.repository.ConsumeRefreshQuota(userID, today, settings.FreeDailyRefreshQuota, 1)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	if !ok {
		return model.DashboardSnapshot{}, ErrRefreshQuotaExhausted
	}

	pile, fetchErr := m.fetchPileViaPublicPool(deviceID)
	if fetchErr != nil {
		_ = m.repository.RefundRefreshQuota(userID, today, receipt)
		runtime.recordFailure(false)
		_ = m.Save()
		return model.DashboardSnapshot{}, fetchErr
	}

	runtime.store.MergeCapturePiles([]model.Pile{pile})
	m.recordSharedPiles(userID, []model.Pile{pile})
	info := runtime.store.Snapshot().Refresh
	info.LastRemoteAt = &now
	info.AttemptedDevices = 1
	info.SuccessfulDevices = 1
	info.FailedDevices = 0
	info.Cached = false
	info.Partial = false
	info.UsedPool = true
	info.PersonalCookieFallback = fallbackFromExpiredPersonal
	if fallbackFromExpiredPersonal {
		info.Message = "个人 Cookie 已失效，已自动降级使用公共池刷新（消耗 1 次额度），请及时更新个人 Cookie"
	} else {
		info.Message = "已通过公共池刷新该充电桩（消耗 1 次额度）"
	}
	runtime.store.SetRefreshInfo(info)
	m.recordRefreshMetrics(userID, info)
	if err := m.saveAndRecordPoolPiles(userID, []model.Pile{pile}); err != nil {
		return model.DashboardSnapshot{}, err
	}
	return m.attachQuota(userID, runtime.store.Snapshot()), nil
}

// refreshViaPublicPool refreshes all owned piles for a free/fallback user,
// serving any piles within the 60s shared cache for free and consuming 1 credit
// per expired pile up to the user's remaining quota.
func (m *Manager) refreshViaPublicPool(userID string, runtime *UserRuntime, fallbackFromExpiredPersonal bool) (model.DashboardSnapshot, error) {
	ids := runtime.client.DeviceIDs()
	if len(ids) == 0 {
		snapshot := runtime.store.Snapshot()
		snapshot.Refresh.Cached = true
		snapshot.Refresh.UsedPool = true
		snapshot.Refresh.Message = "当前还没有添加充电桩"
		return m.attachQuota(userID, snapshot), nil
	}

	now := time.Now().UTC()
	cachedPiles := make([]model.Pile, 0, len(ids))
	expiredIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if pile, ok := m.freshSharedPileSnapshot(id, now); ok {
			cachedPiles = append(cachedPiles, *pile)
		} else {
			expiredIDs = append(expiredIDs, id)
		}
	}

	if len(cachedPiles) > 0 {
		runtime.store.MergeCapturePiles(cachedPiles)
	}
	if len(expiredIDs) == 0 {
		info := runtime.store.Snapshot().Refresh
		info.Cached = true
		info.UsedPool = true
		info.PersonalCookieFallback = fallbackFromExpiredPersonal
		if fallbackFromExpiredPersonal {
			info.Message = "个人 Cookie 已失效，已命中全站最近缓存（未消耗额度），请尽快更新个人 Cookie"
		} else {
			info.Message = "已同步全站最近更新状态（未消耗刷新次数）"
		}
		runtime.store.SetRefreshInfo(info)
		m.recordMetric(userID, "cache")
		_ = m.Save()
		return m.attachQuota(userID, runtime.store.Snapshot()), nil
	}

	settings := m.Settings()
	today := quotaTodayString(now)
	quotaRec, err := m.repository.GetUserQuota(userID, today)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	dailyRem := settings.FreeDailyRefreshQuota - quotaRec.DailyUsed
	if dailyRem < 0 {
		dailyRem = 0
	}
	available := dailyRem + quotaRec.BonusRemaining
	if available <= 0 {
		return model.DashboardSnapshot{}, ErrRefreshQuotaExhausted
	}

	toFetch := expiredIDs
	partialByQuota := false
	if len(toFetch) > available {
		toFetch = expiredIDs[:available]
		partialByQuota = true
	}

	fetchedPiles := make([]model.Pile, 0, len(toFetch))
	var firstErr error
	for _, id := range toFetch {
		receipt, ok, consumeErr := m.repository.ConsumeRefreshQuota(userID, today, settings.FreeDailyRefreshQuota, 1)
		if consumeErr != nil {
			firstErr = consumeErr
			break
		}
		if !ok {
			partialByQuota = true
			break
		}
		pile, fetchErr := m.fetchPileViaPublicPool(id)
		if fetchErr != nil {
			_ = m.repository.RefundRefreshQuota(userID, today, receipt)
			if firstErr == nil {
				firstErr = fetchErr
			}
			if errors.Is(fetchErr, ErrPublicPoolUnavailable) {
				break
			}
			continue
		}
		fetchedPiles = append(fetchedPiles, pile)
	}

	if len(fetchedPiles) == 0 && len(cachedPiles) == 0 {
		if firstErr != nil {
			return model.DashboardSnapshot{}, firstErr
		}
		return model.DashboardSnapshot{}, ErrPublicPoolUnavailable
	}

	if len(fetchedPiles) > 0 {
		runtime.store.MergeCapturePiles(fetchedPiles)
		m.recordSharedPiles(userID, fetchedPiles)
	}

	info := runtime.store.Snapshot().Refresh
	info.LastRemoteAt = &now
	info.AttemptedDevices = len(toFetch)
	info.SuccessfulDevices = len(fetchedPiles)
	info.FailedDevices = len(toFetch) - len(fetchedPiles)
	info.Cached = false
	info.Partial = partialByQuota || info.FailedDevices > 0
	info.UsedPool = true
	info.PersonalCookieFallback = fallbackFromExpiredPersonal
	switch {
	case fallbackFromExpiredPersonal && partialByQuota:
		info.Message = fmt.Sprintf("个人 Cookie 已失效，已自动降级用公共池刷新 %d 个桩（剩余额度不足），请及时更新个人 Cookie", len(fetchedPiles))
	case fallbackFromExpiredPersonal:
		info.Message = fmt.Sprintf("个人 Cookie 已失效，已自动降级用公共池刷新 %d 个桩（消耗 %d 次额度），请及时更新个人 Cookie", len(fetchedPiles), len(fetchedPiles))
	case partialByQuota:
		info.Message = fmt.Sprintf("已刷新 %d 个桩，剩余额度不足，其余请上传个人 Cookie 或贡献新桩获取额度", len(fetchedPiles))
	default:
		info.Message = fmt.Sprintf("已通过公共池刷新 %d 个桩（消耗 %d 次额度）", len(fetchedPiles), len(fetchedPiles))
	}
	runtime.store.SetRefreshInfo(info)
	m.recordRefreshMetrics(userID, info)
	if err := m.saveAndRecordPoolPiles(userID, fetchedPiles); err != nil {
		return model.DashboardSnapshot{}, err
	}
	return m.attachQuota(userID, runtime.store.Snapshot()), nil
}

// AddPileWithCatalog adds a pile from the global shared catalog if already known,
// or probes ele.mocele.com for a brand-new pile and grants contribution bonus credits.
func (m *Manager) AddPileWithCatalog(userID string, req model.PileUpsertRequest) (model.PileCreateResponse, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.PileCreateResponse{}, err
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Number = strings.TrimSpace(req.Number)
	if req.ID == "" && req.Number == "" {
		return model.PileCreateResponse{}, fmt.Errorf("device id or pile number is required")
	}

	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		return model.PileCreateResponse{}, fmt.Errorf("管理员已暂停此账户的远端刷新，暂时无法添加新设备")
	}
	if user.DeviceLimit > 0 && len(runtime.client.DeviceIDs()) >= user.DeviceLimit {
		return model.PileCreateResponse{}, fmt.Errorf("当前账户最多添加 %d 台充电桩", user.DeviceLimit)
	}

	// Step 1: Check global shared catalog by device ID or pile number first.
	var shared *model.SharedPile
	if req.ID != "" {
		shared, _ = m.repository.GetSharedPile(req.ID)
	}
	if shared == nil && req.Number != "" {
		shared, _ = m.repository.FindSharedPileByNumber(req.Number)
	}
	if shared != nil {
		pile, addErr := m.attachSharedPileToUser(userID, runtime, shared, user.DeviceLimit, req)
		if addErr != nil {
			return model.PileCreateResponse{}, addErr
		}
		quota, _ := m.UserRefreshQuota(userID)
		return model.PileCreateResponse{
			Pile:              pile,
			FromSharedCatalog: true,
			RewardGranted:     0,
			Quota:             &quota,
		}, nil
	}

	// Step 2: Brand-new pile not yet in shared catalog -> probe upstream & grant reward on insert!
	now := time.Now().UTC()
	today := quotaTodayString(now)
	allowed, probeErr := m.repository.ConsumeDailyProbeQuota(userID, today, defaultDailyPileProbeLimit)
	if probeErr != nil {
		return model.PileCreateResponse{}, probeErr
	}
	if !allowed {
		return model.PileCreateResponse{}, ErrDailyProbeLimitReached
	}

	usePersonal := m.hasValidPersonalCookie(runtime)
	if req.ID == "" && req.Number != "" {
		var resolvedID string
		var resolveErr error
		if usePersonal {
			runtime.refreshMu.Lock()
			resolvedID, resolveErr = runtime.client.ResolveDeviceIDByNumber(req.Number)
			runtime.refreshMu.Unlock()
			if resolveErr != nil && charger.IsAuthExpired(resolveErr) {
				_ = m.markPersonalCookieExpired(userID, runtime, now)
				_ = m.Save()
				usePersonal = false
			}
		}
		if resolvedID == "" && !usePersonal {
			resolvedID, resolveErr = m.resolveDeviceIDViaPublicPool(req.Number)
		}
		if resolveErr != nil {
			return model.PileCreateResponse{}, resolveErr
		}
		req.ID = resolvedID

		// Re-check shared catalog by resolved device ID (in case number was empty earlier).
		if existing, _ := m.repository.GetSharedPile(req.ID); existing != nil {
			pile, addErr := m.attachSharedPileToUser(userID, runtime, existing, user.DeviceLimit, req)
			if addErr != nil {
				return model.PileCreateResponse{}, addErr
			}
			quota, _ := m.UserRefreshQuota(userID)
			return model.PileCreateResponse{
				Pile:              pile,
				FromSharedCatalog: true,
				RewardGranted:     0,
				Quota:             &quota,
			}, nil
		}
	}

	var fetchedPile model.Pile
	var fetchErr error
	if usePersonal {
		fetchedPile, fetchErr = runtime.client.FetchPileWithCookie(req.ID, runtime.client.Cookie())
		if fetchErr != nil && charger.IsAuthExpired(fetchErr) {
			_ = m.markPersonalCookieExpired(userID, runtime, now)
			_ = m.Save()
			usePersonal = false
		}
	}
	if !usePersonal {
		fetchedPile, fetchErr = m.fetchPileViaPublicPool(req.ID)
	}
	if fetchErr != nil {
		return model.PileCreateResponse{}, fetchErr
	}
	if fetchedPile.Number == "" && req.Number != "" {
		fetchedPile.Number = req.Number
	}

	inserted, upsertErr := m.repository.UpsertSharedPile(fetchedPile, userID, now)
	if upsertErr != nil {
		return model.PileCreateResponse{}, upsertErr
	}

	rewardGranted := 0
	if inserted {
		bonus := m.Settings().PileContributionBonusQuota
		if err := m.repository.GrantContributionBonus(userID, today, bonus); err == nil {
			rewardGranted = bonus
		}
	}

	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	if err := runtime.client.AddDeviceWithLimit(req.ID, user.DeviceLimit); err != nil {
		return model.PileCreateResponse{}, err
	}
	if strings.TrimSpace(req.Name) != "" {
		fetchedPile.Name = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.Address) != "" {
		fetchedPile.Address = strings.TrimSpace(req.Address)
	}
	runtime.store.MergeCapturePiles([]model.Pile{fetchedPile})
	if err := m.Save(); err != nil {
		return model.PileCreateResponse{}, err
	}
	pile := fetchedPile
	for _, candidate := range runtime.store.Snapshot().Piles {
		if candidate.ID == req.ID {
			pile = candidate
			break
		}
	}
	quota, _ := m.UserRefreshQuota(userID)
	return model.PileCreateResponse{
		Pile:              pile,
		FromSharedCatalog: !inserted,
		RewardGranted:     rewardGranted,
		Quota:             &quota,
	}, nil
}

func (m *Manager) attachSharedPileToUser(
	userID string,
	runtime *UserRuntime,
	shared *model.SharedPile,
	deviceLimit int,
	req model.PileUpsertRequest,
) (model.Pile, error) {
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	if err := runtime.client.AddDeviceWithLimit(shared.DeviceID, deviceLimit); err != nil {
		return model.Pile{}, err
	}
	var pile model.Pile
	if shared.LastSnapshot != nil {
		pile = *shared.LastSnapshot
	} else {
		pile = model.Pile{
			ID:      shared.DeviceID,
			Number:  shared.Number,
			Name:    shared.Name,
			Address: shared.Address,
			OpenNum: shared.OpenNum,
			Source:  "shared_catalog",
		}
	}
	if strings.TrimSpace(req.Name) != "" {
		pile.Name = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.Address) != "" {
		pile.Address = strings.TrimSpace(req.Address)
	}
	runtime.store.MergeCapturePiles([]model.Pile{pile})
	if err := m.Save(); err != nil {
		return model.Pile{}, err
	}
	for _, candidate := range runtime.store.Snapshot().Piles {
		if candidate.ID == shared.DeviceID {
			return candidate, nil
		}
	}
	return pile, nil
}
