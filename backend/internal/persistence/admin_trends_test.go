package persistence

import (
	"bytes"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestMetricAggregateUsesCountsAndHalfOpenBoundaries(t *testing.T) {
	store, err := OpenSQLite(t.TempDir()+"/state.db", bytes.Repeat([]byte{0x61}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	start := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	for _, metric := range []struct {
		userID string
		kind   string
		count  int
		at     time.Time
	}{
		{userID: "user-1", kind: "request", count: 7, at: start},
		{userID: "user-2", kind: "request", count: 3, at: start.Add(30 * time.Minute)},
		{userID: "user-1", kind: "remote", count: 4, at: start.Add(time.Minute)},
		{userID: "user-1", kind: "remote_ok", count: 3, at: start.Add(time.Minute)},
		{userID: "user-1", kind: "request", count: 99, at: end},
	} {
		if err := store.RecordMetricCount(metric.userID, metric.kind, metric.count, metric.at); err != nil {
			t.Fatalf("RecordMetricCount(%s): %v", metric.kind, err)
		}
	}

	point, err := store.MetricAggregate(start, end)
	if err != nil {
		t.Fatalf("MetricAggregate: %v", err)
	}
	if point.Requests != 10 || point.ActiveUsers != 2 || point.Remote != 4 || point.RemoteOK != 3 {
		t.Fatalf("unexpected aggregate: %+v", point)
	}
}

func TestOfflinePortCountAtUsesLatestKnownPortState(t *testing.T) {
	store, err := OpenSQLite(t.TempDir()+"/state.db", bytes.Repeat([]byte{0x62}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	if err := store.Save(State{
		Version: 3,
		Users: []model.User{{
			ID: "user-1", Username: "alice", PasswordHash: "hash", Role: model.RoleUser,
			Enabled: true, CreatedAt: now, UpdatedAt: now,
		}},
		UserStates: map[string]UserState{"user-1": {}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	idle := model.PortIdle
	offline := model.PortOffline
	if err := store.RecordPortStatusEvents([]model.PortStatusEvent{
		{UserID: "user-1", DeviceID: "device-1", PortID: 1, ToStatus: model.PortOffline, ChangedAt: now, Source: "remote"},
		{UserID: "user-1", DeviceID: "device-1", PortID: 1, FromStatus: &offline, ToStatus: model.PortIdle, ChangedAt: now.Add(time.Hour), Source: "remote"},
		{UserID: "user-1", DeviceID: "device-1", PortID: 2, FromStatus: &idle, ToStatus: model.PortOffline, ChangedAt: now.Add(2 * time.Hour), Source: "remote"},
	}); err != nil {
		t.Fatalf("RecordPortStatusEvents: %v", err)
	}

	checks := []struct {
		at   time.Time
		want int
	}{
		{at: now.Add(30 * time.Minute), want: 1},
		{at: now.Add(90 * time.Minute), want: 0},
		{at: now.Add(3 * time.Hour), want: 1},
	}
	for _, check := range checks {
		got, err := store.OfflinePortCountAt(check.at)
		if err != nil {
			t.Fatalf("OfflinePortCountAt(%s): %v", check.at, err)
		}
		if got != check.want {
			t.Fatalf("OfflinePortCountAt(%s) = %d, want %d", check.at, got, check.want)
		}
	}

	instants := make([]time.Time, len(checks))
	for index, check := range checks {
		instants[index] = check.at
	}
	counts, err := store.OfflinePortCountsAt(instants)
	if err != nil {
		t.Fatalf("OfflinePortCountsAt: %v", err)
	}
	for index, check := range checks {
		if counts[index] != check.want {
			t.Fatalf("OfflinePortCountsAt(%s) = %d, want %d", check.at, counts[index], check.want)
		}
	}
}

func TestOfflinePortCountsAtRejectsUnorderedInstants(t *testing.T) {
	store, err := OpenSQLite(t.TempDir()+"/state.db", bytes.Repeat([]byte{0x63}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	now := time.Now()
	if _, err := store.OfflinePortCountsAt([]time.Time{now, now.Add(-time.Hour)}); err == nil {
		t.Fatal("expected unordered instants to fail")
	}
}
