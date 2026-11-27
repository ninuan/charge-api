package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestUserCanSubmitPileOrder(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "order-user", Password: "password123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/piles/order",
		strings.NewReader(`{"ids":[]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("reorder status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var snapshot model.DashboardSnapshot
	if err := json.NewDecoder(recorder.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snapshot.Piles) != 0 {
		t.Fatalf("unexpected piles: %+v", snapshot.Piles)
	}
}

func TestHealthExposesReleaseVersion(t *testing.T) {
	server, _, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.handleHealth(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", recorder.Code)
	}
	var payload map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if payload["status"] != "ok" || payload["version"] != "1.5.5" {
		t.Fatalf("unexpected health response: %+v", payload)
	}
}

func TestAdminHealthRequiresAdminAndReturnsRedactedServiceState(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "health-user", Password: "password123", Role: model.RoleUser,
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
	mux := http.NewServeMux()
	server.Register(mux)

	anonymousRecorder := httptest.NewRecorder()
	mux.ServeHTTP(anonymousRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/health", nil))
	if anonymousRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymousRecorder.Code)
	}

	userRequest := httptest.NewRequest(http.MethodGet, "/api/admin/health", nil)
	userRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userSession.Token})
	userRecorder := httptest.NewRecorder()
	mux.ServeHTTP(userRecorder, userRequest)
	if userRecorder.Code != http.StatusForbidden {
		t.Fatalf("user status = %d, want 403", userRecorder.Code)
	}

	if _, err := manager.CreatePublicPoolCookie(model.PublicPoolCookieCreateRequest{
		Label: "pool-1", Cookie: "wxopenid=p1; info=ok1",
	}); err != nil {
		t.Fatalf("seed pool-1: %v", err)
	}
	if _, err := manager.CreatePublicPoolCookie(model.PublicPoolCookieCreateRequest{
		Label: "pool-2", Cookie: "wxopenid=p2; info=ok2",
	}); err != nil {
		t.Fatalf("seed pool-2: %v", err)
	}

	adminRequest := httptest.NewRequest(http.MethodGet, "/api/admin/health", nil)
	adminRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	adminRecorder := httptest.NewRecorder()
	mux.ServeHTTP(adminRecorder, adminRequest)
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("admin status = %d: %s", adminRecorder.Code, adminRecorder.Body.String())
	}
	var payload model.AdminHealth
	if err := json.NewDecoder(adminRecorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if payload.Charge.State != model.HealthHealthy ||
		payload.Database.State != model.HealthHealthy ||
		payload.YYB.State != model.HealthHealthy {
		t.Fatalf("unexpected health payload: %+v", payload)
	}
	body := adminRecorder.Body.String()
	for _, secret := range []string{"YYB_API_SECRET", "charge_state.db", "127.0.0.1:8000"} {
		if strings.Contains(body, secret) {
			t.Fatalf("health response leaked %q: %s", secret, body)
		}
	}
}

func TestAdminResponsesDoNotLeakIntegrationSecrets(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "privacy-user", Password: "password123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := manager.UpdateCookieWithPoolOption(user.ID, "wxopenid=private-open-id; info=private-ref", true); err != nil {
		t.Fatalf("UpdateCookieWithPoolOption: %v", err)
	}
	session, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	var response bytes.Buffer
	for _, path := range []string{"/api/admin/stats", "/api/admin/health"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
		recorder := httptest.NewRecorder()
		if path == "/api/admin/stats" {
			server.handleAdminStats(recorder, request)
		} else {
			server.handleAdminHealth(recorder, request)
		}
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, recorder.Code, recorder.Body.String())
		}
		response.Write(recorder.Body.Bytes())
	}

	for _, secret := range []string{
		"wxopenid=", "deviceid=", `"openId"`, `"ref"`, "private-open-id", "private-ref",
		"YYB_API_SECRET", "charge_state.db", "127.0.0.1:8000",
	} {
		if bytes.Contains(response.Bytes(), []byte(secret)) {
			t.Fatalf("admin response leaked %q: %s", secret, response.String())
		}
	}
}

func TestAdminUsersListsFilteredPageInsteadOfEveryAccount(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	adminSession, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create admin session: %v", err)
	}

	for i := 1; i <= 5; i++ {
		if _, err := manager.CreateUser(model.UserCreateRequest{
			Username: fmt.Sprintf("page-user-%02d", i),
			Password: "password123",
			Role:     model.RoleUser,
		}); err != nil {
			t.Fatalf("Create page user %d: %v", i, err)
		}
	}
	disabled := false
	if _, err := manager.CreateUser(model.UserCreateRequest{
		Username: "page-user-disabled", Password: "password123", Role: model.RoleUser, Enabled: &disabled,
	}); err != nil {
		t.Fatalf("Create disabled page user: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/users?page=2&pageSize=2&search=page-user&account=enabled", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	recorder := httptest.NewRecorder()
	server.handleAdminUsers(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Items      []model.AdminUserSummary `json:"items"`
		Page       int                      `json:"page"`
		PageSize   int                      `json:"pageSize"`
		Total      int                      `json:"total"`
		TotalPages int                      `json:"totalPages"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode paginated users: %v", err)
	}
	if payload.Page != 2 || payload.PageSize != 2 || payload.Total != 5 || payload.TotalPages != 3 {
		t.Fatalf("unexpected page metadata: %+v", payload)
	}
	if len(payload.Items) != 2 || payload.Items[0].User.Username != "page-user-03" || payload.Items[1].User.Username != "page-user-04" {
		t.Fatalf("unexpected second page: %+v", payload.Items)
	}
}

