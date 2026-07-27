package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/yyb"
)

func (m *Manager) SaveYYBBinding(userID string, account yyb.YYBAccount) (*model.YYBBinding, error) {
	now := time.Now().UTC()
	binding := &model.YYBBinding{
		OpenID:        strings.TrimSpace(account.OpenID),
		Ref:           strings.TrimSpace(account.Ref),
		Nickname:      strings.TrimSpace(account.Nickname),
		Avatar:        strings.TrimSpace(account.Avatar),
		Status:        "alive",
		BoundAt:       now,
		LastCheckedAt: nil,
		LastError:     "",
	}
	if binding.Ref == "" {
		return nil, fmt.Errorf("yyb account ref is required")
	}
	if binding.OpenID == "" {
		return nil, fmt.Errorf("yyb account openid is required")
	}
	if err := m.SetYYBBinding(userID, binding); err != nil {
		return nil, err
	}
	return cloneYYBBinding(binding), nil
}

func (m *Manager) ClearYYBBinding(userID string) error {
	return m.SetYYBBinding(userID, nil)
}

func (m *Manager) SyncCookieFromYYB(userID string, deviceID string, yybClient YYBCodeClient, moceleClient MoceleCookieClient) (model.DashboardSnapshot, error) {
	binding, err := m.YYBBinding(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	if binding == nil || binding.Ref == "" {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("binding_missing", deviceID, 0))
		return model.DashboardSnapshot{}, fmt.Errorf("yyb binding is required")
	}
	ctx := context.Background()
	code, err := yybClient.GetCode(ctx, binding.Ref, moceleAppID)
	if err != nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("yyb_get_code_failed", deviceID, err))
		if refreshErr := yybClient.RefreshAccount(ctx, binding.Ref); refreshErr != nil {
			m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("yyb_account_refresh_failed", deviceID, refreshErr))
			err = fmt.Errorf("get code failed: %v; refresh failed: %w", err, refreshErr)
			m.markYYBBindingExpired(userID, binding, err)
			return model.DashboardSnapshot{}, err
		}
		m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("yyb_account_refresh_succeeded", deviceID, 0))
		code, err = yybClient.GetCode(ctx, binding.Ref, moceleAppID)
		if err != nil {
			m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("yyb_get_code_retry_failed", deviceID, err))
			m.markYYBBindingExpired(userID, binding, err)
			return model.DashboardSnapshot{}, err
		}
	}
	m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("yyb_get_code_succeeded", deviceID, 0))
	cookieResult, err := moceleClient.ExchangeCode(ctx, deviceID, code)
	if err != nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError(moceleDiagnosticCode(err), deviceID, err))
		m.markYYBBindingError(userID, binding, err)
		return model.DashboardSnapshot{}, err
	}
	m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("mocele_autologin_succeeded", deviceID, 0))
	now := time.Now().UTC()
	binding.Status = "alive"
	binding.LastError = ""
	binding.LastCheckedAt = &now
	if err := m.SetYYBBinding(userID, binding); err != nil {
		return model.DashboardSnapshot{}, err
	}
	snapshot, err := m.UpdateCookie(userID, cookieResult.Cookie)
	if err != nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("new_cookie_validation_failed", deviceID, err))
		m.markYYBBindingError(userID, binding, err)
		return model.DashboardSnapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) markYYBBindingExpired(userID string, binding *model.YYBBinding, cause error) {
	binding.Status = "expired"
	m.markYYBBindingError(userID, binding, cause)
}

func (m *Manager) markYYBBindingError(userID string, binding *model.YYBBinding, cause error) {
	now := time.Now().UTC()
	binding.LastCheckedAt = &now
	if cause != nil {
		binding.LastError = cause.Error()
	}
	_ = m.SetYYBBinding(userID, binding)
}

func (m *Manager) YYBBinding(userID string) (*model.YYBBinding, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return cloneYYBBinding(runtime.yybBinding), nil
}

func (m *Manager) SetYYBBinding(userID string, binding *model.YYBBinding) error {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.yybBinding = cloneYYBBinding(binding)
	runtime.mu.Unlock()
	return m.Save()
}

func cloneYYBBinding(binding *model.YYBBinding) *model.YYBBinding {
	if binding == nil {
		return nil
	}
	clone := *binding
	if binding.LastCheckedAt != nil {
		value := *binding.LastCheckedAt
		clone.LastCheckedAt = &value
	}
	return &clone
}

func (m *Manager) FirstDeviceID(userID string) (string, bool, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return "", false, err
	}
	ids := runtime.client.DeviceIDs()
	if len(ids) == 0 {
		return "", false, nil
	}
	return ids[0], true, nil
}

func (m *Manager) RecoveryDiagnostics(userID string) ([]model.RecoveryDiagnostic, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return nil, err
	}
	return runtime.recoveryDiagnosticsSnapshot(), nil
}

// RecordOperationDiagnostic persists a fixed, non-sensitive operation result
// for administrator troubleshooting. It intentionally accepts no error value.
func (m *Manager) RecordOperationDiagnostic(userID, operation, code, deviceID string, statusCode int) {
	m.recordDiagnostic(userID, model.RecoveryDiagnostic{
		Operation: operation, Code: code, DeviceSuffix: deviceID, StatusCode: statusCode,
	})
}

func (m *Manager) recordRecoveryDiagnostic(userID string, diagnostic model.RecoveryDiagnostic) {
	diagnostic.Operation = diagnosticOperationRecovery
	m.recordDiagnostic(userID, diagnostic)
}

