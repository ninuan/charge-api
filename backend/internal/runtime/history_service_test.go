package runtime

import (
	"errors"
	"math"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
)

const analyticsDeviceID = "2601201412385560099"

func newAnalyticsManager(t *testing.T, ports []model.Port) (*Manager, string) {
	t.Helper()
	repository := testRepository(t)
	now := time.Now().UTC()
	user := model.User{
		ID: "history-analytics-user", Username: "history-user", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
		DeviceLimit: 10, RefreshEnabled: true,
	}
	pile := model.Pile{
		ID: analyticsDeviceID, Number: "600099", Name: "历史测试桩",
		Address: "测试地址", Status: "在线", Online: true, OpenNum: len(ports), Ports: ports,
	}
	state := persistence.State{
		Version: 3, Users: []model.User{user},
		UserStates: map[string]persistence.UserState{
			user.ID: {Piles: []model.Pile{pile}, DeviceIDs: []string{analyticsDeviceID}},
		},
		Settings: model.RegistrationSettings{
			OpenRegistration: true, DefaultDeviceLimit: 10, DefaultRefreshEnabled: true,
		},
	}
	if err := repository.Save(state); err != nil {
		t.Fatalf("Save analytics state: %v", err)
	}
	manager, err := NewManager(repository, "", parser.DefaultCaptureRequests(), "", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return manager, user.ID
}

func seedHistoryEvents(t *testing.T, manager *Manager, events ...model.PortStatusEvent) {
	t.Helper()
	if err := manager.repository.RecordPortStatusEvents(events); err != nil {
		t.Fatalf("RecordPortStatusEvents: %v", err)
	}
}

func statusPointer(status model.PortStatus) *model.PortStatus {
	value := status
	return &value
}

func TestPortHistoryCalculatesAvailabilitySessionsAndTimeline(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	start := now.Add(-24 * time.Hour)
	manager, userID := newAnalyticsManager(t, []model.Port{{ID: 1, Status: model.PortIdle}})
	seedHistoryEvents(t, manager,
		model.PortStatusEvent{UserID: userID, DeviceID: analyticsDeviceID, PortID: 1, ToStatus: model.PortIdle, ChangedAt: start},
		model.PortStatusEvent{UserID: userID, DeviceID: analyticsDeviceID, PortID: 1, FromStatus: statusPointer(model.PortIdle), ToStatus: model.PortInUse, ChangedAt: start.Add(time.Hour)},
		model.PortStatusEvent{UserID: userID, DeviceID: analyticsDeviceID, PortID: 1, FromStatus: statusPointer(model.PortInUse), ToStatus: model.PortIdle, ChangedAt: start.Add(3 * time.Hour)},
		model.PortStatusEvent{UserID: userID, DeviceID: analyticsDeviceID, PortID: 1, FromStatus: statusPointer(model.PortIdle), ToStatus: model.PortOffline, ChangedAt: start.Add(4 * time.Hour)},
		model.PortStatusEvent{UserID: userID, DeviceID: analyticsDeviceID, PortID: 1, FromStatus: statusPointer(model.PortOffline), ToStatus: model.PortIdle, ChangedAt: start.Add(6 * time.Hour)},
	)

	result, err := manager.portHistoryAt(userID, analyticsDeviceID, 1, "24h", "UTC", now)
	if err != nil {
		t.Fatalf("PortHistory: %v", err)
	}
	metrics := result.Metrics
	if metrics.IdleSeconds != 20*3600 || metrics.InUseSeconds != 2*3600 ||
		metrics.OfflineSeconds != 2*3600 || metrics.GapSeconds != 0 {
		t.Fatalf("unexpected durations: %+v", metrics)
	}
	if metrics.OccupancyPercent == nil || math.Abs(*metrics.OccupancyPercent-9.1) > 0.01 {
		t.Fatalf("occupancy = %v, want 9.1", metrics.OccupancyPercent)
	}
	if metrics.CompletedSessions != 1 || metrics.AverageSessionSeconds == nil || *metrics.AverageSessionSeconds != 7200 {
		t.Fatalf("unexpected session metrics: %+v", metrics)
	}
	if metrics.SampleState != "sufficient" || len(result.Daily) != 1 || len(result.Timeline) != 5 {
		t.Fatalf("unexpected sample or response shape: %+v", result)
	}
	if result.Timeline[0].ToStatus != model.PortIdle || !result.Timeline[0].ChangedAt.Equal(start.Add(6*time.Hour)) {
		t.Fatalf("timeline is not newest first: %+v", result.Timeline)
	}
	if result.HistoryStartedAt == nil || !result.HistoryStartedAt.Equal(start) {
		t.Fatalf("history start = %v, want %v", result.HistoryStartedAt, start)
	}
}

func TestHistorySampleStatesDoNotTreatUnknownOrOfflineAsIdle(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	start := now.Add(-24 * time.Hour)
	tests := []struct {
		name          string
		events        []model.PortStatusEvent
		wantState     string
		wantGap       int64
		wantOffline   int64
		wantOccupancy bool
		wantSessions  int
	}{
		{name: "no data", wantState: "no_data", wantGap: 24 * 3600},
		{
			name:      "baseline after range start",
			events:    []model.PortStatusEvent{{ToStatus: model.PortIdle, ChangedAt: start.Add(6 * time.Hour)}},
			wantState: "partial", wantGap: 6 * 3600, wantOccupancy: true,
		},
		{
			name:      "offline only",
			events:    []model.PortStatusEvent{{ToStatus: model.PortOffline, ChangedAt: start}},
			wantState: "insufficient", wantOffline: 24 * 3600,
		},
		{
			name: "unclosed session",
			events: []model.PortStatusEvent{
				{ToStatus: model.PortIdle, ChangedAt: start},
				{FromStatus: statusPointer(model.PortIdle), ToStatus: model.PortInUse, ChangedAt: start.Add(time.Hour)},
			},
			wantState: "sufficient", wantOccupancy: true, wantSessions: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, userID := newAnalyticsManager(t, []model.Port{{ID: 1, Status: model.PortIdle}})
			for index := range test.events {
				test.events[index].UserID = userID
				test.events[index].DeviceID = analyticsDeviceID
				test.events[index].PortID = 1
			}
			seedHistoryEvents(t, manager, test.events...)
			result, err := manager.portHistoryAt(userID, analyticsDeviceID, 1, "24h", "UTC", now)
			if err != nil {
				t.Fatalf("PortHistory: %v", err)
			}
			if result.Metrics.SampleState != test.wantState || result.Metrics.GapSeconds != test.wantGap ||
				result.Metrics.OfflineSeconds != test.wantOffline || result.Metrics.CompletedSessions != test.wantSessions {
				t.Fatalf("unexpected metrics: %+v", result.Metrics)
			}
			if (result.Metrics.OccupancyPercent != nil) != test.wantOccupancy {
				t.Fatalf("occupancy presence = %v, want %v", result.Metrics.OccupancyPercent, test.wantOccupancy)
			}
			if test.name == "unclosed session" && result.Metrics.AverageSessionSeconds != nil {
				t.Fatalf("unclosed session contributed an average: %+v", result.Metrics)
			}
		})
	}
}

