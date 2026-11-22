package api

import (
	"bytes"
	"encoding/json"
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

func TestPublicPoolAndCookieTestRegressions(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	mux := http.NewServeMux()
	server.Register(mux)

	admin, err := manager.Authenticate("admin", "admin-password-123")
	if err != nil {
		t.Fatalf("authenticate admin: %v", err)
	}
	adminSession, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}

	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "freeuser",
		Password: "FreeUserPass123!",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userSession, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("create user session: %v", err)
	}

	// 1. POST /api/session/cookie/test without a personal cookie returns 400 PERSONAL_COOKIE_REQUIRED.
	testReq := httptest.NewRequest(http.MethodPost, "/api/session/cookie/test", nil)
	testReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userSession.Token})
	testRec := httptest.NewRecorder()
	mux.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusBadRequest || !strings.Contains(testRec.Body.String(), "PERSONAL_COOKIE_REQUIRED") {
		t.Fatalf("cookie test without personal cookie = %d %s, want 400 PERSONAL_COOKIE_REQUIRED", testRec.Code, testRec.Body.String())
	}

	// 2. POST /api/admin/shared-piles/:id/refresh for nonexistent pile returns 404 SHARED_PILE_NOT_FOUND.
	refreshSharedReq := httptest.NewRequest(http.MethodPost, "/api/admin/shared-piles/2601201412385560999/refresh", nil)
	refreshSharedReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	refreshSharedRec := httptest.NewRecorder()
	mux.ServeHTTP(refreshSharedRec, refreshSharedReq)
	if refreshSharedRec.Code != http.StatusNotFound || !strings.Contains(refreshSharedRec.Body.String(), "SHARED_PILE_NOT_FOUND") {
		t.Fatalf("refresh missing shared pile = %d %s, want 404 SHARED_PILE_NOT_FOUND", refreshSharedRec.Code, refreshSharedRec.Body.String())
	}

	// 3. Save personal cookie with joinPublicPool=true -> adds user mutual-aid pool entry.
	cookieBody, _ := json.Marshal(map[string]any{
		"cookie":         "wxopenid=free-user-pool; info=ok",
		"joinPublicPool": true,
	})
	updateCookieReq := httptest.NewRequest(http.MethodPost, "/api/session/cookie", bytes.NewReader(cookieBody))
	updateCookieReq.Header.Set("Content-Type", "application/json")
	updateCookieReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userSession.Token})
	updateCookieRec := httptest.NewRecorder()
	mux.ServeHTTP(updateCookieRec, updateCookieReq)
	if updateCookieRec.Code != http.StatusOK {
		t.Fatalf("save cookie with joinPublicPool = %d %s", updateCookieRec.Code, updateCookieRec.Body.String())
	}
	if manager.PublicPoolAvailableCount() != 1 {
		t.Fatalf("PublicPoolAvailableCount = %d, want 1", manager.PublicPoolAvailableCount())
	}

	// 3a. A second user or admin cannot join/add the same wxopenid (even with different info/order/noise keys).
	user2, err := manager.CreateUser(model.UserCreateRequest{
		Username: "freeuser_dup",
		Password: "FreeUserPass123!",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("create user2: %v", err)
	}
	user2Session, err := sessions.Create(user2.ID)
	if err != nil {
		t.Fatalf("create user2 session: %v", err)
	}
	dupCookieBody, _ := json.Marshal(map[string]any{
		"cookie":         "noise=1; info=other; wxopenid=free-user-pool",
		"joinPublicPool": true,
	})
	dupReq := httptest.NewRequest(http.MethodPost, "/api/session/cookie", bytes.NewReader(dupCookieBody))
	dupReq.Header.Set("Content-Type", "application/json")
	dupReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: user2Session.Token})
	dupRec := httptest.NewRecorder()
	mux.ServeHTTP(dupRec, dupReq)
	if dupRec.Code != http.StatusConflict || !strings.Contains(dupRec.Body.String(), "PUBLIC_POOL_COOKIE_DUPLICATE") {
		t.Fatalf("duplicate user join pool = %d %s, want 409 PUBLIC_POOL_COOKIE_DUPLICATE", dupRec.Code, dupRec.Body.String())
	}

	adminDupBody, _ := json.Marshal(map[string]any{
		"label":  "重复公共号",
		"cookie": "wxopenid=free-user-pool; info=admin-copy",
	})
	adminDupReq := httptest.NewRequest(http.MethodPost, "/api/admin/public-pool", bytes.NewReader(adminDupBody))
	adminDupReq.Header.Set("Content-Type", "application/json")
	adminDupReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	adminDupRec := httptest.NewRecorder()
	mux.ServeHTTP(adminDupRec, adminDupReq)
	if adminDupRec.Code != http.StatusConflict || !strings.Contains(adminDupRec.Body.String(), "PUBLIC_POOL_COOKIE_DUPLICATE") {
		t.Fatalf("duplicate admin create pool cookie = %d %s, want 409 PUBLIC_POOL_COOKIE_DUPLICATE", adminDupRec.Code, adminDupRec.Body.String())
	}

	// Same user refreshing their own cookie (same wxopenid, new info) succeeds.
	selfRefreshBody, _ := json.Marshal(map[string]any{
		"cookie":         "wxopenid=free-user-pool; info=refreshed",
		"joinPublicPool": true,
	})
	selfRefreshReq := httptest.NewRequest(http.MethodPost, "/api/session/cookie", bytes.NewReader(selfRefreshBody))
	selfRefreshReq.Header.Set("Content-Type", "application/json")
	selfRefreshReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userSession.Token})
	selfRefreshRec := httptest.NewRecorder()
	mux.ServeHTTP(selfRefreshRec, selfRefreshReq)
	if selfRefreshRec.Code != http.StatusOK {
		t.Fatalf("self refresh same wxopenid = %d %s, want 200", selfRefreshRec.Code, selfRefreshRec.Body.String())
	}

	// 3b. Admin cannot overwrite a user mutual-aid pool cookie's secret, but can rename its label.
	replaceSecretBody, _ := json.Marshal(map[string]any{
		"label":  "用户互助 · freeuser-renamed",
		"cookie": "wxopenid=admin-injected; info=ok",
	})
	replaceReq := httptest.NewRequest(http.MethodPatch, "/api/admin/public-pool/user_"+user.ID, bytes.NewReader(replaceSecretBody))
	replaceReq.Header.Set("Content-Type", "application/json")
	replaceReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	replaceRec := httptest.NewRecorder()
	mux.ServeHTTP(replaceRec, replaceReq)
	if replaceRec.Code != http.StatusBadRequest {
		t.Fatalf("admin replace mutual-aid cookie secret = %d %s, want 400", replaceRec.Code, replaceRec.Body.String())
	}

	renameOnlyBody, _ := json.Marshal(map[string]any{"label": "用户互助 · freeuser-renamed"})
	renameReq := httptest.NewRequest(http.MethodPatch, "/api/admin/public-pool/user_"+user.ID, bytes.NewReader(renameOnlyBody))
	renameReq.Header.Set("Content-Type", "application/json")
	renameReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	renameRec := httptest.NewRecorder()
	mux.ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK || !strings.Contains(renameRec.Body.String(), `"ownerUserId":"`+user.ID+`"`) {
		t.Fatalf("admin rename mutual-aid cookie = %d %s, want 200 preserving ownerUserId", renameRec.Code, renameRec.Body.String())
	}

	// 4. Deleting the user removes their mutual-aid cookie from public_cookie_pool.
	delUserReq := httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+user.ID, nil)
	delUserReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	delUserRec := httptest.NewRecorder()
	mux.ServeHTTP(delUserRec, delUserReq)
	if delUserRec.Code != http.StatusNoContent {
		t.Fatalf("delete user = %d %s", delUserRec.Code, delUserRec.Body.String())
	}
	if manager.PublicPoolAvailableCount() != 0 {
		t.Fatalf("PublicPoolAvailableCount after user delete = %d, want 0", manager.PublicPoolAvailableCount())
	}
}

