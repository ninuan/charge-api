package api

import (
	"net/http"
	"strings"

	"charge-dashboard/internal/model"
	appruntime "charge-dashboard/internal/runtime"
)

func (s *Server) handleCookieUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req struct {
		Cookie string `json:"cookie"`
	}
	if !decodeJSON(w, r, cookieBodyLimit, &req) {
		return
	}
	req.Cookie = strings.TrimSpace(req.Cookie)
	if req.Cookie == "" {
		s.recordDashboardDiagnostic(user, "update_cookie", "cookie_required", "", http.StatusBadRequest)
		writeCodedError(w, http.StatusBadRequest, "COOKIE_REQUIRED", "请输入 Cookie")
		return
	}
	if len(req.Cookie) > 24*1024 {
		s.recordDashboardDiagnostic(user, "update_cookie", "cookie_too_large", "", http.StatusBadRequest)
		writeCodedError(w, http.StatusBadRequest, "COOKIE_TOO_LARGE", "Cookie 内容过长")
		return
	}

	snapshot, err := s.manager.UpdateCookie(user.ID, req.Cookie)
	if err != nil {
		s.recordDashboardDiagnostic(user, "update_cookie", "cookie_update_failed", "", appruntime.DiagnosticStatusCode(err))
		writePublicOperationError(w, http.StatusBadRequest, "update cookie", "凭据更新失败，请检查内容后重试。", err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) requireYYBIntegration(w http.ResponseWriter, user model.CurrentUser, operation string) bool {
	if s.yybClient == nil || s.moceleClient == nil {
		s.recordDashboardDiagnostic(user, operation, "scan_service_unavailable", "", http.StatusServiceUnavailable)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "扫码服务暂不可用，请稍后重试。"})
		return false
	}
	return true
}

func (s *Server) handleYYBQR(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if !s.requireYYBIntegration(w, user, "scan_login") {
		return
	}
	if r.URL.Path == "/api/session/yyb-qr" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		qr, err := s.yybClient.CreateQR(r.Context())
		if err != nil {
			s.recordDashboardDiagnostic(user, "scan_login", "qr_create_failed", "", appruntime.DiagnosticStatusCode(err))
			writePublicOperationError(w, http.StatusBadGateway, "create YYB QR", "二维码暂时无法生成，请稍后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"sessionId":   qr.SessionID,
			"imageUrl":    qr.ImageURL,
			"imageBase64": qr.ImageBase64,
			"status":      qr.Status,
		})
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/session/yyb-qr/"), "/")
	// sessionID 会被拼进 sidecar 请求路径并参与 HMAC 签名，只放行安全字符，
	// 防止 %2F、?、# 等经解码后借服务端密钥为攻击者构造的请求背书。
	if len(parts) != 2 || !qrSessionIDPattern.MatchString(parts[0]) {
		s.recordDashboardDiagnostic(user, "scan_login", "qr_session_invalid", "", http.StatusNotFound)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "二维码会话已失效，请重新生成二维码"})
		return
	}
	sessionID, action := parts[0], parts[1]
	switch action {
	case "poll":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		result, err := s.yybClient.PollQR(r.Context(), sessionID)
		if err != nil {
			s.recordDashboardDiagnostic(user, "scan_login", "qr_poll_failed", "", appruntime.DiagnosticStatusCode(err))
			writePublicOperationError(w, http.StatusBadGateway, "poll YYB QR", "暂时无法获取扫码状态，请稍后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"sessionId": result.SessionID,
			"status":    result.Status,
			"message":   result.Message,
		})
	case "confirm":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		account, err := s.yybClient.ConfirmQR(r.Context(), sessionID)
		if err != nil {
			s.recordDashboardDiagnostic(user, "scan_login", "qr_confirm_failed", "", appruntime.DiagnosticStatusCode(err))
			writePublicOperationError(w, http.StatusBadGateway, "confirm YYB QR", "暂时无法确认扫码结果，请稍后重试。", err)
			return
		}
		if _, err := s.manager.SaveYYBBinding(user.ID, account); err != nil {
			s.recordDashboardDiagnostic(user, "scan_login", "binding_save_failed", "", appruntime.DiagnosticStatusCode(err))
			writePublicOperationError(w, http.StatusInternalServerError, "save YYB binding", "扫码已确认，但暂时无法保存绑定状态，请稍后重试。", err)
			return
		}
		payload, err := s.yybBindingStatusPayload(user.ID)
		if err != nil {
			writePublicOperationError(w, http.StatusInternalServerError, "load YYB binding", "扫码已确认，但暂时无法读取绑定状态，请稍后重试。", err)
			return
		}
		payload["cookieSynced"] = false
		payload["message"] = "扫码登录已完成。添加充电桩后，系统会自动更新登录凭据"
		if deviceID, ok, err := s.manager.FirstDeviceID(user.ID); err != nil {
			writePublicOperationError(w, http.StatusInternalServerError, "load first device", "扫码已确认，但暂时无法读取设备信息，请稍后重试。", err)
			return
		} else if ok {
			if _, err := s.manager.SyncCookieFromYYB(user.ID, deviceID, s.yybClient, s.moceleClient); err == nil {
				payload["cookieSynced"] = true
				payload["message"] = "扫码登录已完成，登录凭据已自动生效"
			} else {
				s.recordDashboardDiagnostic(user, "sync_cookie", "credential_sync_failed", deviceID, appruntime.DiagnosticStatusCode(err))
				payload["message"] = "扫码登录已完成，但凭据暂未生效；请稍后刷新或重新添加充电桩"
			}
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		s.recordDashboardDiagnostic(user, "scan_login", "qr_session_invalid", "", http.StatusNotFound)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "二维码会话已失效，请重新生成二维码"})
	}
}

