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

const schemaVersion = 1

type Store struct {
	db     *sql.DB
	cipher *cookieCipher
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
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize sqlite database: %w", err)
		}
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

func (s *Store) Load() (State, bool, error) {
	rows, err := s.db.Query(`
		SELECT id, username, password_hash, role, enabled, created_at, updated_at
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
		var enabled int
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.PasswordHash,
			&role,
			&enabled,
			&createdAt,
			&updatedAt,
		); err != nil {
			return State{}, false, fmt.Errorf("scan user: %w", err)
		}
		user.Role = model.UserRole(role)
		user.Enabled = enabled != 0
		user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return State{}, false, fmt.Errorf("parse user created_at: %w", err)
		}
		user.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return State{}, false, fmt.Errorf("parse user updated_at: %w", err)
		}
		state.Users = append(state.Users, user)
	}
	if err := rows.Err(); err != nil {
		return State{}, false, fmt.Errorf("iterate users: %w", err)
	}
	if len(state.Users) == 0 {
		return state, false, nil
	}

	stateRows, err := s.db.Query(`
		SELECT user_id, piles_json, refresh_json, device_ids_json,
		       cookie_nonce, cookie_ciphertext, stats_json
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
		if err := stateRows.Scan(
			&userID,
			&pilesJSON,
			&refreshJSON,
			&deviceIDsJSON,
			&nonce,
			&ciphertext,
			&statsJSON,
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
		state.UserStates[userID] = userState
	}
	if err := stateRows.Err(); err != nil {
		return State{}, false, fmt.Errorf("iterate user states: %w", err)
	}
	return state, true, nil
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
	if _, err := tx.Exec(`DELETE FROM users`); err != nil {
		return fmt.Errorf("clear users: %w", err)
	}

	for _, user := range state.Users {
		if _, err := tx.Exec(`
			INSERT INTO users(
				id, username, password_hash, role, enabled, created_at, updated_at
			) VALUES(?, ?, ?, ?, ?, ?, ?)
		`,
			user.ID,
			user.Username,
			user.PasswordHash,
			string(user.Role),
			user.Enabled,
			user.CreatedAt.Format(time.RFC3339Nano),
			user.UpdatedAt.Format(time.RFC3339Nano),
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

		if _, err := tx.Exec(`
			INSERT INTO user_states(
				user_id, piles_json, refresh_json, device_ids_json,
				cookie_nonce, cookie_ciphertext, stats_json
			) VALUES(?, ?, ?, ?, ?, ?, ?)
		`,
			user.ID,
			pilesJSON,
			refreshJSON,
			deviceIDsJSON,
			nonce,
			ciphertext,
			statsJSON,
		); err != nil {
			return fmt.Errorf("insert state for user %s: %w", user.ID, err)
		}
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
