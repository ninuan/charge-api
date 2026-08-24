package persistence

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

const wxPusherChannel = "wxpusher"

var (
	ErrWxPusherBindingExists      = errors.New("wxpusher binding already exists")
	ErrWxPusherBindingNotEnabled  = errors.New("wxpusher binding is not enabled")
	ErrWxPusherUIDInUse           = errors.New("wxpusher uid already in use")
	ErrWxPusherBindSessionInvalid = errors.New("wxpusher bind session is inactive")
)

func (s *Store) SaveWxPusherBinding(binding model.WxPusherBinding) error {
	if err := validateWxPusherBinding(binding); err != nil {
		return err
	}
	uid := strings.TrimSpace(binding.UID)
	nonce, ciphertext, err := s.cipher.encryptWithAAD(wxPusherBindingAAD(binding.UserID), []byte(uid))
	if err != nil {
		return fmt.Errorf("encrypt wxpusher binding: %w", err)
	}
	fingerprint := sha256.Sum256([]byte(uid))
	result, err := s.db.Exec(`
		INSERT INTO wxpusher_bindings(
			user_id, uid_fingerprint, uid_nonce, uid_ciphertext, enabled, event_types,
			bound_at, updated_at, last_test_at, last_accepted_at,
			last_provider_success_at, last_error_code, last_error_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			uid_fingerprint = excluded.uid_fingerprint,
			uid_nonce = excluded.uid_nonce,
			uid_ciphertext = excluded.uid_ciphertext,
			enabled = excluded.enabled,
			event_types = excluded.event_types,
			bound_at = excluded.bound_at,
			updated_at = excluded.updated_at,
			last_test_at = excluded.last_test_at,
			last_accepted_at = excluded.last_accepted_at,
			last_provider_success_at = excluded.last_provider_success_at,
			last_error_code = excluded.last_error_code,
			last_error_at = excluded.last_error_at
	`,
		binding.UserID,
		fingerprint[:],
		nonce,
		ciphertext,
		binding.Enabled,
		binding.EventTypes,
		binding.BoundAt.UTC().Unix(),
		binding.UpdatedAt.UTC().Unix(),
		unixTimeOrNil(binding.LastTestAt),
		unixTimeOrNil(binding.LastAcceptedAt),
		unixTimeOrNil(binding.LastProviderSuccessAt),
		strings.TrimSpace(binding.LastErrorCode),
		unixTimeOrNil(binding.LastErrorAt),
	)
	if err != nil {
		return fmt.Errorf("save wxpusher binding: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read wxpusher binding result: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("wxpusher binding was not saved")
	}
	return nil
}

func (s *Store) LoadWxPusherBinding(userID string) (model.WxPusherBinding, bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return model.WxPusherBinding{}, false, fmt.Errorf("wxpusher binding requires a user")
	}
	var binding model.WxPusherBinding
	var nonce, ciphertext []byte
	var enabled int
	var boundAt, updatedAt int64
	var lastTestAt, lastAcceptedAt, lastProviderSuccessAt, lastErrorAt sql.NullInt64
	err := s.db.QueryRow(`
		SELECT user_id, uid_nonce, uid_ciphertext, enabled, event_types,
		       bound_at, updated_at, last_test_at, last_accepted_at,
		       last_provider_success_at, last_error_code, last_error_at
		FROM wxpusher_bindings WHERE user_id = ?
	`, userID).Scan(
		&binding.UserID,
		&nonce,
		&ciphertext,
		&enabled,
		&binding.EventTypes,
		&boundAt,
		&updatedAt,
		&lastTestAt,
		&lastAcceptedAt,
		&lastProviderSuccessAt,
		&binding.LastErrorCode,
		&lastErrorAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.WxPusherBinding{}, false, nil
	}
	if err != nil {
		return model.WxPusherBinding{}, false, fmt.Errorf("load wxpusher binding: %w", err)
	}
	uid, err := s.cipher.decryptWithAAD(wxPusherBindingAAD(userID), nonce, ciphertext)
	if err != nil {
		return model.WxPusherBinding{}, false, fmt.Errorf("decrypt wxpusher binding: %w", err)
	}
	binding.UID = string(uid)
	binding.Enabled = enabled != 0
	binding.BoundAt = time.Unix(boundAt, 0).UTC()
	binding.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	binding.LastTestAt = nullableUnixTime(lastTestAt)
	binding.LastAcceptedAt = nullableUnixTime(lastAcceptedAt)
	binding.LastProviderSuccessAt = nullableUnixTime(lastProviderSuccessAt)
	binding.LastErrorAt = nullableUnixTime(lastErrorAt)
	return binding, true, nil
}

