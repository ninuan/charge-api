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
	sessions := auth.NewSessionManager(time.Hour)
	t.Cleanup(sessions.Close)
	server := NewServer(manager, sessions, auth.NewTurnstileVerifier("", "", ""), auth.NewAuthGuard())
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

func TestUserHistoryAPIReturnsOwnedDeviceAndPortAnalytics(t *testing.T) {
	fixture := newHistoryAPIFixture(t)
	const deviceID = "2601201412385560088"

	deviceRecorder := historyAPIRequest(
		t, fixture, fixture.owner.ID,
		"/api/piles/"+deviceID+"/history?range=24h&timezone=UTC",
	)
	if deviceRecorder.Code != http.StatusOK {
		t.Fatalf("device history status = %d: %s", deviceRecorder.Code, deviceRecorder.Body.String())
	}
	if cacheControl := deviceRecorder.Header().Get("Cache-Control"); cacheControl != "private, no-store" {
		t.Fatalf("device history cache control = %q", cacheControl)
	}
	var device model.DeviceHistoryResponse
	if err := json.NewDecoder(deviceRecorder.Body).Decode(&device); err != nil {
		t.Fatalf("decode device history: %v", err)
	}
	if device.Device.ID != deviceID || device.Window.Range != "24h" || device.Window.Timezone != "UTC" || len(device.Ports) != 1 {
		t.Fatalf("unexpected device history: %+v", device)
	}

	portRecorder := historyAPIRequest(
		t, fixture, fixture.owner.ID,
		"/api/piles/"+deviceID+"/ports/1/history?range=7d&timezone=Asia%2FShanghai",
	)
	if portRecorder.Code != http.StatusOK {
		t.Fatalf("port history status = %d: %s", portRecorder.Code, portRecorder.Body.String())
	}
	var port model.PortHistoryResponse
	if err := json.NewDecoder(portRecorder.Body).Decode(&port); err != nil {
		t.Fatalf("decode port history: %v", err)
	}
	if port.PortID != 1 || port.Device.ID != deviceID || len(port.Timeline) != 1 {
		t.Fatalf("unexpected port history: %+v", port)
	}
	for _, secret := range []string{fixture.owner.ID, "passwordHash", "cookieCiphertext"} {
		if strings.Contains(strings.ToLower(portRecorder.Body.String()), strings.ToLower(secret)) {
			t.Fatalf("port history response leaked %q: %s", secret, portRecorder.Body.String())
		}
	}
}

func TestUserHistoryAPIValidatesQueryAndHidesUnownedTargets(t *testing.T) {
	fixture := newHistoryAPIFixture(t)
	const deviceID = "2601201412385560088"
	tests := []struct {
		name   string
		userID string
		path   string
		status int
		code   string
	}{
		{name: "anonymous", path: "/api/piles/" + deviceID + "/history", status: http.StatusUnauthorized},
		{name: "other user", userID: fixture.other.ID, path: "/api/piles/" + deviceID + "/history", status: http.StatusNotFound, code: "HISTORY_NOT_FOUND"},
		{name: "invalid range", userID: fixture.owner.ID, path: "/api/piles/" + deviceID + "/history?range=14d", status: http.StatusBadRequest, code: "HISTORY_QUERY_INVALID"},
		{name: "invalid timezone", userID: fixture.owner.ID, path: "/api/piles/" + deviceID + "/history?timezone=..%2F..%2Fetc%2Fpasswd", status: http.StatusBadRequest, code: "HISTORY_QUERY_INVALID"},
		{name: "unknown port", userID: fixture.owner.ID, path: "/api/piles/" + deviceID + "/ports/2/history", status: http.StatusNotFound, code: "HISTORY_NOT_FOUND"},
		{name: "invalid port", userID: fixture.owner.ID, path: "/api/piles/" + deviceID + "/ports/no/history", status: http.StatusBadRequest, code: "PORT_ID_INVALID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := historyAPIRequest(t, fixture, test.userID, test.path)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
			if test.code != "" && !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("response missing code %q: %s", test.code, recorder.Body.String())
			}
		})
	}
}

func TestHistoryStorageFailureMarksServiceHealthDegraded(t *testing.T) {
	fixture := newHistoryAPIFixture(t)
	if err := fixture.repository.Close(); err != nil {
		t.Fatalf("close repository: %v", err)
	}
	recorder := historyAPIRequest(
		t, fixture, fixture.owner.ID,
		"/api/piles/2601201412385560088/history?range=24h&timezone=UTC",
	)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "HISTORY_UNAVAILABLE") {
		t.Fatalf("storage failure status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if reason := fixture.server.adminDegradation(); !strings.Contains(reason, "端口历史查询失败") {
		t.Fatalf("history failure did not degrade service health: %q", reason)
	}
}

func TestHistoryErrorResponsesUseStableCodes(t *testing.T) {
	server := &Server{healthDegradations: make(map[string]string)}
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{err: appruntime.ErrHistoryQueryInvalid, status: http.StatusBadRequest, code: "HISTORY_QUERY_INVALID"},
		{err: appruntime.ErrHistoryNotFound, status: http.StatusNotFound, code: "HISTORY_NOT_FOUND"},
		{err: appruntime.ErrHistoryTooLarge, status: http.StatusUnprocessableEntity, code: "HISTORY_RANGE_TOO_LARGE"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		server.writeHistoryError(recorder, "history_test", "6001", test.err)
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("error %v returned %d: %s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}
