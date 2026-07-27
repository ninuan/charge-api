package runtime

import (
	"fmt"
	"strings"

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
	return runtime.store.Snapshot(), m.Save()
}

func (m *Manager) AddPile(userID string, req model.PileUpsertRequest) (model.Pile, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.Pile{}, err
	}
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
			_ = m.Save()
			return model.Pile{}, err
		}
		req.ID = resolvedID
	}
	if err := runtime.client.AddDeviceWithLimit(req.ID, user.DeviceLimit); err != nil {
		runtime.recordFailure(false)
		_ = m.Save()
		if charger.IsDeviceLimit(err) {
			return model.Pile{}, fmt.Errorf("当前账户最多添加 %d 台充电桩", user.DeviceLimit)
		}
		return model.Pile{}, err
	}
	if err := runtime.refresh(true); err != nil {
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		m.recordMetric(userID, "cookie_error")
		runtime.client.RemoveDevice(req.ID)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.Pile{}, err
	}
	snapshot := runtime.store.Snapshot()
	m.recordRefreshMetrics(userID, snapshot.Refresh)
	for _, pile := range snapshot.Piles {
		if pile.ID == req.ID {
			updated, _ := runtime.store.UpdatePile(req.ID, req.Name, req.Address, pile.SortOrder)
			return updated, m.Save()
		}
	}
	runtime.client.RemoveDevice(req.ID)
	runtime.recordFailure(false)
	_ = m.Save()
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
		resolvedID, err := runtime.client.ResolveDeviceIDByNumber(req.Number)
		if err != nil {
			return model.Pile{}, err
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
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	if !runtime.store.DeletePile(id) {
		runtime.recordFailure(false)
		_ = m.Save()
		return fmt.Errorf("pile not found")
	}
	runtime.client.RemoveDevice(id)
	return m.Save()
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
	return pile, m.Save()
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
	return snapshot, m.Save()
}

func (m *Manager) Refresh(userID string, force bool) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		m.recordMetric(userID, "cache")
		snapshot := runtime.store.Snapshot()
		snapshot.Refresh.Cached = true
		snapshot.Refresh.Message = "管理员已暂停此账户的远端刷新，当前展示缓存数据"
		return snapshot, nil
	}
	if err := runtime.refresh(force); err != nil {
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		m.recordMetric(userID, "cookie_error")
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	snapshot := runtime.store.Snapshot()
	if snapshot.Refresh.Cached {
		m.recordMetric(userID, "cache")
	} else {
		m.recordRefreshMetrics(userID, snapshot.Refresh)
	}
	return snapshot, m.Save()
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

func (m *Manager) UpdateCookie(userID string, cookie string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	previous := runtime.client.Cookie()
	if err := runtime.client.UpdateCookie(cookie); err != nil {
		runtime.recordFailure(false)
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		snapshot := runtime.store.Snapshot()
		snapshot.Refresh.Cached = true
		snapshot.Refresh.Message = "Cookie 已保存；远端刷新当前已暂停，继续展示缓存数据"
		runtime.store.SetRefreshInfo(snapshot.Refresh)
		if err := m.Save(); err != nil {
			return model.DashboardSnapshot{}, err
		}
		return snapshot, nil
	}
	if err := runtime.refresh(true); err != nil {
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		_ = runtime.client.UpdateCookie(previous)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	snapshot := runtime.store.Snapshot()
	m.recordRefreshMetrics(userID, snapshot.Refresh)
	return snapshot, m.Save()
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
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return
	}
	runtime.store.Unsubscribe(ch)
}
