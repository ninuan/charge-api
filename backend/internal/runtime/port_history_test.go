package runtime

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
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

func TestOrdinaryRefreshDoesNotAccumulateHistory(t *testing.T) {
	remote := newHistoryRemoteState()
	const deviceID = "2601201412385560001"
	server := newIPv4TestServer(t, remote)
	manager, userID := newHistoryManager(t, historyCaptureRequests(server.URL), time.Hour)
	addHistoryDevice(t, manager, userID, deviceID)
	for _, state := range []model.PortStatus{model.PortIdle, model.PortInUse, model.PortIdle} {
		remote.setStatus(deviceID, state)
		snapshot, err := manager.Refresh(userID, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Piles) != 1 || snapshot.Piles[0].Ports[0].Status != state {
			t.Fatalf("snapshot not updated: %+v", snapshot)
		}
	}
	if events := historyEvents(t, manager, userID, deviceID); len(events) != 0 {
		t.Fatalf("ordinary refresh wrote history: %+v", events)
	}
}

func TestReminderEventRecordingIsScopedToActivePiles(t *testing.T) {
	remote := newHistoryRemoteState()
	server := newIPv4TestServer(t, remote)
	manager, userID := newHistoryManager(t, historyCaptureRequests(server.URL), time.Hour)
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	for _, enabled := range []bool{false, true} {
		rule := model.WatchRule{ID: "scoped-rule", UserID: userID, DeviceID: "2601201412385560001", Mode: model.WatchRuleTemporary, Enabled: enabled, CreatedAt: now, UpdatedAt: now, ExpiresAt: &expires, Timezone: "Asia/Shanghai", ActiveWeekdays: 127, StopAfterNotify: true}
		if err := manager.repository.SaveWatchRule(rule); err != nil {
			t.Fatal(err)
		}
		piles := []model.Pile{
			{ID: rule.DeviceID, UpdatedAt: now, Ports: []model.Port{{ID: 1, Status: model.PortInUse, UpdatedAt: now}}},
			{ID: "2601201412385560002", UpdatedAt: now, Ports: []model.Port{{ID: 1, Status: model.PortIdle, UpdatedAt: now}}},
		}
		events, err := manager.recordReminderStatusEvents(userID, piles)
		if err != nil {
			t.Fatal(err)
		}
		if enabled && (len(events) != 1 || events[0].DeviceID != rule.DeviceID) {
			t.Fatalf("active reminder events = %+v", events)
		}
		if !enabled && len(events) != 0 {
			t.Fatalf("disabled reminder wrote events: %+v", events)
		}
		if len(historyEvents(t, manager, userID, piles[1].ID)) != 0 {
			t.Fatal("unwatched pile wrote events")
		}
	}
}
