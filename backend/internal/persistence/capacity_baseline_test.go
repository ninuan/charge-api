package persistence

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"
)

const (
	capacityUsers          = 100
	capacityPortsPerUser   = 10
	capacityHistoryDays    = 90
	capacityEventsPerDay   = 4
	capacityQueryThreshold = 2 * time.Second
	capacityTrendThreshold = 2 * time.Second
)

// TestSQLiteCapacityBaseline is an opt-in release gate because it creates more
// than 360,000 history rows. Run it through `make capacity-baseline`; ordinary
// unit tests stay fast while the release measurement remains reproducible.
func TestSQLiteCapacityBaseline(t *testing.T) {
	if os.Getenv("CHARGE_RUN_CAPACITY_BASELINE") != "1" {
		t.Skip("run with make capacity-baseline")
	}

	path := t.TempDir() + "/capacity.db"
	createV7MigrationFixture(t, path)
	userIDs := extendV7CapacityFixture(t, path)

	migrationStarted := time.Now()
	store, err := OpenSQLite(path, bytes.Repeat([]byte{0x75}, CookieKeySize))
	if err != nil {
		t.Fatalf("migrate capacity fixture: %v", err)
	}
	defer store.Close()
	migrationDuration := time.Since(migrationStarted)

	now := time.Now().UTC().Truncate(time.Hour)
	seedStarted := time.Now()
	seedCapacityHistory(t, store.db, userIDs, now)
	seedDuration := time.Since(seedStarted)

	if _, err := store.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint capacity database: %v", err)
	}
	databaseInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat capacity database: %v", err)
	}

	queryStarted := time.Now()
	events, truncated, err := store.PortStatusEventsForAnalysis(PortStatusEventQuery{
		UserID: userIDs[0], DeviceID: "capacity-device-000",
		Since: now.Add(-7 * 24 * time.Hour), Until: now,
	}, 10_000)
	if err != nil {
		t.Fatalf("query seven-day device history: %v", err)
	}
	queryDuration := time.Since(queryStarted)
	if truncated || len(events) != capacityPortsPerUser*(7*capacityEventsPerDay+1) {
		t.Fatalf("seven-day history rows=%d truncated=%v", len(events), truncated)
	}
	if queryDuration > capacityQueryThreshold {
		t.Fatalf("seven-day device query took %s, threshold %s", queryDuration, capacityQueryThreshold)
	}

	trendStarted := time.Now()
	trendEnds := make([]time.Time, 0, 30)
	for day := 30; day > 0; day-- {
		trendEnds = append(trendEnds, now.Add(-time.Duration(day-1)*24*time.Hour))
	}
	if _, err := store.OfflinePortCountsAt(trendEnds); err != nil {
		t.Fatalf("reconstruct 30-day offline port trend: %v", err)
	}
	var requests int
	for day := 30; day > 0; day-- {
		start := now.Add(-time.Duration(day) * 24 * time.Hour)
		end := start.Add(24 * time.Hour)
		point, err := store.MetricAggregate(start, end)
		if err != nil {
			t.Fatalf("aggregate day %d: %v", day, err)
		}
		requests += point.Requests
	}
	trendDuration := time.Since(trendStarted)
	if requests != capacityUsers*30 {
		t.Fatalf("30-day request count=%d, want %d", requests, capacityUsers*30)
	}
	if trendDuration > capacityTrendThreshold {
		t.Fatalf("30-day admin trend took %s, threshold %s", trendDuration, capacityTrendThreshold)
	}

	cleanupStarted := time.Now()
	deleted, err := store.PrunePortStatusEvents(now.Add(-capacityHistoryDays * 24 * time.Hour))
	if err != nil {
		t.Fatalf("prune capacity history: %v", err)
	}
	cleanupDuration := time.Since(cleanupStarted)
	if deleted != capacityUsers*capacityPortsPerUser {
		t.Fatalf("pruned rows=%d, want %d", deleted, capacityUsers*capacityPortsPerUser)
	}

	var eventCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM port_status_events`).Scan(&eventCount); err != nil {
		t.Fatalf("count retained history: %v", err)
	}
	wantEvents := capacityUsers * capacityPortsPerUser * capacityHistoryDays * capacityEventsPerDay
	if eventCount != wantEvents {
		t.Fatalf("retained history rows=%d, want %d", eventCount, wantEvents)
	}

	t.Logf(
		"capacity baseline: users=%d ports=%d retained_events=%d database=%.1f MiB migration=%s seed=%s seven_day_query=%s thirty_day_trend=%s cleanup=%s",
		capacityUsers,
		capacityUsers*capacityPortsPerUser,
		eventCount,
		float64(databaseInfo.Size())/(1024*1024),
		migrationDuration.Round(time.Millisecond),
		seedDuration.Round(time.Millisecond),
		queryDuration.Round(time.Millisecond),
		trendDuration.Round(time.Millisecond),
		cleanupDuration.Round(time.Millisecond),
	)
}

func extendV7CapacityFixture(t *testing.T, path string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open v7 capacity fixture: %v", err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin v7 capacity fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	userIDs := make([]string, 0, capacityUsers)
	userIDs = append(userIDs, "user-1")
	for index := 1; index < capacityUsers; index++ {
		userID := fmt.Sprintf("capacity-user-%03d", index)
		username := fmt.Sprintf("capacity%03d", index)
		if _, err := tx.Exec(`INSERT INTO users VALUES(?, ?, 'hash', 'user', 1, ?, ?)`,
			userID, username, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert v7 capacity user: %v", err)
		}
		if _, err := tx.Exec(`INSERT INTO user_states VALUES(?, '[]', '{}', '[]', NULL, NULL, '{}')`, userID); err != nil {
			t.Fatalf("insert v7 capacity user state: %v", err)
		}
		userIDs = append(userIDs, userID)
	}

	metricStatement, err := tx.Prepare(`INSERT INTO metrics(user_id, kind, created_at) VALUES(?, ?, ?)`)
	if err != nil {
		t.Fatalf("prepare v7 capacity metrics: %v", err)
	}
	defer metricStatement.Close()
	metricStart := time.Now().UTC().Truncate(time.Hour).Add(-capacityHistoryDays * 24 * time.Hour)
	kinds := []string{"request", "remote", "remote_ok", "remote_failed"}
	for day := 0; day < capacityHistoryDays; day++ {
		createdAt := metricStart.Add(time.Duration(day)*24*time.Hour + time.Hour).Unix()
		for _, userID := range userIDs {
			for _, kind := range kinds {
				if _, err := metricStatement.Exec(userID, kind, createdAt); err != nil {
					t.Fatalf("insert v7 capacity metric: %v", err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit v7 capacity fixture: %v", err)
	}
	return userIDs
}

func seedCapacityHistory(t *testing.T, db *sql.DB, userIDs []string, now time.Time) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin capacity history seed: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	statement, err := tx.Prepare(`
		INSERT INTO port_status_events(
			user_id, device_id, port_id, from_status, to_status, changed_at,
			used_seconds, remaining_text, source
		) VALUES(?, ?, ?, ?, ?, ?, ?, '', 'capacity')
	`)
	if err != nil {
		t.Fatalf("prepare capacity history: %v", err)
	}
	defer statement.Close()

	statuses := []string{"in_use", "idle", "offline", "idle"}
	for userIndex, userID := range userIDs {
		deviceID := fmt.Sprintf("capacity-device-%03d", userIndex)
		for portID := 1; portID <= capacityPortsPerUser; portID++ {
			previous := "idle"
			if _, err := statement.Exec(
				userID, deviceID, portID, nil, previous,
				now.Add(-(capacityHistoryDays+1)*24*time.Hour).Unix(), 0,
			); err != nil {
				t.Fatalf("insert stale capacity baseline: %v", err)
			}
			for day := 0; day < capacityHistoryDays; day++ {
				dayStart := now.Add(-capacityHistoryDays * 24 * time.Hour).Add(time.Duration(day) * 24 * time.Hour)
				for eventIndex, status := range statuses {
					usedSeconds := 0
					if status == "in_use" {
						usedSeconds = 900
					}
					if _, err := statement.Exec(
						userID, deviceID, portID, previous, status,
						dayStart.Add(time.Duration(eventIndex)*6*time.Hour).Unix(), usedSeconds,
					); err != nil {
						t.Fatalf("insert capacity history: %v", err)
					}
					previous = status
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit capacity history: %v", err)
	}
}
