package api

import (
	"context"
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
	"sync"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/model"
	appruntime "charge-dashboard/internal/runtime"
	"charge-dashboard/internal/yyb"
)

const (
	sessionCookieName = "charge_session"
	authBodyLimit     = 16 * 1024
	adminBodyLimit    = 16 * 1024
	pileBodyLimit     = 4 * 1024
	cookieBodyLimit   = 32 * 1024
)

var deviceIDPattern = regexp.MustCompile(`^[0-9]{6,64}$`)
var pileNumberPattern = regexp.MustCompile(`^[0-9]{6,64}$`)
var qrSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

type Server struct {
	manager             *appruntime.Manager
	sessions            *auth.SessionManager
	turnstile           *auth.TurnstileVerifier
	authGuard           *auth.AuthGuard
	captcha             *auth.CaptchaStore
	yybClient           yybSessionClient
	moceleClient        appruntime.MoceleCookieClient
	devMu               sync.Mutex
	devForceAuthExpired bool
}

type yybSessionClient interface {
	appruntime.YYBCodeClient
	CreateQR(ctx context.Context) (yyb.QRSession, error)
	PollQR(ctx context.Context, sessionID string) (yyb.QRPollResult, error)
	ConfirmQR(ctx context.Context, sessionID string) (yyb.YYBAccount, error)
	Health(ctx context.Context) error
}

func (s *Server) SetYYBIntegration(yybClient yybSessionClient, moceleClient appruntime.MoceleCookieClient) {
	s.yybClient = yybClient
	s.moceleClient = moceleClient
}

// EnableDevForceAuthExpired enables a one-time local development refresh simulation.
func (s *Server) EnableDevForceAuthExpired() {
	s.devMu.Lock()
	s.devForceAuthExpired = true
	s.devMu.Unlock()
}

func (s *Server) consumeDevForceAuthExpired() bool {
	s.devMu.Lock()
	defer s.devMu.Unlock()
	if !s.devForceAuthExpired {
		return false
	}
	s.devForceAuthExpired = false
	return true
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
	mux.HandleFunc("/api/user/usage-guide/ack", s.handleUsageGuideAck)
	mux.HandleFunc("/api/admin/users", s.handleAdminUsers)
	mux.HandleFunc("/api/admin/users/", s.handleAdminUserActions)
	mux.HandleFunc("/api/admin/stats", s.handleAdminStats)
	mux.HandleFunc("/api/admin/health", s.handleAdminHealth)
	mux.HandleFunc("/api/admin/settings", s.handleAdminSettings)
	mux.HandleFunc("/api/admin/invites", s.handleAdminInvites)
	mux.HandleFunc("/api/admin/invites/", s.handleAdminInviteActions)
	mux.HandleFunc("/api/piles", s.handlePiles)
	mux.HandleFunc("/api/piles/", s.handlePileActions)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/session/cookie", s.handleCookieUpdate)
	mux.HandleFunc("/api/session/yyb-binding", s.handleYYBBinding)
	mux.HandleFunc("/api/session/yyb-qr", s.handleYYBQR)
	mux.HandleFunc("/api/session/yyb-qr/", s.handleYYBQR)
	mux.HandleFunc("/api/session/mocele-cookie", s.handleMoceleCookie)
	mux.HandleFunc(streamPath, s.handleStream)
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
		writePublicOperationError(w, http.StatusInternalServerError, "generate register captcha", "暂时无法生成验证码，请稍后重试。", err)
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
		writeCodedError(w, http.StatusBadRequest, "AUTH_INPUT_INVALID", "用户名或密码格式无效")
		return
	}
	if !s.allowAuthIdentity(w, ip, req.Username) {
		return
	}
	if err := s.turnstile.Verify(r.Context(), req.CaptchaToken, ip, "login"); err != nil {
		s.writeAuthFailure(w, ip, "", http.StatusBadRequest, "TURNSTILE_INVALID", "verify login turnstile", "人机验证失败，请重试。", err)
		return
	}

	user, err := s.manager.Authenticate(req.Username, req.Password)
	if err != nil {
		s.writeAuthFailure(w, ip, req.Username, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "authenticate user", "用户名或密码错误", err)
		return
	}
	s.authGuard.RecordSuccess(ip, req.Username)

	session, err := s.sessions.Create(user.ID)
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
		s.writeAuthFailure(w, ip, req.Username, http.StatusBadRequest, "REGISTER_CAPTCHA_INVALID", "verify register captcha", "图片验证码错误或已过期，请重新获取。", err)
		return
	}
	if err := s.turnstile.Verify(r.Context(), req.CaptchaToken, ip, "register"); err != nil {
		s.writeAuthFailure(w, ip, "", http.StatusBadRequest, "TURNSTILE_INVALID", "verify register turnstile", "人机验证失败，请重试。", err)
		return
	}

	user, err := s.manager.RegisterUser(req.Username, req.Password, req.InviteCode)
	if err != nil {
		writePublicOperationError(w, http.StatusBadRequest, "register user", "注册失败，请检查填写信息或邀请码后重试。", err)
		return
	}
	s.authGuard.RecordSuccess(ip, req.Username)

	session, err := s.sessions.Create(user.ID)
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "create register session", "注册已完成，但暂时无法创建登录状态，请稍后重新登录。", err)
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
		writePublicOperationError(w, http.StatusBadRequest, "change password", "修改密码失败，请检查当前密码后重试。", err)
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

