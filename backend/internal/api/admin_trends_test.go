package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"charge-dashboard/internal/model"
)

func TestAdminTrendsAPIRequiresAdminAndReturnsStableContract(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "trend-user", Password: "password123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)

	request := func(userID, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if userID != "" {
			session, err := sessions.Create(userID)
			if err != nil {
				t.Fatalf("Create session: %v", err)
			}
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder
	}

	if recorder := request("", "/api/admin/trends"); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", recorder.Code)
	}
	if recorder := request(user.ID, "/api/admin/trends"); recorder.Code != http.StatusForbidden {
		t.Fatalf("ordinary user status = %d, want 403", recorder.Code)
	}
	if recorder := request(admin.ID, "/api/admin/trends?range=90d"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}

	recorder := request(admin.ID, "/api/admin/trends?range=7d&timezone=Asia%2FShanghai")
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache control = %q", recorder.Header().Get("Cache-Control"))
	}
	var result model.AdminTrends
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode admin trends: %v", err)
	}
	if result.Window.Range != "7d" || result.Window.Timezone != "Asia/Shanghai" || result.Window.BucketUnit != "day" || len(result.Points) != 7 {
		t.Fatalf("unexpected trend response: %+v", result)
	}
	if result.Summary.RemoteSuccessRate != nil {
		t.Fatalf("empty success rate should be null: %+v", result.Summary)
	}
}

func TestAdminTrendsAPIRejectsUnsupportedMethod(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	session, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/trends", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	server.handleAdminTrends(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}
