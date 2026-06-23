package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/model"
	appruntime "charge-dashboard/internal/runtime"
)

const (
	sessionCookieName = "charge_session"
	authBodyLimit     = 16 * 1024
	adminBodyLimit    = 16 * 1024
	pileBodyLimit     = 4 * 1024
	cookieBodyLimit   = 32 * 1024
)

var deviceIDPattern = regexp.MustCompile(`^[0-9]{6,64}$`)

type Server struct {
	manager   *appruntime.Manager
	sessions  *auth.SessionManager
	turnstile *auth.TurnstileVerifier
	authGuard *auth.AuthGuard
	captcha   *auth.CaptchaStore
}

func NewServer(
	manager *appruntime.Manager,
	sessions *auth.SessionManager,
	turnstile *auth.TurnstileVerifier,
	authGuard *auth.AuthGuard,
) *Server {
	return &Server{
		manager:   manager,
		sessions:  sessions,
		turnstile: turnstile,
		authGuard: authGuard,
		captcha:   auth.NewCaptchaStore(),
	}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/config", s.handleAuthConfig)
	mux.HandleFunc("/api/auth/register-captcha", s.handleRegisterCaptcha)
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/register", s.handleRegister)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/me", s.handleMe)
	mux.HandleFunc("/api/auth/password", s.handlePassword)
	mux.HandleFunc("/api/auth/sessions", s.handleSessions)
	mux.HandleFunc("/api/auth/sessions/others", s.handleOtherSessions)
	mux.HandleFunc("/api/admin/users", s.handleAdminUsers)
	mux.HandleFunc("/api/admin/users/", s.handleAdminUserActions)
	mux.HandleFunc("/api/admin/stats", s.handleAdminStats)
	mux.HandleFunc("/api/admin/settings", s.handleAdminSettings)
	mux.HandleFunc("/api/admin/invites", s.handleAdminInvites)
	mux.HandleFunc("/api/admin/invites/", s.handleAdminInviteActions)
	mux.HandleFunc("/api/piles", s.handlePiles)
	mux.HandleFunc("/api/piles/", s.handlePileActions)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/session/cookie", s.handleCookieUpdate)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/healthz", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"authConfigVersion":      2,
		"turnstileEnabled":       s.turnstile.Enabled(),
		"turnstileSiteKey":       s.turnstile.SiteKey(),
		"registerCaptchaEnabled": true,
		"registrationOpen":       s.manager.Settings().OpenRegistration,
		"inviteRequired":         s.manager.Settings().InviteRequired,
	})
}