func TestAdminUserDetailAndDangerousActionAudit(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	adminSession, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create admin session: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)

	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users",
		strings.NewReader(`{"username":"detail-user","password":"password123","role":"user"}`),
	)
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	createRecorder := httptest.NewRecorder()
	mux.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created model.CurrentUser
	if err := json.NewDecoder(createRecorder.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if _, err := sessions.Create(created.ID, auth.SessionClientInfo{
		Browser: "Safari", OS: "iOS", DeviceType: "手机",
		IPLabel: "192.168.*.*",
	}); err != nil {
		t.Fatalf("Create target session: %v", err)
	}

	detailRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/admin/users/"+created.ID+"/detail",
		nil,
	)
	detailRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	detailRecorder := httptest.NewRecorder()
	mux.ServeHTTP(detailRecorder, detailRequest)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", detailRecorder.Code, detailRecorder.Body.String())
	}
	var detail model.AdminUserDetail
	if err := json.NewDecoder(detailRecorder.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Summary.User.Username != "detail-user" ||
		len(detail.Sessions) != 1 || detail.Sessions[0].IPLabel != "192.168.*.*" {
		t.Fatalf("unexpected detail: %+v", detail)
	}

	auditRequest := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	auditRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	auditRecorder := httptest.NewRecorder()
	mux.ServeHTTP(auditRecorder, auditRequest)
	if auditRecorder.Code != http.StatusOK {
		t.Fatalf("audit status = %d: %s", auditRecorder.Code, auditRecorder.Body.String())
	}
	var audit model.AuditPage
	if err := json.NewDecoder(auditRecorder.Body).Decode(&audit); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if audit.Total != 1 || audit.Items[0].Action != "user.create" ||
		audit.Items[0].TargetLabel != "detail-user" {
		t.Fatalf("unexpected audit: %+v", audit)
	}
}

