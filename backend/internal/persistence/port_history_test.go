package persistence

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestPortStatusEventsRoundTripAndMaintenance(t *testing.T) {
	store, err := OpenSQLite(
		t.TempDir()+"/state.db",
		bytes.Repeat([]byte{0x51}, CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	if err := store.Save(State{
		Version: 3,
		Users: []model.User{{
			ID: "user-1", Username: "alice", PasswordHash: "hash",
			Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
		}},
		UserStates: map[string]UserState{"user-1": {}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	idle := model.PortIdle
	inUse := model.PortInUse
	events := []model.PortStatusEvent{
		{
			UserID: "user-1", DeviceID: "device-1", PortID: 1,
			ToStatus: model.PortIdle, ChangedAt: now, Source: "remote",
		},
		{
			UserID: "user-1", DeviceID: "device-1", PortID: 1,
			FromStatus: &idle, ToStatus: model.PortInUse,
			ChangedAt: now.Add(time.Hour), UsedSeconds: 300, RemainingText: "25 分钟", Source: "remote",
		},
		{
			UserID: "user-1", DeviceID: "device-1", PortID: 1,
			FromStatus: &inUse, ToStatus: model.PortIdle,
			ChangedAt: now.Add(2 * time.Hour), UsedSeconds: 3600, Source: "remote",
		},
		{
			UserID: "user-1", DeviceID: "device-1", PortID: 2,
			ToStatus: model.PortOffline, ChangedAt: now.Add(30 * time.Minute), Source: "remote",
		},
	}
	if err := store.RecordPortStatusEvents(events); err != nil {
		t.Fatalf("RecordPortStatusEvents: %v", err)
	}

	latest, err := store.LatestPortStatuses("user-1", "device-1")
	if err != nil {
		t.Fatalf("LatestPortStatuses: %v", err)
	}
	if len(latest) != 2 || latest[1].ToStatus != model.PortIdle || latest[2].ToStatus != model.PortOffline {
		t.Fatalf("unexpected latest statuses: %+v", latest)
	}
	if latest[1].FromStatus == nil || *latest[1].FromStatus != model.PortInUse {
		t.Fatalf("latest transition missing previous status: %+v", latest[1])
	}

	portID := 1
	result, err := store.PortStatusEvents(PortStatusEventQuery{
		UserID: "user-1", DeviceID: "device-1", PortID: &portID,
		Since: now.Add(30 * time.Minute), Until: now.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("PortStatusEvents: %v", err)
	}
	if len(result) != 2 || result[0].ToStatus != model.PortInUse || result[1].ToStatus != model.PortIdle {
		t.Fatalf("unexpected ranged events: %+v", result)
	}
	if result[0].RemainingText != "25 分钟" || !result[0].ChangedAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("event fields did not round-trip: %+v", result[0])
	}

	pruned, err := store.PrunePortStatusEvents(now.Add(45 * time.Minute))
	if err != nil {
		t.Fatalf("PrunePortStatusEvents: %v", err)
	}
	if pruned != 2 {
		t.Fatalf("pruned = %d, want 2", pruned)
	}
	deleted, err := store.DeletePortStatusEvents("user-1", "device-1")
	if err != nil {
		t.Fatalf("DeletePortStatusEvents: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
}

func TestRecordPortStatusEventsRejectsInvalidBatchAtomically(t *testing.T) {
	store, err := OpenSQLite(
		t.TempDir()+"/state.db",
		bytes.Repeat([]byte{0x52}, CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.Save(State{
		Version: 3,
		Users: []model.User{{
			ID: "user-1", Username: "alice", PasswordHash: "hash",
			Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
		}},
		UserStates: map[string]UserState{"user-1": {}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	err = store.RecordPortStatusEvents([]model.PortStatusEvent{
		{UserID: "user-1", DeviceID: "device-1", PortID: 1, ToStatus: model.PortIdle, ChangedAt: now},
		{UserID: "user-1", DeviceID: "device-1", PortID: 0, ToStatus: model.PortIdle, ChangedAt: now},
	})
	if err == nil {
		t.Fatal("expected invalid port id error")
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM port_status_events`).Scan(&count); err != nil {
		t.Fatalf("count port events: %v", err)
	}
	if count != 0 {
		t.Fatalf("invalid batch inserted %d events", count)
	}
}

func TestPortStatusEventsCascadeWhenUserIsDeleted(t *testing.T) {
	store, err := OpenSQLite(
		t.TempDir()+"/state.db",
		bytes.Repeat([]byte{0x54}, CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.Save(State{
		Version: 3,
		Users: []model.User{{
			ID: "user-1", Username: "alice", PasswordHash: "hash",
			Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
		}},
		UserStates: map[string]UserState{"user-1": {}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.RecordPortStatusEvents([]model.PortStatusEvent{{
		UserID: "user-1", DeviceID: "device-1", PortID: 1,
		ToStatus: model.PortIdle, ChangedAt: now,
	}}); err != nil {
		t.Fatalf("RecordPortStatusEvents: %v", err)
	}
	if _, err := store.db.Exec(`DELETE FROM users WHERE id='user-1'`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM port_status_events`).Scan(&count); err != nil {
		t.Fatalf("count port events: %v", err)
	}
	if count != 0 {
		t.Fatalf("user deletion left %d port events", count)
	}
}

func TestPortStatusRangeQueryUsesCompositeIndex(t *testing.T) {
	store, err := OpenSQLite(
		t.TempDir()+"/state.db",
		bytes.Repeat([]byte{0x53}, CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	rows, err := store.db.Query(`
		EXPLAIN QUERY PLAN
		SELECT id FROM port_status_events
		WHERE user_id = ? AND device_id = ? AND port_id = ?
		  AND changed_at >= ? AND changed_at < ?
		ORDER BY changed_at, id LIMIT ?
	`, "user-1", "device-1", 1, 0, time.Now().Unix(), 1000)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		details = append(details, detail)
	}
	joined := strings.Join(details, "\n")
	if !strings.Contains(joined, "port_status_events_user_device_port_time_idx") {
		t.Fatalf("range query did not use composite history index:\n%s", joined)
	}
}