func TestHistoryUsesFromStatusAcrossRetentionBoundary(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	start := now.Add(-24 * time.Hour)
	manager, userID := newAnalyticsManager(t, []model.Port{{ID: 1, Status: model.PortIdle}})
	seedHistoryEvents(t, manager,
		model.PortStatusEvent{
			UserID: userID, DeviceID: analyticsDeviceID, PortID: 1,
			FromStatus: statusPointer(model.PortIdle), ToStatus: model.PortInUse,
			ChangedAt: start.Add(6 * time.Hour),
		},
		model.PortStatusEvent{
			UserID: userID, DeviceID: analyticsDeviceID, PortID: 1,
			FromStatus: statusPointer(model.PortInUse), ToStatus: model.PortIdle,
			ChangedAt: start.Add(7 * time.Hour),
		},
	)
	result, err := manager.portHistoryAt(userID, analyticsDeviceID, 1, "24h", "UTC", now)
	if err != nil {
		t.Fatalf("PortHistory: %v", err)
	}
	if result.Metrics.GapSeconds != 0 || result.Metrics.IdleSeconds != 23*3600 ||
		result.Metrics.InUseSeconds != 3600 || result.Metrics.CompletedSessions != 1 ||
		result.Metrics.AverageSessionSeconds == nil || *result.Metrics.AverageSessionSeconds != 3600 {
		t.Fatalf("retention boundary was not reconstructed: %+v", result.Metrics)
	}
}

func TestDeviceHistoryBuildsDailyHeatmapAndQualifiedInsights(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	window, _, err := normalizeHistoryWindow("30d", "Asia/Shanghai", now)
	if err != nil {
		t.Fatalf("normalizeHistoryWindow: %v", err)
	}
	manager, userID := newAnalyticsManager(t, []model.Port{
		{ID: 1, Status: model.PortIdle}, {ID: 2, Status: model.PortIdle},
	})
	seedHistoryEvents(t, manager,
		model.PortStatusEvent{UserID: userID, DeviceID: analyticsDeviceID, PortID: 1, ToStatus: model.PortIdle, ChangedAt: window.Start},
		model.PortStatusEvent{UserID: userID, DeviceID: analyticsDeviceID, PortID: 2, ToStatus: model.PortIdle, ChangedAt: window.Start},
	)
	result, err := manager.deviceHistoryAt(userID, analyticsDeviceID, "30d", "Asia/Shanghai", now)
	if err != nil {
		t.Fatalf("DeviceHistory: %v", err)
	}
	if len(result.Ports) != 2 || len(result.Daily) != 30 || len(result.Heatmap) != 168 {
		t.Fatalf("unexpected device history shape: ports=%d daily=%d heatmap=%d", len(result.Ports), len(result.Daily), len(result.Heatmap))
	}
	if result.QuietSuggestion == nil || len(result.BusiestHours) != 3 {
		t.Fatalf("qualified heatmap did not produce insights: quiet=%+v busiest=%+v", result.QuietSuggestion, result.BusiestHours)
	}
	if result.QuietSuggestion.OccupancyPercent != 0 || result.QuietSuggestion.SampleDates < 3 {
		t.Fatalf("unexpected quiet suggestion: %+v", result.QuietSuggestion)
	}
}

