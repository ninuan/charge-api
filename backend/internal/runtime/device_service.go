package runtime

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/charger"
	"charge-dashboard/internal/model"
)

func (m *Manager) Snapshot(userID string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	return m.attachQuota(userID, runtime.store.Snapshot()), nil
}

func (m *Manager) AddPile(userID string, req model.PileUpsertRequest) (model.Pile, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.Pile{}, err
	}
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	req.ID = strings.TrimSpace(req.ID)
	req.Number = strings.TrimSpace(req.Number)
	req.Name = strings.TrimSpace(req.Name)
	req.Address = strings.TrimSpace(req.Address)
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		return model.Pile{}, fmt.Errorf("管理员已暂停此账户的远端刷新，暂时无法验证新设备")
	}
	if req.ID == "" {
		if req.Number == "" {
			return model.Pile{}, fmt.Errorf("请输入桩号或设备长ID")
		}
		if user.DeviceLimit > 0 && len(runtime.client.DeviceIDs()) >= user.DeviceLimit {
			return model.Pile{}, fmt.Errorf("当前账户最多添加 %d 台充电桩", user.DeviceLimit)
		}
		resolvedID, err := runtime.client.ResolveDeviceIDByNumber(req.Number)
		if err != nil {
			runtime.recordFailure(false)
			_ = m.saveUserState(userID)
			return model.Pile{}, err
		}
		req.ID = resolvedID
	}
	if err := runtime.client.AddDeviceWithLimit(req.ID, user.DeviceLimit); err != nil {
		runtime.recordFailure(false)
		_ = m.saveUserState(userID)
		if charger.IsDeviceLimit(err) {
			return model.Pile{}, fmt.Errorf("当前账户最多添加 %d 台充电桩", user.DeviceLimit)
		}
		return model.Pile{}, err
	}
	remotePiles, err := runtime.refresh(true)
	if err != nil {
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		if charger.IsAuthExpired(err) {
			m.recordMetric(userID, "cookie_error")
		}
		runtime.client.RemoveDevice(req.ID)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.saveUserState(userID)
		return model.Pile{}, err
	}
	snapshot := runtime.store.Snapshot()
	m.recordRefreshMetrics(userID, snapshot.Refresh)
	for _, pile := range snapshot.Piles {
		if pile.ID == req.ID {
			updated, _ := runtime.store.UpdatePile(req.ID, req.Name, req.Address, pile.SortOrder)
			return updated, m.saveAndRecordRemotePiles(userID, remotePiles)
		}
	}
	runtime.client.RemoveDevice(req.ID)
	runtime.recordFailure(false)
	if saveErr := m.saveAndRecordRemotePiles(userID, remotePiles); saveErr != nil {
		return model.Pile{}, saveErr
	}
	return model.Pile{}, fmt.Errorf("device %s was not returned by remote API", req.ID)
}

func (m *Manager) AddPileWithYYB(userID string, req model.PileUpsertRequest, yybClient YYBCodeClient, moceleClient MoceleCookieClient) (model.Pile, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.Pile{}, err
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Number = strings.TrimSpace(req.Number)
	if req.ID == "" && req.Number != "" {
		user, _ := m.User(userID)
		if !user.RefreshEnabled {
			return model.Pile{}, fmt.Errorf("管理员已暂停此账户的远端刷新，暂时无法验证新设备")
		}
		if user.DeviceLimit > 0 && len(runtime.client.DeviceIDs()) >= user.DeviceLimit {
			return model.Pile{}, fmt.Errorf("当前账户最多添加 %d 台充电桩", user.DeviceLimit)
		}
		runtime.refreshMu.Lock()
		resolvedID, resolveErr := runtime.client.ResolveDeviceIDByNumber(req.Number)
		runtime.refreshMu.Unlock()
		if resolveErr != nil {
			return model.Pile{}, resolveErr
		}
		req.ID = resolvedID
	}

	pile, err := m.AddPile(userID, req)
	if err == nil {
		return pile, nil
	}
	if !charger.IsAuthExpired(err) {
		return model.Pile{}, err
	}
	if req.ID == "" {
		return model.Pile{}, err
	}
	binding, bindingErr := m.YYBBinding(userID)
	if bindingErr != nil {
		return model.Pile{}, bindingErr
	}
	if binding == nil || binding.Ref == "" {
		return model.Pile{}, ErrYYBBindingRequired
	}
	if _, syncErr := m.SyncCookieFromYYB(userID, req.ID, yybClient, moceleClient); syncErr != nil {
		return model.Pile{}, fmt.Errorf("自动更新登录凭据失败: %w", syncErr)
	}
	return m.AddPile(userID, req)
}