func (s *Store) RecordWxPusherDeliveryAccepted(userID string, at time.Time) error {
	_, err := s.db.Exec(`
		UPDATE wxpusher_bindings
		SET last_accepted_at=?, last_error_code='', last_error_at=NULL, updated_at=?
		WHERE user_id=?
	`, at.UTC().Unix(), at.UTC().Unix(), strings.TrimSpace(userID))
	if err != nil {
		return fmt.Errorf("record wxpusher accepted delivery: %w", err)
	}
	return nil
}

func (s *Store) RecordWxPusherProviderSuccess(userID string, at time.Time) error {
	_, err := s.db.Exec(`
		UPDATE wxpusher_bindings
		SET last_provider_success_at=?, last_error_code='', last_error_at=NULL, updated_at=?
		WHERE user_id=?
	`, at.UTC().Unix(), at.UTC().Unix(), strings.TrimSpace(userID))
	if err != nil {
		return fmt.Errorf("record wxpusher provider success: %w", err)
	}
	return nil
}

func (s *Store) RecordWxPusherDeliveryError(userID, code string, at time.Time) error {
	_, err := s.db.Exec(`
		UPDATE wxpusher_bindings
		SET last_error_code=?, last_error_at=?, updated_at=?
		WHERE user_id=?
	`, strings.TrimSpace(code), at.UTC().Unix(), at.UTC().Unix(), strings.TrimSpace(userID))
	if err != nil {
		return fmt.Errorf("record wxpusher delivery error: %w", err)
	}
	return nil
}

func (s *Store) CreateWxPusherTestDelivery(delivery model.NotificationDelivery) error {
	delivery = normalizeNotificationDelivery(delivery)
	if !delivery.IsTest || delivery.NotificationID != nil || delivery.Status != model.NotificationDeliveryPending {
		return fmt.Errorf("wxpusher test delivery is invalid")
	}
	if err := validateNotificationDelivery(delivery); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin wxpusher test delivery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		INSERT INTO notification_deliveries(
			id, notification_id, user_id, channel, status, is_test, attempt_count,
			next_attempt_at, claimed_at, provider_record_id,
			provider_message_content_id, last_error_code, last_error_message,
			accepted_at, provider_succeeded_at, created_at, updated_at
		) VALUES(?, NULL, ?, ?, ?, 1, 0, ?, NULL, '', '', '', '', NULL, NULL, ?, ?)
	`, delivery.ID, delivery.UserID, delivery.Channel, delivery.Status,
		unixTimeOrNil(delivery.NextAttemptAt), delivery.CreatedAt.UTC().Unix(), delivery.UpdatedAt.UTC().Unix()); err != nil {
		return fmt.Errorf("insert wxpusher test delivery: %w", err)
	}
	result, err := tx.Exec(`
		UPDATE wxpusher_bindings SET last_test_at=?, updated_at=?
		WHERE user_id=? AND enabled=1
	`, delivery.CreatedAt.UTC().Unix(), delivery.UpdatedAt.UTC().Unix(), delivery.UserID)
	if err != nil {
		return fmt.Errorf("record wxpusher test delivery: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read wxpusher test delivery binding: %w", err)
	}
	if rows != 1 {
		return ErrWxPusherBindingNotEnabled
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit wxpusher test delivery: %w", err)
	}
	return nil
}

func (s *Store) DeleteWxPusherBinding(userID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, fmt.Errorf("delete wxpusher binding requires a user")
	}
	result, err := s.db.Exec(`DELETE FROM wxpusher_bindings WHERE user_id = ?`, userID)
	if err != nil {
		return false, fmt.Errorf("delete wxpusher binding: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (s *Store) SaveWxPusherBindSession(session model.WxPusherBindSession) error {
	if err := validateWxPusherBindSession(session); err != nil {
		return err
	}
	nonce, ciphertext, err := s.cipher.encryptWithAAD(
		wxPusherBindSessionAAD(session.UserID, session.ID),
		[]byte(strings.TrimSpace(session.ProviderCode)),
	)
	if err != nil {
		return fmt.Errorf("encrypt wxpusher bind session: %w", err)
	}
	result, err := s.db.Exec(`
		INSERT INTO wxpusher_bind_sessions(
			id, user_id, provider_code_nonce, provider_code_ciphertext, qr_url,
			expires_at, next_poll_at, created_at, completed_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider_code_nonce = excluded.provider_code_nonce,
			provider_code_ciphertext = excluded.provider_code_ciphertext,
			qr_url = excluded.qr_url,
			expires_at = excluded.expires_at,
			next_poll_at = excluded.next_poll_at,
			completed_at = excluded.completed_at
		WHERE wxpusher_bind_sessions.user_id = excluded.user_id
	`,
		session.ID,
		session.UserID,
		nonce,
		ciphertext,
		session.QRURL,
		session.ExpiresAt.UTC().Unix(),
		session.NextPollAt.UTC().Unix(),
		session.CreatedAt.UTC().Unix(),
		unixTimeOrNil(session.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("save wxpusher bind session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read wxpusher bind session result: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("wxpusher bind session belongs to another user")
	}
	return nil
}

func (s *Store) LoadWxPusherBindSession(userID, sessionID string) (model.WxPusherBindSession, bool, error) {
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	if userID == "" || sessionID == "" {
		return model.WxPusherBindSession{}, false, fmt.Errorf("wxpusher bind session requires user and id")
	}
	var session model.WxPusherBindSession
	var nonce, ciphertext []byte
	var expiresAt, nextPollAt, createdAt int64
	var completedAt sql.NullInt64
	err := s.db.QueryRow(`
		SELECT id, user_id, provider_code_nonce, provider_code_ciphertext,
		       qr_url, expires_at, next_poll_at, created_at, completed_at
		FROM wxpusher_bind_sessions WHERE user_id = ? AND id = ?
	`, userID, sessionID).Scan(
		&session.ID,
		&session.UserID,
		&nonce,
		&ciphertext,
		&session.QRURL,
		&expiresAt,
		&nextPollAt,
		&createdAt,
		&completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.WxPusherBindSession{}, false, nil
	}
	if err != nil {
		return model.WxPusherBindSession{}, false, fmt.Errorf("load wxpusher bind session: %w", err)
	}
	providerCode, err := s.cipher.decryptWithAAD(wxPusherBindSessionAAD(userID, sessionID), nonce, ciphertext)
	if err != nil {
		return model.WxPusherBindSession{}, false, fmt.Errorf("decrypt wxpusher bind session: %w", err)
	}
	session.ProviderCode = string(providerCode)
	session.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	session.NextPollAt = time.Unix(nextPollAt, 0).UTC()
	session.CreatedAt = time.Unix(createdAt, 0).UTC()
	session.CompletedAt = nullableUnixTime(completedAt)
	return session, true, nil
}

func (s *Store) LoadActiveWxPusherBindSession(userID string) (model.WxPusherBindSession, bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return model.WxPusherBindSession{}, false, fmt.Errorf("wxpusher bind session requires a user")
	}
	var sessionID string
	err := s.db.QueryRow(`
		SELECT id FROM wxpusher_bind_sessions
		WHERE user_id = ? AND completed_at IS NULL
		ORDER BY created_at DESC LIMIT 1
	`, userID).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.WxPusherBindSession{}, false, nil
	}
	if err != nil {
		return model.WxPusherBindSession{}, false, fmt.Errorf("load active wxpusher bind session: %w", err)
	}
	return s.LoadWxPusherBindSession(userID, sessionID)
}

func (s *Store) CountWxPusherBindSessionsSince(userID string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM wxpusher_bind_sessions WHERE user_id = ? AND created_at >= ?
	`, strings.TrimSpace(userID), since.UTC().Unix()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count wxpusher bind sessions: %w", err)
	}
	return count, nil
}

