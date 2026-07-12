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

const schemaVersion = 4

type Store struct {
	db     *sql.DB
	cipher *secretCipher
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
	store := &Store{db: db, cipher: cipher}
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
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS metrics_created_at_idx ON metrics(created_at)`,
		`CREATE INDEX IF NOT EXISTS metrics_user_time_idx ON metrics(user_id, created_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize sqlite database: %w", err)
		}
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
	if err := s.ensureColumn("user_states", "yyb_binding_nonce", "BLOB"); err != nil {
		return err
	}
	if err := s.ensureColumn("user_states", "yyb_binding_ciphertext", "BLOB"); err != nil {
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
	TokenHash []byte
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
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
		INSERT INTO sessions(token_hash, user_id, created_at, expires_at)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(token_hash) DO UPDATE SET
			user_id = excluded.user_id,
			created_at = excluded.created_at,
			expires_at = excluded.expires_at
	`,
		record.TokenHash,
		record.UserID,
		record.CreatedAt.UnixNano(),
		record.ExpiresAt.UnixNano(),
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
	var createdAt, expiresAt int64
	err := s.db.QueryRow(`
		SELECT token_hash, user_id, created_at, expires_at
		FROM sessions WHERE token_hash = ?
	`, tokenHash).Scan(&record.TokenHash, &record.UserID, &createdAt, &expiresAt)
	if err == sql.ErrNoRows {
		return SessionRecord{}, false, nil
	}
	if err != nil {
		return SessionRecord{}, false, fmt.Errorf("load session: %w", err)
	}
	record.CreatedAt = time.Unix(0, createdAt)
	record.ExpiresAt = time.Unix(0, expiresAt)
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
	rows, err := s.db.Query(`SELECT token_hash, user_id, created_at, expires_at FROM sessions WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SessionRecord
	for rows.Next() {
		var record SessionRecord
		var createdAt, expiresAt int64
		if err := rows.Scan(&record.TokenHash, &record.UserID, &createdAt, &expiresAt); err != nil {
			return nil, err
		}
		record.CreatedAt = time.Unix(0, createdAt)
		record.ExpiresAt = time.Unix(0, expiresAt)
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) DeleteOtherSessions(userID string, currentHash []byte) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id=? AND token_hash<>?`, userID, currentHash)
	return err
}

func (s *Store) Load() (State, bool, error) {
	rows, err := s.db.Query(`
		SELECT id, username, password_hash, role, enabled, created_at, updated_at,
		       device_limit, refresh_enabled, usage_guide_ack_at
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
		var enabled, refreshEnabled int
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
			&usageGuideAckAt,
		); err != nil {
			return State{}, false, fmt.Errorf("scan user: %w", err)
		}
		user.Role = model.UserRole(role)
		user.Enabled = enabled != 0
		user.RefreshEnabled = refreshEnabled != 0
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
		       yyb_binding_nonce, yyb_binding_ciphertext
		FROM user_states
	`)
	if err != nil {
		return State{}, false, fmt.Errorf("query user states: %w", err)
	}
	defer stateRows.Close()

	for stateRows.Next() {
		var userID string
		var pilesJSON, refreshJSON, deviceIDsJSON, statsJSON []byte
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
				device_limit, refresh_enabled, usage_guide_ack_at
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				username=excluded.username, password_hash=excluded.password_hash,
				role=excluded.role, enabled=excluded.enabled, updated_at=excluded.updated_at,
				device_limit=excluded.device_limit, refresh_enabled=excluded.refresh_enabled,
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
				yyb_binding_nonce, yyb_binding_ciphertext
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	_, err := s.db.Exec(`INSERT INTO metrics(user_id, kind, created_at) VALUES(?,?,?)`, userID, kind, at.Unix())
	return err
}

func (s *Store) PruneMetrics(before time.Time) error {
	_, err := s.db.Exec(`DELETE FROM metrics WHERE created_at < ?`, before.Unix())
	return err
}

func (s *Store) MetricSeries(since time.Time, bucketSeconds int64) ([]model.MetricPoint, error) {
	rows, err := s.db.Query(`
		SELECT (created_at / ?) * ?, kind, COUNT(*), COUNT(DISTINCT user_id)
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