func (s *Server) handlePiles(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		snapshot, err := s.manager.Snapshot(user.ID)
		if err != nil {
			writePublicOperationError(w, http.StatusInternalServerError, "load pile snapshot", "暂时无法加载充电桩信息，请稍后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	case http.MethodPost:
		var req model.PileUpsertRequest
		if !decodeJSON(w, r, pileBodyLimit, &req) {
			return
		}
		req.ID = strings.TrimSpace(req.ID)
		req.Number = strings.TrimSpace(req.Number)
		if req.ID == "" && req.Number == "" {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_identifier_required", "", http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_IDENTIFIER_REQUIRED", "请输入桩号或设备长ID")
			return
		}
		if req.ID != "" && !deviceIDPattern.MatchString(req.ID) {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_id_invalid", req.ID, http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_ID_INVALID", "设备ID必须是 6-64 位数字")
			return
		}
		if req.Number != "" && !pileNumberPattern.MatchString(req.Number) {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_number_invalid", "", http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_NUMBER_INVALID", "桩号必须是 6-64 位数字")
			return
		}
		if len(req.Name) > 128 || len(req.Number) > 64 || len(req.Status) > 32 || len(req.Address) > 256 {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_fields_invalid", req.ID, http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_FIELDS_INVALID", "充电桩字段长度超出限制")
			return
		}
		if req.OpenNum < 0 || req.OpenNum > 20 {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_port_count_invalid", req.ID, http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_PORT_COUNT_INVALID", "充电口数量必须在 1-20 之间")
			return
		}
		var pile model.Pile
		var err error
		if s.yybClient != nil && s.moceleClient != nil {
			pile, err = s.manager.AddPileWithYYB(user.ID, req, s.yybClient, s.moceleClient)
		} else {
			pile, err = s.manager.AddPile(user.ID, req)
		}
		if err != nil {
			s.recordDashboardDiagnostic(user, "add_pile", "add_pile_failed", req.ID, appruntime.DiagnosticStatusCode(err))
			writeAddPileError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, pile)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) recordDashboardDiagnostic(user model.CurrentUser, operation, code, deviceID string, statusCode int) {
	s.manager.RecordOperationDiagnostic(user.ID, operation, code, deviceID, statusCode)
}

func writeAddPileError(w http.ResponseWriter, err error) {
	if errors.Is(err, appruntime.ErrYYBBindingRequired) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code":  "YYB_BINDING_REQUIRED",
			"error": "请先完成扫码登录绑定，再添加充电桩",
		})
		return
	}
	writePublicOperationError(w, http.StatusBadRequest, "add pile", "添加充电桩失败，请检查桩号后重试。", err)
}