func (m *Manager) DeletePile(userID string, id string) error {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return err
	}
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	if !runtime.store.DeletePile(id) {
		runtime.recordFailure(false)
		_ = m.saveUserState(userID)
		return fmt.Errorf("pile not found")
	}
	runtime.client.RemoveDevice(id)
	m.invalidateBackgroundPileValidation(userID, id)
	if err := m.saveUserState(userID); err != nil {
		return err
	}
	if err := m.repository.DeleteWatchDataForPile(userID, id); err != nil {
		return err
	}
	_, err = m.repository.DeletePortStatusEvents(userID, id)
	m.wakeReminderScheduler()
	return err
}

func (m *Manager) UpdatePile(userID, id, name, address string, sortOrder int) (model.Pile, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.Pile{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	pile, ok := runtime.store.UpdatePile(id, strings.TrimSpace(name), strings.TrimSpace(address), sortOrder)
	if !ok {
		return model.Pile{}, fmt.Errorf("pile not found")
	}
	return pile, m.saveUserState(userID)
}

func (m *Manager) ReorderPiles(userID string, ids []string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	snapshot, err := runtime.store.ReorderPiles(ids)
	if err != nil {
		runtime.recordFailure(false)
		return model.DashboardSnapshot{}, err
	}
	return snapshot, m.saveUserState(userID)
}

func (m *Manager) Refresh(userID string, force bool) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		m.recordMetric(userID, "cache")
		snapshot := runtime.store.Snapshot()
		snapshot.Refresh.Cached = true
		snapshot.Refresh.Message = "管理员已暂停此账户的远端刷新，当前展示缓存数据"
		return m.attachQuota(userID, snapshot), nil
	}
	if !m.hasUnlimitedPrivilege(userID, runtime) {
		return m.refreshViaPublicPool(userID, runtime, runtime.client.HasPersonalCookie() && runtime.isPersonalCookieExpired())
	}
	remotePiles, err := runtime.refresh(force)
	if err != nil {
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		if charger.IsAuthExpired(err) {
			if notificationErr := m.markPersonalCookieExpired(userID, runtime, time.Now().UTC()); notificationErr != nil {
				err = errors.Join(err, fmt.Errorf("record expired credential notification: %w", notificationErr))
			}
			if m.PublicPoolAvailableCount() > 0 {
				return m.refreshViaPublicPool(userID, runtime, true)
			}
			_ = m.saveUserState(userID)
			return model.DashboardSnapshot{}, err
		}
		runtime.recordFailure(false)
		_ = m.saveUserState(userID)
		return model.DashboardSnapshot{}, err
	}
	runtime.setPersonalCookieExpired(false)
	m.recordSharedPiles(userID, remotePiles)
	snapshot := runtime.store.Snapshot()
	if snapshot.Refresh.Cached {
		m.recordMetric(userID, "cache")
	} else {
		m.recordRefreshMetrics(userID, snapshot.Refresh)
	}
	if err := m.saveAndRecordRemotePiles(userID, remotePiles); err != nil {
		return model.DashboardSnapshot{}, err
	}
	return m.attachQuota(userID, snapshot), nil
}

func (m *Manager) RefreshWithYYB(userID string, force bool, yybClient YYBCodeClient, moceleClient MoceleCookieClient) (model.DashboardSnapshot, error) {
	snapshot, err := m.Refresh(userID, force)
	if err == nil || !charger.IsAuthExpired(err) {
		return snapshot, err
	}
	m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("remote_auth_rejected", "", err))
	if yybClient == nil || moceleClient == nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("recovery_unavailable", "", 0))
		return model.DashboardSnapshot{}, err
	}
	return m.RecoverRefreshWithYYB(userID, yybClient, moceleClient)
}

