package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	appruntime "charge-dashboard/internal/runtime"
)

func TestAdminCannotUseDashboardAPI(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	session, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/piles", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	server.handlePiles(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin dashboard request returned %d, want 403", recorder.Code)
	}
}

func TestAdminUpdateRevokesTargetUserSessions(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "alice",
		Password: "password123",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	adminSession, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create admin session: %v", err)
	}
	userSession, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create user session: %v", err)
	}

	body := strings.NewReader(`{"password":"new-password-123"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+user.ID, body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	recorder := httptest.NewRecorder()
	server.handleAdminUserActions(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("update returned %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, ok := sessions.Get(userSession.Token); ok {
		t.Fatal("target user's previous session remains valid")
	}
}

func TestDecodeJSONRejectsOversizedAndUnknownFields(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/test",
			strings.NewReader(`{"value":"`+strings.Repeat("x", 128)+`"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		var target struct {
			Value string `json:"value"`
		}
		if decodeJSON(recorder, request, 32, &target) {
			t.Fatal("expected oversized JSON to be rejected")
		}
		if recorder.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413", recorder.Code)
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(`{"unknown":true}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		var target struct {
			Value string `json:"value"`
		}
		if decodeJSON(recorder, request, 1024, &target) {
			t.Fatal("expected unknown field to be rejected")
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", recorder.Code)
		}
	})
}

func TestRegisterRequiresCaptcha(t *testing.T) {
	server, _, _ := newTestServer(t)

	body := strings.NewReader(`{"username":"alice","password":"password123","captchaToken":""}`)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.handleRegister(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("register returned %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "验证码") {
		t.Fatalf("response does not mention captcha: %s", recorder.Body.String())
	}
}

func TestRegisterAcceptsGeneratedCaptcha(t *testing.T) {
	server, _, _ := newTestServer(t)

	captchaRecorder := httptest.NewRecorder()
	captchaRequest := httptest.NewRequest(http.MethodGet, "/api/auth/register-captcha", nil)
	server.handleRegisterCaptcha(captchaRecorder, captchaRequest)
	if captchaRecorder.Code != http.StatusOK {
		t.Fatalf("captcha returned %d: %s", captchaRecorder.Code, captchaRecorder.Body.String())
	}
	var challenge struct {
		ID    string `json:"id"`
		Image string `json:"image"`
	}
	if err := json.NewDecoder(captchaRecorder.Body).Decode(&challenge); err != nil {
		t.Fatalf("decode captcha: %v", err)
	}
	answer := captchaAnswerFromImage(t, challenge.Image)

	payload := map[string]string{
		"username":      "alice",
		"password":      "password123",
		"captchaToken":  "",
		"captchaId":     challenge.ID,
		"captchaAnswer": answer,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(bodyBytes))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.handleRegister(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("register returned %d, want 201: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSecureCookieHonorsTrustedForwardedProto(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://charge.example/api/auth/login", nil)
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()
	setSessionCookie(recorder, request, auth.Session{
		Token:     "token",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	response := recorder.Result()
	defer response.Body.Close()
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("expected Secure cookie, got %+v", cookies)
	}
}

func newTestServer(t *testing.T) (*Server, *appruntime.Manager, *auth.SessionManager) {
	t.Helper()
	repository, err := persistence.OpenSQLite(
		t.TempDir()+"/state.db",
		bytes.Repeat([]byte{0x55}, persistence.CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close repository: %v", err)
		}
	})
	manager, err := appruntime.NewManager(
		repository,
		"",
		parser.DefaultCaptureRequests(),
		"admin-password-123",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	sessions := auth.NewSessionManager(time.Hour)
	t.Cleanup(sessions.Close)
	return NewServer(manager, sessions, auth.NewTurnstileVerifier("", "", ""), auth.NewAuthGuard()), manager, sessions
}

func findUser(t *testing.T, manager *appruntime.Manager, username string) model.CurrentUser {
	t.Helper()
	for _, summary := range manager.ListUsers() {
		if summary.User.Username == username {
			return summary.User
		}
	}
	encoded, _ := json.Marshal(manager.ListUsers())
	t.Fatalf("user %q not found in %s", username, encoded)
	return model.CurrentUser{}
}

func captchaAnswerFromImage(t *testing.T, image string) string {
	t.Helper()
	const prefix = "data:image/svg+xml;base64,"
	if !strings.HasPrefix(image, prefix) {
		t.Fatalf("captcha image has unexpected prefix: %q", image)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(image, prefix))
	if err != nil {
		t.Fatalf("decode captcha image: %v", err)
	}
	matches := regexp.MustCompile(`>([23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{5})</text>`).FindSubmatch(decoded)
	if len(matches) != 2 {
		t.Fatalf("captcha answer not found in svg: %s", decoded)
	}
	return string(matches[1])
}
