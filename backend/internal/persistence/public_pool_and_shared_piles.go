package persistence

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/charger"
	"charge-dashboard/internal/model"
)

var ErrPublicPoolCookieDuplicate = errors.New("public pool cookie already exists")

type PublicPoolCookieRecord struct {
	model.PublicPoolCookie
	Cookie string
}

type UserQuotaRecord struct {
	UserID               string
	DailyDate            string
	DailyUsed            int
	BonusRemaining       int
	ContributedPileCount int
	DailyProbeDate       string
	DailyProbeCount      int
	UpdatedAt            time.Time
}

type QuotaConsumeReceipt struct {
	FromDaily int
	FromBonus int
}

func (s *Store) ensurePublicPoolAndSharedPiles() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS public_cookie_pool (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			cookie_nonce BLOB NOT NULL,
			cookie_ciphertext BLOB NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
			status TEXT NOT NULL DEFAULT 'healthy' CHECK(status IN ('healthy', 'expired')),
			consecutive_failures INTEGER NOT NULL DEFAULT 0 CHECK(consecutive_failures >= 0),
			last_used_at INTEGER,
			last_error TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS shared_piles (
			device_id TEXT PRIMARY KEY,
			number TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			address TEXT NOT NULL DEFAULT '',
			open_num INTEGER NOT NULL DEFAULT 0,
			contributed_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			last_snapshot_json BLOB,
			last_fetched_at INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS shared_piles_number_idx ON shared_piles(number)`,
		`CREATE INDEX IF NOT EXISTS shared_piles_updated_idx ON shared_piles(updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS user_refresh_quotas (
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			daily_date TEXT NOT NULL DEFAULT '',
			daily_used INTEGER NOT NULL DEFAULT 0 CHECK(daily_used >= 0),
			bonus_remaining INTEGER NOT NULL DEFAULT 0 CHECK(bonus_remaining >= 0),
			contributed_pile_count INTEGER NOT NULL DEFAULT 0 CHECK(contributed_pile_count >= 0),
			daily_probe_date TEXT NOT NULL DEFAULT '',
			daily_probe_count INTEGER NOT NULL DEFAULT 0 CHECK(daily_probe_count >= 0),
			updated_at INTEGER NOT NULL
		)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("ensure public pool and shared piles tables: %w", err)
		}
	}
	if err := s.ensureColumn("public_cookie_pool", "owner_user_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("public_cookie_pool", "cookie_fingerprint", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.backfillPublicPoolFingerprints(); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS public_cookie_pool_fingerprint_uidx
		 ON public_cookie_pool(cookie_fingerprint)
		 WHERE cookie_fingerprint <> ''`,
	); err != nil {
		return fmt.Errorf("create public pool fingerprint unique index: %w", err)
	}
	if err := s.ensureColumn("shared_piles", "meta_locked", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return s.backfillSharedPilesOnce()
}

func (s *Store) cookieFingerprint(cookie string) string {
	identity := charger.CookieIdentity(cookie)
	if identity == "" {
		return ""
	}
	return s.cipher.fingerprint(identity)
}

func (s *Store) backfillPublicPoolFingerprints() error {
	rows, err := s.db.Query(
		`SELECT id, cookie_nonce, cookie_ciphertext, COALESCE(cookie_fingerprint, '')
		 FROM public_cookie_pool
		 ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return fmt.Errorf("query public pool for fingerprint backfill: %w", err)
	}
	type poolRow struct {
		id         string
		nonce      []byte
		ciphertext []byte
		fp         string
	}
	var all []poolRow
	for rows.Next() {
		var r poolRow
		if scanErr := rows.Scan(&r.id, &r.nonce, &r.ciphertext, &r.fp); scanErr != nil {
			_ = rows.Close()
			return scanErr
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	seen := make(map[string]bool, len(all))
	for _, r := range all {
		if r.fp != "" {
			seen[r.fp] = true
		}
	}
	for _, r := range all {
		if r.fp != "" {
			continue
		}
		plaintext, decErr := s.cipher.decryptWithAAD(publicPoolAAD(r.id), r.nonce, r.ciphertext)
		if decErr != nil {
			continue
		}
		fp := s.cookieFingerprint(string(plaintext))
		if fp == "" || seen[fp] {
			continue
		}
		seen[fp] = true
		if _, err := s.db.Exec(
			`UPDATE public_cookie_pool SET cookie_fingerprint = ? WHERE id = ?`,
			fp, r.id,
		); err != nil {
			return fmt.Errorf("backfill public pool fingerprint %s: %w", r.id, err)
		}
	}
	return nil
}

func (s *Store) HasPublicPoolCookieDuplicate(excludeID, cookie string) (bool, error) {
	fp := s.cookieFingerprint(cookie)
	if fp == "" {
		return false, nil
	}
	var exists int
	err := s.db.QueryRow(
		`SELECT 1 FROM public_cookie_pool WHERE cookie_fingerprint = ? AND id <> ? LIMIT 1`,
		fp, strings.TrimSpace(excludeID),
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func isPublicPoolFingerprintConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") && strings.Contains(msg, "cookie_fingerprint")
}

func (s *Store) backfillSharedPilesOnce() error {
	if _, ok, err := s.metadata("schema_v14_shared_piles_backfilled"); err != nil {
		return err
	} else if ok {
		return nil
	}
	rows, err := s.db.Query(`SELECT user_id, piles_json FROM user_states`)
	if err != nil {
		return fmt.Errorf("query user states for shared piles backfill: %w", err)
	}
	type userPilesRow struct {
		userID string
		raw    []byte
	}
	var allRows []userPilesRow
	for rows.Next() {
		var r userPilesRow
		if scanErr := rows.Scan(&r.userID, &r.raw); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("scan user state piles: %w", scanErr)
		}
		allRows = append(allRows, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	now := time.Now().UTC().Unix()
	for _, row := range allRows {
		if len(row.raw) == 0 {
			continue
		}
		var piles []model.Pile
		if unmarshalErr := json.Unmarshal(row.raw, &piles); unmarshalErr != nil {
			continue
		}
		for _, pile := range piles {
			deviceID := strings.TrimSpace(pile.ID)
			if deviceID == "" {
				continue
			}
			snapshotBytes, _ := json.Marshal(pile)
			var fetchedAt any
			if !pile.UpdatedAt.IsZero() {
				fetchedAt = pile.UpdatedAt.UTC().Unix()
			}
			_, _ = s.db.Exec(
				`INSERT OR IGNORE INTO shared_piles
				 (device_id, number, name, address, open_num, contributed_by_user_id, last_snapshot_json, last_fetched_at, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				deviceID,
				strings.TrimSpace(pile.Number),
				strings.TrimSpace(pile.Name),
				strings.TrimSpace(pile.Address),
				pile.OpenNum,
				row.userID,
				snapshotBytes,
				fetchedAt,
				now,
				now,
			)
		}
	}
	_, err = s.db.Exec(
		`INSERT INTO metadata(key, value) VALUES('schema_v14_shared_piles_backfilled', '1')
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
	)
	return err
}

func publicPoolAAD(id string) string {
	return "public_cookie_pool:" + id
}

func (s *Store) CreatePublicPoolCookie(entry PublicPoolCookieRecord) (model.PublicPoolCookie, error) {
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	if entry.Status == "" {
		entry.Status = "healthy"
	}
	fp := s.cookieFingerprint(entry.Cookie)
	if dup, dupErr := s.HasPublicPoolCookieDuplicate(entry.ID, entry.Cookie); dupErr != nil {
		return model.PublicPoolCookie{}, fmt.Errorf("check duplicate public pool cookie: %w", dupErr)
	} else if dup {
		return model.PublicPoolCookie{}, ErrPublicPoolCookieDuplicate
	}
	nonce, ciphertext, err := s.cipher.encryptWithAAD(publicPoolAAD(entry.ID), []byte(entry.Cookie))
	if err != nil {
		return model.PublicPoolCookie{}, fmt.Errorf("encrypt public pool cookie: %w", err)
	}
	enabledInt := 0
	if entry.Enabled {
		enabledInt = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO public_cookie_pool
		 (id, label, cookie_fingerprint, cookie_nonce, cookie_ciphertext, enabled, status, consecutive_failures, last_used_at, last_error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, NULL, '', ?, ?)`,
		entry.ID,
		strings.TrimSpace(entry.Label),
		fp,
		nonce,
		ciphertext,
		enabledInt,
		entry.Status,
		entry.CreatedAt.Unix(),
		entry.UpdatedAt.Unix(),
	)
	if err != nil {
		if isPublicPoolFingerprintConstraintErr(err) {
			return model.PublicPoolCookie{}, ErrPublicPoolCookieDuplicate
		}
		return model.PublicPoolCookie{}, fmt.Errorf("insert public pool cookie: %w", err)
	}
	return entry.PublicPoolCookie, nil
}

func (s *Store) ListPublicPoolCookies() ([]model.PublicPoolCookie, error) {
	rows, err := s.db.Query(
		`SELECT id, label, COALESCE(owner_user_id, ''), enabled, status, consecutive_failures, last_used_at, last_error, created_at, updated_at
		 FROM public_cookie_pool
		 ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list public pool cookies: %w", err)
	}
	defer rows.Close()
	result := make([]model.PublicPoolCookie, 0)
	for rows.Next() {
		var item model.PublicPoolCookie
		var enabledInt int
		var lastUsed sql.NullInt64
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&item.ID, &item.Label, &item.OwnerUserID, &enabledInt, &item.Status,
			&item.ConsecutiveFailures, &lastUsed, &item.LastError,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan public pool cookie: %w", err)
		}
		item.Enabled = enabledInt != 0
		if lastUsed.Valid {
			t := time.Unix(lastUsed.Int64, 0).UTC()
			item.LastUsedAt = &t
		}
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CountHealthyPublicPoolCookies() (int, error) {
	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM public_cookie_pool WHERE enabled = 1 AND status = 'healthy'`,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count healthy public pool cookies: %w", err)
	}
	return count, nil
}

func (s *Store) ListHealthyPublicPoolCookies() ([]PublicPoolCookieRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, label, COALESCE(owner_user_id, ''), cookie_nonce, cookie_ciphertext, enabled, status, consecutive_failures, last_used_at, last_error, created_at, updated_at
		 FROM public_cookie_pool
		 WHERE enabled = 1 AND status = 'healthy'
		 ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list healthy public pool cookies: %w", err)
	}
	defer rows.Close()
	result := make([]PublicPoolCookieRecord, 0)
	for rows.Next() {
		var rec PublicPoolCookieRecord
		var nonce, ciphertext []byte
		var enabledInt int
		var lastUsed sql.NullInt64
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&rec.ID, &rec.Label, &rec.OwnerUserID, &nonce, &ciphertext, &enabledInt, &rec.Status,
			&rec.ConsecutiveFailures, &lastUsed, &rec.LastError,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan healthy public pool cookie: %w", err)
		}
		plaintext, err := s.cipher.decryptWithAAD(publicPoolAAD(rec.ID), nonce, ciphertext)
		if err != nil {
			return nil, fmt.Errorf("decrypt public pool cookie %s: %w", rec.ID, err)
		}
		rec.Cookie = string(plaintext)
		rec.Enabled = enabledInt != 0
		if lastUsed.Valid {
			t := time.Unix(lastUsed.Int64, 0).UTC()
			rec.LastUsedAt = &t
		}
		rec.CreatedAt = time.Unix(createdAt, 0).UTC()
		rec.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		result = append(result, rec)
	}
	return result, rows.Err()
}

func (s *Store) UpdatePublicPoolCookie(id string, label *string, cookie *string, enabled *bool) (model.PublicPoolCookie, error) {
	var existing model.PublicPoolCookie
	var existingEnabled int
	var lastUsed sql.NullInt64
	var createdAt, updatedAt int64
	err := s.db.QueryRow(
		`SELECT id, label, COALESCE(owner_user_id, ''), enabled, status, consecutive_failures, last_used_at, last_error, created_at, updated_at
		 FROM public_cookie_pool WHERE id = ?`,
		id,
	).Scan(
		&existing.ID, &existing.Label, &existing.OwnerUserID, &existingEnabled, &existing.Status,
		&existing.ConsecutiveFailures, &lastUsed, &existing.LastError,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.PublicPoolCookie{}, sql.ErrNoRows
	}
	if err != nil {
		return model.PublicPoolCookie{}, fmt.Errorf("load public pool cookie %s: %w", id, err)
	}
	existing.Enabled = existingEnabled != 0
	if lastUsed.Valid {
		t := time.Unix(lastUsed.Int64, 0).UTC()
		existing.LastUsedAt = &t
	}
	existing.CreatedAt = time.Unix(createdAt, 0).UTC()
	now := time.Now().UTC()
	existing.UpdatedAt = now

	if label != nil {
		existing.Label = strings.TrimSpace(*label)
	}
	if enabled != nil {
		existing.Enabled = *enabled
	}
	enabledInt := 0
	if existing.Enabled {
		enabledInt = 1
	}

	if cookie != nil && strings.TrimSpace(*cookie) != "" {
		trimmedCookie := strings.TrimSpace(*cookie)
		fp := s.cookieFingerprint(trimmedCookie)
		if dup, dupErr := s.HasPublicPoolCookieDuplicate(id, trimmedCookie); dupErr != nil {
			return model.PublicPoolCookie{}, fmt.Errorf("check duplicate public pool cookie: %w", dupErr)
		} else if dup {
			return model.PublicPoolCookie{}, ErrPublicPoolCookieDuplicate
		}
		nonce, ciphertext, encErr := s.cipher.encryptWithAAD(publicPoolAAD(id), []byte(trimmedCookie))
		if encErr != nil {
			return model.PublicPoolCookie{}, fmt.Errorf("encrypt updated public pool cookie: %w", encErr)
		}
		existing.Status = "healthy"
		existing.ConsecutiveFailures = 0
		existing.LastError = ""
		_, err = s.db.Exec(
			`UPDATE public_cookie_pool
			 SET label = ?, cookie_fingerprint = ?, cookie_nonce = ?, cookie_ciphertext = ?, enabled = ?, status = 'healthy',
			     consecutive_failures = 0, last_error = '', updated_at = ?
			 WHERE id = ?`,
			existing.Label, fp, nonce, ciphertext, enabledInt, now.Unix(), id,
		)
	} else {
		_, err = s.db.Exec(
			`UPDATE public_cookie_pool SET label = ?, enabled = ?, updated_at = ? WHERE id = ?`,
			existing.Label, enabledInt, now.Unix(), id,
		)
	}
	if err != nil {
		if isPublicPoolFingerprintConstraintErr(err) {
			return model.PublicPoolCookie{}, ErrPublicPoolCookieDuplicate
		}
		return model.PublicPoolCookie{}, fmt.Errorf("update public pool cookie %s: %w", id, err)
	}
	return existing, nil
}

func (s *Store) MarkPublicPoolCookieResult(id string, expired bool, errMsg string) error {
	now := time.Now().UTC().Unix()
	if expired {
		_, err := s.db.Exec(
			`UPDATE public_cookie_pool
			 SET status = 'expired', consecutive_failures = consecutive_failures + 1,
			     last_used_at = ?, last_error = ?, updated_at = ?
			 WHERE id = ?`,
			now, errMsg, now, id,
		)
		return err
	}
	if errMsg != "" {
		_, err := s.db.Exec(
			`UPDATE public_cookie_pool
			 SET consecutive_failures = consecutive_failures + 1,
			     last_used_at = ?, last_error = ?, updated_at = ?
			 WHERE id = ?`,
			now, errMsg, now, id,
		)
		return err
	}
	_, err := s.db.Exec(
		`UPDATE public_cookie_pool
		 SET status = 'healthy', consecutive_failures = 0,
		     last_used_at = ?, last_error = '', updated_at = ?
		 WHERE id = ?`,
		now, now, id,
	)
	return err
}

func (s *Store) GetPublicPoolCookie(id string) (*PublicPoolCookieRecord, error) {
	var rec PublicPoolCookieRecord
	var nonce, ciphertext []byte
	var enabledInt int
	var lastUsed sql.NullInt64
	var createdAt, updatedAt int64
	err := s.db.QueryRow(
		`SELECT id, label, COALESCE(owner_user_id, ''), cookie_nonce, cookie_ciphertext, enabled, status, consecutive_failures, last_used_at, last_error, created_at, updated_at
		 FROM public_cookie_pool WHERE id = ?`,
		id,
	).Scan(
		&rec.ID, &rec.Label, &rec.OwnerUserID, &nonce, &ciphertext, &enabledInt, &rec.Status,
		&rec.ConsecutiveFailures, &lastUsed, &rec.LastError,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("load public pool cookie %s: %w", id, err)
	}
	plaintext, err := s.cipher.decryptWithAAD(publicPoolAAD(rec.ID), nonce, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt public pool cookie %s: %w", rec.ID, err)
	}
	rec.Cookie = string(plaintext)
	rec.Enabled = enabledInt != 0
	if lastUsed.Valid {
		t := time.Unix(lastUsed.Int64, 0).UTC()
		rec.LastUsedAt = &t
	}
	rec.CreatedAt = time.Unix(createdAt, 0).UTC()
	rec.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return &rec, nil
}

func (s *Store) UpsertUserPublicPoolCookie(userID, label, cookie string) (model.PublicPoolCookie, error) {
	trimmedUserID := strings.TrimSpace(userID)
	trimmedCookie := strings.TrimSpace(cookie)
	id := "user_" + trimmedUserID
	fp := s.cookieFingerprint(trimmedCookie)
	if dup, dupErr := s.HasPublicPoolCookieDuplicate(id, trimmedCookie); dupErr != nil {
		return model.PublicPoolCookie{}, fmt.Errorf("check duplicate user pool cookie: %w", dupErr)
	} else if dup {
		return model.PublicPoolCookie{}, ErrPublicPoolCookieDuplicate
	}
	now := time.Now().UTC()
	nonce, ciphertext, err := s.cipher.encryptWithAAD(publicPoolAAD(id), []byte(trimmedCookie))
	if err != nil {
		return model.PublicPoolCookie{}, fmt.Errorf("encrypt user pool cookie: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO public_cookie_pool
		 (id, label, owner_user_id, cookie_fingerprint, cookie_nonce, cookie_ciphertext, enabled, status, consecutive_failures, last_used_at, last_error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 1, 'healthy', 0, NULL, '', ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		     label = excluded.label,
		     owner_user_id = excluded.owner_user_id,
		     cookie_fingerprint = excluded.cookie_fingerprint,
		     cookie_nonce = excluded.cookie_nonce,
		     cookie_ciphertext = excluded.cookie_ciphertext,
		     enabled = 1,
		     status = 'healthy',
		     consecutive_failures = 0,
		     last_error = '',
		     updated_at = excluded.updated_at`,
		id, strings.TrimSpace(label), trimmedUserID, fp, nonce, ciphertext, now.Unix(), now.Unix(),
	)
	if err != nil {
		if isPublicPoolFingerprintConstraintErr(err) {
			return model.PublicPoolCookie{}, ErrPublicPoolCookieDuplicate
		}
		return model.PublicPoolCookie{}, fmt.Errorf("upsert user pool cookie: %w", err)
	}
	rec, err := s.GetPublicPoolCookie(id)
	if err != nil {
		return model.PublicPoolCookie{}, err
	}
	return rec.PublicPoolCookie, nil
}

func (s *Store) GetUserPublicPoolCookie(userID string) (*model.PublicPoolCookie, error) {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil, nil
	}
	id := "user_" + trimmed
	var item model.PublicPoolCookie
	var enabledInt int
	var lastUsed sql.NullInt64
	var createdAt, updatedAt int64
	err := s.db.QueryRow(
		`SELECT id, label, COALESCE(owner_user_id, ''), enabled, status, consecutive_failures, last_used_at, last_error, created_at, updated_at
		 FROM public_cookie_pool WHERE id = ?`,
		id,
	).Scan(
		&item.ID, &item.Label, &item.OwnerUserID, &enabledInt, &item.Status,
		&item.ConsecutiveFailures, &lastUsed, &item.LastError,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load user public pool cookie %s: %w", id, err)
	}
	item.Enabled = enabledInt != 0
	if lastUsed.Valid {
		t := time.Unix(lastUsed.Int64, 0).UTC()
		item.LastUsedAt = &t
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return &item, nil
}

func (s *Store) DeleteUserPublicPoolCookie(userID string) error {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM public_cookie_pool WHERE id = ? OR owner_user_id = ?`, "user_"+trimmed, trimmed)
	return err
}

func (s *Store) MarkUserPublicPoolCookieExpired(userID, errMsg string) error {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil
	}
	now := time.Now().UTC().Unix()
	_, err := s.db.Exec(
		`UPDATE public_cookie_pool
		 SET status = 'expired',
		     consecutive_failures = consecutive_failures + 1,
		     last_error = ?,
		     updated_at = ?
		 WHERE id = ? OR owner_user_id = ?`,
		errMsg, now, "user_"+trimmed, trimmed,
	)
	return err
}

func (s *Store) DeletePublicPoolCookie(id string) error {
	res, err := s.db.Exec(`DELETE FROM public_cookie_pool WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete public pool cookie: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpsertSharedPile inserts or updates a shared pile record.
// inserted is true ONLY when this device_id did not previously exist in shared_piles.
func (s *Store) UpsertSharedPile(pile model.Pile, contributorUserID string, fetchedAt time.Time) (inserted bool, err error) {
	deviceID := strings.TrimSpace(pile.ID)
	if deviceID == "" {
		return false, fmt.Errorf("device id is required")
	}
	now := time.Now().UTC()
	if fetchedAt.IsZero() {
		fetchedAt = now
	}
	snapshotBytes, err := json.Marshal(pile)
	if err != nil {
		return false, fmt.Errorf("marshal shared pile snapshot: %w", err)
	}
	var contributor any
	if strings.TrimSpace(contributorUserID) != "" {
		contributor = strings.TrimSpace(contributorUserID)
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO shared_piles
		 (device_id, number, name, address, open_num, contributed_by_user_id, last_snapshot_json, last_fetched_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		deviceID,
		strings.TrimSpace(pile.Number),
		strings.TrimSpace(pile.Name),
		strings.TrimSpace(pile.Address),
		pile.OpenNum,
		contributor,
		snapshotBytes,
		fetchedAt.UTC().Unix(),
		now.Unix(),
		now.Unix(),
	)
	if err != nil {
		return false, fmt.Errorf("insert shared pile: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows > 0 {
		return true, nil
	}
	var metaLocked int
	var existingName, existingAddress string
	if scanErr := s.db.QueryRow(
		`SELECT COALESCE(meta_locked, 0), name, address FROM shared_piles WHERE device_id = ?`,
		deviceID,
	).Scan(&metaLocked, &existingName, &existingAddress); scanErr == nil && metaLocked != 0 {
		pile.Name = existingName
		pile.Address = existingAddress
		if updatedBytes, mErr := json.Marshal(pile); mErr == nil {
			snapshotBytes = updatedBytes
		}
	}
	_, err = s.db.Exec(
		`UPDATE shared_piles
		 SET number = CASE WHEN ? <> '' THEN ? ELSE number END,
		     name = CASE WHEN COALESCE(meta_locked, 0) = 1 THEN name WHEN ? <> '' THEN ? ELSE name END,
		     address = CASE WHEN COALESCE(meta_locked, 0) = 1 THEN address WHEN ? <> '' THEN ? ELSE address END,
		     open_num = CASE WHEN ? > 0 THEN ? ELSE open_num END,
		     last_snapshot_json = ?,
		     last_fetched_at = ?,
		     updated_at = ?
		 WHERE device_id = ?`,
		strings.TrimSpace(pile.Number), strings.TrimSpace(pile.Number),
		strings.TrimSpace(pile.Name), strings.TrimSpace(pile.Name),
		strings.TrimSpace(pile.Address), strings.TrimSpace(pile.Address),
		pile.OpenNum, pile.OpenNum,
		snapshotBytes,
		fetchedAt.UTC().Unix(),
		now.Unix(),
		deviceID,
	)
	if err != nil {
		return false, fmt.Errorf("update shared pile: %w", err)
	}
	return false, nil
}

func (s *Store) GetSharedPile(deviceID string) (*model.SharedPile, error) {
	return s.scanSingleSharedPile(
		`SELECT device_id, number, name, address, open_num, COALESCE(contributed_by_user_id, ''), last_snapshot_json, last_fetched_at, created_at, updated_at
		 FROM shared_piles WHERE device_id = ?`,
		strings.TrimSpace(deviceID),
	)
}

func (s *Store) FindSharedPileByNumber(number string) (*model.SharedPile, error) {
	return s.scanSingleSharedPile(
		`SELECT device_id, number, name, address, open_num, COALESCE(contributed_by_user_id, ''), last_snapshot_json, last_fetched_at, created_at, updated_at
		 FROM shared_piles WHERE number = ? LIMIT 1`,
		strings.TrimSpace(number),
	)
}

func (s *Store) scanSingleSharedPile(query string, arg any) (*model.SharedPile, error) {
	var item model.SharedPile
	var snapshotRaw []byte
	var lastFetched sql.NullInt64
	var createdAt, updatedAt int64
	err := s.db.QueryRow(query, arg).Scan(
		&item.DeviceID, &item.Number, &item.Name, &item.Address, &item.OpenNum,
		&item.ContributedByUserID, &snapshotRaw, &lastFetched, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastFetched.Valid {
		t := time.Unix(lastFetched.Int64, 0).UTC()
		item.LastFetchedAt = &t
	}
	if len(snapshotRaw) > 0 {
		var p model.Pile
		if json.Unmarshal(snapshotRaw, &p) == nil {
			item.LastSnapshot = &p
		}
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return &item, nil
}

func (s *Store) SearchSharedPiles(query string, limit int) ([]model.SharedPile, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	q := strings.TrimSpace(query)
	var rows *sql.Rows
	var err error
	if q == "" {
		rows, err = s.db.Query(
			`SELECT device_id, number, name, address, open_num, COALESCE(contributed_by_user_id, ''), last_snapshot_json, last_fetched_at, created_at, updated_at
			 FROM shared_piles
			 ORDER BY updated_at DESC, device_id ASC
			 LIMIT ?`,
			limit,
		)
	} else {
		like := "%" + q + "%"
		rows, err = s.db.Query(
			`SELECT device_id, number, name, address, open_num, COALESCE(contributed_by_user_id, ''), last_snapshot_json, last_fetched_at, created_at, updated_at
			 FROM shared_piles
			 WHERE device_id LIKE ? OR number LIKE ? OR name LIKE ? OR address LIKE ?
			 ORDER BY CASE WHEN number = ? OR device_id = ? THEN 0 ELSE 1 END, updated_at DESC
			 LIMIT ?`,
			like, like, like, like, q, q, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("search shared piles: %w", err)
	}
	defer rows.Close()
	result := make([]model.SharedPile, 0)
	for rows.Next() {
		var item model.SharedPile
		var snapshotRaw []byte
		var lastFetched sql.NullInt64
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&item.DeviceID, &item.Number, &item.Name, &item.Address, &item.OpenNum,
			&item.ContributedByUserID, &snapshotRaw, &lastFetched, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan shared pile: %w", err)
		}
		if lastFetched.Valid {
			t := time.Unix(lastFetched.Int64, 0).UTC()
			item.LastFetchedAt = &t
		}
		if len(snapshotRaw) > 0 {
			var p model.Pile
			if json.Unmarshal(snapshotRaw, &p) == nil {
				item.LastSnapshot = &p
			}
		}
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetUserQuota(userID string, today string) (UserQuotaRecord, error) {
	var rec UserQuotaRecord
	var updatedAt int64
	err := s.db.QueryRow(
		`SELECT user_id, daily_date, daily_used, bonus_remaining, contributed_pile_count, daily_probe_date, daily_probe_count, updated_at
		 FROM user_refresh_quotas WHERE user_id = ?`,
		userID,
	).Scan(
		&rec.UserID, &rec.DailyDate, &rec.DailyUsed, &rec.BonusRemaining,
		&rec.ContributedPileCount, &rec.DailyProbeDate, &rec.DailyProbeCount, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UserQuotaRecord{UserID: userID, DailyDate: today, DailyProbeDate: today}, nil
	}
	if err != nil {
		return UserQuotaRecord{}, fmt.Errorf("get user quota: %w", err)
	}
	if rec.DailyDate != today {
		rec.DailyDate = today
		rec.DailyUsed = 0
	}
	if rec.DailyProbeDate != today {
		rec.DailyProbeDate = today
		rec.DailyProbeCount = 0
	}
	rec.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return rec, nil
}

// ConsumeRefreshQuota atomically deducts `amount` refresh credits (daily free first, then permanent bonus).
func (s *Store) ConsumeRefreshQuota(userID string, today string, dailyLimit int, amount int) (QuotaConsumeReceipt, bool, error) {
	if amount <= 0 {
		return QuotaConsumeReceipt{}, true, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return QuotaConsumeReceipt{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Unix()
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO user_refresh_quotas
		 (user_id, daily_date, daily_used, bonus_remaining, contributed_pile_count, daily_probe_date, daily_probe_count, updated_at)
		 VALUES (?, ?, 0, 0, 0, ?, 0, ?)`,
		userID, today, today, now,
	); err != nil {
		return QuotaConsumeReceipt{}, false, err
	}

	var dailyDate string
	var dailyUsed, bonusRemaining int
	if err := tx.QueryRow(
		`SELECT daily_date, daily_used, bonus_remaining FROM user_refresh_quotas WHERE user_id = ?`,
		userID,
	).Scan(&dailyDate, &dailyUsed, &bonusRemaining); err != nil {
		return QuotaConsumeReceipt{}, false, err
	}
	if dailyDate != today {
		dailyUsed = 0
	}
	dailyRem := dailyLimit - dailyUsed
	if dailyRem < 0 {
		dailyRem = 0
	}
	if dailyRem+bonusRemaining < amount {
		return QuotaConsumeReceipt{}, false, nil
	}
	fromDaily := amount
	if fromDaily > dailyRem {
		fromDaily = dailyRem
	}
	fromBonus := amount - fromDaily

	if _, err := tx.Exec(
		`UPDATE user_refresh_quotas
		 SET daily_date = ?, daily_used = ?, bonus_remaining = ?, updated_at = ?
		 WHERE user_id = ?`,
		today, dailyUsed+fromDaily, bonusRemaining-fromBonus, now, userID,
	); err != nil {
		return QuotaConsumeReceipt{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return QuotaConsumeReceipt{}, false, err
	}
	return QuotaConsumeReceipt{FromDaily: fromDaily, FromBonus: fromBonus}, true, nil
}

func (s *Store) RefundRefreshQuota(userID string, today string, receipt QuotaConsumeReceipt) error {
	if receipt.FromDaily <= 0 && receipt.FromBonus <= 0 {
		return nil
	}
	now := time.Now().UTC().Unix()
	_, err := s.db.Exec(
		`UPDATE user_refresh_quotas
		 SET daily_used = CASE WHEN daily_date = ? THEN MAX(0, daily_used - ?) ELSE daily_used END,
		     bonus_remaining = bonus_remaining + ?,
		     updated_at = ?
		 WHERE user_id = ?`,
		today, receipt.FromDaily, receipt.FromBonus, now, userID,
	)
	return err
}

func (s *Store) GrantContributionBonus(userID string, today string, bonus int) error {
	now := time.Now().UTC().Unix()
	if bonus < 0 {
		bonus = 0
	}
	_, err := s.db.Exec(
		`INSERT INTO user_refresh_quotas
		 (user_id, daily_date, daily_used, bonus_remaining, contributed_pile_count, daily_probe_date, daily_probe_count, updated_at)
		 VALUES (?, ?, 0, ?, 1, ?, 0, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		     bonus_remaining = bonus_remaining + excluded.bonus_remaining,
		     contributed_pile_count = contributed_pile_count + 1,
		     updated_at = excluded.updated_at`,
		userID, today, bonus, today, now,
	)
	return err
}

func (s *Store) SetUserBonusQuota(userID string, today string, bonus int) error {
	if bonus < 0 {
		bonus = 0
	}
	now := time.Now().UTC().Unix()
	_, err := s.db.Exec(
		`INSERT INTO user_refresh_quotas
		 (user_id, daily_date, daily_used, bonus_remaining, contributed_pile_count, daily_probe_date, daily_probe_count, updated_at)
		 VALUES (?, ?, 0, ?, 0, ?, 0, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		     bonus_remaining = excluded.bonus_remaining,
		     updated_at = excluded.updated_at`,
		userID, today, bonus, today, now,
	)
	return err
}

func (s *Store) UpdateSharedPileMeta(deviceID, name, address string) (*model.SharedPile, error) {
	deviceID = strings.TrimSpace(deviceID)
	existing, err := s.GetSharedPile(deviceID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, sql.ErrNoRows
	}
	now := time.Now().UTC()
	nextName := strings.TrimSpace(name)
	if nextName == "" {
		nextName = existing.Name
	}
	nextAddress := strings.TrimSpace(address)
	var snapshotBytes []byte
	if existing.LastSnapshot != nil {
		snap := *existing.LastSnapshot
		snap.Name = nextName
		snap.Address = nextAddress
		snapshotBytes, _ = json.Marshal(snap)
	}
	if len(snapshotBytes) > 0 {
		_, err = s.db.Exec(
			`UPDATE shared_piles SET name = ?, address = ?, meta_locked = 1, last_snapshot_json = ?, updated_at = ? WHERE device_id = ?`,
			nextName, nextAddress, snapshotBytes, now.Unix(), deviceID,
		)
	} else {
		_, err = s.db.Exec(
			`UPDATE shared_piles SET name = ?, address = ?, meta_locked = 1, updated_at = ? WHERE device_id = ?`,
			nextName, nextAddress, now.Unix(), deviceID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("update shared pile meta: %w", err)
	}
	return s.GetSharedPile(deviceID)
}

func (s *Store) DeleteSharedPile(deviceID string) error {
	res, err := s.db.Exec(`DELETE FROM shared_piles WHERE device_id = ?`, strings.TrimSpace(deviceID))
	if err != nil {
		return fmt.Errorf("delete shared pile: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ConsumeDailyProbeQuota(userID string, today string, maxProbes int) (bool, error) {
	if maxProbes <= 0 {
		return true, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Unix()
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO user_refresh_quotas
		 (user_id, daily_date, daily_used, bonus_remaining, contributed_pile_count, daily_probe_date, daily_probe_count, updated_at)
		 VALUES (?, ?, 0, 0, 0, ?, 0, ?)`,
		userID, today, today, now,
	); err != nil {
		return false, err
	}
	var probeDate string
	var probeCount int
	if err := tx.QueryRow(
		`SELECT daily_probe_date, daily_probe_count FROM user_refresh_quotas WHERE user_id = ?`,
		userID,
	).Scan(&probeDate, &probeCount); err != nil {
		return false, err
	}
	if probeDate != today {
		probeCount = 0
	}
	if probeCount >= maxProbes {
		return false, nil
	}
	if _, err := tx.Exec(
		`UPDATE user_refresh_quotas SET daily_probe_date = ?, daily_probe_count = ?, updated_at = ? WHERE user_id = ?`,
		today, probeCount+1, now, userID,
	); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