func TestAdminIncidentCanBeAcknowledgedWithNote(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	adminSession, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create admin session: %v", err)
	}
	now := time.Now()
	if err := manager.RecordSystemIncident(model.SystemException{
		ID: "operations-backup", Username: "系统", Type: "backup",
		Level: "warning", Message: "最近备份失败", Time: now,
	}); err != nil {
		t.Fatalf("RecordSystemIncident: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)
	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/admin/incidents/operations-backup",
		strings.NewReader(`{"status":"acknowledged","note":"已安排重新备份"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("incident status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var issue model.SystemException
	if err := json.NewDecoder(recorder.Body).Decode(&issue); err != nil {
		t.Fatalf("decode incident: %v", err)
	}
	if issue.Status != "acknowledged" || issue.Note != "已安排重新备份" ||
		issue.HandledBy != "admin" || issue.HandledAt == nil {
		t.Fatalf("unexpected incident: %+v", issue)
	}
}

func TestWriteAddPileErrorRedactsUnexpectedError(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeAddPileError(recorder, fmt.Errorf("dial internal-yyb:8443 token=secret"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if strings.Contains(recorder.Body.String(), "internal-yyb") || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("response leaked an internal error: %s", recorder.Body.String())
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error != "添加充电桩失败，请检查桩号后重试。" {
		t.Fatalf("error = %q", payload.Error)
	}
}

func TestWriteCodedErrorReturnsOnlyStablePublicDetails(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeCodedError(recorder, http.StatusBadRequest, "PILE_NUMBER_INVALID", "桩号必须是 6-64 位数字")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var payload struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != "PILE_NUMBER_INVALID" || payload.Error != "桩号必须是 6-64 位数字" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestPileValidationReturnsStablePublicCode(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{Username: "pile-code-user", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/piles", strings.NewReader(`{"number":"not-a-number"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()

	server.handlePiles(recorder, request)

	var payload struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusBadRequest || payload.Code != "PILE_NUMBER_INVALID" || payload.Error != "桩号必须是 6-64 位数字" {
		t.Fatalf("unexpected response: status=%d payload=%#v", recorder.Code, payload)
	}
	diagnostics, err := manager.RecoveryDiagnostics(user.ID)
	if err != nil {
		t.Fatalf("RecoveryDiagnostics: %v", err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Operation != "add_pile" || diagnostics[0].Code != "pile_number_invalid" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestAuthLockRecordsOneSanitizedSecurityDiagnostic(t *testing.T) {
	server, manager, _ := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{Username: "locked-user", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		server.writeAuthFailure(httptest.NewRecorder(), "203.0.113.11", user.Username, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "authenticate user", "用户名或密码错误", errors.New("password mismatch"))
	}

	diagnostics, err := manager.RecoveryDiagnostics(user.ID)
	if err != nil {
		t.Fatalf("RecoveryDiagnostics: %v", err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Operation != "auth_protection" || diagnostics[0].Code != "auth_rate_limited" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestAdminResetPasswordGeneratesTemporaryPasswordAndRevokesSessions(t *testing.T) {
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

	request := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+user.ID+"/reset-password", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	recorder := httptest.NewRecorder()
	server.handleAdminUserActions(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("reset returned %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("temporary password response is cacheable: %q", recorder.Header().Get("Cache-Control"))
	}
	var payload model.TemporaryPasswordResponse
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode temporary password: %v", err)
	}
	if len(payload.TemporaryPassword) < 8 {
		t.Fatal("temporary password is unexpectedly short")
	}
	if _, ok := sessions.Get(userSession.Token); ok {
		t.Fatal("target user's previous session remains valid")
	}
	current, err := manager.Authenticate(user.Username, payload.TemporaryPassword)
	if err != nil {
		t.Fatalf("Authenticate with temporary password: %v", err)
	}
	if !current.MustChangePassword {
		t.Fatal("temporary-password login did not require a password change")
	}
}

func TestAdminInviteIsGeneratedByServer(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	session, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/invites", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	server.handleAdminInvites(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("create invite returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var invite model.InviteCode
	if err := json.NewDecoder(recorder.Body).Decode(&invite); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	if !strings.HasPrefix(invite.Code, "CHG-") {
		t.Fatalf("invite code = %q, want CHG- prefix", invite.Code)
	}
}

func TestAdminInvitesArePaginated(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	session, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	for _, code := range []string{"CHG-PAGE-ONE", "CHG-PAGE-TWO", "CHG-PAGE-THREE"} {
		if _, err := manager.CreateInvite(code, nil); err != nil {
			t.Fatalf("CreateInvite(%q): %v", code, err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/invites?page=2&pageSize=2", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	server.handleAdminInvites(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("list invites returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var page model.InviteCodePage
	if err := json.NewDecoder(recorder.Body).Decode(&page); err != nil {
		t.Fatalf("decode invite page: %v", err)
	}
	if page.Page != 2 || page.PageSize != 2 || page.Total != 3 || page.TotalPages != 2 {
		t.Fatalf("unexpected page metadata: %+v", page)
	}
	if len(page.Items) != 1 {
		t.Fatalf("page item count = %d, want 1", len(page.Items))
	}
}

func TestUserCanAcknowledgeUsageGuide(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "alice",
		Password: "password123",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	ackRequest := httptest.NewRequest(http.MethodPost, "/api/user/usage-guide/ack", nil)
	ackRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	ackRecorder := httptest.NewRecorder()
	server.handleUsageGuideAck(ackRecorder, ackRequest)

	if ackRecorder.Code != http.StatusOK {
		t.Fatalf("ack returned %d: %s", ackRecorder.Code, ackRecorder.Body.String())
	}
	var acknowledged model.CurrentUser
	if err := json.NewDecoder(ackRecorder.Body).Decode(&acknowledged); err != nil {
		t.Fatalf("decode ack response: %v", err)
	}
	if acknowledged.UsageGuideAckAt == nil {
		t.Fatal("ack response did not include usageGuideAckAt")
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	meRecorder := httptest.NewRecorder()
	server.handleMe(meRecorder, meRequest)

	if meRecorder.Code != http.StatusOK {
		t.Fatalf("me returned %d: %s", meRecorder.Code, meRecorder.Body.String())
	}
	var current model.CurrentUser
	if err := json.NewDecoder(meRecorder.Body).Decode(&current); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if current.UsageGuideAckAt == nil || !current.UsageGuideAckAt.Equal(*acknowledged.UsageGuideAckAt) {
		t.Fatalf("usage guide ack did not persist through me: ack=%v me=%v", acknowledged.UsageGuideAckAt, current.UsageGuideAckAt)
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

	body := strings.NewReader(`{"username":"alice","password":"password123"}`)
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

func TestAuthConfigExposesBuiltInCaptcha(t *testing.T) {
	server, _, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.handleAuthConfig(recorder, httptest.NewRequest(http.MethodGet, "/api/auth/config", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("auth config returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var config map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&config); err != nil {
		t.Fatalf("decode auth config: %v", err)
	}
	if config["loginCaptchaEnabled"] != true || config["registerCaptchaEnabled"] != true {
		t.Fatalf("unexpected captcha config: %#v", config)
	}
	if _, exists := config["hcaptchaEnabled"]; exists {
		t.Fatalf("removed hCaptcha field is still exposed: %#v", config)
	}
}

func TestRegisterAcceptsGeneratedCaptcha(t *testing.T) {
	server, manager, _ := newTestServer(t)
	invite, err := manager.CreateInvite("TEST-INVITE", nil)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	captchaRecorder := httptest.NewRecorder()
	server.captcha = &fixedCaptcha{answer: "24682"}
	captchaRequest := httptest.NewRequest(http.MethodGet, "/api/auth/captcha", nil)
	server.handleAuthCaptcha(captchaRecorder, captchaRequest)
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
	payload := map[string]string{
		"username":      "alice",
		"password":      "password123",
		"captchaId":     challenge.ID,
		"captchaAnswer": "24682",
		"inviteCode":    invite.Code,
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

func TestInviteOnlyRegistrationThroughAPI(t *testing.T) {
	server, manager, _ := newTestServer(t)
	server.captcha = &fixedCaptcha{answer: "24682"}
	settings := manager.Settings()
	settings.OpenRegistration = false
	settings.InviteRequired = true
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	invite, err := manager.CreateInvite("", nil)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	captchaRecorder := httptest.NewRecorder()
	server.handleAuthCaptcha(captchaRecorder, httptest.NewRequest(http.MethodGet, "/api/auth/captcha", nil))
	var challenge struct {
		ID    string `json:"id"`
		Image string `json:"image"`
	}
	if err := json.NewDecoder(captchaRecorder.Body).Decode(&challenge); err != nil {
		t.Fatalf("decode captcha: %v", err)
	}
	payload, err := json.Marshal(map[string]string{
		"username":      "invite-only-user",
		"password":      "password123",
		"captchaId":     challenge.ID,
		"captchaAnswer": "24682",
		"inviteCode":    invite.Code,
	})
	if err != nil {
		t.Fatalf("marshal registration payload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.handleRegister(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("invite-only register returned %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRegisterReturnsTypedErrorCodes(t *testing.T) {
	server, manager, _ := newTestServer(t)
	server.captcha = &fixedCaptcha{answer: "24682"}

	postRegister := func(username, inviteCode string) *httptest.ResponseRecorder {
		t.Helper()
		captchaRecorder := httptest.NewRecorder()
		server.handleAuthCaptcha(captchaRecorder, httptest.NewRequest(http.MethodGet, "/api/auth/captcha", nil))
		var challenge struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(captchaRecorder.Body).Decode(&challenge); err != nil {
			t.Fatalf("decode captcha: %v", err)
		}
		payload, err := json.Marshal(map[string]string{
			"username":      username,
			"password":      "password123",
			"captchaId":     challenge.ID,
			"captchaAnswer": "24682",
			"inviteCode":    inviteCode,
		})
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.handleRegister(rec, req)
		return rec
	}

	if rec := postRegister("dup-user", ""); rec.Code != http.StatusCreated {
		t.Fatalf("first register returned %d: %s", rec.Code, rec.Body.String())
	}
	if rec := postRegister("dup-user", ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "REGISTER_USERNAME_TAKEN") {
		t.Fatalf("duplicate register = %d %s, want 409 REGISTER_USERNAME_TAKEN", rec.Code, rec.Body.String())
	}
	if rec := postRegister("bad-invite-user", "NO-SUCH-INVITE"); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "REGISTER_INVITE_INVALID") {
		t.Fatalf("invalid invite register = %d %s, want 400 REGISTER_INVITE_INVALID", rec.Code, rec.Body.String())
	}

	settings := manager.Settings()
	settings.OpenRegistration = false
	settings.InviteRequired = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if rec := postRegister("closed-user", ""); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "REGISTER_CLOSED") {
		t.Fatalf("closed register = %d %s, want 403 REGISTER_CLOSED", rec.Code, rec.Body.String())
	}
}

func TestWithRootRedirectUsesSessionRoleWithoutCaching(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "redirect-user", Password: "password123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	userSession, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("create user session: %v", err)
	}
	var adminID string
	for _, summary := range manager.ListUsers() {
		if summary.User.Role == model.RoleAdmin {
			adminID = summary.User.ID
			break
		}
	}
	adminSession, err := sessions.Create(adminID)
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}

	nextCalled := false
	handler := WithCacheHeaders(server.WithRootRedirect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})))

	for _, tc := range []struct {
		name         string
		token        string
		wantLocation string
	}{
		{"anonymous", "", "/login/"},
		{"invalid token", "bogus-token", "/login/"},
		{"regular user", userSession.Token, "/dashboard/"},
		{"admin", adminSession.Token, "/admin/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.token != "" {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tc.token})
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != tc.wantLocation {
				t.Fatalf("Location = %q, want %q", got, tc.wantLocation)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !nextCalled || rec.Code != http.StatusOK {
		t.Fatalf("non-root path did not pass through: nextCalled=%v code=%d", nextCalled, rec.Code)
	}
}

func TestLoginRequiresCaptchaAfterTwoCredentialFailures(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.captcha = &fixedCaptcha{answer: "24682"}
	login := func(password, captchaID, captchaAnswer string) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(map[string]string{
			"username": "admin", "password": password,
			"captchaId": captchaID, "captchaAnswer": captchaAnswer,
		})
		if err != nil {
			t.Fatalf("marshal login: %v", err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "203.0.113.20:4567"
		recorder := httptest.NewRecorder()
		server.handleLogin(recorder, request)
		return recorder
	}

	first := login("wrong-password", "", "")
	if first.Code != http.StatusUnauthorized || !strings.Contains(first.Body.String(), "AUTH_INVALID_CREDENTIALS") {
		t.Fatalf("first failure = %d %s", first.Code, first.Body.String())
	}
	second := login("wrong-password", "", "")
	if second.Code != http.StatusUnauthorized || !strings.Contains(second.Body.String(), "LOGIN_CAPTCHA_REQUIRED") {
		t.Fatalf("second failure = %d %s", second.Code, second.Body.String())
	}
	withoutCaptcha := login("admin-password-123", "", "")
	if withoutCaptcha.Code != http.StatusBadRequest || !strings.Contains(withoutCaptcha.Body.String(), "LOGIN_CAPTCHA_INVALID") {
		t.Fatalf("login without captcha = %d %s", withoutCaptcha.Code, withoutCaptcha.Body.String())
	}
	withCaptcha := login("admin-password-123", "fixed-captcha", "24682")
	if withCaptcha.Code != http.StatusOK {
		t.Fatalf("login with captcha = %d %s", withCaptcha.Code, withCaptcha.Body.String())
	}
	if server.authGuard.RequiresCaptcha("203.0.113.20", "admin") {
		t.Fatal("captcha requirement was not cleared after successful login")
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
	return NewServer(manager, sessions, auth.NewAuthGuard()), manager, sessions
}

func newTestServerWithDevice(t *testing.T, deviceID string) (*Server, *appruntime.Manager, *auth.SessionManager, model.User) {
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
	now := time.Now()
	user := model.User{ID: "user-seeded", Username: "seeded", PasswordHash: "hash", Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now, DeviceLimit: 10, RefreshEnabled: false}
	state := persistence.State{
		Version: 3,
		Users:   []model.User{user},
		UserStates: map[string]persistence.UserState{
			user.ID: {DeviceIDs: []string{deviceID}},
		},
		Settings: model.RegistrationSettings{OpenRegistration: true, InviteRequired: true, DefaultDeviceLimit: 10, DefaultRefreshEnabled: true, StatsRetentionDays: 90},
	}
	if err := repository.Save(state); err != nil {
		t.Fatalf("Save seeded state: %v", err)
	}
	manager, err := appruntime.NewManager(repository, "", parser.DefaultCaptureRequests(), "", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	sessions := auth.NewSessionManager(time.Hour)
	t.Cleanup(sessions.Close)
	return NewServer(manager, sessions, auth.NewAuthGuard()), manager, sessions, user
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

type fixedCaptcha struct {
	answer string
}

func (c *fixedCaptcha) Generate() (auth.CaptchaChallenge, error) {
	return auth.CaptchaChallenge{
		ID: "fixed-captcha", Image: "data:image/png;base64,test", ExpiresAt: time.Now().Add(2 * time.Minute),
	}, nil
}

func (c *fixedCaptcha) Verify(id string, answer string) error {
	if id != "fixed-captcha" || answer != c.answer {
		return errors.New("invalid captcha")
	}
	return nil
}

func TestRetiredYYBEndpointsReturnNotFound(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "retired-yyb-user",
		Password: "password123",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/session/yyb-binding"},
		{http.MethodDelete, "/api/session/yyb-binding"},
		{http.MethodPost, "/api/session/yyb-qr"},
		{http.MethodGet, "/api/session/yyb-qr/sid-1/poll"},
		{http.MethodPost, "/api/session/mocele-cookie"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s returned %d, want 404: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestCookieUpdateResponseDoesNotEchoCookie(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "cookieuser",
		Password: "password123",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	secretCookie := "sid=secret-cookie; info=secret-info; wxopenid=secret-openid; verifycode=secret-code"
	payload, err := json.Marshal(map[string]string{"cookie": secretCookie})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/session/cookie", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	server.handleCookieUpdate(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("cookie update returned %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, secret := range []string{"secret-cookie", "secret-info", "secret-openid", "secret-code"} {
		if strings.Contains(body, secret) {
			t.Fatalf("cookie update response leaked %q: %s", secret, body)
		}
	}
}

func TestClientIPIgnoresForgedForwardedForPrefix(t *testing.T) {
	proxied := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	proxied.RemoteAddr = "127.0.0.1:54321"
	proxied.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9")
	if got := clientIP(proxied); got != "203.0.113.9" {
		t.Fatalf("clientIP with a proxy appended chain = %q, want the rightmost entry", got)
	}

	proxied.Header.Set("X-Real-IP", "203.0.113.9")
	proxied.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := clientIP(proxied); got != "203.0.113.9" {
		t.Fatalf("clientIP with X-Real-IP = %q, want the proxy supplied value", got)
	}

	direct := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	direct.RemoteAddr = "198.51.100.7:44444"
	direct.Header.Set("X-Forwarded-For", "1.2.3.4")
	direct.Header.Set("X-Real-IP", "1.2.3.4")
	if got := clientIP(direct); got != "198.51.100.7" {
		t.Fatalf("clientIP for a direct connection = %q, want the socket peer", got)
	}
}
