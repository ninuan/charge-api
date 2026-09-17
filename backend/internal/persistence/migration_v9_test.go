package persistence

import (
	"bytes"
	"database/sql"
	"testing"
)

func TestSQLiteMigratesV8ToV10WithoutLosingData(t *testing.T) {
	path := t.TempDir() + "/state.db"
	createV8MigrationFixture(t, path)
	key := bytes.Repeat([]byte{0x62}, CookieKeySize)

	store, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite v8 fixture: %v", err)
	}
	assertV8DataPreservedInV10(t, store)
	if err := store.Close(); err != nil {
		t.Fatalf("close migrated store: %v", err)
	}

	reopened, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	defer reopened.Close()
	assertV8DataPreservedInV10(t, reopened)
}

func createV8MigrationFixture(t *testing.T, path string) {
	t.Helper()
	createV7MigrationFixture(t, path)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open v8 fixture database: %v", err)
	}
	defer db.Close()
	statements := []string{
		`UPDATE metadata SET value='8' WHERE key='schema_version'`,
		`INSERT INTO metadata(key, value) VALUES(
			'registration_settings',
			'{"openRegistration":true,"inviteRequired":true,"defaultDeviceLimit":10,"defaultRefreshEnabled":true,"statsRetentionDays":90,"portHistoryRetentionDays":90}'
		)`,
		`ALTER TABLE users ADD COLUMN device_limit INTEGER NOT NULL DEFAULT 10`,
		`ALTER TABLE users ADD COLUMN refresh_enabled INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE users ADD COLUMN usage_guide_ack_at TEXT`,
		`ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE user_states ADD COLUMN yyb_binding_nonce BLOB`,
		`ALTER TABLE user_states ADD COLUMN yyb_binding_ciphertext BLOB`,
		`ALTER TABLE user_states ADD COLUMN recovery_diagnostics_json BLOB NOT NULL DEFAULT '[]'`,
		`ALTER TABLE sessions ADD COLUMN browser TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN os TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN device_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN ip_label TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN last_active_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE metrics ADD COLUMN count INTEGER NOT NULL DEFAULT 1`,
		`CREATE TABLE invite_codes (
			id TEXT PRIMARY KEY, code TEXT NOT NULL UNIQUE, enabled INTEGER NOT NULL,
			created_at TEXT NOT NULL, expires_at TEXT, used_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE port_status_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_id TEXT NOT NULL, port_id INTEGER NOT NULL, from_status TEXT,
			to_status TEXT NOT NULL, changed_at INTEGER NOT NULL,
			used_seconds INTEGER NOT NULL DEFAULT 0,
			remaining_text TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'remote'
		)`,
		`INSERT INTO port_status_events(
			user_id, device_id, port_id, to_status, changed_at, used_seconds, remaining_text, source
		) VALUES('user-1', 'device-1', 1, 'idle', 100, 0, '', 'remote')`,
		`CREATE TABLE service_health_checks (
			id INTEGER PRIMARY KEY AUTOINCREMENT, service TEXT NOT NULL, state TEXT NOT NULL,
			message TEXT NOT NULL, checked_at INTEGER NOT NULL
		)`,
		`INSERT INTO service_health_checks(service, state, message, checked_at)
		 VALUES('scan', 'healthy', 'ok', 100)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create v8 fixture with %q: %v", statement, err)
		}
	}
}

func assertV8DataPreservedInV10(t *testing.T, store *Store) {
	t.Helper()
	var version string
	if err := store.db.QueryRow(`SELECT value FROM metadata WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != "13" {
		t.Fatalf("schema version = %s, want 13", version)
	}
	for table, want := range map[string]int{
		"users": 1, "user_states": 1, "sessions": 1, "metrics": 1,
		"admin_audit_logs": 1, "admin_incidents": 1, "port_status_events": 1,
		"service_health_checks": 1, "watch_rules": 0,
		"notification_preferences": 0, "notifications": 0, "watch_refresh_states": 0,
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
	var settings string
	if err := store.db.QueryRow(`SELECT value FROM metadata WHERE key='registration_settings'`).Scan(&settings); err != nil {
		t.Fatalf("read registration settings: %v", err)
	}
	if !bytes.Contains([]byte(settings), []byte(`"portHistoryRetentionDays":90`)) {
		t.Fatalf("legacy settings changed during schema migration: %s", settings)
	}
}
