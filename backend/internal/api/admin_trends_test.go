package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestAdminTrendsCSVMatchesTrendRangeAndExcludesSecrets(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	admin := findUser(t, manager, "admin")
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "csv-user", Password: "csv-password-123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for range 3 {
		if _, err := manager.Snapshot(user.ID); err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
	}
	session, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	mux := http.NewServeMux()
	server.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/api/admin/trends.csv?range=24h&timezone=UTC", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/csv; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, "charge-trends-24h-") {
		t.Fatalf("content disposition = %q", disposition)
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache control = %q", recorder.Header().Get("Cache-Control"))
	}
	body := recorder.Body.Bytes()
	if !bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("CSV is missing UTF-8 BOM")
	}
	for _, secret := range []string{"csv-user", "csv-password-123", "charge_session", "User-Agent"} {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("CSV leaked %q", secret)
		}
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\uFEFF"))).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(records) != 25 || len(records[0]) != 9 {
		t.Fatalf("unexpected CSV shape: rows=%d columns=%d", len(records), len(records[0]))
	}
	totalRequests := 0
	for _, record := range records[1:] {
		value, err := strconv.Atoi(record[2])
		if err != nil {
			t.Fatalf("parse request count %q: %v", record[2], err)
		}
		totalRequests += value
	}
	if totalRequests != 3 {
		t.Fatalf("CSV request total = %d, want 3", totalRequests)
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
