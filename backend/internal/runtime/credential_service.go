package runtime

import (
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

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
	_ = m.saveUserState(userID)
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
		"yyb_account_refresh_expired":       "扫码服务未能恢复登录状态，请重新扫码；失败原因需检查扫码服务",
		"yyb_account_refresh_unknown":       "扫码服务缺少账号恢复信息，请重新扫码",
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