func (s *Server) handlePileActions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}

	if r.URL.Path == "/api/piles/order" {
		if r.Method != http.MethodPut {
			methodNotAllowed(w)
			return
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		if !decodeJSON(w, r, pileBodyLimit, &req) {
			return
		}
		snapshot, err := s.manager.ReorderPiles(user.ID, req.IDs)
		if err != nil {
			writePublicOperationError(w, http.StatusBadRequest, "reorder piles", "调整充电桩顺序失败，请刷新后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if !deviceIDPattern.MatchString(parts[2]) {
		s.recordDashboardDiagnostic(user, "add_pile", "pile_id_invalid", parts[2], http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.manager.DeletePile(user.ID, parts[2]); err != nil {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_delete_failed", parts[2], appruntime.DiagnosticStatusCode(err))
			writePublicOperationError(w, http.StatusNotFound, "delete pile", "未找到对应充电桩或暂时无法删除，请稍后重试。", err)
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
			s.recordDashboardDiagnostic(user, "add_pile", "pile_update_failed", parts[2], appruntime.DiagnosticStatusCode(err))
			writePublicOperationError(w, http.StatusBadRequest, "update pile", "更新充电桩失败，请检查填写信息后重试。", err)
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

	var snapshot model.DashboardSnapshot
	var err error
	if s.consumeDevForceAuthExpired() {
		if s.yybClient == nil || s.moceleClient == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "本地鉴权失效测试需要先配置 YYB 扫码服务"})
			return
		}
		snapshot, err = s.manager.RecoverRefreshWithYYB(user.ID, s.yybClient, s.moceleClient)
	} else if s.yybClient != nil && s.moceleClient != nil {
		snapshot, err = s.manager.RefreshWithYYB(user.ID, false, s.yybClient, s.moceleClient)
	} else {
		snapshot, err = s.manager.Refresh(user.ID, false)
	}
	if err != nil {
		s.recordDashboardDiagnostic(user, "refresh", "refresh_failed", "", appruntime.DiagnosticStatusCode(err))
		writePublicOperationError(w, http.StatusInternalServerError, "refresh piles", "暂时无法刷新设备状态，请稍后重试。", err)
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

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.manager.ListUsersPage(adminUserListQuery(r)))
	case http.MethodPost:
		var req model.UserCreateRequest
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		user, err := s.manager.CreateUser(req)
		if err != nil {
			writePublicOperationError(w, http.StatusBadRequest, "create admin user", "创建用户失败，请检查填写信息后重试。", err)
			return
		}
		writeJSON(w, http.StatusCreated, user)
	default:
		methodNotAllowed(w)
	}
}

func adminUserListQuery(r *http.Request) model.AdminUserListQuery {
	values := r.URL.Query()
	return model.AdminUserListQuery{
		Page:       queryInt(values.Get("page"), 1),
		PageSize:   queryInt(values.Get("pageSize"), 15),
		Search:     values.Get("search"),
		Account:    values.Get("account"),
		Credential: values.Get("credential"),
		Health:     values.Get("health"),
	}
}

func queryInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
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
			writePublicOperationError(w, http.StatusBadRequest, "update admin user", "更新用户失败，请检查填写信息后重试。", err)
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
			writePublicOperationError(w, http.StatusBadRequest, "delete admin user", "删除用户失败，请稍后重试。", err)
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

func (s *Server) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	result := model.AdminHealth{
		CheckedAt: time.Now(),
		Charge: model.ServiceHealth{
			State: model.HealthHealthy, Message: "服务正常",
		},
		Database: model.ServiceHealth{
			State: model.HealthHealthy, Message: "存储正常",
		},
		YYB: model.ServiceHealth{
			State: model.HealthUnavailable, Message: "扫码服务未配置",
		},
	}
	if err := s.manager.Ping(ctx); err != nil {
		result.Database = model.ServiceHealth{
			State: model.HealthUnavailable, Message: "存储暂不可用",
		}
	}
	if s.yybClient != nil {
		if err := s.yybClient.Health(ctx); err != nil {
			result.YYB = model.ServiceHealth{
				State: model.HealthDegraded, Message: "扫码服务连接异常",
			}
		} else {
			result.YYB = model.ServiceHealth{
				State: model.HealthHealthy, Message: "扫码服务正常",
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
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
			writePublicOperationError(w, http.StatusBadRequest, "update settings", "保存系统策略失败，请检查设置后重试。", err)
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
		values := r.URL.Query()
		writeJSON(w, http.StatusOK, s.manager.ListInviteCodesPage(
			queryInt(values.Get("page"), 1),
			queryInt(values.Get("pageSize"), 20),
		))
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
			writePublicOperationError(w, http.StatusBadRequest, "create invite", "创建邀请码失败，请稍后重试。", err)
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
		writePublicOperationError(w, http.StatusNotFound, "delete invite", "邀请码不存在或暂时无法删除，请稍后重试。", err)
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
		writePublicOperationError(w, http.StatusInternalServerError, "subscribe stream", "暂时无法建立实时连接，请稍后重试。", err)
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

func writeCodedError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}

func writePublicOperationError(w http.ResponseWriter, status int, operation, message string, err error) {
	log.Printf("%s: %v", operation, err)
	writeJSON(w, status, map[string]string{"error": message})
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

func (s *Server) writeAuthFailure(w http.ResponseWriter, ip string, username string, status int, code, operation, message string, err error) {
	if locked, retryAfter := s.authGuard.RecordFailure(ip, username); locked {
		if userID, ok := s.manager.UserIDByUsername(username); ok {
			s.manager.RecordOperationDiagnostic(userID, "auth_protection", "auth_rate_limited", "", http.StatusTooManyRequests)
		}
		writeRateLimit(w, retryAfter, "失败次数过多，已临时锁定")
		return
	}
	log.Printf("%s: %v", operation, err)
	writeCodedError(w, status, code, message)
}

func writeRateLimit(w http.ResponseWriter, retryAfter time.Duration, message string) {
	seconds := int(retryAfter.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"code":              "RATE_LIMITED",
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
		// X-Real-IP 由反向代理用 proxy_set_header 覆写，客户端无法影响。
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
		// nginx 的 $proxy_add_x_forwarded_for 把真实对端追加在客户端提交的
		// X-Forwarded-For 之后，因此只有最右一段可信；取最左会让限流和登录
		// 失败锁定被任意伪造的头绕过。
		if forwarded := rightmostForwardedFor(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
	}
	if host == "" {
		return "unknown"
	}
	return host
}

func rightmostForwardedFor(header string) string {
	parts := strings.Split(header, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if value := strings.TrimSpace(parts[i]); value != "" {
			return value
		}
	}
	return ""
}
