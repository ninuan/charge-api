package runtime

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/persistence"
)

var (
	ErrRegistrationClosed       = errors.New("当前未开放注册")
	ErrInviteRegistrationClosed = errors.New("当前未开放邀请码注册")
	ErrInviteRequired           = errors.New("请输入有效邀请码")
	ErrInviteInvalid            = errors.New("邀请码无效或已过期")
	ErrUsernameExists           = errors.New("username already exists")
	ErrInvalidRegisterInput     = errors.New("invalid register input")
)

func newUser(username string, password string, role model.UserRole, enabled bool) (model.User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 {
		return model.User{}, fmt.Errorf("用户名长度需要在 3 到 64 个字符之间")
	}
	if err := validateNewPassword(password); err != nil {
		return model.User{}, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return model.User{}, err
	}
	now := time.Now()
	return model.User{
		ID:           randomID("usr"),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Enabled:      enabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func validateNewPassword(password string) error {
	if len(password) < 8 || len(password) > 128 {
		return fmt.Errorf("密码长度需要在 8 到 128 个字符之间")
	}
	return nil
}

func publicUser(user model.User) model.CurrentUser {
	return model.CurrentUser{
		ID:                 user.ID,
		Username:           user.Username,
		Role:               user.Role,
		Enabled:            user.Enabled,
		CreatedAt:          user.CreatedAt,
		DeviceLimit:        user.DeviceLimit,
		RefreshEnabled:     user.RefreshEnabled,
		MustChangePassword: user.MustChangePassword,
		UsageGuideAckAt:    user.UsageGuideAckAt,
	}
}

func (m *Manager) ChangePassword(userID, currentPassword, newPassword string) (model.CurrentUser, error) {
	m.mu.RLock()
	user, ok := m.users[userID]
	m.mu.RUnlock()
	if !ok || !user.Enabled {
		return model.CurrentUser{}, fmt.Errorf("用户不存在")
	}
	originalHash := user.PasswordHash
	valid, _ := auth.VerifyPassword(currentPassword, originalHash)
	if !valid {
		return model.CurrentUser{}, fmt.Errorf("当前密码错误")
	}
	if err := validateNewPassword(newPassword); err != nil {
		return model.CurrentUser{}, err
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return model.CurrentUser{}, err
	}
	m.saveMu.Lock()
	m.mu.Lock()
	current, ok := m.users[userID]
	if !ok || !current.Enabled {
		m.mu.Unlock()
		m.saveMu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("用户不存在")
	}
	if current.PasswordHash != originalHash {
		m.mu.Unlock()
		m.saveMu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("密码已被其他操作更新，请重试")
	}
	previous := current
	current.PasswordHash = hash
	current.MustChangePassword = false
	current.UpdatedAt = time.Now()
	m.users[userID] = current
	m.mu.Unlock()
	if err := m.saveUserStateLocked(userID); err != nil {
		m.mu.Lock()
		m.users[userID] = previous
		m.mu.Unlock()
		m.saveMu.Unlock()
		return model.CurrentUser{}, err
	}
	m.saveMu.Unlock()
	m.cleanupInitialPasswordFile(userID)
	return publicUser(current), nil
}

func (m *Manager) AdminUserDetail(userID string) (model.AdminUserDetail, error) {
	m.mu.RLock()
	user, ok := m.users[userID]
	runtime := m.runtimes[userID]
	m.mu.RUnlock()
	if !ok {
		return model.AdminUserDetail{}, fmt.Errorf("user not found")
	}
	settings := m.Settings()
	now := time.Now()
	today := quotaTodayString(now)
	poolAvail := m.PublicPoolAvailableCount()
	var quota *model.UserRefreshQuota
	if user.Enabled && runtime != nil {
		if q, qErr := m.userRefreshQuotaWithPoolCount(user.ID, poolAvail); qErr == nil {
			quota = &q
		}
	}
	summary := buildAdminUserSummary(user, runtime, quota)
	_ = settings
	_ = today
	piles := []model.Pile{}
	if runtime != nil {
		piles = runtime.store.Snapshot().Piles
		if piles == nil {
			piles = []model.Pile{}
		}
	}
	return model.AdminUserDetail{Summary: summary, Piles: piles}, nil
}

func (m *Manager) ResetUserPassword(userID string) (string, error) {
	temporaryPassword, err := randomPassword()
	if err != nil {
		return "", err
	}
	hash, err := auth.HashPassword(temporaryPassword)
	if err != nil {
		return "", err
	}
	m.saveMu.Lock()
	m.mu.Lock()
	user, ok := m.users[userID]
	if !ok {
		m.mu.Unlock()
		m.saveMu.Unlock()
		return "", fmt.Errorf("user not found")
	}
	previous := user
	user.PasswordHash = hash
	user.MustChangePassword = true
	user.UpdatedAt = time.Now()
	m.users[userID] = user
	m.mu.Unlock()
	if err := m.saveUserStateLocked(userID); err != nil {
		m.mu.Lock()
		m.users[userID] = previous
		m.mu.Unlock()
		m.saveMu.Unlock()
		return "", err
	}
	m.saveMu.Unlock()
	return temporaryPassword, nil
}

func randomPassword() (string, error) {
	token := make([]byte, 18)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func (m *Manager) Authenticate(username string, password string) (model.CurrentUser, error) {
	username = strings.TrimSpace(username)
	m.mu.RLock()
	var matched model.User
	found := false
	for _, user := range m.users {
		if user.Username != username {
			continue
		}
		matched = user
		found = true
		break
	}
	m.mu.RUnlock()

	if !found {
		// 不存在的用户也要付出一次 argon2 的耗时，
		// 否则响应时间差可以用来枚举有效用户名。
		auth.VerifyDummyPassword(password)
		return model.CurrentUser{}, fmt.Errorf("用户名或密码错误")
	}
	if !matched.Enabled {
		auth.VerifyDummyPassword(password)
		return model.CurrentUser{}, fmt.Errorf("用户已被禁用")
	}

	valid, needsUpgrade := auth.VerifyPassword(password, matched.PasswordHash)
	if !valid {
		return model.CurrentUser{}, fmt.Errorf("用户名或密码错误")
	}
	if needsUpgrade {
		hash, err := auth.HashPassword(password)
		if err != nil {
			return model.CurrentUser{}, err
		}
		m.saveMu.Lock()
		m.mu.Lock()
		current, ok := m.users[matched.ID]
		if !ok || !current.Enabled {
			m.mu.Unlock()
			m.saveMu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("用户已被禁用")
		}
		if current.PasswordHash != matched.PasswordHash {
			m.mu.Unlock()
			m.saveMu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("用户名或密码错误")
		}
		previous := current
		current.PasswordHash = hash
		current.UpdatedAt = time.Now()
		m.users[matched.ID] = current
		matched = current
		m.mu.Unlock()
		if err := m.saveUserStateLocked(matched.ID); err != nil {
			m.mu.Lock()
			m.users[matched.ID] = previous
			m.mu.Unlock()
			m.saveMu.Unlock()
			return model.CurrentUser{}, err
		}
		m.saveMu.Unlock()
	}
	return publicUser(matched), nil
}

func (m *Manager) User(id string) (model.CurrentUser, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.users[id]
	if !ok || !user.Enabled {
		return model.CurrentUser{}, false
	}
	return publicUser(user), true
}

// UserIDByUsername returns an existing account identifier for internal
// security-event attribution. It intentionally exposes no account details.
func (m *Manager) UserIDByUsername(username string) (string, bool) {
	username = strings.TrimSpace(username)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, user := range m.users {
		if user.Username == username {
			return user.ID, true
		}
	}
	return "", false
}

func (m *Manager) AcknowledgeUsageGuide(userID string) (model.CurrentUser, error) {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	m.mu.Lock()
	user, ok := m.users[userID]
	if !ok || !user.Enabled {
		m.mu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("user not found or disabled")
	}
	previous := user
	if user.UsageGuideAckAt == nil {
		now := time.Now()
		user.UsageGuideAckAt = &now
		user.UpdatedAt = now
		m.users[userID] = user
	}
	m.mu.Unlock()
	if err := m.saveUserStateLocked(userID); err != nil {
		m.mu.Lock()
		m.users[userID] = previous
		m.mu.Unlock()
		return model.CurrentUser{}, err
	}
	return publicUser(user), nil
}

func (m *Manager) ListUsers() []model.AdminUserSummary {
	m.mu.RLock()
	userIDs := make([]string, 0, len(m.users))
	for id := range m.users {
		userIDs = append(userIDs, id)
	}
	sort.Slice(userIDs, func(i, j int) bool {
		return m.users[userIDs[i]].Username < m.users[userIDs[j]].Username
	})
	users := make([]model.User, 0, len(userIDs))
	runtimes := make([]*UserRuntime, 0, len(userIDs))
	for _, id := range userIDs {
		users = append(users, m.users[id])
		runtimes = append(runtimes, m.runtimes[id])
	}
	m.mu.RUnlock()

	settings := m.Settings()
	now := time.Now()
	today := quotaTodayString(now)
	poolAvail := m.PublicPoolAvailableCount()
	quotasByUser, quotasErr := m.repository.ListAllUserQuotas(today)
	healthyPoolOwners, poolOwnersErr := m.repository.ListHealthyPoolOwnerIDs()
	summaries := make([]model.AdminUserSummary, 0, len(users))
	for i, user := range users {
		runtime := runtimes[i]
		var quota *model.UserRefreshQuota
		if runtime != nil && user.Enabled {
			if quotasErr == nil && poolOwnersErr == nil {
				rec, ok := quotasByUser[user.ID]
				if !ok {
					rec = persistence.UserQuotaRecord{UserID: user.ID, DailyDate: today, DailyProbeDate: today}
				}
				q := buildUserRefreshQuota(runtime, settings, now, poolAvail, rec, healthyPoolOwners[user.ID])
				quota = &q
			} else if q, qErr := m.userRefreshQuotaWithPoolCount(user.ID, poolAvail); qErr == nil {
				quota = &q
			}
		}
		summaries = append(summaries, buildAdminUserSummary(user, runtime, quota))
	}
	return summaries
}

func buildAdminUserSummary(user model.User, runtime *UserRuntime, quota *model.UserRefreshQuota) model.AdminUserSummary {
	summary := model.AdminUserSummary{
		User:              publicUser(user),
		DeviceIDs:         []string{},
		Credential:        model.CredentialSummary{State: model.CredentialUnbound},
		SnapshotUpdatedAt: user.CreatedAt,
		Quota:             quota,
	}
	if runtime != nil {
		snapshot := runtime.store.Snapshot()
		summary.Stats = runtime.statsSnapshot()
		summary.Dashboard = snapshot.Statistics
		summary.DeviceIDs = runtime.client.DeviceIDs()
		summary.Credential = credentialSummary(runtime, len(summary.DeviceIDs))
		summary.HasCookie = summary.Credential.HasCredential
		summary.SnapshotUpdatedAt = latestSnapshotDataTime(snapshot, user.CreatedAt)
		summary.LastRefresh = snapshot.Refresh
		rawDiagnostics := runtime.recoveryDiagnosticsSnapshot()
		filteredDiagnostics := make([]model.RecoveryDiagnostic, 0, len(rawDiagnostics))
		for _, d := range rawDiagnostics {
			if isRetiredDiagnostic(d) {
				continue
			}
			filteredDiagnostics = append(filteredDiagnostics, d)
		}
		summary.RecoveryDiagnostics = filteredDiagnostics
	}
	return summary
}

func (m *Manager) ListUsersPage(query model.AdminUserListQuery) model.AdminUserPage {
	query = normalizeAdminUserListQuery(query)
	items := make([]model.AdminUserSummary, 0)
	for _, summary := range m.ListUsers() {
		if !matchesAdminUserListQuery(summary, query) {
			continue
		}
		items = append(items, summary)
	}

	total := len(items)
	totalPages := int(math.Ceil(float64(total) / float64(query.PageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if query.Page > totalPages {
		query.Page = totalPages
	}
	start := (query.Page - 1) * query.PageSize
	end := start + query.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pageItems := items[start:end]
	if pageItems == nil {
		pageItems = []model.AdminUserSummary{}
	}
	return model.AdminUserPage{
		Items:      pageItems,
		Page:       query.Page,
		PageSize:   query.PageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

func normalizeAdminUserListQuery(query model.AdminUserListQuery) model.AdminUserListQuery {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 15
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	query.Search = strings.TrimSpace(strings.ToLower(query.Search))
	return query
}

func matchesAdminUserListQuery(summary model.AdminUserSummary, query model.AdminUserListQuery) bool {
	if query.Search != "" && !strings.Contains(strings.ToLower(summary.User.Username), query.Search) {
		return false
	}
	switch query.Account {
	case "enabled":
		if !summary.User.Enabled {
			return false
		}
	case "disabled":
		if summary.User.Enabled {
			return false
		}
	}
	if query.Credential != "" && query.Credential != "all" {
		if !matchesAdminUserCredentialFilter(summary, query.Credential) {
			return false
		}
	}
	switch query.Health {
	case "healthy":
		if hasAdminUserRisk(summary) {
			return false
		}
	case "risk":
		if !hasAdminUserRisk(summary) {
			return false
		}
	}
	return true
}

func matchesAdminUserCredentialFilter(summary model.AdminUserSummary, filter string) bool {
	if summary.User.Role == model.RoleAdmin {
		return false
	}
	if summary.Quota != nil {
		switch filter {
		case "unbound":
			return !summary.Quota.HasPersonalCookie
		case "healthy":
			return summary.Quota.UnlimitedActive
		case "pool_unjoined":
			return summary.Quota.HasPersonalCookie &&
				summary.Quota.PersonalCookieStatus == "valid" &&
				!summary.Quota.UnlimitedActive
		case "expired":
			return summary.Quota.PersonalCookieStatus == "expired"
		}
	}
	return string(summary.Credential.State) == filter
}

func hasAdminUserRisk(summary model.AdminUserSummary) bool {
	hasPersonalCookieExpired := summary.Quota != nil && summary.Quota.PersonalCookieStatus == "expired"
	return !summary.User.Enabled || hasPersonalCookieExpired || summary.LastRefresh.FailedDevices > 0 || summary.Dashboard.OfflinePorts > 0
}

func isRetiredDiagnostic(d model.RecoveryDiagnostic) bool {
	if d.Operation == diagnosticOperationScan || d.Operation == diagnosticOperationSync || d.Operation == diagnosticOperationRecovery {
		return true
	}
	for _, prefix := range []string{"yyb_", "qr_", "binding_", "mocele_autologin_", "recovery_"} {
		if strings.HasPrefix(d.Code, prefix) {
			return true
		}
	}
	switch d.Code {
	case "scan_service_unavailable", "credential_sync_failed", "new_cookie_validation_failed":
		return true
	default:
		return false
	}
}

func credentialSummary(runtime *UserRuntime, _ int) model.CredentialSummary {
	if runtime == nil {
		return model.CredentialSummary{State: model.CredentialUnbound}
	}
	hasCookie := strings.TrimSpace(runtime.client.Cookie()) != ""
	result := model.CredentialSummary{
		Bound:         hasCookie,
		HasCredential: hasCookie,
	}
	switch {
	case !hasCookie:
		result.State = model.CredentialUnbound
	case runtime.isPersonalCookieExpired():
		result.State = model.CredentialExpired
	default:
		result.State = model.CredentialHealthy
	}
	return result
}

func latestSnapshotDataTime(snapshot model.DashboardSnapshot, fallback time.Time) time.Time {
	latest := fallback
	if snapshot.Refresh.LastRemoteAt != nil && snapshot.Refresh.LastRemoteAt.After(latest) {
		latest = *snapshot.Refresh.LastRemoteAt
	}
	for _, pile := range snapshot.Piles {
		if pile.UpdatedAt.After(latest) {
			latest = pile.UpdatedAt
		}
		for _, port := range pile.Ports {
			if port.UpdatedAt.After(latest) {
				latest = port.UpdatedAt
			}
		}
	}
	return latest
}

func (m *Manager) CreateUser(req model.UserCreateRequest) (model.CurrentUser, error) {
	role := req.Role
	if role == "" {
		role = model.RoleUser
	}
	if role != model.RoleAdmin && role != model.RoleUser {
		return model.CurrentUser{}, fmt.Errorf("invalid role")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	user, err := newUser(req.Username, req.Password, role, enabled)
	if err != nil {
		return model.CurrentUser{}, err
	}
	m.mu.RLock()
	user.DeviceLimit = m.settings.DefaultDeviceLimit
	user.RefreshEnabled = m.settings.DefaultRefreshEnabled
	m.mu.RUnlock()
	if req.DeviceLimit != nil {
		user.DeviceLimit = *req.DeviceLimit
	}
	if req.RefreshEnabled != nil {
		user.RefreshEnabled = *req.RefreshEnabled
	}
	if user.DeviceLimit < 1 || user.DeviceLimit > 100 {
		return model.CurrentUser{}, fmt.Errorf("设备额度需要在 1 到 100 之间")
	}

	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	m.mu.Lock()
	for _, existing := range m.users {
		if existing.Username == user.Username {
			m.mu.Unlock()
			return model.CurrentUser{}, ErrUsernameExists
		}
	}
	m.users[user.ID] = user
	m.runtimes[user.ID] = newUserRuntime(m.requests, persistence.UserState{}, m.minInterval)
	m.mu.Unlock()

	if err := m.saveUserStateLocked(user.ID); err != nil {
		m.mu.Lock()
		delete(m.users, user.ID)
		delete(m.runtimes, user.ID)
		m.mu.Unlock()
		return model.CurrentUser{}, err
	}
	return publicUser(user), nil
}

// ValidateRegistrationPreconditions performs a cheap read-only check of
// registration policy, invite validity, and username uniqueness before callers
// consume Argon2id concurrency slots.
func (m *Manager) ValidateRegistrationPreconditions(username string, inviteCode string) error {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 {
		return fmt.Errorf("%w: 用户名长度需要在 3 到 64 个字符之间", ErrInvalidRegisterInput)
	}
	inviteCode = strings.TrimSpace(inviteCode)
	now := time.Now()
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.settings.OpenRegistration && !m.settings.InviteRequired {
		return ErrRegistrationClosed
	}
	if inviteCode != "" {
		if !m.settings.InviteRequired {
			return ErrInviteRegistrationClosed
		}
		matched := false
		for _, invite := range m.invites {
			if invite.Enabled && invite.Code == inviteCode &&
				(invite.ExpiresAt == nil || invite.ExpiresAt.After(now)) {
				matched = true
				break
			}
		}
		if !matched {
			return ErrInviteInvalid
		}
	} else if !m.settings.OpenRegistration {
		return ErrInviteRequired
	}
	for _, existing := range m.users {
		if existing.Username == username {
			return ErrUsernameExists
		}
	}
	return nil
}

func (m *Manager) RegisterUser(username string, password string, inviteCode string) (model.CurrentUser, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 {
		return model.CurrentUser{}, fmt.Errorf("%w: 用户名长度需要在 3 到 64 个字符之间", ErrInvalidRegisterInput)
	}
	if len(password) < 8 || len(password) > 128 {
		return model.CurrentUser{}, fmt.Errorf("%w: 密码长度需要在 8 到 128 个字符之间", ErrInvalidRegisterInput)
	}
	if err := m.ValidateRegistrationPreconditions(username, inviteCode); err != nil {
		return model.CurrentUser{}, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return model.CurrentUser{}, err
	}

	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	m.mu.Lock()
	if !m.settings.OpenRegistration && !m.settings.InviteRequired {
		m.mu.Unlock()
		return model.CurrentUser{}, ErrRegistrationClosed
	}
	var usedInviteID string
	var updatedInvite *model.InviteCode
	inviteCode = strings.TrimSpace(inviteCode)
	now := time.Now()
	if inviteCode != "" {
		if !m.settings.InviteRequired {
			m.mu.Unlock()
			return model.CurrentUser{}, ErrInviteRegistrationClosed
		}
		for id, invite := range m.invites {
			if invite.Enabled && invite.Code == inviteCode &&
				(invite.ExpiresAt == nil || invite.ExpiresAt.After(now)) {
				usedInviteID = id
				break
			}
		}
		if usedInviteID == "" {
			m.mu.Unlock()
			return model.CurrentUser{}, ErrInviteInvalid
		}
	} else if !m.settings.OpenRegistration {
		m.mu.Unlock()
		return model.CurrentUser{}, ErrInviteRequired
	}
	for _, existing := range m.users {
		if existing.Username == username {
			m.mu.Unlock()
			return model.CurrentUser{}, ErrUsernameExists
		}
	}
	user := model.User{
		ID:             randomID("usr"),
		Username:       username,
		PasswordHash:   hash,
		Role:           model.RoleUser,
		Enabled:        true,
		CreatedAt:      now,
		UpdatedAt:      now,
		DeviceLimit:    m.settings.DefaultDeviceLimit,
		RefreshEnabled: m.settings.DefaultRefreshEnabled,
	}
	if user.DeviceLimit < 1 || user.DeviceLimit > 100 {
		m.mu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("设备额度需要在 1 到 100 之间")
	}
	m.users[user.ID] = user
	m.runtimes[user.ID] = newUserRuntime(m.requests, persistence.UserState{}, m.minInterval)
	if usedInviteID != "" {
		invite := m.invites[usedInviteID]
		invite.UsedCount++
		m.invites[usedInviteID] = invite
		copyInvite := invite
		updatedInvite = &copyInvite
	}
	m.mu.Unlock()

	if saveErr := m.repository.SaveUserAndInvite(user, persistence.UserState{}, updatedInvite); saveErr != nil {
		m.mu.Lock()
		delete(m.users, user.ID)
		delete(m.runtimes, user.ID)
		if usedInviteID != "" {
			if invite, ok := m.invites[usedInviteID]; ok && invite.UsedCount > 0 {
				invite.UsedCount--
				m.invites[usedInviteID] = invite
			}
		}
		m.mu.Unlock()
		return model.CurrentUser{}, saveErr
	}
	return publicUser(user), nil
}

func (m *Manager) UpdateUser(id string, req model.UserUpdateRequest) (model.CurrentUser, error) {
	m.saveMu.Lock()
	m.mu.Lock()
	user, ok := m.users[id]
	if !ok {
		m.mu.Unlock()
		m.saveMu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("user not found")
	}
	previous := user

	if req.Role != nil {
		if *req.Role != model.RoleAdmin && *req.Role != model.RoleUser {
			m.mu.Unlock()
			m.saveMu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("invalid role")
		}
		user.Role = *req.Role
	}
	if req.Enabled != nil {
		user.Enabled = *req.Enabled
	}
	if req.DeviceLimit != nil {
		if *req.DeviceLimit < 1 || *req.DeviceLimit > 100 {
			m.mu.Unlock()
			m.saveMu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("设备额度需要在 1 到 100 之间")
		}
		user.DeviceLimit = *req.DeviceLimit
	}
	if req.RefreshEnabled != nil {
		user.RefreshEnabled = *req.RefreshEnabled
	}
	activeAdmins := 0
	for userID, existing := range m.users {
		if userID == id {
			existing = user
		}
		if existing.Role == model.RoleAdmin && existing.Enabled {
			activeAdmins++
		}
	}
	if activeAdmins == 0 {
		m.mu.Unlock()
		m.saveMu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("至少保留一个可用管理员")
	}
	user.UpdatedAt = time.Now()
	m.users[id] = user
	m.mu.Unlock()

	if err := m.saveUserStateLocked(id); err != nil {
		m.mu.Lock()
		m.users[id] = previous
		m.mu.Unlock()
		m.saveMu.Unlock()
		return model.CurrentUser{}, err
	}
	m.saveMu.Unlock()
	m.wakeReminderScheduler()
	return publicUser(user), nil
}

func (m *Manager) DeleteUser(id string) error {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()
	m.saveMu.Lock()
	m.mu.Lock()
	prevUser, ok := m.users[id]
	if !ok {
		m.mu.Unlock()
		m.saveMu.Unlock()
		return fmt.Errorf("user not found")
	}
	prevRuntime := m.runtimes[id]
	adminCount := 0
	for userID, user := range m.users {
		if userID != id && user.Role == model.RoleAdmin && user.Enabled {
			adminCount++
		}
	}
	if prevUser.Role == model.RoleAdmin && adminCount == 0 {
		m.mu.Unlock()
		m.saveMu.Unlock()
		return fmt.Errorf("至少保留一个可用管理员")
	}
	delete(m.users, id)
	delete(m.runtimes, id)
	m.mu.Unlock()

	err := m.repository.DeleteUser(id)
	if err != nil {
		m.mu.Lock()
		m.users[id] = prevUser
		if prevRuntime != nil {
			m.runtimes[id] = prevRuntime
		}
		m.mu.Unlock()
		m.saveMu.Unlock()
		return err
	}
	m.saveMu.Unlock()
	m.wxPusherMu.Lock()
	delete(m.wxPusherAttempts, id)
	delete(m.wxPusherAttempts, "test_recheck:"+id)
	m.wxPusherMu.Unlock()
	m.invalidateBackgroundCredentialValidation(id)
	m.wakeReminderScheduler()
	return nil
}
