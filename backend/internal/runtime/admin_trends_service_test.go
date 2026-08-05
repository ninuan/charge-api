package runtime

import (
	"testing"
	"time"
)

func TestRecordMetricCountWritesOneCountedRow(t *testing.T) {
	repository := testRepository(t)
	manager := &Manager{repository: repository, runtimes: map[string]*UserRuntime{}}

	manager.recordMetricCount("user-1", "request", 7)
	operations, err := repository.OperationsStatus(90)
	if err != nil {
		t.Fatalf("OperationsStatus: %v", err)
	}
	if operations.MetricRows != 1 {
		t.Fatalf("metric rows = %d, want 1", operations.MetricRows)
	}
	points, err := repository.MetricSeries(time.Now().Add(-time.Hour), 3600)
	if err != nil {
		t.Fatalf("MetricSeries: %v", err)
	}
	if len(points) != 1 || points[0].Requests != 7 {
		t.Fatalf("metric count was not preserved: %+v", points)
	}
}

func TestAdminTrendWindowsUseExpectedRangeAndTimezoneBoundaries(t *testing.T) {
	manager := &Manager{repository: testRepository(t), runtimes: map[string]*UserRuntime{}}
	now := time.Date(2026, 8, 5, 10, 30, 45, 0, time.UTC)
	tests := []struct {
		name       string
		rangeName  string
		timezone   string
		wantPoints int
		wantUnit   string
		wantStart  time.Time
	}{
		{
			name: "rolling 24 hours", rangeName: "24h", timezone: "UTC",
			wantPoints: 24, wantUnit: "hour", wantStart: now.Add(-24 * time.Hour),
		},
		{
			name: "seven Shanghai calendar days", rangeName: "7d", timezone: "Asia/Shanghai",
			wantPoints: 7, wantUnit: "day", wantStart: time.Date(2026, 7, 29, 16, 0, 0, 0, time.UTC),
		},
		{
			name: "thirty UTC calendar days", rangeName: "30d", timezone: "UTC",
			wantPoints: 30, wantUnit: "day", wantStart: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := manager.adminTrendsAt(test.rangeName, test.timezone, now)
			if err != nil {
				t.Fatalf("adminTrendsAt: %v", err)
			}
			if len(result.Points) != test.wantPoints || result.Window.BucketUnit != test.wantUnit {
				t.Fatalf("unexpected buckets: window=%+v points=%d", result.Window, len(result.Points))
			}
			if !result.Window.Start.Equal(test.wantStart) || !result.Window.End.Equal(now) {
				t.Fatalf("unexpected window: %+v", result.Window)
			}
		})
	}
}

func TestAdminTrendDailyBucketsRespectDaylightSavingTime(t *testing.T) {
	manager := &Manager{repository: testRepository(t), runtimes: map[string]*UserRuntime{}}
	now := time.Date(2026, 3, 9, 16, 0, 0, 0, time.UTC)
	result, err := manager.adminTrendsAt("7d", "America/New_York", now)
	if err != nil {
		t.Fatalf("adminTrendsAt: %v", err)
	}
	foundShortDay := false
	for _, point := range result.Points {
		if point.End.Sub(point.Start) == 23*time.Hour {
			foundShortDay = true
		}
	}
	if !foundShortDay {
		t.Fatalf("DST transition did not produce a 23-hour calendar bucket: %+v", result.Points)
	}
}

func TestAdminTrendsAggregateCountsAndNullableSuccessRate(t *testing.T) {
	repository := testRepository(t)
	manager := &Manager{repository: repository, runtimes: map[string]*UserRuntime{}}
	now := time.Date(2026, 8, 5, 10, 30, 0, 0, time.UTC)
	for _, metric := range []struct {
		userID string
		kind   string
		count  int
	}{
		{userID: "user-1", kind: "request", count: 7},
		{userID: "user-2", kind: "request", count: 3},
		{userID: "user-1", kind: "remote", count: 4},
		{userID: "user-1", kind: "remote_ok", count: 3},
		{userID: "user-1", kind: "remote_failed", count: 1},
	} {
		if err := repository.RecordMetricCount(metric.userID, metric.kind, metric.count, now.Add(-time.Hour)); err != nil {
			t.Fatalf("RecordMetricCount(%s): %v", metric.kind, err)
		}
	}
	result, err := manager.adminTrendsAt("24h", "UTC", now)
	if err != nil {
		t.Fatalf("adminTrendsAt: %v", err)
	}
	if result.Summary.Requests != 10 || result.Summary.ActiveUsers != 2 || result.Summary.RemoteAttempts != 4 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
	if result.Summary.RemoteSuccessRate == nil || *result.Summary.RemoteSuccessRate != 75 {
		t.Fatalf("unexpected success rate: %+v", result.Summary.RemoteSuccessRate)
	}
	if result.Points[0].RemoteSuccessRate != nil {
		t.Fatalf("empty bucket success rate should be null: %+v", result.Points[0])
	}
	if defaultResult, err := manager.adminTrendsAt("", "", now); err != nil || defaultResult.Window.Range != "24h" || defaultResult.Window.Timezone != "Asia/Shanghai" {
		t.Fatalf("unexpected default trend query: result=%+v err=%v", defaultResult.Window, err)
	}
	if _, err := manager.adminTrendsAt("90d", "UTC", now); err == nil {
		t.Fatal("expected invalid range to fail")
	}
	if _, err := manager.adminTrendsAt("7d", "../etc/passwd", now); err == nil {
		t.Fatal("expected invalid timezone to fail")
	}
}