func (s *Store) CompleteExpiredWxPusherBindSessions(userID string, now time.Time) error {
	_, err := s.db.Exec(`
		UPDATE wxpusher_bind_sessions SET completed_at = ?
		WHERE user_id = ? AND completed_at IS NULL AND expires_at <= ?
	`, now.UTC().Unix(), strings.TrimSpace(userID), now.UTC().Unix())
	if err != nil {
		return fmt.Errorf("complete expired wxpusher bind sessions: %w", err)
	}
	return nil
}

func (s *Store) CompleteWxPusherBindSession(userID, sessionID string, completedAt time.Time) error {
	result, err := s.db.Exec(`
		UPDATE wxpusher_bind_sessions SET completed_at = ?
		WHERE user_id = ? AND id = ? AND completed_at IS NULL
	`, completedAt.UTC().Unix(), strings.TrimSpace(userID), strings.TrimSpace(sessionID))
	if err != nil {
		return fmt.Errorf("complete wxpusher bind session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read wxpusher bind session completion: %w", err)
	}
	if rows != 1 {
		return ErrWxPusherBindSessionInvalid
	}
	return nil
}

// BindWxPusherUID atomically claims a provider UID and completes its scan
// session. This prevents two users polling the same scanned QR result from
// both observing success.
func (s *Store) BindWxPusherUID(userID, sessionID, uid string, now time.Time) error {
	userID, sessionID, uid = strings.TrimSpace(userID), strings.TrimSpace(sessionID), strings.TrimSpace(uid)
	if userID == "" || sessionID == "" || uid == "" {
		return ErrWxPusherBindSessionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin wxpusher binding: %w", err)
	}
	defer tx.Rollback()
	var expiresAt int64
	if err := tx.QueryRow(`
		SELECT expires_at FROM wxpusher_bind_sessions
		WHERE user_id = ? AND id = ? AND completed_at IS NULL
	`, userID, sessionID).Scan(&expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWxPusherBindSessionInvalid
		}
		return fmt.Errorf("load wxpusher binding session: %w", err)
	}
	if expiresAt <= now.UTC().Unix() {
		return ErrWxPusherBindSessionInvalid
	}
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM wxpusher_bindings WHERE user_id = ?`, userID).Scan(&existing); err != nil {
		return fmt.Errorf("check wxpusher binding: %w", err)
	}
	if existing != 0 {
		return ErrWxPusherBindingExists
	}
	fingerprint := sha256.Sum256([]byte(uid))
	var owner string
	err = tx.QueryRow(`SELECT user_id FROM wxpusher_bindings WHERE uid_fingerprint = ?`, fingerprint[:]).Scan(&owner)
	if err == nil && owner != userID {
		return ErrWxPusherUIDInUse
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check wxpusher uid owner: %w", err)
	}
	nonce, ciphertext, err := s.cipher.encryptWithAAD(wxPusherBindingAAD(userID), []byte(uid))
	if err != nil {
		return fmt.Errorf("encrypt wxpusher binding: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO wxpusher_bindings(
			user_id, uid_fingerprint, uid_nonce, uid_ciphertext, enabled,
			event_types, bound_at, updated_at
		) VALUES(?, ?, ?, ?, 1, ?, ?, ?)
	`, userID, fingerprint[:], nonce, ciphertext, model.WxPusherDefaultEventTypes, now.UTC().Unix(), now.UTC().Unix()); err != nil {
		return fmt.Errorf("insert wxpusher binding: %w", err)
	}
	result, err := tx.Exec(`
		UPDATE wxpusher_bind_sessions SET completed_at = ?
		WHERE user_id = ? AND id = ? AND completed_at IS NULL
	`, now.UTC().Unix(), userID, sessionID)
	if err != nil {
		return fmt.Errorf("complete wxpusher binding session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return ErrWxPusherBindSessionInvalid
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit wxpusher binding: %w", err)
	}
	return nil
}

