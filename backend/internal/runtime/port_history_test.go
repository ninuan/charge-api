package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	"charge-dashboard/internal/yyb"
)

type historyRemoteState struct {
	mu       sync.Mutex
	statuses map[string]model.PortStatus
	failures map[string]bool
	calls    int
}

func newHistoryRemoteState() *historyRemoteState {
	return &historyRemoteState{
		statuses: make(map[string]model.PortStatus),
		failures: make(map[string]bool),
	}
}

func (s *historyRemoteState) setStatus(deviceID string, status model.PortStatus) {
	s.mu.Lock()
	s.statuses[deviceID] = status
	s.mu.Unlock()
}

func (s *historyRemoteState) setFailure(deviceID string, failed bool) {
	s.mu.Lock()
	s.failures[deviceID] = failed
	s.mu.Unlock()
}

func (s *historyRemoteState) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *historyRemoteState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	values, _ := url.ParseQuery(string(body))
	deviceID := values.Get("id")

	s.mu.Lock()
	s.calls++
	status := s.statuses[deviceID]
	failed := s.failures[deviceID]
	s.mu.Unlock()
	if failed {
		http.Error(w, "remote failed", http.StatusBadGateway)
		return
	}
	if status == "" {
		status = model.PortIdle
	}

	payload := map[string]any{
		"id": deviceID, "number": deviceID, "name": "测试充电桩",
		"status": "在线", "opennum": 1,
	}
	switch status {
	case model.PortInUse:
		payload["used"] = []int{1}
		payload["useds"] = []map[string]any{{"i": 1, "u": 120, "s": "28 分钟"}}
	case model.PortOffline:
		payload["status"] = "离线"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func historyCaptureRequests(endpoint string) []parser.CaptureRequest {
	return []parser.CaptureRequest{{
		Name: "history-test", URL: endpoint, Method: http.MethodPost,
		Body: "id=YOUR_DEVICE_LONG_ID",
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
	}}
}

func newHistoryManager(t *testing.T, requests []parser.CaptureRequest, minInterval time.Duration) (*Manager, string) {
	t.Helper()
	manager, err := NewManager(
		testRepository(t), "", requests, "admin-password-123", minInterval,
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	for userID, user := range manager.users {
		if user.Username == "admin" {
			return manager, userID
		}
	}
	t.Fatal("initial administrator was not created")
	return nil, ""
}

func addHistoryDevice(t *testing.T, manager *Manager, userID, deviceID string) {
	t.Helper()
	if err := manager.runtimes[userID].client.AddDevice(deviceID); err != nil {
		t.Fatalf("AddDevice: %v", err)
	}
	if err := manager.Save(); err != nil {
		t.Fatalf("save device: %v", err)
	}
}

func historyEvents(t *testing.T, manager *Manager, userID, deviceID string) []model.PortStatusEvent {
	t.Helper()
	events, err := manager.repository.PortStatusEvents(persistence.PortStatusEventQuery{
		UserID: userID, DeviceID: deviceID,
	})
	if err != nil {
		t.Fatalf("PortStatusEvents: %v", err)
	}
	return events
}

func TestRefreshRecordsOnlyRemotePortStatusChanges(t *testing.T) {
	remote := newHistoryRemoteState()
	const deviceID = "2601201412385560001"
	remote.setStatus(deviceID, model.PortIdle)
	server := newIPv4TestServer(t, remote)
	manager, userID := newHistoryManager(t, historyCaptureRequests(server.URL), time.Hour)
	addHistoryDevice(t, manager, userID, deviceID)

	for index := 0; index < 3; index++ {
		if _, err := manager.Refresh(userID, true); err != nil {
			t.Fatalf("same-state refresh %d: %v", index+1, err)
		}
	}
	events := historyEvents(t, manager, userID, deviceID)
	if len(events) != 1 || events[0].FromStatus != nil || events[0].ToStatus != model.PortIdle {
		t.Fatalf("same snapshots did not produce one baseline: %+v", events)
	}

	remote.setStatus(deviceID, model.PortInUse)
	if _, err := manager.Refresh(userID, true); err != nil {
		t.Fatalf("idle to in-use refresh: %v", err)
	}
	remote.setStatus(deviceID, model.PortIdle)
	if _, err := manager.Refresh(userID, true); err != nil {
		t.Fatalf("in-use to idle refresh: %v", err)
	}
	events = historyEvents(t, manager, userID, deviceID)
	if len(events) != 3 || events[1].FromStatus == nil || *events[1].FromStatus != model.PortIdle ||
		events[1].ToStatus != model.PortInUse || events[2].FromStatus == nil ||
		*events[2].FromStatus != model.PortInUse || events[2].ToStatus != model.PortIdle {
		t.Fatalf("unexpected transition sequence: %+v", events)
	}

	remote.setStatus(deviceID, model.PortInUse)
	remoteCalls := remote.callCount()
	snapshot, err := manager.Refresh(userID, false)
	if err != nil {
		t.Fatalf("cached refresh: %v", err)
	}
	if !snapshot.Refresh.Cached || remote.callCount() != remoteCalls {
		t.Fatalf("refresh did not use cache: %+v calls=%d/%d", snapshot.Refresh, remote.callCount(), remoteCalls)
	}
	if got := len(historyEvents(t, manager, userID, deviceID)); got != 3 {
		t.Fatalf("cached refresh inserted history, events=%d", got)
	}
}

func TestPartialRefreshRecordsOnlySuccessfulDevices(t *testing.T) {
	remote := newHistoryRemoteState()
	const (
		successID = "2601201412385560001"
		failureID = "2601201412385560002"
	)
	remote.setStatus(successID, model.PortIdle)
	remote.setFailure(failureID, true)
	server := newIPv4TestServer(t, remote)
	manager, userID := newHistoryManager(t, historyCaptureRequests(server.URL), 30*time.Second)
	addHistoryDevice(t, manager, userID, successID)
	addHistoryDevice(t, manager, userID, failureID)

	snapshot, err := manager.Refresh(userID, true)
	if err != nil {
		t.Fatalf("partial refresh: %v", err)
	}
	if !snapshot.Refresh.Partial || snapshot.Refresh.SuccessfulDevices != 1 || snapshot.Refresh.FailedDevices != 1 {
		t.Fatalf("unexpected partial refresh info: %+v", snapshot.Refresh)
	}
	if got := len(historyEvents(t, manager, userID, successID)); got != 1 {
		t.Fatalf("successful device events = %d, want 1", got)
	}
	if got := len(historyEvents(t, manager, userID, failureID)); got != 0 {
		t.Fatalf("failed device produced %d events", got)
	}

	remote.setFailure(successID, true)
	if _, err := manager.Refresh(userID, true); err == nil {
		t.Fatal("expected fully failed refresh to return an error")
	}
	if got := len(historyEvents(t, manager, userID, successID)); got != 1 {
		t.Fatalf("fully failed refresh changed successful device history to %d events", got)
	}
	if got := len(historyEvents(t, manager, userID, failureID)); got != 0 {
		t.Fatalf("fully failed refresh produced %d failed-device events", got)
	}
}

func TestPortStatusHistorySurvivesRestartWithoutDuplicateBaseline(t *testing.T) {
	remote := newHistoryRemoteState()
	const deviceID = "2601201412385560001"
	remote.setStatus(deviceID, model.PortIdle)
	server := newIPv4TestServer(t, remote)
	requests := historyCaptureRequests(server.URL)
	path := t.TempDir() + "/state.db"
	key := bytes.Repeat([]byte{0x62}, persistence.CookieKeySize)

	repository, err := persistence.OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	manager, err := NewManager(repository, "", requests, "admin-password-123", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	var userID string
	for id, user := range manager.users {
		if user.Username == "admin" {
			userID = id
		}
	}
	addHistoryDevice(t, manager, userID, deviceID)
	if _, err := manager.Refresh(userID, true); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("close first repository: %v", err)
	}

	repository, err = persistence.OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("reopen SQLite: %v", err)
	}
	defer repository.Close()
	manager, err = NewManager(repository, "", requests, "admin-password-123", 30*time.Second)
	if err != nil {
		t.Fatalf("restart manager: %v", err)
	}
	if _, err := manager.Refresh(userID, true); err != nil {
		t.Fatalf("refresh after restart: %v", err)
	}
	if got := len(historyEvents(t, manager, userID, deviceID)); got != 1 {
		t.Fatalf("restart inserted duplicate baseline, events=%d", got)
	}
}

func TestHistoryRecordingCoversAddPileCookieAndYYBRefresh(t *testing.T) {
	remote := newHistoryRemoteState()
	const deviceID = "2601201412385560001"
	remote.setStatus(deviceID, model.PortIdle)
	server := newIPv4TestServer(t, remote)
	manager, userID := newHistoryManager(t, historyCaptureRequests(server.URL), 30*time.Second)

	if _, err := manager.AddPile(userID, model.PileUpsertRequest{ID: deviceID}); err != nil {
		t.Fatalf("AddPile: %v", err)
	}
	remote.setStatus(deviceID, model.PortInUse)
	if _, err := manager.UpdateCookie(userID, "deviceid="+deviceID+"; sid=manual"); err != nil {
		t.Fatalf("UpdateCookie: %v", err)
	}
	if _, err := manager.SaveYYBBinding(userID, yyb.YYBAccount{Ref: "ref-1", OpenID: "openid-1"}); err != nil {
		t.Fatalf("SaveYYBBinding: %v", err)
	}
	remote.setStatus(deviceID, model.PortIdle)
	if _, err := manager.RecoverRefreshWithYYB(
		userID,
		&fakeYYBClient{codes: []string{"wx-code"}},
		&fakeMoceleClient{cookie: fmt.Sprintf("deviceid=%s; wxopenid=open; info=info", deviceID)},
	); err != nil {
		t.Fatalf("RecoverRefreshWithYYB: %v", err)
	}

	events := historyEvents(t, manager, userID, deviceID)
	if len(events) != 3 || events[0].ToStatus != model.PortIdle ||
		events[1].ToStatus != model.PortInUse || events[2].ToStatus != model.PortIdle {
		t.Fatalf("refresh entry points did not share history recording: %+v", events)
	}
}

func TestDeletingPileAndUserRemovesPortStatusHistory(t *testing.T) {
	remote := newHistoryRemoteState()
	const deviceID = "2601201412385560001"
	remote.setStatus(deviceID, model.PortIdle)
	server := newIPv4TestServer(t, remote)
	manager, userID := newHistoryManager(t, historyCaptureRequests(server.URL), 30*time.Second)
	if _, err := manager.AddPile(userID, model.PileUpsertRequest{ID: deviceID}); err != nil {
		t.Fatalf("AddPile: %v", err)
	}
	if err := manager.DeletePile(userID, deviceID); err != nil {
		t.Fatalf("DeletePile: %v", err)
	}
	if got := len(historyEvents(t, manager, userID, deviceID)); got != 0 {
		t.Fatalf("deleted pile retained %d history events", got)
	}

	created, err := manager.CreateUser(model.UserCreateRequest{
		Username: "history-user", Password: "password123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	now := time.Now().UTC()
	if _, err := manager.repository.RecordPortStatusTransitions(created.ID, []model.Pile{{
		ID: deviceID, Ports: []model.Port{{ID: 1, Status: model.PortIdle, UpdatedAt: now}},
	}}); err != nil {
		t.Fatalf("seed user history: %v", err)
	}
	if err := manager.DeleteUser(created.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if got := len(historyEvents(t, manager, created.ID, deviceID)); got != 0 {
		t.Fatalf("deleted user retained %d history events", got)
	}
}
