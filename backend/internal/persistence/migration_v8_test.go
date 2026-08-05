package persistence

import (
	"bytes"
	"database/sql"
	"testing"
	"time"
)

func TestSQLiteMigratesV7ToV8WithoutLosingData(t *testing.T) {
	path := t.TempDir() + "/state.db"
	createV7MigrationFixture(t, path)
	key := bytes.Repeat([]byte{0x61}, CookieKeySize)

	store, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite v7 fixture: %v", err)
	}
	assertV8MigrationData(t, store)
	if err := store.Close(); err != nil {
		t.Fatalf("close migrated store: %v", err)
	}

	reopened, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	defer reopened.Close()
	assertV8MigrationData(t, reopened)
}

func createV7MigrationFixture(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	defer db.Close()

	statements := []string{
		`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO metadata(key, value) VALUES('schema_version', '7')`,
		`CREATE TABLE users (
			id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
			role TEXT NOT NULL, enabled INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		)`,
		`INSERT INTO users VALUES(
			'user-1', 'alice', 'hash', 'user', 1,
			'2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z'
		)`,
		`CREATE TABLE user_states (
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			piles_json BLOB NOT NULL, refresh_json BLOB NOT NULL, device_ids_json BLOB NOT NULL,
			cookie_nonce BLOB, cookie_ciphertext BLOB, stats_json BLOB NOT NULL
		)`,
		`INSERT INTO user_states VALUES(
			'user-1', '[{"id":"device-1"}]', '{}', '["device-1"]', X'0102', X'0304', '{}'
		)`,
		`CREATE TABLE sessions (
			token_hash BLOB PRIMARY KEY, user_id TEXT NOT NULL, created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL
		)`,
		`INSERT INTO sessions VALUES(X'0506', 'user-1', 100, 200)`,
		`CREATE TABLE metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL,
			kind TEXT NOT NULL, created_at INTEGER NOT NULL
		)`,
		`INSERT INTO metrics(user_id, kind, created_at) VALUES('user-1', 'remote', 100)`,
		`CREATE TABLE admin_audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT, actor_id TEXT NOT NULL, actor TEXT NOT NULL,
			action TEXT NOT NULL, target_type TEXT NOT NULL, target_id TEXT NOT NULL,
			target_label TEXT NOT NULL, result TEXT NOT NULL, message TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`INSERT INTO admin_audit_logs(
			actor_id, actor, action, target_type, target_id, target_label, result, message, created_at
		) VALUES('admin', 'admin', 'user.update', 'user', 'user-1', 'alice', 'success', '', 100)`,
		`CREATE TABLE admin_incidents (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, username TEXT NOT NULL,
			device_id TEXT NOT NULL DEFAULT '', type TEXT NOT NULL, level TEXT NOT NULL,
			message TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'open', note TEXT NOT NULL DEFAULT '',
			occurrences INTEGER NOT NULL DEFAULT 1, handled_by TEXT NOT NULL DEFAULT '', handled_at INTEGER,
			first_seen_at INTEGER NOT NULL, last_seen_at INTEGER NOT NULL
		)`,
		`INSERT INTO admin_incidents(
			id, user_id, username, device_id, type, level, message, first_seen_at, last_seen_at
		) VALUES('issue-1', 'user-1', 'alice', 'device-1', 'offline', 'warning', 'offline', 100, 100)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create v7 fixture with %q: %v", statement, err)
		}
	}
}

func assertV8MigrationData(t *testing.T, store *Store) {
	t.Helper()
	var version string
	if err := store.db.QueryRow(`SELECT value FROM metadata WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != "8" {
		t.Fatalf("schema version = %s, want 8", version)
	}

	for table, want := range map[string]int{
		"users": 1, "user_states": 1, "sessions": 1, "metrics": 1,
		"admin_audit_logs": 1, "admin_incidents": 1, "port_status_events": 0,
	} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("%s rows = %d, want %d", table, count, want)
		}
	}

	var pilesJSON, deviceIDsJSON string
	var cookieNonce, cookieCiphertext []byte
	if err := store.db.QueryRow(`
		SELECT piles_json, device_ids_json, cookie_nonce, cookie_ciphertext
		FROM user_states WHERE user_id='user-1'
	`).Scan(&pilesJSON, &deviceIDsJSON, &cookieNonce, &cookieCiphertext); err != nil {
		t.Fatalf("read preserved user state: %v", err)
	}
	if pilesJSON != `[{"id":"device-1"}]` || deviceIDsJSON != `["device-1"]` ||
		!bytes.Equal(cookieNonce, []byte{1, 2}) || !bytes.Equal(cookieCiphertext, []byte{3, 4}) {
		t.Fatalf("user state changed during migration")
	}

	var metricCount int
	if err := store.db.QueryRow(`SELECT count FROM metrics WHERE id=1`).Scan(&metricCount); err != nil {
		t.Fatalf("read migrated metric count: %v", err)
	}
	if metricCount != 1 {
		t.Fatalf("migrated metric count = %d, want 1", metricCount)
	}
	points, err := store.MetricSeries(time.Unix(0, 0), 3600)
	if err != nil {
		t.Fatalf("MetricSeries after migration: %v", err)
	}
	if len(points) != 1 || points[0].Remote != 1 {
		t.Fatalf("migrated metric series changed: %+v", points)
	}
}