func TestPortHistoryCapsTimelineWithoutTruncatingAnalytics(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	start := now.Add(-24 * time.Hour)
	manager, userID := newAnalyticsManager(t, []model.Port{{ID: 1, Status: model.PortIdle}})
	events := make([]model.PortStatusEvent, 0, historyTimelineLimit+5)
	status := model.PortIdle
	events = append(events, model.PortStatusEvent{
		UserID: userID, DeviceID: analyticsDeviceID, PortID: 1,
		ToStatus: status, ChangedAt: start,
	})
	for index := 1; index < historyTimelineLimit+5; index++ {
		previous := status
		if status == model.PortIdle {
			status = model.PortInUse
		} else {
			status = model.PortIdle
		}
		events = append(events, model.PortStatusEvent{
			UserID: userID, DeviceID: analyticsDeviceID, PortID: 1,
			FromStatus: statusPointer(previous), ToStatus: status,
			ChangedAt: start.Add(time.Duration(index) * time.Minute),
		})
	}
	seedHistoryEvents(t, manager, events...)
	result, err := manager.portHistoryAt(userID, analyticsDeviceID, 1, "24h", "UTC", now)
	if err != nil {
		t.Fatalf("PortHistory: %v", err)
	}
	if !result.TimelineTruncated || len(result.Timeline) != historyTimelineLimit {
		t.Fatalf("timeline cap = truncated %v, length %d", result.TimelineTruncated, len(result.Timeline))
	}
	if result.Metrics.CompletedSessions != (historyTimelineLimit+4)/2 {
		t.Fatalf("analytics were truncated with timeline: %+v", result.Metrics)
	}
}

func TestSevenDayDeviceHistoryHandlesCapacityBaseline(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	window, _, err := normalizeHistoryWindow("7d", "Asia/Shanghai", now)
	if err != nil {
		t.Fatalf("normalizeHistoryWindow: %v", err)
	}
	ports := make([]model.Port, 0, 10)
	for portID := 1; portID <= 10; portID++ {
		ports = append(ports, model.Port{ID: portID, Status: model.PortIdle})
	}
	manager, userID := newAnalyticsManager(t, ports)
	events := make([]model.PortStatusEvent, 0, 1700)
	for portID := 1; portID <= 10; portID++ {
		status := model.PortIdle
		events = append(events, model.PortStatusEvent{
			UserID: userID, DeviceID: analyticsDeviceID, PortID: portID,
			ToStatus: status, ChangedAt: window.Start,
		})
		for changedAt := window.Start.Add(time.Hour); changedAt.Before(window.End); changedAt = changedAt.Add(time.Hour) {
			previous := status
			if status == model.PortIdle {
				status = model.PortInUse
			} else {
				status = model.PortIdle
			}
			events = append(events, model.PortStatusEvent{
				UserID: userID, DeviceID: analyticsDeviceID, PortID: portID,
				FromStatus: statusPointer(previous), ToStatus: status, ChangedAt: changedAt,
			})
		}
	}
	seedHistoryEvents(t, manager, events...)
	started := time.Now()
	result, err := manager.deviceHistoryAt(userID, analyticsDeviceID, "7d", "Asia/Shanghai", now)
	if err != nil {
		t.Fatalf("DeviceHistory capacity baseline: %v", err)
	}
	if len(result.Ports) != 10 || len(result.Daily) != 7 || len(result.Heatmap) != 168 {
		t.Fatalf("unexpected capacity response shape: ports=%d daily=%d heatmap=%d", len(result.Ports), len(result.Daily), len(result.Heatmap))
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("seven-day capacity query took %v", elapsed)
	}
}

func TestHistoryRejectsUnknownTargetRangeAndTimezone(t *testing.T) {
	now := time.Now().UTC()
	manager, userID := newAnalyticsManager(t, []model.Port{{ID: 1, Status: model.PortIdle}})
	if _, err := manager.deviceHistoryAt(userID, "2601201412385560000", "7d", "UTC", now); !errors.Is(err, ErrHistoryNotFound) {
		t.Fatalf("unknown device error = %v", err)
	}
	if _, err := manager.portHistoryAt(userID, analyticsDeviceID, 2, "7d", "UTC", now); !errors.Is(err, ErrHistoryNotFound) {
		t.Fatalf("unknown port error = %v", err)
	}
	if _, _, err := normalizeHistoryWindow("14d", "UTC", now); !errors.Is(err, ErrHistoryQueryInvalid) {
		t.Fatalf("invalid range error = %v", err)
	}
	if _, _, err := normalizeHistoryWindow("7d", "../../etc/passwd", now); !errors.Is(err, ErrHistoryQueryInvalid) {
		t.Fatalf("invalid timezone error = %v", err)
	}
}