func (m *Manager) RecoverRefreshWithYYB(userID string, yybClient YYBCodeClient, moceleClient MoceleCookieClient) (model.DashboardSnapshot, error) {
	deviceID, ok, deviceErr := m.FirstDeviceID(userID)
	if deviceErr != nil || !ok {
		if deviceErr != nil {
			return model.DashboardSnapshot{}, deviceErr
		}
		m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("recovery_unavailable", "", 0))
		return model.DashboardSnapshot{}, fmt.Errorf("no device is available for automatic credential recovery")
	}
	snapshot, syncErr := m.SyncCookieFromYYB(userID, deviceID, yybClient, moceleClient)
	if syncErr != nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("recovery_failed", deviceID, syncErr))
		return model.DashboardSnapshot{}, fmt.Errorf("自动更新登录凭据失败: %w", syncErr)
	}
	m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("recovery_succeeded", deviceID, 0))
	return snapshot, nil
}

func (m *Manager) UpdateCookie(userID string, rawCookie string) (model.DashboardSnapshot, error) {
	return m.UpdateCookieWithPoolOption(userID, rawCookie, false)
}

func (m *Manager) UpdateCookieWithPoolOption(userID string, rawCookie string, joinPool bool) (model.DashboardSnapshot, error) {
	cookie, err := charger.ExtractCookie(rawCookie)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	previous := runtime.client.Cookie()
	if err := runtime.client.UpdateCookie(cookie); err != nil {
		runtime.recordFailure(false)
		_ = m.saveUserState(userID)
		return model.DashboardSnapshot{}, err
	}
	m.invalidateBackgroundCredentialValidation(userID)
	user, _ := m.User(userID)
	if joinPool {
		if dup, dupErr := m.repository.HasPublicPoolCookieDuplicate("user_"+strings.TrimSpace(userID), cookie); dupErr != nil {
			_ = runtime.client.UpdateCookie(previous)
			return model.DashboardSnapshot{}, dupErr
		} else if dup {
			_ = runtime.client.UpdateCookie(previous)
			return model.DashboardSnapshot{}, ErrPublicPoolCookieDuplicate
		}
	}
	syncPool := func() error {
		if joinPool {
			label := "用户互助 · " + user.Username
			if user.Username == "" {
				label = "用户互助 · " + userID
			}
			_, err := m.repository.UpsertUserPublicPoolCookie(userID, label, cookie)
			return err
		}
		return m.repository.DeleteUserPublicPoolCookie(userID)
	}
	if !user.RefreshEnabled {
		if joinPool {
			_ = runtime.client.UpdateCookie(previous)
			return model.DashboardSnapshot{}, fmt.Errorf("管理员已暂停此账户的远端刷新，暂时无法验证并加入互助公共池")
		}
		runtime.setPersonalCookieExpired(false)
		_ = m.repository.DeleteUserPublicPoolCookie(userID)
		snapshot := runtime.store.Snapshot()
		snapshot.Refresh.Cached = true
		snapshot.Refresh.Message = "Cookie 已保存；远端刷新当前已暂停，继续展示缓存数据"
		runtime.store.SetRefreshInfo(snapshot.Refresh)
		if err := m.saveUserState(userID); err != nil {
			return model.DashboardSnapshot{}, err
		}
		return m.attachQuota(userID, snapshot), nil
	}
	if len(runtime.client.DeviceIDs()) == 0 {
		if shared, _ := m.repository.SearchSharedPiles("", 1); len(shared) > 0 {
			if _, probeErr := runtime.client.FetchPileWithCookie(shared[0].DeviceID, cookie); probeErr != nil {
				_ = runtime.client.UpdateCookie(previous)
				runtime.recordFailure(false)
				_ = m.saveUserState(userID)
				return model.DashboardSnapshot{}, probeErr
			}
		}
		if err := syncPool(); err != nil {
			_ = runtime.client.UpdateCookie(previous)
			return model.DashboardSnapshot{}, err
		}
		now := time.Now().UTC()
		runtime.setPersonalCookieExpired(false)
		runtime.recordRemoteOK(now)
		_ = m.resolveCredentialExpired(userID, now)
		snapshot := runtime.store.Snapshot()
		if joinPool {
			snapshot.Refresh.Message = "个人 Cookie 已加入互助公共池，已解锁不限次刷新与空位提醒"
		} else {
			snapshot.Refresh.Message = "个人 Cookie 已保存"
		}
		runtime.store.SetRefreshInfo(snapshot.Refresh)
		if err := m.saveUserState(userID); err != nil {
			return model.DashboardSnapshot{}, err
		}
		return m.attachQuota(userID, snapshot), nil
	}
	remotePiles, err := runtime.refresh(true)
	if err != nil {
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		_ = runtime.client.UpdateCookie(previous)
		runtime.recordFailure(false)
		_ = m.saveUserState(userID)
		return model.DashboardSnapshot{}, err
	}
	if err := syncPool(); err != nil {
		_ = runtime.client.UpdateCookie(previous)
		return model.DashboardSnapshot{}, err
	}
	runtime.setPersonalCookieExpired(false)
	runtime.recordRemoteOK(time.Now().UTC())
	m.recordSharedPiles(userID, remotePiles)
	snapshot := runtime.store.Snapshot()
	m.recordRefreshMetrics(userID, snapshot.Refresh)
	if err := m.saveAndRecordRemotePiles(userID, remotePiles); err != nil {
		return model.DashboardSnapshot{}, err
	}
	return m.attachQuota(userID, snapshot), nil
}