func TestFreeUserRefreshWithEmptyPoolReturns503WithoutCredentialAlert(t *testing.T) {
	repo, err := persistence.OpenSQLite(
		t.TempDir()+"/state.db",
		bytes.Repeat([]byte{0x66}, persistence.CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	manager, err := appruntime.NewManager(repo, "", parser.DefaultCaptureRequests(), "admin-password-123", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	sessions := auth.NewSessionManager(time.Hour)
	server := NewServer(manager, sessions, auth.NewAuthGuard())
	mux := http.NewServeMux()
	server.Register(mux)

	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "freeuser2",
		Password: "FreeUserPass123!",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userSession, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("create user session: %v", err)
	}

	// Seed an aged shared pile and attach it to the free user (zero upstream network calls).
	const pileID = "2601201412385560888"
	if _, err := repo.UpsertSharedPile(model.Pile{
		ID:        pileID,
		Number:    "610888",
		Name:      "东门车棚",
		Address:   "东门",
		OpenNum:   4,
		Online:    true,
		UpdatedAt: time.Now().UTC().Add(-10 * time.Minute),
	}, "", time.Now().UTC().Add(-10*time.Minute)); err != nil {
		t.Fatalf("UpsertSharedPile: %v", err)
	}
	if _, err := manager.AddPileWithCatalog(user.ID, model.PileUpsertRequest{ID: pileID}); err != nil {
		t.Fatalf("AddPileWithCatalog: %v", err)
	}

	// With 1 owned pile (expired cache) and empty public pool, POST /api/refresh must return 503 PUBLIC_POOL_UNAVAILABLE.
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
	refreshReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userSession.Token})
	refreshRec := httptest.NewRecorder()
	mux.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusServiceUnavailable || !strings.Contains(refreshRec.Body.String(), "PUBLIC_POOL_UNAVAILABLE") {
		t.Fatalf("free user refresh with empty pool = %d %s, want 503 PUBLIC_POOL_UNAVAILABLE", refreshRec.Code, refreshRec.Body.String())
	}

	// Ensure no spurious credential_expired notification was created for the free user.
	notifications, err := manager.Notifications(user.ID, "", "all", 20)
	if err != nil {
		t.Fatalf("Notifications: %v", err)
	}
	for _, n := range notifications.Items {
		if n.Type == model.NotificationCredentialExpired {
			t.Fatalf("unexpected credential_expired notification for free user: %+v", n)
		}
	}

	// Verify admin stats overview OpenIssues matches visible Exceptions and does NOT flag free user as unbound/risk.
	admin, err := manager.Authenticate("admin", "admin-password-123")
	if err != nil {
		t.Fatalf("authenticate admin: %v", err)
	}
	adminSession, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}
	statsReq := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	statsReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	statsRec := httptest.NewRecorder()
	mux.ServeHTTP(statsRec, statsReq)
	if statsRec.Code != http.StatusOK {
		t.Fatalf("admin stats = %d %s", statsRec.Code, statsRec.Body.String())
	}
	var stats model.AdminStats
	if err := json.NewDecoder(statsRec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode admin stats: %v", err)
	}
	if stats.Overview.OpenIssues != len(stats.Exceptions) {
		t.Fatalf("OpenIssues=%d != len(Exceptions)=%d", stats.Overview.OpenIssues, len(stats.Exceptions))
	}
	for _, exc := range stats.Exceptions {
		if exc.UserID == user.ID && exc.Type == "credential" {
			t.Fatalf("free user should not generate credential incident: %+v", exc)
		}
	}

	// Verify admin cannot blank a shared pile's name.
	blankNameBody, _ := json.Marshal(map[string]string{"name": "   ", "address": "新地址"})
	blankReq := httptest.NewRequest(http.MethodPatch, "/api/admin/shared-piles/"+pileID, bytes.NewReader(blankNameBody))
	blankReq.Header.Set("Content-Type", "application/json")
	blankReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminSession.Token})
	blankRec := httptest.NewRecorder()
	mux.ServeHTTP(blankRec, blankReq)
	if blankRec.Code != http.StatusBadRequest {
		t.Fatalf("blank shared pile name = %d %s, want 400", blankRec.Code, blankRec.Body.String())
	}
}
