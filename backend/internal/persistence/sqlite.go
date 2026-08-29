package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charge-dashboard/internal/model"

	_ "modernc.org/sqlite"
)

const schemaVersion = 12

type Store struct {
	db     *sql.DB
	cipher *secretCipher
	path   string
}

func OpenSQLite(path string, cookieKey []byte) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	cipher, err := newCookieCipher(cookieKey)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, cipher: cipher, path: path}
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure database permissions: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) initialize() error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = FULL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			enabled INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS user_states (
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			piles_json BLOB NOT NULL,
			refresh_json BLOB NOT NULL,
			device_ids_json BLOB NOT NULL,
			cookie_nonce BLOB,
			cookie_ciphertext BLOB,
			stats_json BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash BLOB PRIMARY KEY,
			user_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS invite_codes (
			id TEXT PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT,
			used_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			count INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS metrics_created_at_idx ON metrics(created_at)`,
		`CREATE INDEX IF NOT EXISTS metrics_user_time_idx ON metrics(user_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS port_status_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_id TEXT NOT NULL,
			port_id INTEGER NOT NULL,
			from_status TEXT,
			to_status TEXT NOT NULL,
			changed_at INTEGER NOT NULL,
			used_seconds INTEGER NOT NULL DEFAULT 0,
			remaining_text TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'remote'
		)`,
		`CREATE INDEX IF NOT EXISTS port_status_events_user_device_port_time_idx
			ON port_status_events(user_id, device_id, port_id, changed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS port_status_events_user_time_idx
			ON port_status_events(user_id, changed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS port_status_events_changed_at_idx
			ON port_status_events(changed_at)`,
		`CREATE TABLE IF NOT EXISTS watch_rules (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_id TEXT NOT NULL,
			port_id INTEGER,
			notify_idle INTEGER NOT NULL DEFAULT 0 CHECK(notify_idle IN (0, 1)),
			mode TEXT NOT NULL DEFAULT 'recurring' CHECK(mode IN ('temporary', 'recurring')),
			enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
			active_weekdays INTEGER NOT NULL DEFAULT 127 CHECK(active_weekdays BETWEEN 1 AND 127),
			active_start_minute INTEGER NOT NULL DEFAULT 0 CHECK(active_start_minute BETWEEN 0 AND 1439),
			active_end_minute INTEGER NOT NULL DEFAULT 0 CHECK(active_end_minute BETWEEN 0 AND 1439),
			timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai' CHECK(length(trim(timezone)) > 0),
			expires_at INTEGER,
			completed_at INTEGER,
			completion_reason TEXT NOT NULL DEFAULT '' CHECK(completion_reason IN ('', 'notified', 'expired', 'cancelled')),
			stop_after_notify INTEGER NOT NULL DEFAULT 0 CHECK(stop_after_notify IN (0, 1)),
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			CHECK(length(trim(device_id)) > 0),
			CHECK(port_id IS NULL OR port_id > 0),
			CHECK(notify_idle = 0 OR port_id IS NOT NULL),
			CHECK(
				(mode = 'recurring' AND expires_at IS NULL AND completed_at IS NULL AND completion_reason = '' AND stop_after_notify = 0)
				OR
				(mode = 'temporary' AND expires_at IS NOT NULL AND expires_at > created_at AND stop_after_notify = 1
					AND ((completed_at IS NULL AND completion_reason = '')
						OR (completed_at IS NOT NULL AND completed_at >= created_at AND completion_reason IN ('notified', 'expired', 'cancelled'))))
			)
		)`,
		`CREATE INDEX IF NOT EXISTS watch_rules_user_updated_idx
			ON watch_rules(user_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS notification_preferences (
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			browser_enabled INTEGER NOT NULL DEFAULT 0 CHECK(browser_enabled IN (0, 1)),
			quiet_hours_enabled INTEGER NOT NULL DEFAULT 1 CHECK(quiet_hours_enabled IN (0, 1)),
			quiet_start_minute INTEGER NOT NULL DEFAULT 1320 CHECK(quiet_start_minute BETWEEN 0 AND 1439),
			quiet_end_minute INTEGER NOT NULL DEFAULT 480 CHECK(quiet_end_minute BETWEEN 0 AND 1439),
			timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai' CHECK(length(trim(timezone)) > 0),
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS notifications (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			type TEXT NOT NULL CHECK(type IN ('port_idle', 'credential_expired', 'pile_offline', 'pile_recovered')),
			severity TEXT NOT NULL CHECK(severity IN ('info', 'warning', 'critical')),
			title TEXT NOT NULL CHECK(length(trim(title)) > 0),
			message TEXT NOT NULL CHECK(length(trim(message)) > 0),
			device_id TEXT NOT NULL DEFAULT '',
			port_id INTEGER,
			source_event_id INTEGER REFERENCES port_status_events(id) ON DELETE SET NULL,
			dedupe_key TEXT NOT NULL DEFAULT '',
			read_at INTEGER,
			resolved_at INTEGER,
			occurrence_count INTEGER NOT NULL DEFAULT 1 CHECK(occurrence_count >= 1),
			last_occurred_at INTEGER NOT NULL DEFAULT 0 CHECK(last_occurred_at >= 0),
			created_at INTEGER NOT NULL,
			CHECK(port_id IS NULL OR port_id > 0),
			CHECK(type <> 'port_idle' OR (length(trim(device_id)) > 0 AND port_id IS NOT NULL))
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS notifications_source_event_unique_idx
			ON notifications(source_event_id) WHERE source_event_id IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS notifications_active_dedupe_unique_idx
			ON notifications(user_id, dedupe_key)
			WHERE dedupe_key <> '' AND resolved_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS notifications_user_unread_time_idx
			ON notifications(user_id, read_at, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS notifications_user_resolved_time_idx
			ON notifications(user_id, resolved_at, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS wxpusher_bindings (
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			uid_fingerprint BLOB NOT NULL UNIQUE CHECK(length(uid_fingerprint) = 32),
			uid_nonce BLOB NOT NULL CHECK(length(uid_nonce) > 0),
			uid_ciphertext BLOB NOT NULL CHECK(length(uid_ciphertext) > 0),
			enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
			event_types INTEGER NOT NULL DEFAULT 7 CHECK(event_types BETWEEN 0 AND 15),
			bound_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			last_test_at INTEGER,
			last_accepted_at INTEGER,
			last_provider_success_at INTEGER,
			last_error_code TEXT NOT NULL DEFAULT '' CHECK(length(last_error_code) <= 64),
			last_error_at INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS wxpusher_bind_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider_code_nonce BLOB NOT NULL CHECK(length(provider_code_nonce) > 0),
			provider_code_ciphertext BLOB NOT NULL CHECK(length(provider_code_ciphertext) > 0),
			qr_url TEXT NOT NULL CHECK(length(qr_url) BETWEEN 1 AND 2048 AND lower(qr_url) LIKE 'https://%'),
			expires_at INTEGER NOT NULL,
			next_poll_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			completed_at INTEGER,
			CHECK(expires_at > created_at),
			CHECK(next_poll_at >= created_at),
			CHECK(completed_at IS NULL OR completed_at >= created_at)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS wxpusher_bind_sessions_user_active_unique_idx
			ON wxpusher_bind_sessions(user_id) WHERE completed_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS wxpusher_bind_sessions_expiry_idx
			ON wxpusher_bind_sessions(expires_at, completed_at)`,
		`CREATE TABLE IF NOT EXISTS notification_deliveries (
			id TEXT PRIMARY KEY,
			notification_id TEXT REFERENCES notifications(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			channel TEXT NOT NULL DEFAULT 'wxpusher' CHECK(channel = 'wxpusher'),
			status TEXT NOT NULL CHECK(status IN ('pending', 'sending', 'accepted', 'provider_succeeded', 'retry_wait', 'suppressed', 'uncertain', 'failed', 'cancelled')),
			is_test INTEGER NOT NULL DEFAULT 0 CHECK(is_test IN (0, 1)),
			attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(attempt_count >= 0),
			next_attempt_at INTEGER,
			claimed_at INTEGER,
			provider_record_id TEXT NOT NULL DEFAULT '' CHECK(length(provider_record_id) <= 128),
			provider_message_content_id TEXT NOT NULL DEFAULT '' CHECK(length(provider_message_content_id) <= 128),
			last_error_code TEXT NOT NULL DEFAULT '' CHECK(length(last_error_code) <= 64),
			last_error_message TEXT NOT NULL DEFAULT '' CHECK(length(last_error_message) <= 512),
			accepted_at INTEGER,
			provider_succeeded_at INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			CHECK(notification_id IS NOT NULL OR is_test = 1)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS notification_deliveries_notification_channel_unique_idx
			ON notification_deliveries(notification_id, channel) WHERE notification_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS notification_deliveries_status_due_idx
			ON notification_deliveries(status, next_attempt_at, created_at)`,
		`CREATE INDEX IF NOT EXISTS notification_deliveries_user_time_idx
			ON notification_deliveries(user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS watch_refresh_states (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_id TEXT NOT NULL,
			next_attempt_at INTEGER NOT NULL,
			last_attempt_at INTEGER,
			last_success_at INTEGER,
			consecutive_failures INTEGER NOT NULL DEFAULT 0 CHECK(consecutive_failures >= 0),
			paused_reason TEXT NOT NULL DEFAULT '',
			quota_date TEXT NOT NULL DEFAULT '',
			quota_used INTEGER NOT NULL DEFAULT 0 CHECK(quota_used >= 0),
			availability_known INTEGER NOT NULL DEFAULT 0 CHECK(availability_known IN (0, 1)),
			had_idle_port INTEGER NOT NULL DEFAULT 0 CHECK(had_idle_port IN (0, 1)),
			availability_event_id INTEGER NOT NULL DEFAULT 0 CHECK(availability_event_id >= 0),
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(user_id, device_id),
			CHECK(length(trim(device_id)) > 0)
		)`,
		`CREATE INDEX IF NOT EXISTS watch_refresh_states_due_idx
			ON watch_refresh_states(next_attempt_at, paused_reason)`,
		`CREATE TABLE IF NOT EXISTS admin_audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_id TEXT NOT NULL,
			actor TEXT NOT NULL,
			action TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			target_label TEXT NOT NULL,
			result TEXT NOT NULL,
			message TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS admin_audit_created_at_idx ON admin_audit_logs(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS admin_incidents (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			username TEXT NOT NULL,
			device_id TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			note TEXT NOT NULL DEFAULT '',
			occurrences INTEGER NOT NULL DEFAULT 1,
			handled_by TEXT NOT NULL DEFAULT '',
			handled_at INTEGER,
			first_seen_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS admin_incidents_status_time_idx ON admin_incidents(status, last_seen_at DESC)`,
		`CREATE TABLE IF NOT EXISTS service_health_checks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			service TEXT NOT NULL,
			state TEXT NOT NULL,
			message TEXT NOT NULL,
			checked_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS service_health_service_time_idx ON service_health_checks(service, checked_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize sqlite database: %w", err)
		}
	}
	if err := s.normalizeWatchRulesToPiles(); err != nil {
		return err
	}
	if err := s.ensureColumn("users", "device_limit", "INTEGER NOT NULL DEFAULT 10"); err != nil {
		return err
	}
	if err := s.ensureColumn("users", "refresh_enabled", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.ensureColumn("users", "usage_guide_ack_at", "TEXT"); err != nil {
		return err
	}
	if err := s.ensureColumn("users", "must_change_password", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("user_states", "yyb_binding_nonce", "BLOB"); err != nil {
		return err
	}
	if err := s.ensureColumn("user_states", "yyb_binding_ciphertext", "BLOB"); err != nil {
		return err
	}
	if err := s.ensureColumn("user_states", "recovery_diagnostics_json", "BLOB NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.ensureColumn("metrics", "count", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.ensureColumn("watch_refresh_states", "availability_known", "INTEGER NOT NULL DEFAULT 0 CHECK(availability_known IN (0, 1))"); err != nil {
		return err
	}
	if err := s.ensureColumn("watch_refresh_states", "had_idle_port", "INTEGER NOT NULL DEFAULT 0 CHECK(had_idle_port IN (0, 1))"); err != nil {
		return err
	}
	if err := s.ensureColumn("watch_refresh_states", "availability_event_id", "INTEGER NOT NULL DEFAULT 0 CHECK(availability_event_id >= 0)"); err != nil {
		return err
	}
	if err := s.ensureColumn("watch_rules", "mode", "TEXT NOT NULL DEFAULT 'recurring' CHECK(mode IN ('temporary', 'recurring'))"); err != nil {
		return err
	}
	if err := s.ensureColumn("watch_rules", "expires_at", "INTEGER"); err != nil {
		return err
	}
	if err := s.ensureColumn("watch_rules", "completed_at", "INTEGER"); err != nil {
		return err
	}
	if err := s.ensureColumn("watch_rules", "completion_reason", "TEXT NOT NULL DEFAULT '' CHECK(completion_reason IN ('', 'notified', 'expired', 'cancelled'))"); err != nil {
		return err
	}
	if err := s.ensureColumn("watch_rules", "stop_after_notify", "INTEGER NOT NULL DEFAULT 0 CHECK(stop_after_notify IN (0, 1))"); err != nil {
		return err
	}
	if err := s.ensureColumn("notifications", "occurrence_count", "INTEGER NOT NULL DEFAULT 1 CHECK(occurrence_count >= 1)"); err != nil {
		return err
	}
	if err := s.ensureColumn("notifications", "last_occurred_at", "INTEGER NOT NULL DEFAULT 0 CHECK(last_occurred_at >= 0)"); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE notifications SET last_occurred_at = created_at WHERE last_occurred_at = 0`); err != nil {
		return fmt.Errorf("backfill notification occurrence time: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS notifications_user_last_occurrence_idx ON notifications(user_id, last_occurred_at DESC, id DESC)`); err != nil {
		return fmt.Errorf("create notification occurrence index: %w", err)
	}
	for column, definition := range map[string]string{
		"browser":        "TEXT NOT NULL DEFAULT ''",
		"os":             "TEXT NOT NULL DEFAULT ''",
		"device_type":    "TEXT NOT NULL DEFAULT ''",
		"ip_label":       "TEXT NOT NULL DEFAULT ''",
		"last_active_at": "INTEGER NOT NULL DEFAULT 0",
	} {
		if err := s.ensureColumn("sessions", column, definition); err != nil {
			return err
		}
	}
	if err := s.migrateSchemaV10(); err != nil {
		return err
	}
	if err := s.ensureSchemaV10Objects(); err != nil {
		return err
	}
	if err := s.migrateSchemaV12(); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO metadata(key, value) VALUES('schema_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		fmt.Sprintf("%d", schemaVersion),
	)
	if err != nil {
		return fmt.Errorf("write schema version: %w", err)
	}
	return nil
}

// migrateSchemaV12 retires fixed-schedule reminders without deleting their
// configuration. Old rows remain available for rollback, but cannot be picked
// up by the scheduler after this migration.
func (s *Store) migrateSchemaV12() error {
	if _, ok, err := s.metadata("schema_v12_recurring_retired"); err != nil {
		return err
	} else if ok {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema v12 migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE watch_rules SET enabled=0 WHERE mode='recurring' AND enabled=1`); err != nil {
		return fmt.Errorf("disable recurring watch rules: %w", err)
	}
	var rawSettings string
	err = tx.QueryRow(`SELECT value FROM metadata WHERE key='registration_settings'`).Scan(&rawSettings)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read settings for schema v12: %w", err)
	}
	if err == nil {
		settings := make(map[string]any)
		if err := json.Unmarshal([]byte(rawSettings), &settings); err != nil {
			return fmt.Errorf("parse settings for schema v12: %w", err)
		}
		settings["recurringRemindersEnabled"] = false
		encoded, err := json.Marshal(settings)
		if err != nil {
			return fmt.Errorf("encode settings for schema v12: %w", err)
		}
		if _, err := tx.Exec(`UPDATE metadata SET value=? WHERE key='registration_settings'`, string(encoded)); err != nil {
			return fmt.Errorf("save settings for schema v12: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO metadata(key, value) VALUES('schema_v12_recurring_retired', '1')`); err != nil {
		return fmt.Errorf("mark schema v12 migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema v12 migration: %w", err)
	}
	return nil
}

func (s *Store) migrateSchemaV10() error {
	if _, ok, err := s.metadata("schema_v10_contracts"); err != nil {
		return err
	} else if ok {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema v10 migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		UPDATE watch_rules
		SET mode = 'recurring', expires_at = NULL, completed_at = NULL,
			completion_reason = '', stop_after_notify = 0
	`); err != nil {
		return fmt.Errorf("migrate recurring watch rules: %w", err)
	}
	var rawSettings string
	err = tx.QueryRow(`SELECT value FROM metadata WHERE key='registration_settings'`).Scan(&rawSettings)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read settings for schema v10: %w", err)
	}
	if err == nil {
		settings := make(map[string]any)
		if err := json.Unmarshal([]byte(rawSettings), &settings); err != nil {
			return fmt.Errorf("parse settings for schema v10: %w", err)
		}
		if _, exists := settings["recurringRemindersEnabled"]; !exists {
			settings["recurringRemindersEnabled"] = true
			encoded, err := json.Marshal(settings)
			if err != nil {
				return fmt.Errorf("encode settings for schema v10: %w", err)
			}
			if _, err := tx.Exec(`UPDATE metadata SET value=? WHERE key='registration_settings'`, string(encoded)); err != nil {
				return fmt.Errorf("save settings for schema v10: %w", err)
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO metadata(key, value) VALUES('schema_v10_contracts', '1')`); err != nil {
		return fmt.Errorf("mark schema v10 migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema v10 migration: %w", err)
	}
	return nil
}

func (s *Store) ensureSchemaV10Objects() error {
	statements := []string{
		`DROP INDEX IF EXISTS watch_rules_user_pile_unique_idx`,
		`CREATE UNIQUE INDEX IF NOT EXISTS watch_rules_user_active_pile_unique_idx
			ON watch_rules(user_id, device_id)
			WHERE enabled = 1 AND completed_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS watch_rules_scheduler_v10_idx
			ON watch_rules(enabled, mode, completed_at, expires_at, device_id)`,
		`CREATE TRIGGER IF NOT EXISTS watch_rules_v10_insert_guard
			BEFORE INSERT ON watch_rules
			WHEN NOT (
				(NEW.mode = 'recurring' AND NEW.expires_at IS NULL AND NEW.completed_at IS NULL
					AND NEW.completion_reason = '' AND NEW.stop_after_notify = 0)
				OR
				(NEW.mode = 'temporary' AND NEW.expires_at IS NOT NULL AND NEW.expires_at > NEW.created_at
					AND NEW.stop_after_notify = 1
					AND ((NEW.completed_at IS NULL AND NEW.completion_reason = '')
						OR (NEW.completed_at IS NOT NULL AND NEW.completed_at >= NEW.created_at
							AND NEW.completion_reason IN ('notified', 'expired', 'cancelled'))))
			)
			BEGIN SELECT RAISE(ABORT, 'invalid watch rule lifecycle'); END`,
		`CREATE TRIGGER IF NOT EXISTS watch_rules_v10_update_guard
			BEFORE UPDATE ON watch_rules
			WHEN NOT (
				(NEW.mode = 'recurring' AND NEW.expires_at IS NULL AND NEW.completed_at IS NULL
					AND NEW.completion_reason = '' AND NEW.stop_after_notify = 0)
				OR
				(NEW.mode = 'temporary' AND NEW.expires_at IS NOT NULL AND NEW.expires_at > NEW.created_at
					AND NEW.stop_after_notify = 1
					AND ((NEW.completed_at IS NULL AND NEW.completion_reason = '')
						OR (NEW.completed_at IS NOT NULL AND NEW.completed_at >= NEW.created_at
							AND NEW.completion_reason IN ('notified', 'expired', 'cancelled'))))
			)
			BEGIN SELECT RAISE(ABORT, 'invalid watch rule lifecycle'); END`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("ensure schema v10 object: %w", err)
		}
	}
	return nil
}

func (s *Store) normalizeWatchRulesToPiles() error {
	if _, ok, err := s.metadata("watch_rules_pile_level"); err != nil {
		return err
	} else if ok {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin pile-level watch rule migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Former favorites never triggered remote requests, so silently turning
	// them into reminders would be surprising. Only old active reminder rows
	// migrate; multiple port rules for one pile collapse to the most recently
	// updated rule and retain its schedule and enabled state.
	if _, err := tx.Exec(`DELETE FROM watch_rules WHERE notify_idle = 0`); err != nil {
		return fmt.Errorf("remove legacy watch favorites: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM watch_rules
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (
					PARTITION BY user_id, device_id
					ORDER BY enabled DESC, updated_at DESC, id
				) AS position
				FROM watch_rules
			) WHERE position > 1
		)
	`); err != nil {
		return fmt.Errorf("merge legacy port watch rules: %w", err)
	}
	if _, err := tx.Exec(`UPDATE watch_rules SET port_id = NULL, notify_idle = 0`); err != nil {
		return fmt.Errorf("normalize pile watch rule targets: %w", err)
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS watch_rules_user_target_unique_idx`); err != nil {
		return fmt.Errorf("drop legacy watch target index: %w", err)
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS watch_rules_enabled_pile_idx`); err != nil {
		return fmt.Errorf("drop legacy watch scheduler index: %w", err)
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS watch_rules_user_pile_unique_idx ON watch_rules(user_id, device_id)`); err != nil {
		return fmt.Errorf("create pile watch rule index: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS watch_rules_enabled_pile_idx ON watch_rules(enabled, device_id)`); err != nil {
		return fmt.Errorf("create pile watch scheduler index: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO metadata(key, value) VALUES('watch_rules_pile_level', '1')`); err != nil {
		return fmt.Errorf("mark pile watch rule migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pile-level watch rule migration: %w", err)
	}
	return nil
}

func (s *Store) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if _, err := s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

type SessionRecord struct {
	TokenHash    []byte
	UserID       string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastActiveAt time.Time
	Browser      string
	OS           string
	DeviceType   string
	IPLabel      string
}

func (s *Store) SaveSession(record SessionRecord, maxPerUser int) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin session transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec(`
		INSERT INTO sessions(
			token_hash, user_id, created_at, expires_at, last_active_at,
			browser, os, device_type, ip_label
		)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(token_hash) DO UPDATE SET
			user_id = excluded.user_id,
			created_at = excluded.created_at,
			expires_at = excluded.expires_at,
			last_active_at = excluded.last_active_at,
			browser = excluded.browser,
			os = excluded.os,
			device_type = excluded.device_type,
			ip_label = excluded.ip_label
	`,
		record.TokenHash,
		record.UserID,
		record.CreatedAt.UnixNano(),
		record.ExpiresAt.UnixNano(),
		record.LastActiveAt.UnixNano(),
		record.Browser,
		record.OS,
		record.DeviceType,
		record.IPLabel,
	); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	if maxPerUser > 0 {
		if _, err := tx.Exec(`
			DELETE FROM sessions
			WHERE token_hash IN (
				SELECT token_hash FROM sessions
				WHERE user_id = ?
				ORDER BY created_at DESC
				LIMIT -1 OFFSET ?
			)
		`, record.UserID, maxPerUser); err != nil {
			return fmt.Errorf("limit user sessions: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session transaction: %w", err)
	}
	return nil
}

func (s *Store) LoadSession(tokenHash []byte) (SessionRecord, bool, error) {
	var record SessionRecord
	var createdAt, expiresAt, lastActiveAt int64
	err := s.db.QueryRow(`
		SELECT token_hash, user_id, created_at, expires_at, last_active_at,
		       browser, os, device_type, ip_label
		FROM sessions WHERE token_hash = ?
	`, tokenHash).Scan(
		&record.TokenHash, &record.UserID, &createdAt, &expiresAt, &lastActiveAt,
		&record.Browser, &record.OS, &record.DeviceType, &record.IPLabel,
	)
	if err == sql.ErrNoRows {
		return SessionRecord{}, false, nil
	}
	if err != nil {
		return SessionRecord{}, false, fmt.Errorf("load session: %w", err)
	}
	record.CreatedAt = time.Unix(0, createdAt)
	record.ExpiresAt = time.Unix(0, expiresAt)
	if lastActiveAt == 0 {
		lastActiveAt = createdAt
	}
	record.LastActiveAt = time.Unix(0, lastActiveAt)
	return record, true, nil
}

func (s *Store) DeleteSession(tokenHash []byte) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) DeleteUserSessions(userID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpiredSessions(now time.Time) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now.UnixNano())
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

func (s *Store) ListUserSessions(userID string) ([]SessionRecord, error) {
	rows, err := s.db.Query(`
		SELECT token_hash, user_id, created_at, expires_at, last_active_at,
		       browser, os, device_type, ip_label
		FROM sessions WHERE user_id=? ORDER BY last_active_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SessionRecord
	for rows.Next() {
		var record SessionRecord
		var createdAt, expiresAt, lastActiveAt int64
		if err := rows.Scan(
			&record.TokenHash, &record.UserID, &createdAt, &expiresAt, &lastActiveAt,
			&record.Browser, &record.OS, &record.DeviceType, &record.IPLabel,
		); err != nil {
			return nil, err
		}
		record.CreatedAt = time.Unix(0, createdAt)
		record.ExpiresAt = time.Unix(0, expiresAt)
		if lastActiveAt == 0 {
			lastActiveAt = createdAt
		}
		record.LastActiveAt = time.Unix(0, lastActiveAt)
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) TouchSession(tokenHash []byte, at time.Time) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET last_active_at=? WHERE token_hash=?`,
		at.UnixNano(),
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (s *Store) DeleteOtherSessions(userID string, currentHash []byte) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id=? AND token_hash<>?`, userID, currentHash)
	return err
}

func (s *Store) Load() (State, bool, error) {
	rows, err := s.db.Query(`
		SELECT id, username, password_hash, role, enabled, created_at, updated_at,
		       device_limit, refresh_enabled, must_change_password, usage_guide_ack_at
		FROM users ORDER BY username
	`)
	if err != nil {
		return State{}, false, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	state := State{
		Version:    schemaVersion,
		UserStates: make(map[string]UserState),
	}
	for rows.Next() {
		var user model.User
		var role string
		var enabled, refreshEnabled, mustChangePassword int
		var createdAt string
		var updatedAt string
		var usageGuideAckAt sql.NullString
		if err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.PasswordHash,
			&role,
			&enabled,
			&createdAt,
			&updatedAt,
			&user.DeviceLimit,
			&refreshEnabled,
			&mustChangePassword,
			&usageGuideAckAt,
		); err != nil {
			return State{}, false, fmt.Errorf("scan user: %w", err)
		}
		user.Role = model.UserRole(role)
		user.Enabled = enabled != 0
		user.RefreshEnabled = refreshEnabled != 0
		user.MustChangePassword = mustChangePassword != 0
		user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return State{}, false, fmt.Errorf("parse user created_at: %w", err)
		}
		user.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return State{}, false, fmt.Errorf("parse user updated_at: %w", err)
		}
		if usageGuideAckAt.Valid {
			ackAt, err := time.Parse(time.RFC3339Nano, usageGuideAckAt.String)
			if err != nil {
				return State{}, false, fmt.Errorf("parse user usage_guide_ack_at: %w", err)
			}
			user.UsageGuideAckAt = &ackAt
		}
		state.Users = append(state.Users, user)
	}
	if err := rows.Err(); err != nil {
		return State{}, false, fmt.Errorf("iterate users: %w", err)
	}
	if len(state.Users) == 0 {
		return state, false, nil
	}
	if raw, ok, err := s.metadata("registration_settings"); err != nil {
		return State{}, false, err
	} else if ok {
		if err := json.Unmarshal([]byte(raw), &state.Settings); err != nil {
			return State{}, false, fmt.Errorf("parse registration settings: %w", err)
		}
	}
	inviteRows, err := s.db.Query(`SELECT id, code, enabled, created_at, expires_at, used_count FROM invite_codes ORDER BY created_at DESC`)
	if err != nil {
		return State{}, false, err
	}
	for inviteRows.Next() {
		var invite model.InviteCode
		var enabled int
		var createdAt string
		var expiresAt sql.NullString
		if err := inviteRows.Scan(&invite.ID, &invite.Code, &enabled, &createdAt, &expiresAt, &invite.UsedCount); err != nil {
			inviteRows.Close()
			return State{}, false, err
		}
		invite.Enabled = enabled != 0
		invite.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if expiresAt.Valid {
			value, parseErr := time.Parse(time.RFC3339Nano, expiresAt.String)
			if parseErr == nil {
				invite.ExpiresAt = &value
			}
		}
		state.Invites = append(state.Invites, invite)
	}
	inviteRows.Close()

	stateRows, err := s.db.Query(`
		SELECT user_id, piles_json, refresh_json, device_ids_json,
			cookie_nonce, cookie_ciphertext, stats_json,
			yyb_binding_nonce, yyb_binding_ciphertext, recovery_diagnostics_json
		FROM user_states
	`)
	if err != nil {
		return State{}, false, fmt.Errorf("query user states: %w", err)
	}
	defer stateRows.Close()

	for stateRows.Next() {
		var userID string
		var pilesJSON, refreshJSON, deviceIDsJSON, statsJSON, recoveryDiagnosticsJSON []byte
		var nonce, ciphertext []byte
		var yybBindingNonce, yybBindingCiphertext []byte
		if err := stateRows.Scan(
			&userID,
			&pilesJSON,
			&refreshJSON,
			&deviceIDsJSON,
			&nonce,
			&ciphertext,
			&statsJSON,
			&yybBindingNonce,
			&yybBindingCiphertext,
			&recoveryDiagnosticsJSON,
		); err != nil {
			return State{}, false, fmt.Errorf("scan user state: %w", err)
		}

		var userState UserState
		if err := json.Unmarshal(pilesJSON, &userState.Piles); err != nil {
			return State{}, false, fmt.Errorf("parse piles for user %s: %w", userID, err)
		}
		if err := json.Unmarshal(refreshJSON, &userState.Refresh); err != nil {
			return State{}, false, fmt.Errorf("parse refresh for user %s: %w", userID, err)
		}
		if err := json.Unmarshal(deviceIDsJSON, &userState.DeviceIDs); err != nil {
			return State{}, false, fmt.Errorf("parse device IDs for user %s: %w", userID, err)
		}
		if err := json.Unmarshal(statsJSON, &userState.Stats); err != nil {
			return State{}, false, fmt.Errorf("parse stats for user %s: %w", userID, err)
		}
		if len(recoveryDiagnosticsJSON) > 0 {
			if err := json.Unmarshal(recoveryDiagnosticsJSON, &userState.RecoveryDiagnostics); err != nil {
				return State{}, false, fmt.Errorf("parse recovery diagnostics for user %s: %w", userID, err)
			}
		}
		userState.Cookie, err = s.cipher.decrypt(userID, nonce, ciphertext)
		if err != nil {
			return State{}, false, err
		}
		if len(yybBindingCiphertext) > 0 {
			bindingJSON, err := s.cipher.decryptWithAAD(yybBindingAAD(userID), yybBindingNonce, yybBindingCiphertext)
			if err != nil {
				return State{}, false, err
			}
			var binding model.YYBBinding
			if err := json.Unmarshal(bindingJSON, &binding); err != nil {
				return State{}, false, fmt.Errorf("parse yyb binding for user %s: %w", userID, err)
			}
			userState.YYBBinding = &binding
		}
		state.UserStates[userID] = userState
	}
	if err := stateRows.Err(); err != nil {
		return State{}, false, fmt.Errorf("iterate user states: %w", err)
	}
	return state, true, nil
}

func (s *Store) metadata(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return value, err == nil, err
}

func yybBindingAAD(userID string) string {
	return "charge:user_state:yyb_binding:" + userID
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339Nano)
}

func (s *Store) Save(state State) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin state transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec(`DELETE FROM user_states`); err != nil {
		return fmt.Errorf("clear user states: %w", err)
	}
	for _, user := range state.Users {
		if _, err := tx.Exec(`
			INSERT INTO users(
				id, username, password_hash, role, enabled, created_at, updated_at,
				device_limit, refresh_enabled, must_change_password, usage_guide_ack_at
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				username=excluded.username, password_hash=excluded.password_hash,
				role=excluded.role, enabled=excluded.enabled, updated_at=excluded.updated_at,
				device_limit=excluded.device_limit, refresh_enabled=excluded.refresh_enabled,
				must_change_password=excluded.must_change_password,
				usage_guide_ack_at=excluded.usage_guide_ack_at
		`,
			user.ID,
			user.Username,
			user.PasswordHash,
			string(user.Role),
			user.Enabled,
			user.CreatedAt.Format(time.RFC3339Nano),
			user.UpdatedAt.Format(time.RFC3339Nano),
			user.DeviceLimit,
			user.RefreshEnabled,
			user.MustChangePassword,
			formatOptionalTime(user.UsageGuideAckAt),
		); err != nil {
			return fmt.Errorf("insert user %s: %w", user.ID, err)
		}

		userState := state.UserStates[user.ID]
		pilesJSON, err := json.Marshal(userState.Piles)
		if err != nil {
			return fmt.Errorf("encode piles for user %s: %w", user.ID, err)
		}
		refreshJSON, err := json.Marshal(userState.Refresh)
		if err != nil {
			return fmt.Errorf("encode refresh for user %s: %w", user.ID, err)
		}
		deviceIDsJSON, err := json.Marshal(userState.DeviceIDs)
		if err != nil {
			return fmt.Errorf("encode device IDs for user %s: %w", user.ID, err)
		}
		statsJSON, err := json.Marshal(userState.Stats)
		if err != nil {
			return fmt.Errorf("encode stats for user %s: %w", user.ID, err)
		}
		recoveryDiagnosticsJSON, err := json.Marshal(userState.RecoveryDiagnostics)
		if err != nil {
			return fmt.Errorf("encode recovery diagnostics for user %s: %w", user.ID, err)
		}
		nonce, ciphertext, err := s.cipher.encrypt(user.ID, userState.Cookie)
		if err != nil {
			return err
		}
		var yybBindingNonce, yybBindingCiphertext []byte
		if userState.YYBBinding != nil {
			bindingJSON, err := json.Marshal(userState.YYBBinding)
			if err != nil {
				return fmt.Errorf("encode yyb binding for user %s: %w", user.ID, err)
			}
			yybBindingNonce, yybBindingCiphertext, err = s.cipher.encryptWithAAD(yybBindingAAD(user.ID), bindingJSON)
			if err != nil {
				return fmt.Errorf("encrypt yyb binding for user %s: %w", user.ID, err)
			}
		}

		if _, err := tx.Exec(`
			INSERT INTO user_states(
				user_id, piles_json, refresh_json, device_ids_json,
				cookie_nonce, cookie_ciphertext, stats_json,
				yyb_binding_nonce, yyb_binding_ciphertext, recovery_diagnostics_json
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			user.ID,
			pilesJSON,
			refreshJSON,
			deviceIDsJSON,
			nonce,
			ciphertext,
			statsJSON,
			yybBindingNonce,
			yybBindingCiphertext,
			recoveryDiagnosticsJSON,
		); err != nil {
			return fmt.Errorf("insert state for user %s: %w", user.ID, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id NOT IN (SELECT user_id FROM user_states)`); err != nil {
		return fmt.Errorf("remove deleted users: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM invite_codes`); err != nil {
		return err
	}
	for _, invite := range state.Invites {
		var expiresAt any
		if invite.ExpiresAt != nil {
			expiresAt = invite.ExpiresAt.Format(time.RFC3339Nano)
		}
		if _, err := tx.Exec(`INSERT INTO invite_codes(id, code, enabled, created_at, expires_at, used_count) VALUES(?,?,?,?,?,?)`,
			invite.ID, invite.Code, invite.Enabled, invite.CreatedAt.Format(time.RFC3339Nano), expiresAt, invite.UsedCount); err != nil {
			return err
		}
	}
	settingsJSON, err := json.Marshal(state.Settings)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO metadata(key,value) VALUES('registration_settings',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(settingsJSON)); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO metadata(key, value) VALUES('state_version', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, fmt.Sprintf("%d", state.Version)); err != nil {
		return fmt.Errorf("write state version: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO metadata(key, value) VALUES('saved_at', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, time.Now().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("write saved timestamp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit state transaction: %w", err)
	}
	return nil
}

func (s *Store) RecordMetric(userID, kind string, at time.Time) error {
	return s.RecordMetricCount(userID, kind, 1, at)
}

func (s *Store) RecordMetricCount(userID, kind string, count int, at time.Time) error {
	if count <= 0 {
		return fmt.Errorf("metric count must be positive")
	}
	_, err := s.db.Exec(
		`INSERT INTO metrics(user_id, kind, created_at, count) VALUES(?,?,?,?)`,
		userID,
		kind,
		at.Unix(),
		count,
	)
	return err
}

// MetricKindCount returns the independently recorded count for one metric
// kind. Background reminder metrics deliberately use their own kinds so they
// can be inspected without entering interactive request and active-user totals.
func (s *Store) MetricKindCount(kind string, since time.Time) (int, error) {
	var count int
	if err := s.db.QueryRow(
		`SELECT COALESCE(SUM(count), 0) FROM metrics WHERE kind = ? AND created_at >= ?`,
		kind,
		since.Unix(),
	).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

const retentionPruneBatchSize = 500

func (s *Store) PruneMetrics(before time.Time) (int64, error) {
	return s.pruneRowsInBatches("metrics", "created_at", before.Unix())
}

func (s *Store) pruneRowsInBatches(table, timeColumn string, before int64) (int64, error) {
	return s.pruneRowsInBatchesWhere(table, timeColumn, before, "1=1")
}

func (s *Store) pruneRowsInBatchesWhere(table, timeColumn string, before int64, condition string) (int64, error) {
	var total int64
	query := fmt.Sprintf(
		`DELETE FROM %s WHERE id IN (
			SELECT id FROM %s WHERE %s AND %s < ? ORDER BY %s, id LIMIT ?
		)`,
		table,
		table,
		condition,
		timeColumn,
		timeColumn,
	)
	for {
		result, err := s.db.Exec(query, before, retentionPruneBatchSize)
		if err != nil {
			return total, fmt.Errorf("prune %s: %w", table, err)
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("count pruned %s rows: %w", table, err)
		}
		total += deleted
		if deleted < retentionPruneBatchSize {
			return total, nil
		}
	}
}

func (s *Store) MetricSeries(since time.Time, bucketSeconds int64) ([]model.MetricPoint, error) {
	rows, err := s.db.Query(`
		SELECT (created_at / ?) * ?, kind, SUM(count), COUNT(DISTINCT user_id)
		FROM metrics WHERE created_at >= ?
		GROUP BY 1, kind ORDER BY 1
	`, bucketSeconds, bucketSeconds, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := map[int64]*model.MetricPoint{}
	var order []int64
	for rows.Next() {
		var bucket int64
		var kind string
		var count, active int
		if err := rows.Scan(&bucket, &kind, &count, &active); err != nil {
			return nil, err
		}
		point := points[bucket]
		if point == nil {
			point = &model.MetricPoint{Time: time.Unix(bucket, 0)}
			points[bucket] = point
			order = append(order, bucket)
		}
		switch kind {
		case "request":
			point.Requests += count
			point.ActiveUsers += active
		case "remote":
			point.Remote += count
		case "cache":
			point.CacheHits += count
		case "remote_ok":
			point.RemoteOK += count
		case "remote_failed":
			point.RemoteFailed += count
		case "cookie_error":
			point.CookieErrors += count
		}
	}
	result := make([]model.MetricPoint, 0, len(order))
	for _, bucket := range order {
		result = append(result, *points[bucket])
	}
	return result, rows.Err()
}
