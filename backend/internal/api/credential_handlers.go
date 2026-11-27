package api

import (
	"errors"
	"net/http"
	"strings"

	"charge-dashboard/internal/charger"
	appruntime "charge-dashboard/internal/runtime"
)

func (s *Server) handleCookieUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodDelete {
		snapshot, err := s.manager.ClearPersonalCookie(user.ID)
		if err != nil {
			writePublicOperationError(w, http.StatusInternalServerError, "clear cookie", "暂时无法清除个人 Cookie，请稍后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req struct {
		Cookie         string `json:"cookie"`
		JoinPublicPool bool   `json:"joinPublicPool"`
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

	snapshot, err := s.manager.UpdateCookieWithPoolOption(user.ID, req.Cookie, req.JoinPublicPool)
	if err != nil {
		if errors.Is(err, appruntime.ErrPublicPoolCookieDuplicate) {
			s.recordDashboardDiagnostic(user, "update_cookie", "cookie_duplicate", "", http.StatusConflict)
			writeCodedError(w, http.StatusConflict, "PUBLIC_POOL_COOKIE_DUPLICATE", "该 Cookie 已在互助公共池中，请勿与其他账户共用同一个凭据。")
			return
		}
		s.recordDashboardDiagnostic(user, "update_cookie", "cookie_update_failed", "", appruntime.DiagnosticStatusCode(err))
		writePublicOperationError(w, http.StatusBadRequest, "update cookie", "登录信息更新失败，请检查内容后重试。", err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleCookieTest(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	snapshot, err := s.manager.TestPersonalCookie(user.ID)
	if err != nil {
		if errors.Is(err, appruntime.ErrPersonalCookieRequired) {
			writeCodedError(w, http.StatusBadRequest, "PERSONAL_COOKIE_REQUIRED", "当前账户尚未配置个人 Cookie")
			return
		}
		if charger.IsAuthExpired(err) {
			writeJSON(w, http.StatusOK, map[string]any{
				"valid":    false,
				"error":    "个人 Cookie 已失效，请尽快更新",
				"snapshot": snapshot,
			})
			return
		}
		logStructuredError("test_personal_cookie", user.ID, err)
		writeCodedError(w, http.StatusBadGateway, "COOKIE_TEST_FAILED", "测试个人 Cookie 失败，请检查网络或先添加至少 1 台充电桩后重试。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":    true,
		"snapshot": snapshot,
	})
}

func (s *Server) handleCookieJoinPool(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		snapshot, err := s.manager.JoinPublicPoolWithExistingCookie(user.ID)
		if err != nil {
			if errors.Is(err, appruntime.ErrPublicPoolCookieDuplicate) {
				writeCodedError(w, http.StatusConflict, "PUBLIC_POOL_COOKIE_DUPLICATE", "该 Cookie 已在互助公共池中，请勿与其他账户共用同一个凭据。")
				return
			}
			logStructuredError("join_public_pool", user.ID, err)
			writeCodedError(w, http.StatusBadRequest, "JOIN_POOL_FAILED", "加入互助公共池失败，请确认当前个人 Cookie 有效后重试。")
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	case http.MethodDelete:
		snapshot, err := s.manager.LeavePublicPool(user.ID)
		if err != nil {
			writePublicOperationError(w, http.StatusInternalServerError, "leave public pool", "退出互助公共池失败，请稍后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	default:
		methodNotAllowed(w)
	}
}