func (m *Manager) saveAndRecordRemotePiles(userID string, piles []model.Pile) error {
	return m.saveAndRecordPiles(userID, piles, true)
}

func (m *Manager) saveAndRecordPoolPiles(userID string, piles []model.Pile) error {
	return m.saveAndRecordPiles(userID, piles, false)
}

func (m *Manager) saveAndRecordPiles(userID string, piles []model.Pile, validatePersonalCredential bool) error {
	if err := m.saveUserState(userID); err != nil {
		return err
	}
	if len(piles) == 0 {
		return nil
	}
	events, err := m.recordReminderStatusEvents(userID, piles)
	if err != nil {
		return fmt.Errorf("record remote port status transitions: %w", err)
	}
	if err := m.processPileAvailability(userID, piles, events); err != nil {
		return fmt.Errorf("deliver remote port notifications: %w", err)
	}
	if validatePersonalCredential {
		now := time.Now().UTC()
		m.markBackgroundCredentialsValidated(userID, piles, now)
		if err := m.resolveCredentialExpired(userID, now); err != nil {
			return fmt.Errorf("resolve credential notification: %w", err)
		}
	}
	return nil
}

func (m *Manager) Subscribe(userID string) (chan model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return nil, err
	}
	runtime.recordRequest()
	return runtime.store.Subscribe(), nil
}

func (m *Manager) Unsubscribe(userID string, ch chan model.DashboardSnapshot) {
	m.mu.RLock()
	runtime := m.runtimes[userID]
	m.mu.RUnlock()
	if runtime == nil {
		return
	}
	runtime.store.Unsubscribe(ch)
}

// Manual refreshes only persist events needed for active reminders. The table
// remains the notification recovery/outbox source, not a usage history feed.
func (m *Manager) recordReminderStatusEvents(userID string, piles []model.Pile) ([]model.PortStatusEvent, error) {
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return nil, err
	}
	watched := make(map[string]bool)
	now := time.Now().UTC()
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		active, _, err := reminderRuleActiveAt(rule, now)
		if err != nil {
			return nil, err
		}
		if active {
			watched[rule.DeviceID] = true
		}
	}
	selected := make([]model.Pile, 0, len(piles))
	for _, pile := range piles {
		if watched[pile.ID] {
			selected = append(selected, pile)
		}
	}
	return m.repository.RecordPortStatusTransitions(userID, selected)
}
