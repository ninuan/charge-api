package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/mocele"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	appruntime "charge-dashboard/internal/runtime"
	"charge-dashboard/internal/yyb"
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

func TestDevForceAuthExpiredIsConsumedOnce(t *testing.T) {
	server, _, _ := newTestServer(t)
	if server.consumeDevForceAuthExpired() {
		t.Fatal("test switch should be disabled by default")
	}
	server.EnableDevForceAuthExpired()
	if !server.consumeDevForceAuthExpired() {
		t.Fatal("test switch was not consumed")
	}
	if server.consumeDevForceAuthExpired() {
		t.Fatal("test switch must only affect one refresh")
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
	server.SetYYBIntegration(&fakeAPIYYBClient{}, &fakeAPIMoceleClient{})
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
	now := time.Now().UTC()
	if err := manager.SetYYBBinding(user.ID, &model.YYBBinding{
		OpenID: "private-open-id", Ref: "private-ref", Nickname: "Privacy User",
		Status: "alive", BoundAt: now, LastCheckedAt: &now,
	}); err != nil {
		t.Fatalf("SetYYBBinding: %v", err)
	}
	session, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	server.SetYYBIntegration(&fakeAPIYYBClient{}, &fakeAPIMoceleClient{})

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

func TestWriteAddPileErrorReturnsFriendlyMessageWhenYYBBindingIsMissing(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeAddPileError(recorder, appruntime.ErrYYBBindingRequired)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var payload struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != "YYB_BINDING_REQUIRED" {
		t.Fatalf("code = %q, want YYB_BINDING_REQUIRED", payload.Code)
	}
	if payload.Error != "请先完成扫码登录绑定，再添加充电桩" {
		t.Fatalf("error = %q", payload.Error)
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
	server, manager, _ := newTestServer(t)
	invite, err := manager.CreateInvite("TEST-INVITE", nil)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

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
	server.handleRegisterCaptcha(captchaRecorder, httptest.NewRequest(http.MethodGet, "/api/auth/register-captcha", nil))
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
		"captchaToken":  "",
		"captchaId":     challenge.ID,
		"captchaAnswer": captchaAnswerFromImage(t, challenge.Image),
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
	return NewServer(manager, sessions, auth.NewTurnstileVerifier("", "", ""), auth.NewAuthGuard()), manager, sessions, user
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

func TestYYBBindingStatusResponseIsRedacted(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "yybuser",
		Password: "password123",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := manager.SetYYBBinding(user.ID, &model.YYBBinding{
		OpenID:        "yyb-openid-secret-1234",
		Ref:           "secret-ref",
		Nickname:      "display name",
		Avatar:        "https://avatar.example/secret-avatar.png",
		Status:        "alive",
		BoundAt:       now,
		LastCheckedAt: &now,
	}); err != nil {
		t.Fatalf("SetYYBBinding: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	mux := http.NewServeMux()
	server.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/api/session/yyb-binding", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("binding status returned %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, secret := range []string{"yyb-openid-secret", "secret-ref", "secret-avatar"} {
		if strings.Contains(body, secret) {
			t.Fatalf("binding response leaked %q: %s", secret, body)
		}
	}
	var payload struct {
		Bound         bool   `json:"bound"`
		OpenIDSuffix  string `json:"openidSuffix"`
		Nickname      string `json:"nickname"`
		Status        string `json:"status"`
		BoundAt       string `json:"boundAt"`
		LastCheckedAt string `json:"lastCheckedAt"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&payload); err != nil {
		t.Fatalf("decode binding response: %v", err)
	}
	if !payload.Bound || payload.OpenIDSuffix != "1234" || payload.Nickname != "display name" || payload.Status != "alive" || payload.BoundAt == "" || payload.LastCheckedAt == "" {
		t.Fatalf("unexpected binding payload: %#v", payload)
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

func TestYYBBindingOwnershipUsesCurrentSessionUser(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	userA, err := manager.CreateUser(model.UserCreateRequest{Username: "owner-a", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("CreateUser A: %v", err)
	}
	userB, err := manager.CreateUser(model.UserCreateRequest{Username: "owner-b", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("CreateUser B: %v", err)
	}
	sessionA, err := sessions.Create(userA.ID)
	if err != nil {
		t.Fatalf("Create session A: %v", err)
	}
	sessionB, err := sessions.Create(userB.ID)
	if err != nil {
		t.Fatalf("Create session B: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)

	setBinding(t, manager, userA.ID, "yyb-openid-a-1234", "ref-a", "owner A")

	bodyA := getBindingBody(t, mux, sessionA.Token)
	if !strings.Contains(bodyA, `"bound":true`) || !strings.Contains(bodyA, `"openidSuffix":"1234"`) || !strings.Contains(bodyA, `"nickname":"owner A"`) {
		t.Fatalf("user A binding response = %s", bodyA)
	}
	bodyB := getBindingBody(t, mux, sessionB.Token)
	if !strings.Contains(bodyB, `"bound":false`) {
		t.Fatalf("user B should not see user A binding: %s", bodyB)
	}

	setBinding(t, manager, userB.ID, "yyb-openid-b-5678", "ref-b", "owner B")
	bodyB = getBindingBody(t, mux, sessionB.Token)
	if !strings.Contains(bodyB, `"openidSuffix":"5678"`) || !strings.Contains(bodyB, `"nickname":"owner B"`) {
		t.Fatalf("user B self binding response = %s", bodyB)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/session/yyb-binding", nil)
	deleteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionB.Token})
	deleteRec := httptest.NewRecorder()
	mux.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete binding returned %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	bodyB = getBindingBody(t, mux, sessionB.Token)
	if !strings.Contains(bodyB, `"bound":false`) {
		t.Fatalf("user B binding not deleted: %s", bodyB)
	}
	bodyA = getBindingBody(t, mux, sessionA.Token)
	if !strings.Contains(bodyA, `"nickname":"owner A"`) {
		t.Fatalf("deleting user B binding affected user A: %s", bodyA)
	}
}

func TestYYBBindingRejectsAnonymousRequests(t *testing.T) {
	server, _, _ := newTestServer(t)
	mux := http.NewServeMux()
	server.Register(mux)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		body := strings.NewReader(`{"openid":"secret"}`)
		request := httptest.NewRequest(method, "/api/session/yyb-binding", body)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s anonymous returned %d, want 401: %s", method, recorder.Code, recorder.Body.String())
		}
	}
}

func TestYYBBindingRejectsClientSuppliedRef(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{Username: "ref-injection", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)

	body := bytes.NewReader([]byte(`{"openid":"attacker","ref":"1","status":"alive"}`))
	request := httptest.NewRequest(http.MethodPost, "/api/session/yyb-binding", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post binding returned %d, want 405: %s", recorder.Code, recorder.Body.String())
	}

	binding, err := manager.YYBBinding(user.ID)
	if err != nil {
		t.Fatalf("YYBBinding: %v", err)
	}
	if binding != nil {
		t.Fatalf("client supplied ref was persisted: %+v", binding)
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

func setBinding(t *testing.T, manager *appruntime.Manager, userID, openID, ref, nickname string) {
	t.Helper()
	if err := manager.SetYYBBinding(userID, &model.YYBBinding{
		OpenID:   openID,
		Ref:      ref,
		Nickname: nickname,
		Status:   "alive",
		BoundAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SetYYBBinding: %v", err)
	}
}

func getBindingBody(t *testing.T, handler http.Handler, token string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/session/yyb-binding", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get binding returned %d: %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}

func TestYYBProxyEndpointsRequireLoginAndConfiguration(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{Username: "yyb-proxy", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)

	anonymous := httptest.NewRequest(http.MethodPost, "/api/session/yyb-qr", nil)
	anonymousRecorder := httptest.NewRecorder()
	mux.ServeHTTP(anonymousRecorder, anonymous)
	if anonymousRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous yyb-qr returned %d, want 401", anonymousRecorder.Code)
	}

	configuredRequest := httptest.NewRequest(http.MethodPost, "/api/session/yyb-qr", nil)
	configuredRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	configuredRecorder := httptest.NewRecorder()
	mux.ServeHTTP(configuredRecorder, configuredRequest)
	if configuredRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured yyb-qr returned %d, want 503: %s", configuredRecorder.Code, configuredRecorder.Body.String())
	}
}

func TestYYBProxyFlowBindsCurrentUserAndSyncsCookie(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{Username: "yyb-flow", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	yybClient := &fakeAPIYYBClient{code: "wx-code-from-yyb"}
	moceleClient := &fakeAPIMoceleClient{cookie: "deviceid=2601201412385560001; org=1; openindex=7; wxopenid=open; info=info"}
	server.SetYYBIntegration(yybClient, moceleClient)
	mux := http.NewServeMux()
	server.Register(mux)

	createReq := httptest.NewRequest(http.MethodPost, "/api/session/yyb-qr", nil)
	createReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create qr returned %d: %s", createRec.Code, createRec.Body.String())
	}
	if !strings.Contains(createRec.Body.String(), `"sessionId":"sid-1"`) || !strings.Contains(createRec.Body.String(), `"imageUrl":"/qr/sid-1/image"`) {
		t.Fatalf("create qr body = %s", createRec.Body.String())
	}

	pollReq := httptest.NewRequest(http.MethodGet, "/api/session/yyb-qr/sid-1/poll", nil)
	pollReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	pollRec := httptest.NewRecorder()
	mux.ServeHTTP(pollRec, pollReq)
	if pollRec.Code != http.StatusOK || !strings.Contains(pollRec.Body.String(), `"status":"confirmed"`) {
		t.Fatalf("poll returned %d: %s", pollRec.Code, pollRec.Body.String())
	}

	confirmReq := httptest.NewRequest(http.MethodPost, "/api/session/yyb-qr/sid-1/confirm", nil)
	confirmReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	confirmRec := httptest.NewRecorder()
	mux.ServeHTTP(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("confirm returned %d: %s", confirmRec.Code, confirmRec.Body.String())
	}
	if !strings.Contains(confirmRec.Body.String(), `"cookieSynced":false`) {
		t.Fatalf("confirm without devices should report cookieSynced=false: %s", confirmRec.Body.String())
	}
	binding, err := manager.YYBBinding(user.ID)
	if err != nil {
		t.Fatalf("YYBBinding: %v", err)
	}
	if binding == nil || binding.Ref != "ref-1" || binding.OpenID != "openid-1" || binding.Status != "alive" {
		t.Fatalf("binding = %#v", binding)
	}

	body := strings.NewReader(`{"deviceId":"2601201412385560001"}`)
	syncReq := httptest.NewRequest(http.MethodPost, "/api/session/mocele-cookie", body)
	syncReq.Header.Set("Content-Type", "application/json")
	syncReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	syncRec := httptest.NewRecorder()
	mux.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("sync returned %d: %s", syncRec.Code, syncRec.Body.String())
	}
	if yybClient.getCodeRef != "ref-1" || yybClient.getCodeAppID != "wx9cbffc15d3cb7739" {
		t.Fatalf("getCode ref/appid = %q/%q", yybClient.getCodeRef, yybClient.getCodeAppID)
	}
	if moceleClient.deviceID != "2601201412385560001" || moceleClient.code != "wx-code-from-yyb" {
		t.Fatalf("mocele call = device %q code %q", moceleClient.deviceID, moceleClient.code)
	}
}

func TestYYBConfirmSyncsCookieWhenUserAlreadyHasDevice(t *testing.T) {
	server, _, sessions, user := newTestServerWithDevice(t, "2601201412385560001")
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	yybClient := &fakeAPIYYBClient{code: "wx-code-existing"}
	moceleClient := &fakeAPIMoceleClient{cookie: "deviceid=2601201412385560001; org=1; openindex=7; wxopenid=open; info=info"}
	server.SetYYBIntegration(yybClient, moceleClient)
	mux := http.NewServeMux()
	server.Register(mux)

	confirmReq := httptest.NewRequest(http.MethodPost, "/api/session/yyb-qr/sid-1/confirm", nil)
	confirmReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	confirmRec := httptest.NewRecorder()
	mux.ServeHTTP(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("confirm returned %d: %s", confirmRec.Code, confirmRec.Body.String())
	}
	if !strings.Contains(confirmRec.Body.String(), `"cookieSynced":true`) {
		t.Fatalf("confirm with device should report cookieSynced=true: %s", confirmRec.Body.String())
	}
	if moceleClient.deviceID != "2601201412385560001" || moceleClient.code != "wx-code-existing" {
		t.Fatalf("mocele call = device %q code %q", moceleClient.deviceID, moceleClient.code)
	}
}

type fakeAPIYYBClient struct {
	code         string
	getCodeRef   string
	getCodeAppID string
	healthErr    error
}

func (f *fakeAPIYYBClient) CreateQR(ctx context.Context) (yyb.QRSession, error) {
	return yyb.QRSession{SessionID: "sid-1", ImageURL: "/qr/sid-1/image", ImageBase64: "data:image/jpeg;base64,abc", Status: "waiting"}, nil
}

func (f *fakeAPIYYBClient) PollQR(ctx context.Context, sessionID string) (yyb.QRPollResult, error) {
	return yyb.QRPollResult{SessionID: sessionID, Status: "confirmed", Message: "ready"}, nil
}

func (f *fakeAPIYYBClient) ConfirmQR(ctx context.Context, sessionID string) (yyb.YYBAccount, error) {
	return yyb.YYBAccount{Ref: "ref-1", OpenID: "openid-1", Nickname: "Alice", Avatar: "https://avatar.example/a.png", Status: "alive"}, nil
}

func (f *fakeAPIYYBClient) GetCode(ctx context.Context, ref string, appID string) (string, error) {
	f.getCodeRef = ref
	f.getCodeAppID = appID
	return f.code, nil
}

func (f *fakeAPIYYBClient) RefreshAccount(ctx context.Context, ref string) error { return nil }

func (f *fakeAPIYYBClient) Health(context.Context) error { return f.healthErr }

type fakeAPIMoceleClient struct {
	cookie   string
	deviceID string
	code     string
}

func (f *fakeAPIMoceleClient) ExchangeCode(ctx context.Context, deviceID string, code string) (mocele.CookieResult, error) {
	f.deviceID = deviceID
	f.code = code
	return mocele.CookieResult{Cookie: f.cookie, WXOpenID: "open", Info: "info"}, nil
}
