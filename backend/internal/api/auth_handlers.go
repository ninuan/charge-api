package api

import (
	"errors"
	"net/http"
	"strings"

	"charge-dashboard/internal/model"
	appruntime "charge-dashboard/internal/runtime"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	ip := clientIP(r)
	if !s.allowAuthRate(w, ip) {
		return
	}

	var req model.LoginRequest
	if !decodeJSON(w, r, authBodyLimit, &req) {
		return
	}
	if len(strings.TrimSpace(req.Username)) < 3 || len(req.Username) > 64 || len(req.Password) == 0 || len(req.Password) > 128 {
		writeCodedError(w, http.StatusBadRequest, "AUTH_INPUT_INVALID", "用户名或密码格式无效")
		return
	}
	if !s.allowAuthIdentity(w, ip, req.Username) {
		return
	}
	if s.authGuard.RequiresCaptcha(ip, req.Username) {
		if err := s.captcha.Verify(req.CaptchaID, req.CaptchaAnswer); err != nil {
			s.writeAuthFailure(w, ip, "", http.StatusBadRequest, "LOGIN_CAPTCHA_INVALID", "verify login captcha", "图片验证码错误或已过期，请重新获取。", err)
			return
		}
	}

	releaseAuth, ok := s.acquireAuthSlot(w)
	if !ok {
		return
	}
	defer releaseAuth()

	user, err := s.manager.Authenticate(req.Username, req.Password)
	if err != nil {
		s.writeLoginFailure(w, ip, req.Username, err)
		return
	}
	s.authGuard.RecordSuccess(ip, req.Username)

	session, err := s.sessions.Create(user.ID, sessionClientInfo(r))
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "create login session", "登录暂时不可用，请稍后重试。", err)
		return
	}
	setSessionCookie(w, r, session)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	ip := clientIP(r)
	if !s.allowAuthRate(w, ip) {
		return
	}

	var req model.LoginRequest
	if !decodeJSON(w, r, authBodyLimit, &req) {
		return
	}
	if len(strings.TrimSpace(req.Username)) < 3 || len(req.Username) > 64 || len(req.Password) < 8 || len(req.Password) > 128 {
		writeCodedError(w, http.StatusBadRequest, "REGISTER_INPUT_INVALID", "用户名需要 3-64 个字符，密码需要 8-128 个字符")
		return
	}
	if !s.allowAuthIdentity(w, ip, req.Username) {
		return
	}
	if err := s.captcha.Verify(req.CaptchaID, req.CaptchaAnswer); err != nil {
		s.writeAuthFailure(w, ip, "", http.StatusBadRequest, "REGISTER_CAPTCHA_INVALID", "verify register captcha", "图片验证码错误或已过期，请重新获取。", err)
		return
	}

	releaseAuth, ok := s.acquireAuthSlot(w)
	if !ok {
		return
	}
	defer releaseAuth()

	user, err := s.manager.RegisterUser(req.Username, req.Password, req.InviteCode)
	if err != nil {
		writeRegisterError(w, err)
		return
	}
	s.authGuard.RecordSuccess(ip, req.Username)

	session, err := s.sessions.Create(user.ID, sessionClientInfo(r))
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "create register session", "注册已完成，但暂时无法创建登录状态，请稍后重新登录。", err)
		return
	}
	setSessionCookie(w, r, session)
	writeJSON(w, http.StatusCreated, user)
}

func writeRegisterError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appruntime.ErrUsernameExists):
		writeCodedError(w, http.StatusConflict, "REGISTER_USERNAME_TAKEN", "该用户名已被注册，请换一个用户名。")
	case errors.Is(err, appruntime.ErrRegistrationClosed):
		writeCodedError(w, http.StatusForbidden, "REGISTER_CLOSED", "当前未开放自助注册，请联系管理员开通账户。")
	case errors.Is(err, appruntime.ErrInviteRegistrationClosed):
		writeCodedError(w, http.StatusForbidden, "REGISTER_INVITE_DISABLED", "当前未开放邀请码注册。")
	case errors.Is(err, appruntime.ErrInviteRequired):
		writeCodedError(w, http.StatusBadRequest, "REGISTER_INVITE_REQUIRED", "请输入有效邀请码。")
	case errors.Is(err, appruntime.ErrInviteInvalid):
		writeCodedError(w, http.StatusBadRequest, "REGISTER_INVITE_INVALID", "邀请码无效或已过期，请核对后重试。")
	case errors.Is(err, appruntime.ErrInvalidRegisterInput):
		writeCodedError(w, http.StatusBadRequest, "REGISTER_INPUT_INVALID", "用户名需要 3-64 个字符，密码需要 8-128 个字符")
	default:
		writePublicOperationError(w, http.StatusInternalServerError, "register user", "注册暂时不可用，请稍后重试。", err)
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.Delete(cookie.Value)
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	ip := clientIP(r)
	if !s.allowAuthRate(w, ip) || !s.allowAuthIdentity(w, ip, user.Username) {
		return
	}
	var req model.PasswordChangeRequest
	if !decodeJSON(w, r, authBodyLimit, &req) {
		return
	}
	releaseAuth, ok := s.acquireAuthSlot(w)
	if !ok {
		return
	}
	defer releaseAuth()

	changedUser, err := s.manager.ChangePassword(user.ID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		s.writeAuthFailure(w, ip, user.Username, http.StatusBadRequest, "PASSWORD_CHANGE_FAILED", "change password", "修改密码失败，请检查当前密码后重试。", err)
		return
	}
	s.authGuard.RecordSuccess(ip, user.Username)
	cookie, _ := r.Cookie(sessionCookieName)
	currentToken := ""
	if cookie != nil {
		currentToken = cookie.Value
	}
	if err := s.sessions.DeleteOthers(user.ID, currentToken); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密码已修改，但撤销其他会话失败"})
		return
	}
	writeJSON(w, http.StatusOK, changedUser)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	cookie, _ := r.Cookie(sessionCookieName)
	current := ""
	if cookie != nil {
		current = cookie.Value
	}
	sessions, err := s.sessions.List(user.ID, current)
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "list sessions", "暂时无法读取登录设备，请稍后重试。", err)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleOtherSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	cookie, _ := r.Cookie(sessionCookieName)
	current := ""
	if cookie != nil {
		current = cookie.Value
	}
	if err := s.sessions.DeleteOthers(user.ID, current); err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "delete other sessions", "暂时无法退出其他登录设备，请稍后重试。", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUsageGuideAck(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	current, err := s.manager.AcknowledgeUsageGuide(user.ID)
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "acknowledge usage guide", "暂时无法保存阅读状态，请稍后重试。", err)
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"authConfigVersion":      4,
		"loginCaptchaEnabled":    true,
		"registerCaptchaEnabled": true,
		"registrationOpen":       s.manager.Settings().OpenRegistration,
		"inviteRequired":         s.manager.Settings().InviteRequired,
	})
}

func (s *Server) handleAuthCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if !s.allowAuthRate(w, clientIP(r)) {
		return
	}

	challenge, err := s.captcha.Generate()
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "generate auth captcha", "暂时无法生成验证码，请稍后重试。", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, challenge)
}
