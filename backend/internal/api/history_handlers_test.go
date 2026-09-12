package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	appruntime "charge-dashboard/internal/runtime"
)

type historyAPIFixture struct {
	server     *Server
	mux        *http.ServeMux
	manager    *appruntime.Manager
	sessions   *auth.SessionManager
	repository *persistence.Store
	owner      model.User
	other      model.User
}

func newHistoryAPIFixture(t *testing.T) historyAPIFixture {
	t.Helper()
	repository, err := persistence.OpenSQLite(
		t.TempDir()+"/state.db",
		bytes.Repeat([]byte{0x68}, persistence.CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	now := time.Now().UTC().Truncate(time.Second)
	owner := model.User{
		ID: "history-owner", Username: "history-owner", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
		DeviceLimit: 10, RefreshEnabled: true,
	}
	other := model.User{
		ID: "history-other", Username: "history-other", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
		DeviceLimit: 10, RefreshEnabled: true,
	}
	const deviceID = "2601201412385560088"
	pile := model.Pile{
		ID: deviceID, Number: "600088", Name: "API 历史测试桩", Status: "在线", Online: true,
		OpenNum: 1, Ports: []model.Port{{ID: 1, Status: model.PortIdle, UpdatedAt: now}},
	}
	if err := repository.Save(persistence.State{
		Version: 3, Users: []model.User{owner, other},
		UserStates: map[string]persistence.UserState{
			owner.ID: {Piles: []model.Pile{pile}, DeviceIDs: []string{deviceID}},
			other.ID: {},
		},
		Settings: model.RegistrationSettings{
			OpenRegistration: true, DefaultDeviceLimit: 10, DefaultRefreshEnabled: true,
		},
	}); err != nil {
		t.Fatalf("Save fixture: %v", err)
	}
	if err := repository.RecordPortStatusEvents([]model.PortStatusEvent{{
		UserID: owner.ID, DeviceID: deviceID, PortID: 1,
		ToStatus: model.PortIdle, ChangedAt: now.Add(-time.Hour), Source: "remote",
	}}); err != nil {
		t.Fatalf("seed history: %v", err)
	}
	manager, err := appruntime.NewManager(repository, "", parser.DefaultCaptureRequests(), "", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	settings := manager.Settings()
	settings.ScheduledPowerOffEnabled = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("disable scheduled power-off for API fixture: %v", err)
	}
	sessions := auth.NewSessionManager(time.Hour)
	t.Cleanup(sessions.Close)
	server := NewServer(manager, sessions, auth.NewAuthGuard())
	mux := http.NewServeMux()
	server.Register(mux)
	return historyAPIFixture{
		server: server, mux: mux, manager: manager, sessions: sessions,
		repository: repository, owner: owner, other: other,
	}
}

func historyAPIRequest(t *testing.T, fixture historyAPIFixture, userID, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if userID != "" {
		session, err := fixture.sessions.Create(userID)
		if err != nil {
			t.Fatalf("Create session: %v", err)
		}
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	}
	recorder := httptest.NewRecorder()
	fixture.mux.ServeHTTP(recorder, request)
	return recorder
}

func TestHistoryEndpointsAreRetiredAndStillRequireAuthentication(t *testing.T) {
	fixture := newHistoryAPIFixture(t)
	for _, path := range []string{"/api/piles/2601201412385560088/history", "/api/piles/2601201412385560088/ports/1/history"} {
		for _, user := range []string{fixture.owner.ID, fixture.other.ID} {
			response := historyAPIRequest(t, fixture, user, path)
			if response.Code != http.StatusGone || !bytes.Contains(response.Body.Bytes(), []byte("HISTORY_REMOVED")) {
				t.Fatalf("retired endpoint = %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("retired response must not be cached")
			}
		}
		if response := historyAPIRequest(t, fixture, "", path); response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated = %d", response.Code)
		}
	}
}
