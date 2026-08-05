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
	healthMu            sync.RWMutex
	healthDegradations  map[string]string
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
		manager:            manager,
		sessions:           sessions,
		turnstile:          turnstile,
		authGuard:          authGuard,
		captcha:            auth.NewCaptchaStore(),
		healthDegradations: make(map[string]string),
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
	mux.HandleFunc("/api/admin/trends", s.handleAdminTrends)
	mux.HandleFunc("/api/admin/trends.csv", s.handleAdminTrendsCSV)
	mux.HandleFunc("/api/admin/health", s.handleAdminHealth)
	mux.HandleFunc("/api/admin/incidents", s.handleAdminIncidents)
	mux.HandleFunc("/api/admin/incidents/", s.handleAdminIncidentActions)
	mux.HandleFunc("/api/admin/audit", s.handleAdminAudit)
	mux.HandleFunc("/api/admin/operations", s.handleAdminOperations)
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
	s.sessions.Touch(cookie.Value)
	user, ok := s.manager.User(session.UserID)
	if !ok {
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user disabled or removed"})
		return model.CurrentUser{}, false
	}
	return user, true
}

func sessionClientInfo(r *http.Request) auth.SessionClientInfo {
	userAgent := strings.ToLower(r.UserAgent())
	browser := "其他浏览器"
	switch {
	case strings.Contains(userAgent, "edg/"):
		browser = "Edge"
	case strings.Contains(userAgent, "firefox/"):
		browser = "Firefox"
	case strings.Contains(userAgent, "chrome/") || strings.Contains(userAgent, "crios/"):
		browser = "Chrome"
	case strings.Contains(userAgent, "safari/"):
		browser = "Safari"
	}
	osName := "其他系统"
	switch {
	case strings.Contains(userAgent, "iphone") || strings.Contains(userAgent, "ipad"):
		osName = "iOS"
	case strings.Contains(userAgent, "android"):
		osName = "Android"
	case strings.Contains(userAgent, "windows"):
		osName = "Windows"
	case strings.Contains(userAgent, "mac os"):
		osName = "macOS"
	case strings.Contains(userAgent, "linux"):
		osName = "Linux"
	}
	deviceType := "电脑"
	switch {
	case strings.Contains(userAgent, "ipad") || strings.Contains(userAgent, "tablet"):
		deviceType = "平板"
	case strings.Contains(userAgent, "mobile") || strings.Contains(userAgent, "iphone") || strings.Contains(userAgent, "android"):
		deviceType = "手机"
	}
	return auth.SessionClientInfo{
		Browser: browser, OS: osName, DeviceType: deviceType,
		IPLabel: maskedIP(clientIP(r)),
	}
}

func maskedIP(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return "未知网络"
	}
	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.*.*", v4[0], v4[1])
	}
	parts := strings.Split(ip.String(), ":")
	if len(parts) > 2 {
		return parts[0] + ":" + parts[1] + ":*"
	}
	return "IPv6"
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