func (s *Store) DeleteWxPusherChannel(userID string, now time.Time) (bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, fmt.Errorf("delete wxpusher channel requires a user")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin delete wxpusher channel: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM wxpusher_bindings WHERE user_id = ?`, userID)
	if err != nil {
		return false, fmt.Errorf("delete wxpusher binding: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE wxpusher_bind_sessions SET completed_at = ?
		WHERE user_id = ? AND completed_at IS NULL
	`, now.UTC().Unix(), userID); err != nil {
		return false, fmt.Errorf("complete wxpusher sessions: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE notification_deliveries
		SET status = 'cancelled', next_attempt_at = NULL, claimed_at = NULL, updated_at = ?
		WHERE user_id = ? AND channel = ? AND status IN ('pending', 'retry_wait')
	`, now.UTC().Unix(), userID, wxPusherChannel); err != nil {
		return false, fmt.Errorf("cancel wxpusher deliveries: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read wxpusher delete result: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit delete wxpusher channel: %w", err)
	}
	return rows > 0, nil
}

func (s *Store) SaveNotificationDelivery(delivery model.NotificationDelivery) error {
	delivery = normalizeNotificationDelivery(delivery)
	if err := validateNotificationDelivery(delivery); err != nil {
		return err
	}
	result, err := s.db.Exec(`
		INSERT INTO notification_deliveries(
			id, notification_id, user_id, channel, status, is_test, attempt_count,
			next_attempt_at, claimed_at, provider_record_id,
			provider_message_content_id, last_error_code, last_error_message,
			accepted_at, provider_succeeded_at, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			notification_id = excluded.notification_id,
			channel = excluded.channel,
			status = excluded.status,
			is_test = excluded.is_test,
			attempt_count = excluded.attempt_count,
			next_attempt_at = excluded.next_attempt_at,
			claimed_at = excluded.claimed_at,
			provider_record_id = excluded.provider_record_id,
			provider_message_content_id = excluded.provider_message_content_id,
			last_error_code = excluded.last_error_code,
			last_error_message = excluded.last_error_message,
			accepted_at = excluded.accepted_at,
			provider_succeeded_at = excluded.provider_succeeded_at,
			updated_at = excluded.updated_at
		WHERE notification_deliveries.user_id = excluded.user_id
	`,
		delivery.ID,
		stringPointerOrNil(delivery.NotificationID),
		delivery.UserID,
		delivery.Channel,
		delivery.Status,
		delivery.IsTest,
		delivery.AttemptCount,
		unixTimeOrNil(delivery.NextAttemptAt),
		unixTimeOrNil(delivery.ClaimedAt),
		delivery.ProviderRecordID,
		delivery.ProviderMessageContentID,
		delivery.LastErrorCode,
		delivery.LastErrorMessage,
		unixTimeOrNil(delivery.AcceptedAt),
		unixTimeOrNil(delivery.ProviderSucceededAt),
		delivery.CreatedAt.UTC().Unix(),
		delivery.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("save notification delivery: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read notification delivery result: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("notification delivery belongs to another user")
	}
	return nil
}

func (s *Store) LoadNotificationDelivery(userID, deliveryID string) (model.NotificationDelivery, bool, error) {
	userID = strings.TrimSpace(userID)
	deliveryID = strings.TrimSpace(deliveryID)
	if userID == "" || deliveryID == "" {
		return model.NotificationDelivery{}, false, fmt.Errorf("notification delivery requires user and id")
	}
	delivery, err := scanNotificationDelivery(s.db.QueryRow(notificationDeliverySelect+` WHERE user_id = ? AND id = ?`, userID, deliveryID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.NotificationDelivery{}, false, nil
	}
	if err != nil {
		return model.NotificationDelivery{}, false, err
	}
	return delivery, true, nil
}

func (s *Store) ListNotificationDeliveries(userID string, limit int) ([]model.NotificationDelivery, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("notification deliveries require a user")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.Query(notificationDeliverySelect+`
		WHERE user_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification deliveries: %w", err)
	}
	defer rows.Close()
	deliveries := make([]model.NotificationDelivery, 0)
	for rows.Next() {
		delivery, err := scanNotificationDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification deliveries: %w", err)
	}
	return deliveries, nil
}

func (s *Store) LoadLatestNotificationDelivery(userID string, isTest bool) (model.NotificationDelivery, bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return model.NotificationDelivery{}, false, fmt.Errorf("latest notification delivery requires a user")
	}
	testValue := 0
	if isTest {
		testValue = 1
	}
	delivery, err := scanNotificationDelivery(s.db.QueryRow(notificationDeliverySelect+`
		WHERE user_id = ? AND is_test = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, userID, testValue))
	if errors.Is(err, sql.ErrNoRows) {
		return model.NotificationDelivery{}, false, nil
	}
	if err != nil {
		return model.NotificationDelivery{}, false, err
	}
	return delivery, true, nil
}

func (s *Store) CountTestNotificationDeliveriesSince(userID string, since time.Time) (int, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || since.IsZero() {
		return 0, fmt.Errorf("test notification delivery count requires user and time")
	}
	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM notification_deliveries
		WHERE user_id=? AND channel=? AND is_test=1 AND created_at>=?
	`, userID, wxPusherChannel, since.UTC().Unix()).Scan(&count); err != nil {
		return 0, fmt.Errorf("count test notification deliveries: %w", err)
	}
	return count, nil
}

func (s *Store) WxPusherOperationsStatus(since time.Time) (model.WxPusherOperationsStatus, error) {
	var result model.WxPusherOperationsStatus
	var oldestPending, lastAccepted, lastProviderSuccess, lastFailure sql.NullInt64
	err := s.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM wxpusher_bindings WHERE enabled=1),
			COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='sending' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='accepted' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='retry_wait' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='uncertain' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END), 0),
			MIN(CASE WHEN status IN ('pending','sending','accepted','retry_wait')
				THEN created_at END),
			COALESCE(SUM(CASE WHEN created_at>=? AND attempt_count>0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at>=? AND accepted_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN created_at>=? AND provider_succeeded_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			MAX(accepted_at), MAX(provider_succeeded_at),
			MAX(CASE WHEN last_error_code<>'' THEN updated_at END)
		FROM notification_deliveries
		WHERE channel=?
	`, since.UTC().Unix(), since.UTC().Unix(), since.UTC().Unix(), wxPusherChannel).Scan(
		&result.ActiveBindings,
		&result.PendingDeliveries,
		&result.SendingDeliveries,
		&result.AcceptedPendingDeliveries,
		&result.RetryingDeliveries,
		&result.UncertainDeliveries,
		&result.FailedDeliveries,
		&oldestPending,
		&result.Attempts24Hours,
		&result.Accepted24Hours,
		&result.ProviderSucceeded24Hours,
		&lastAccepted,
		&lastProviderSuccess,
		&lastFailure,
	)
	if err != nil {
		return result, fmt.Errorf("summarize wxpusher deliveries: %w", err)
	}
	result.OldestPendingAt = nullableUnixTime(oldestPending)
	result.LastAcceptedAt = nullableUnixTime(lastAccepted)
	result.LastProviderSuccessAt = nullableUnixTime(lastProviderSuccess)
	result.LastFailureAt = nullableUnixTime(lastFailure)

	errorRows, err := s.db.Query(`
		SELECT user_id, last_error_code
		FROM notification_deliveries
		WHERE channel=? AND updated_at>=? AND last_error_code<>''
		ORDER BY updated_at DESC, id DESC
	`, wxPusherChannel, since.UTC().Unix())
	if err != nil {
		return result, fmt.Errorf("query wxpusher delivery errors: %w", err)
	}
	affectedBindings := make(map[string]struct{})
	for errorRows.Next() {
		var userID, code string
		if err := errorRows.Scan(&userID, &code); err != nil {
			errorRows.Close()
			return result, fmt.Errorf("scan wxpusher delivery error: %w", err)
		}
		category := wxPusherOperationsErrorCategory(code)
		if result.LastErrorCategory == "" {
			result.LastErrorCategory = category
		}
		if category == "binding_invalid" {
			result.BindingFailures24Hours++
			affectedBindings[userID] = struct{}{}
		} else {
			result.SystemFailures24Hours++
		}
	}
	if err := errorRows.Close(); err != nil {
		return result, fmt.Errorf("close wxpusher delivery error rows: %w", err)
	}
	if err := errorRows.Err(); err != nil {
		return result, fmt.Errorf("iterate wxpusher delivery errors: %w", err)
	}
	result.AffectedBindingUsers = len(affectedBindings)

	outcomeRows, err := s.db.Query(`
		SELECT status, last_error_code
		FROM notification_deliveries
		WHERE channel=? AND (attempt_count>0 OR accepted_at IS NOT NULL)
		ORDER BY updated_at DESC, id DESC
		LIMIT 100
	`, wxPusherChannel)
	if err != nil {
		return result, fmt.Errorf("query wxpusher recent outcomes: %w", err)
	}
	for outcomeRows.Next() {
		var status model.NotificationDeliveryStatus
		var code string
		if err := outcomeRows.Scan(&status, &code); err != nil {
			outcomeRows.Close()
			return result, fmt.Errorf("scan wxpusher recent outcome: %w", err)
		}
		category := wxPusherOperationsErrorCategory(code)
		if category == "binding_invalid" || status == model.NotificationDeliverySuppressed || status == model.NotificationDeliveryCancelled {
			continue
		}
		if category != "" {
			result.ConsecutiveSystemFailures++
			continue
		}
		if status == model.NotificationDeliveryAccepted || status == model.NotificationDeliveryProviderSucceeded {
			break
		}
	}
	if err := outcomeRows.Close(); err != nil {
		return result, fmt.Errorf("close wxpusher recent outcome rows: %w", err)
	}
	if err := outcomeRows.Err(); err != nil {
		return result, fmt.Errorf("iterate wxpusher recent outcomes: %w", err)
	}
	return result, nil
}

func wxPusherOperationsErrorCategory(code string) string {
	switch strings.TrimSpace(code) {
	case "":
		return ""
	case "wxpusher_invalid_uid", "wxpusher_recipient_rejected":
		return "binding_invalid"
	case "wxpusher_invalid_token", "wxpusher_unconfigured":
		return "configuration"
	case "wxpusher_rate_limited":
		return "rate_limited"
	case "wxpusher_provider_timeout", "wxpusher_provider_unavailable":
		return "provider_unavailable"
	case "wxpusher_invalid_response", "provider_status_unknown", "send_outcome_unknown", "record_id_reentered_send_queue":
		return "delivery_unconfirmed"
	default:
		return "internal"
	}
}

func (s *Store) ClaimNotificationDeliveries(now, staleBefore time.Time, limit int) ([]model.NotificationDelivery, error) {
	if now.IsZero() || staleBefore.IsZero() || !staleBefore.Before(now) || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("notification delivery claim parameters are invalid")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin notification delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`
		SELECT id
		FROM notification_deliveries
		WHERE status IN ('pending', 'retry_wait')
		  AND COALESCE(next_attempt_at, created_at) <= ?
		ORDER BY COALESCE(next_attempt_at, created_at), created_at, id
		LIMIT ?
	`, now.UTC().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("select notification deliveries to claim: %w", err)
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan notification delivery claim: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close notification delivery claim rows: %w", err)
	}
	claimed := make([]model.NotificationDelivery, 0, len(ids))
	for _, id := range ids {
		result, err := tx.Exec(`
			UPDATE notification_deliveries
			SET status='sending', claimed_at=?, attempt_count=attempt_count+1, updated_at=?
			WHERE id=? AND status IN ('pending', 'retry_wait')
			  AND COALESCE(next_attempt_at, created_at) <= ?
		`, now.UTC().Unix(), now.UTC().Unix(), id, now.UTC().Unix())
		if err != nil {
			return nil, fmt.Errorf("claim notification delivery %s: %w", id, err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("read notification delivery claim %s: %w", id, err)
		}
		if updated != 1 {
			continue
		}
		delivery, err := scanNotificationDelivery(tx.QueryRow(notificationDeliverySelect+` WHERE id = ?`, id))
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, delivery)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit notification delivery claim: %w", err)
	}
	return claimed, nil
}

// RecoverStaleSendingDeliveries deliberately does not resend a request whose
// process died after it was placed on the wire. Without a provider record id
// the outcome is ambiguous, so automatic recovery must prefer one missed
// notification over a duplicate push.
func (s *Store) RecoverStaleSendingDeliveries(staleBefore, now time.Time) (int, error) {
	if staleBefore.IsZero() || now.IsZero() || !staleBefore.Before(now) {
		return 0, fmt.Errorf("stale notification delivery recovery parameters are invalid")
	}
	result, err := s.db.Exec(`
		UPDATE notification_deliveries
		SET status='uncertain', next_attempt_at=NULL, claimed_at=NULL,
		    last_error_code='send_outcome_unknown', last_error_message='', updated_at=?
		WHERE status='sending' AND claimed_at IS NOT NULL AND claimed_at <= ?
	`, now.UTC().Unix(), staleBefore.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("recover stale notification deliveries: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read stale notification delivery recovery: %w", err)
	}
	return int(count), nil
}

// ClaimAcceptedNotificationDeliveries leases provider status queries while
// preserving the accepted state. A sendRecordId-bearing task is never moved
// back into the sending queue.
func (s *Store) ClaimAcceptedNotificationDeliveries(now, staleBefore time.Time, limit int) ([]model.NotificationDelivery, error) {
	if now.IsZero() || staleBefore.IsZero() || !staleBefore.Before(now) || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("accepted notification delivery claim parameters are invalid")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin accepted notification delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`
		SELECT id FROM notification_deliveries
		WHERE status='accepted'
		  AND provider_record_id <> ''
		  AND COALESCE(next_attempt_at, created_at) <= ?
		  AND (claimed_at IS NULL OR claimed_at <= ?)
		ORDER BY COALESCE(next_attempt_at, created_at), created_at, id
		LIMIT ?
	`, now.UTC().Unix(), staleBefore.UTC().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("select accepted notification deliveries: %w", err)
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan accepted notification delivery claim: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close accepted notification delivery claim rows: %w", err)
	}
	claimed := make([]model.NotificationDelivery, 0, len(ids))
	for _, id := range ids {
		result, err := tx.Exec(`
			UPDATE notification_deliveries SET claimed_at=?, updated_at=?
			WHERE id=? AND status='accepted' AND provider_record_id <> ''
			  AND COALESCE(next_attempt_at, created_at) <= ?
			  AND (claimed_at IS NULL OR claimed_at <= ?)
		`, now.UTC().Unix(), now.UTC().Unix(), id, now.UTC().Unix(), staleBefore.UTC().Unix())
		if err != nil {
			return nil, fmt.Errorf("claim accepted notification delivery %s: %w", id, err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("read accepted notification delivery claim %s: %w", id, err)
		}
		if updated != 1 {
			continue
		}
		delivery, err := scanNotificationDelivery(tx.QueryRow(notificationDeliverySelect+` WHERE id = ?`, id))
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, delivery)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit accepted notification delivery claim: %w", err)
	}
	return claimed, nil
}

func (s *Store) PruneWxPusherHistory(before time.Time) (int64, int64, error) {
	if before.IsZero() {
		return 0, 0, fmt.Errorf("wxpusher retention boundary is required")
	}
	deliveries, err := s.pruneRowsInBatchesWhere(
		"notification_deliveries",
		"updated_at",
		before.UTC().Unix(),
		"status IN ('provider_succeeded', 'suppressed', 'uncertain', 'failed', 'cancelled')",
	)
	if err != nil {
		return 0, 0, fmt.Errorf("prune wxpusher deliveries: %w", err)
	}
	sessions, err := s.pruneRowsInBatchesWhere(
		"wxpusher_bind_sessions",
		"expires_at",
		before.UTC().Unix(),
		"expires_at IS NOT NULL",
	)
	if err != nil {
		return deliveries, 0, fmt.Errorf("prune wxpusher bind sessions: %w", err)
	}
	return deliveries, sessions, nil
}

const notificationDeliverySelect = `
	SELECT id, notification_id, user_id, channel, status, is_test, attempt_count,
	       next_attempt_at, claimed_at, provider_record_id,
	       provider_message_content_id, last_error_code, last_error_message,
	       accepted_at, provider_succeeded_at, created_at, updated_at
	FROM notification_deliveries`

func scanNotificationDelivery(scanner interface{ Scan(...any) error }) (model.NotificationDelivery, error) {
	var delivery model.NotificationDelivery
	var notificationID sql.NullString
	var isTest int
	var nextAttemptAt, claimedAt, acceptedAt, providerSucceededAt sql.NullInt64
	var createdAt, updatedAt int64
	if err := scanner.Scan(
		&delivery.ID,
		&notificationID,
		&delivery.UserID,
		&delivery.Channel,
		&delivery.Status,
		&isTest,
		&delivery.AttemptCount,
		&nextAttemptAt,
		&claimedAt,
		&delivery.ProviderRecordID,
		&delivery.ProviderMessageContentID,
		&delivery.LastErrorCode,
		&delivery.LastErrorMessage,
		&acceptedAt,
		&providerSucceededAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.NotificationDelivery{}, fmt.Errorf("scan notification delivery: %w", err)
	}
	if notificationID.Valid {
		value := notificationID.String
		delivery.NotificationID = &value
	}
	delivery.IsTest = isTest != 0
	delivery.NextAttemptAt = nullableUnixTime(nextAttemptAt)
	delivery.ClaimedAt = nullableUnixTime(claimedAt)
	delivery.AcceptedAt = nullableUnixTime(acceptedAt)
	delivery.ProviderSucceededAt = nullableUnixTime(providerSucceededAt)
	delivery.CreatedAt = time.Unix(createdAt, 0).UTC()
	delivery.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return delivery, nil
}

func validateWxPusherBinding(binding model.WxPusherBinding) error {
	if strings.TrimSpace(binding.UserID) == "" || strings.TrimSpace(binding.UID) == "" {
		return fmt.Errorf("wxpusher binding requires user and uid")
	}
	if binding.EventTypes < 0 || binding.EventTypes > model.WxPusherAllEventTypes {
		return fmt.Errorf("wxpusher binding event types are invalid")
	}
	if binding.BoundAt.IsZero() || binding.UpdatedAt.IsZero() {
		return fmt.Errorf("wxpusher binding requires timestamps")
	}
	if len(strings.TrimSpace(binding.LastErrorCode)) > 64 {
		return fmt.Errorf("wxpusher binding error code is too long")
	}
	return nil
}

func validateWxPusherBindSession(session model.WxPusherBindSession) error {
	if strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.UserID) == "" ||
		strings.TrimSpace(session.ProviderCode) == "" {
		return fmt.Errorf("wxpusher bind session requires id, user, and provider code")
	}
	parsedURL, err := url.Parse(session.QRURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil || len(session.QRURL) > 2048 {
		return fmt.Errorf("wxpusher bind session qr url is invalid")
	}
	if session.CreatedAt.IsZero() || session.ExpiresAt.IsZero() || session.NextPollAt.IsZero() ||
		!session.ExpiresAt.After(session.CreatedAt) || session.NextPollAt.Before(session.CreatedAt) {
		return fmt.Errorf("wxpusher bind session timestamps are invalid")
	}
	if session.CompletedAt != nil && session.CompletedAt.Before(session.CreatedAt) {
		return fmt.Errorf("wxpusher bind session completion is invalid")
	}
	return nil
}

func validateNotificationDelivery(delivery model.NotificationDelivery) error {
	if strings.TrimSpace(delivery.ID) == "" || strings.TrimSpace(delivery.UserID) == "" {
		return fmt.Errorf("notification delivery requires id and user")
	}
	if delivery.Channel != wxPusherChannel {
		return fmt.Errorf("notification delivery channel is invalid")
	}
	if delivery.NotificationID == nil && !delivery.IsTest {
		return fmt.Errorf("notification delivery requires a notification or test marker")
	}
	if delivery.NotificationID != nil && strings.TrimSpace(*delivery.NotificationID) == "" {
		return fmt.Errorf("notification delivery notification id is invalid")
	}
	switch delivery.Status {
	case model.NotificationDeliveryPending, model.NotificationDeliverySending,
		model.NotificationDeliveryAccepted, model.NotificationDeliveryProviderSucceeded,
		model.NotificationDeliveryRetryWait, model.NotificationDeliverySuppressed,
		model.NotificationDeliveryUncertain, model.NotificationDeliveryFailed,
		model.NotificationDeliveryCancelled:
	default:
		return fmt.Errorf("notification delivery status is invalid")
	}
	if delivery.AttemptCount < 0 || delivery.CreatedAt.IsZero() || delivery.UpdatedAt.IsZero() {
		return fmt.Errorf("notification delivery counters or timestamps are invalid")
	}
	if len(delivery.ProviderRecordID) > 128 || len(delivery.ProviderMessageContentID) > 128 ||
		len(delivery.LastErrorCode) > 64 || len(delivery.LastErrorMessage) > 512 {
		return fmt.Errorf("notification delivery provider metadata is too long")
	}
	return nil
}

func normalizeNotificationDelivery(delivery model.NotificationDelivery) model.NotificationDelivery {
	if delivery.Channel == "" {
		delivery.Channel = wxPusherChannel
	}
	return delivery
}

func stringPointerOrNil(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}

func wxPusherBindingAAD(userID string) string {
	return "charge:wxpusher:binding:" + userID
}

func wxPusherBindSessionAAD(userID, sessionID string) string {
	return "charge:wxpusher:bind_session:" + userID + ":" + sessionID
}