func (s *Server) handleRegisterCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if !s.allowAuthRate(w, clientIP(r)) {
		return
	}

	challenge, err := s.captcha.Generate()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, challenge)
}

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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "用户名或密码格式无效"})
		return
	}
	if !s.allowAuthIdentity(w, ip, req.Username) {
		return
	}
	if err := s.turnstile.Verify(r.Context(), req.CaptchaToken, ip, "login"); err != nil {
		s.writeAuthFailure(w, ip, "", http.StatusBadRequest, err.Error())
		return
	}

	user, err := s.manager.Authenticate(req.Username, req.Password)
	if err != nil {
		s.writeAuthFailure(w, ip, req.Username, http.StatusUnauthorized, err.Error())
		return
	}
	s.authGuard.RecordSuccess(ip, req.Username)

	session, err := s.sessions.Create(user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "用户名需要 3-64 个字符，密码需要 8-128 个字符"})
		return
	}
	if !s.allowAuthIdentity(w, ip, req.Username) {
		return
	}
	if err := s.captcha.Verify(req.CaptchaID, req.CaptchaAnswer); err != nil {
		s.writeAuthFailure(w, ip, req.Username, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.turnstile.Verify(r.Context(), req.CaptchaToken, ip, "register"); err != nil {
		s.writeAuthFailure(w, ip, "", http.StatusBadRequest, err.Error())
		return
	}

	user, err := s.manager.RegisterUser(req.Username, req.Password, req.InviteCode)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.authGuard.RecordSuccess(ip, req.Username)

	session, err := s.sessions.Create(user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	setSessionCookie(w, r, session)
	writeJSON(w, http.StatusCreated, user)
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
	var req model.PasswordChangeRequest
	if !decodeJSON(w, r, authBodyLimit, &req) {
		return
	}
	if err := s.manager.ChangePassword(user.ID, req.CurrentPassword, req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cookie, _ := r.Cookie(sessionCookieName)
	current := ""
	if cookie != nil {
		current = cookie.Value
	}
	if err := s.sessions.DeleteOthers(user.ID, current); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密码已修改，但撤销其他会话失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePiles(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		snapshot, err := s.manager.Snapshot(user.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	case http.MethodPost:
		var req model.PileUpsertRequest
		if !decodeJSON(w, r, pileBodyLimit, &req) {
			return
		}
		req.ID = strings.TrimSpace(req.ID)
		if !deviceIDPattern.MatchString(req.ID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "设备ID必须是 6-64 位数字"})
			return
		}
		if len(req.Name) > 128 || len(req.Number) > 64 || len(req.Status) > 32 || len(req.Address) > 256 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "充电桩字段长度超出限制"})
			return
		}
		if req.OpenNum < 0 || req.OpenNum > 20 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "充电口数量必须在 1-20 之间"})
			return
		}
		pile, err := s.manager.AddPile(user.ID, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, pile)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handlePileActions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if !deviceIDPattern.MatchString(parts[2]) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.manager.DeletePile(user.ID, parts[2]); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch:
		var req struct {
			Name      string `json:"name"`
			Address   string `json:"address"`
			SortOrder int    `json:"sortOrder"`
		}
		if !decodeJSON(w, r, pileBodyLimit, &req) {
			return
		}
		pile, err := s.manager.UpdatePile(user.ID, parts[2], req.Name, req.Address, req.SortOrder)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, pile)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	snapshot, err := s.manager.Refresh(user.ID, false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cookie is required"})
		return
	}
	if len(req.Cookie) > 24*1024 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cookie is too large"})
		return
	}

	snapshot, err := s.manager.UpdateCookie(user.ID, req.Cookie)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.manager.ListUsers())
	case http.MethodPost:
		var req model.UserCreateRequest
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		user, err := s.manager.CreateUser(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, user)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminUserActions(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	userID := parts[3]
	switch r.Method {
	case http.MethodPatch:
		var req model.UserUpdateRequest
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		user, err := s.manager.UpdateUser(userID, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if req.Password != nil || req.Role != nil || req.Enabled != nil {
			if err := s.sessions.DeleteUser(userID); err != nil {
				log.Printf("revoke sessions for user %s: %v", userID, err)
				clearSessionCookie(w, r)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "用户已更新，但撤销旧登录状态失败"})
				return
			}
		}
		if userID == admin.ID {
			clearSessionCookie(w, r)
		}
		writeJSON(w, http.StatusOK, user)
	case http.MethodDelete:
		if err := s.manager.DeleteUser(userID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := s.sessions.DeleteUser(userID); err != nil {
			log.Printf("revoke sessions for deleted user %s: %v", userID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "用户已删除，但清理登录状态失败"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.AdminStats())
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.manager.Settings())
	case http.MethodPut:
		var req model.RegistrationSettings
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		if err := s.manager.UpdateSettings(req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, s.manager.Settings())
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminInvites(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.manager.InviteCodes())
	case http.MethodPost:
		var req struct {
			Code      string     `json:"code,omitempty"`
			ExpiresAt *time.Time `json:"expiresAt,omitempty"`
		}
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		invite, err := s.manager.CreateInvite(req.Code, req.ExpiresAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, invite)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminInviteActions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/invites/")
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := s.manager.DeleteInvite(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	ch, err := s.manager.Subscribe(user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer s.manager.Unsubscribe(user.ID, ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case snapshot, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(snapshot)
			if err != nil {
				log.Printf("marshal snapshot: %v", err)
				continue
			}
			if _, err = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (model.CurrentUser, bool) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return model.CurrentUser{}, false
	}
	if user.Role != model.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin permission required"})
		return model.CurrentUser{}, false
	}
	return user, true
}

func (s *Server) requireDashboardUser(w http.ResponseWriter, r *http.Request) (model.CurrentUser, bool) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return model.CurrentUser{}, false
	}
	if user.Role != model.RoleUser {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "ordinary user permission required"})
		return model.CurrentUser{}, false
	}
	return user, true
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (model.CurrentUser, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login required"})
		return model.CurrentUser{}, false
	}
	session, ok := s.sessions.Get(cookie.Value)
	if !ok {
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session expired"})
		return model.CurrentUser{}, false
	}
	user, ok := s.manager.User(session.UserID)
	if !ok {
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user disabled or removed"})
		return model.CurrentUser{}, false
	}
	return user, true
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, session auth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		Expires:  session.ExpiresAt,
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, target any) bool {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "content type must be application/json"})
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body is too large"})
			return false
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must contain one JSON object"})
		return false
	}
	return true
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback() &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func (s *Server) allowAuthRate(w http.ResponseWriter, ip string) bool {
	if allowed, retryAfter := s.authGuard.AllowRequest(ip); !allowed {
		writeRateLimit(w, retryAfter, "请求过于频繁，请稍后再试")
		return false
	}
	return true
}

func (s *Server) allowAuthIdentity(w http.ResponseWriter, ip string, username string) bool {
	if locked, retryAfter := s.authGuard.Locked(ip, username); locked {
		writeRateLimit(w, retryAfter, "登录或验证失败次数过多，请稍后再试")
		return false
	}
	return true
}

func (s *Server) writeAuthFailure(w http.ResponseWriter, ip string, username string, status int, message string) {
	if locked, retryAfter := s.authGuard.RecordFailure(ip, username); locked {
		writeRateLimit(w, retryAfter, "失败次数过多，已临时锁定")
		return
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func writeRateLimit(w http.ResponseWriter, retryAfter time.Duration, message string) {
	seconds := int(retryAfter.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":             message,
		"retryAfterSeconds": seconds,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	parsed := net.ParseIP(host)
	if parsed != nil && parsed.IsLoopback() {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			return forwarded
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
	}
	if host == "" {
		return "unknown"
	}
	return host
}