func (s *Server) handleMoceleCookie(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if !s.requireYYBIntegration(w, user, "sync_cookie") {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		DeviceID string `json:"deviceId"`
	}
	if !decodeJSON(w, r, pileBodyLimit, &req) {
		return
	}
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if !deviceIDPattern.MatchString(req.DeviceID) {
		s.recordDashboardDiagnostic(user, "sync_cookie", "device_id_invalid", req.DeviceID, http.StatusBadRequest)
		writeCodedError(w, http.StatusBadRequest, "DEVICE_ID_INVALID", "设备 ID 格式无效")
		return
	}
	snapshot, err := s.manager.SyncCookieFromYYB(user.ID, req.DeviceID, s.yybClient, s.moceleClient)
	if err != nil {
		s.recordDashboardDiagnostic(user, "sync_cookie", "credential_sync_failed", req.DeviceID, appruntime.DiagnosticStatusCode(err))
		writePublicOperationError(w, http.StatusBadGateway, "sync YYB cookie", "暂时无法同步登录凭据，请稍后重试。", err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleYYBBinding(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	// 绑定只能由 handleYYBQRConfirm 的扫码流程写入：ref 决定服务端向 sidecar
	// 取哪个账号的 code，客户端提供的 ref 等同于任意账号凭据的读取权限。
	switch r.Method {
	case http.MethodGet:
		s.writeYYBBindingStatus(w, user.ID)
	case http.MethodDelete:
		if err := s.manager.ClearYYBBinding(user.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "删除绑定状态失败"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) writeYYBBindingStatus(w http.ResponseWriter, userID string) {
	payload, err := s.yybBindingStatusPayload(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "读取绑定状态失败"})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) yybBindingStatusPayload(userID string) (map[string]any, error) {
	binding, err := s.manager.YYBBinding(userID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return map[string]any{"bound": false}, nil
	}
	payload := map[string]any{
		"bound":        true,
		"openidSuffix": suffix(binding.OpenID, 4),
		"nickname":     binding.Nickname,
		"status":       binding.Status,
		"boundAt":      binding.BoundAt,
	}
	if binding.LastCheckedAt != nil {
		payload["lastCheckedAt"] = binding.LastCheckedAt
	}
	return payload, nil
}

func suffix(value string, n int) string {
	if n <= 0 || len(value) <= n {
		return value
	}
	return value[len(value)-n:]
}
