package persistence

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"testing"
)

func TestSQLiteMigratesV9ToCurrentSchemaAndRetiresRecurringBehavior(t *testing.T) {
	path := t.TempDir() + "/state.db"
	createV9MigrationFixture(t, path)
	key := bytes.Repeat([]byte{0x63}, CookieKeySize)

	store, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite v9 fixture: %v", err)
	}
	assertV9DataPreservedInCurrentSchema(t, store)
	if err := store.Close(); err != nil {
		t.Fatalf("close migrated store: %v", err)
	}

	reopened, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	defer reopened.Close()
	assertV9DataPreservedInCurrentSchema(t, reopened)
}

func createV9MigrationFixture(t *testing.T, path string) {
	t.Helper()
	createV8MigrationFixture(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open v9 fixture database: %v", err)
	}
	defer db.Close()
	statements := []string{
		`UPDATE metadata SET value='9' WHERE key='schema_version'`,
		`UPDATE metadata SET value='{"openRegistration":true,"inviteRequired":true,"defaultDeviceLimit":10,"defaultRefreshEnabled":true,"statsRetentionDays":90,"portHistoryRetentionDays":90,"backgroundRemindersEnabled":true,"watchRefreshIntervalMinutes":10,"watchPileLimitPerUser":5,"watchDailyRefreshQuota":480,"notificationRetentionDays":90,"scheduledPowerOffEnabled":true,"scheduledPowerOffStartMinute":1380,"scheduledPowerOffEndMinute":420,"scheduledPowerOffTimezone":"Asia/Shanghai","powerRestoreJitterMinutes":10}' WHERE key='registration_settings'`,
		`CREATE TABLE watch_rules (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_id TEXT NOT NULL,
			port_id INTEGER,
			notify_idle INTEGER NOT NULL DEFAULT 0 CHECK(notify_idle IN (0, 1)),
			enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
			active_weekdays INTEGER NOT NULL DEFAULT 127 CHECK(active_weekdays BETWEEN 1 AND 127),
			active_start_minute INTEGER NOT NULL DEFAULT 0 CHECK(active_start_minute BETWEEN 0 AND 1439),
			active_end_minute INTEGER NOT NULL DEFAULT 0 CHECK(active_end_minute BETWEEN 0 AND 1439),
			timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX watch_rules_user_pile_unique_idx ON watch_rules(user_id, device_id)`,
		`CREATE INDEX watch_rules_enabled_pile_idx ON watch_rules(enabled, device_id)`,
		`INSERT INTO metadata(key, value) VALUES('watch_rules_pile_level', '1')`,
		`INSERT INTO watch_rules(
			id,user_id,device_id,port_id,notify_idle,enabled,active_weekdays,
			active_start_minute,active_end_minute,timezone,created_at,updated_at
		) VALUES('legacy-recurring','user-1','device-1',NULL,0,1,31,480,1320,'Asia/Shanghai',100,200)`,
		`CREATE TABLE notification_preferences (
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			browser_enabled INTEGER NOT NULL DEFAULT 0,
			quiet_hours_enabled INTEGER NOT NULL DEFAULT 1,
			quiet_start_minute INTEGER NOT NULL DEFAULT 1320,
			quiet_end_minute INTEGER NOT NULL DEFAULT 480,
			timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
			updated_at INTEGER NOT NULL
		)`,
		`INSERT INTO notification_preferences VALUES('user-1',1,1,1320,480,'Asia/Shanghai',200)`,
		`CREATE TABLE notifications (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			type TEXT NOT NULL,
			severity TEXT NOT NULL,
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			device_id TEXT NOT NULL DEFAULT '',
			port_id INTEGER,
			source_event_id INTEGER REFERENCES port_status_events(id) ON DELETE SET NULL,
			dedupe_key TEXT NOT NULL DEFAULT '',
			read_at INTEGER,
			resolved_at INTEGER,
			created_at INTEGER NOT NULL
		)`,
		`INSERT INTO notifications(
			id,user_id,type,severity,title,message,device_id,port_id,source_event_id,created_at
		) VALUES('notice-1','user-1','port_idle','info','idle','idle','device-1',1,1,200)`,
		`CREATE TABLE watch_refresh_states (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_id TEXT NOT NULL,
			next_attempt_at INTEGER NOT NULL,
			last_attempt_at INTEGER,
			last_success_at INTEGER,
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			paused_reason TEXT NOT NULL DEFAULT '',
			quota_date TEXT NOT NULL DEFAULT '',
			quota_used INTEGER NOT NULL DEFAULT 0,
			availability_known INTEGER NOT NULL DEFAULT 0,
			had_idle_port INTEGER NOT NULL DEFAULT 0,
			availability_event_id INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(user_id, device_id)
		)`,
		`INSERT INTO watch_refresh_states VALUES('user-1','device-1',300,200,200,0,'','2026-08-17',4,1,0,1,200)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create v9 fixture with %q: %v", statement, err)
		}
	}
}

func assertV9DataPreservedInCurrentSchema(t *testing.T, store *Store) {
	t.Helper()
	var version string
	if err := store.db.QueryRow(`SELECT value FROM metadata WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != "12" {
		t.Fatalf("schema version = %s, want 12", version)
	}
	var mode, completionReason string
	var expiresAt, completedAt any
	var stopAfterNotify, enabled int
	if err := store.db.QueryRow(`
		SELECT mode, expires_at, completed_at, completion_reason, stop_after_notify, enabled
		FROM watch_rules WHERE id='legacy-recurring'
	`).Scan(&mode, &expiresAt, &completedAt, &completionReason, &stopAfterNotify, &enabled); err != nil {
		t.Fatalf("read migrated watch rule: %v", err)
	}
	if mode != "recurring" || enabled != 0 || expiresAt != nil || completedAt != nil || completionReason != "" || stopAfterNotify != 0 {
		t.Fatalf("legacy rule was not safely retired: mode=%s enabled=%d expires=%v completed=%v reason=%s stop=%d", mode, enabled, expiresAt, completedAt, completionReason, stopAfterNotify)
	}
	var rawSettings string
	if err := store.db.QueryRow(`SELECT value FROM metadata WHERE key='registration_settings'`).Scan(&rawSettings); err != nil {
		t.Fatalf("read migrated settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(rawSettings), &settings); err != nil {
		t.Fatalf("parse migrated settings: %v", err)
	}
	if settings["recurringRemindersEnabled"] != false || settings["backgroundRemindersEnabled"] != true {
		t.Fatalf("reminder compatibility settings changed: %+v", settings)
	}
	for table, want := range map[string]int{
		"users": 1, "watch_rules": 1, "notification_preferences": 1,
		"notifications": 1, "watch_refresh_states": 1,
		"wxpusher_bindings": 0, "wxpusher_bind_sessions": 0, "notification_deliveries": 0,
	} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("%s rows = %d, want %d", table, count, want)
		}
	}
}