func (m *Manager) recordDiagnostic(userID string, diagnostic model.RecoveryDiagnostic) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return
	}
	// Callers may carry an upstream error. Normalize every displayable field here
	// so a future call site cannot accidentally persist a cookie, code, ref, or
	// raw response body.
	diagnostic.Operation = normalizeDiagnosticOperation(diagnostic.Operation)
	diagnostic.Message = recoveryDiagnosticMessage(diagnostic.Code)
	diagnostic.DeviceSuffix = deviceSuffix(diagnostic.DeviceSuffix)
	if diagnostic.StatusCode < 100 || diagnostic.StatusCode > 599 {
		diagnostic.StatusCode = 0
	}
	if diagnostic.At.IsZero() {
		diagnostic.At = time.Now().UTC()
	}
	runtime.mu.Lock()
	runtime.recoveryDiagnostics = append(runtime.recoveryDiagnostics, diagnostic)
	if overflow := len(runtime.recoveryDiagnostics) - maxRecoveryDiagnostics; overflow > 0 {
		runtime.recoveryDiagnostics = append([]model.RecoveryDiagnostic(nil), runtime.recoveryDiagnostics[overflow:]...)
	}
	runtime.mu.Unlock()
	_ = m.Save()
}

func recoveryDiagnostic(code string, deviceID string, statusCode int) model.RecoveryDiagnostic {
	return model.RecoveryDiagnostic{
		Operation: diagnosticOperationRecovery, Code: code, Message: recoveryDiagnosticMessage(code), DeviceSuffix: deviceSuffix(deviceID), StatusCode: statusCode,
	}
}

func recoveryDiagnosticWithError(code string, deviceID string, err error) model.RecoveryDiagnostic {
	return recoveryDiagnostic(code, deviceID, DiagnosticStatusCode(err))
}

func recoveryDiagnosticMessage(code string) string {
	messages := map[string]string{
		"pile_identifier_required":          "未填写桩号或设备长 ID",
		"pile_id_invalid":                   "设备长 ID 格式不正确",
		"pile_number_invalid":               "桩号格式不正确",
		"pile_fields_invalid":               "充电桩字段长度超出限制",
		"pile_port_count_invalid":           "充电口数量不在允许范围内",
		"add_pile_failed":                   "添加充电桩时未能完成远端校验",
		"pile_update_failed":                "更新充电桩信息失败",
		"pile_delete_failed":                "删除充电桩失败",
		"refresh_failed":                    "刷新设备状态失败，请稍后重试",
		"cookie_required":                   "未填写登录凭据",
		"cookie_too_large":                  "登录凭据内容过长",
		"cookie_update_failed":              "更新登录凭据失败，请检查凭据后重试",
		"qr_create_failed":                  "扫码登录二维码生成失败",
		"qr_poll_failed":                    "扫码登录状态获取失败",
		"qr_confirm_failed":                 "扫码登录确认失败",
		"qr_session_invalid":                "扫码登录会话已失效",
		"scan_service_unavailable":          "扫码登录服务暂不可用",
		"binding_save_failed":               "扫码登录绑定保存失败",
		"credential_sync_failed":            "登录凭据同步失败",
		"device_id_invalid":                 "设备长 ID 格式不正确",
		"auth_rate_limited":                 "登录防护触发临时限流",
		"remote_auth_rejected":              "远端拒绝原登录凭据，开始自动恢复",
		"recovery_unavailable":              "无法自动恢复：缺少已绑定的账号或设备",
		"binding_missing":                   "无法同步凭据：尚未完成扫码登录绑定",
		"yyb_get_code_failed":               "扫码服务未能生成临时登录凭据",
		"yyb_account_refresh_failed":        "扫码服务刷新已绑定账号失败",
		"yyb_account_refresh_succeeded":     "扫码服务已刷新已绑定账号",
		"yyb_get_code_retry_failed":         "刷新账号后仍无法生成临时登录凭据",
		"yyb_get_code_succeeded":            "扫码服务已生成临时登录凭据",
		"mocele_autologin_missing_info":     "自动登录未返回必要的 info 凭据",
		"mocele_autologin_missing_wxopenid": "自动登录未返回必要的 wxopenid 凭据",
		"mocele_autologin_failed":           "自动登录服务请求失败",
		"mocele_autologin_succeeded":        "自动登录服务已生成新凭据",
		"new_cookie_validation_failed":      "新凭据校验设备接口失败",
		"recovery_succeeded":                "登录凭据已自动恢复并校验成功",
		"recovery_failed":                   "登录凭据自动恢复失败",
	}
	if message, ok := messages[code]; ok {
		return message
	}
	return "用户操作未能完成，请稍后重试"
}

func normalizeDiagnosticOperation(operation string) string {
	switch operation {
	case diagnosticOperationAddPile, diagnosticOperationRefresh, diagnosticOperationCookie, diagnosticOperationScan, diagnosticOperationSync, diagnosticOperationAuth:
		return operation
	default:
		return diagnosticOperationRecovery
	}
}

func moceleDiagnosticCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "missing info cookie"):
		return "mocele_autologin_missing_info"
	case strings.Contains(message, "missing wxopenid cookie"):
		return "mocele_autologin_missing_wxopenid"
	default:
		return "mocele_autologin_failed"
	}
}

// DiagnosticStatusCode extracts only a valid HTTP status from a diagnostic
// error. Callers must never persist or expose the original error text.
func DiagnosticStatusCode(err error) int {
	if err == nil {
		return 0
	}
	match := recoveryStatusCodePattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 0
	}
	var statusCode int
	_, _ = fmt.Sscanf(match[1], "%d", &statusCode)
	return statusCode
}

func deviceSuffix(deviceID string) string {
	deviceID = strings.TrimSpace(deviceID)
	if len(deviceID) <= 4 {
		return deviceID
	}
	return deviceID[len(deviceID)-4:]
}
