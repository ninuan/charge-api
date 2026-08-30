package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

const defaultNotificationListLimit = 100

var ErrNotificationCursorNotFound = errors.New("notification cursor not found")

type NotificationPageQuery struct {
	UserID   string
	CursorID string
	Status   string
	Limit    int
}

func (s *Store) SaveWatchRule(rule model.WatchRule) error {
	rule = normalizeWatchRuleLifecycle(rule)
	if err := validateWatchRule(rule); err != nil {
		return err
	}
	result, err := s.db.Exec(`
		INSERT INTO watch_rules(
			id, user_id, device_id, port_id, notify_idle, mode, enabled,
			active_weekdays, active_start_minute, active_end_minute, timezone,
			expires_at, completed_at, completion_reason, stop_after_notify,
			created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			device_id = excluded.device_id,
			mode = excluded.mode,
			enabled = excluded.enabled,
			active_weekdays = excluded.active_weekdays,
			active_start_minute = excluded.active_start_minute,
			active_end_minute = excluded.active_end_minute,
			timezone = excluded.timezone,
			expires_at = excluded.expires_at,
			completed_at = excluded.completed_at,
			completion_reason = excluded.completion_reason,
			stop_after_notify = excluded.stop_after_notify,
			updated_at = excluded.updated_at
		WHERE watch_rules.user_id = excluded.user_id
	`,
		rule.ID,
		rule.UserID,
		rule.DeviceID,
		nil,
		false,
		rule.Mode,
		rule.Enabled,
		rule.ActiveWeekdays,
		rule.ActiveStartMinute,
		rule.ActiveEndMinute,
		rule.Timezone,
		unixTimeOrNil(rule.ExpiresAt),
		unixTimeOrNil(rule.CompletedAt),
		rule.CompletionReason,
		rule.StopAfterNotify,
		rule.CreatedAt.UTC().Unix(),
		rule.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("save watch rule: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read saved watch rule result: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("watch rule belongs to another user")
	}
	return nil
}

func (s *Store) ListWatchRules(userID string) ([]model.WatchRule, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("watch rules require a user")
	}
	rows, err := s.db.Query(`
		SELECT id, user_id, device_id, mode, enabled,
		       active_weekdays, active_start_minute, active_end_minute, timezone,
		       expires_at, completed_at, completion_reason, stop_after_notify,
		       created_at, updated_at
		FROM watch_rules
		WHERE user_id = ?
		ORDER BY updated_at DESC, id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list watch rules: %w", err)
	}
	defer rows.Close()

	rules := make([]model.WatchRule, 0)
	for rows.Next() {
		var rule model.WatchRule
		var enabled, stopAfterNotify int
		var expiresAt, completedAt sql.NullInt64
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&rule.ID,
			&rule.UserID,
			&rule.DeviceID,
			&rule.Mode,
			&enabled,
			&rule.ActiveWeekdays,
			&rule.ActiveStartMinute,
			&rule.ActiveEndMinute,
			&rule.Timezone,
			&expiresAt,
			&completedAt,
			&rule.CompletionReason,
			&stopAfterNotify,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan watch rule: %w", err)
		}
		rule.Enabled = enabled != 0
		rule.StopAfterNotify = stopAfterNotify != 0
		rule.ExpiresAt = nullableUnixTime(expiresAt)
		rule.CompletedAt = nullableUnixTime(completedAt)
		rule.CreatedAt = time.Unix(createdAt, 0).UTC()
		rule.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watch rules: %w", err)
	}
	return rules, nil
}

// CompleteWatchRule closes one active temporary task without deleting its
// history. The conditional update makes repeated completion attempts safe.
func (s *Store) CompleteWatchRule(userID, ruleID string, reason model.WatchCompletionReason, at time.Time) (bool, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(ruleID) == "" || at.IsZero() {
		return false, fmt.Errorf("complete watch rule requires user, rule, and time")
	}
	switch reason {
	case model.WatchCompletionNotified, model.WatchCompletionExpired, model.WatchCompletionCancelled:
	default:
		return false, fmt.Errorf("watch rule completion reason is invalid")
	}
	result, err := s.db.Exec(`
		UPDATE watch_rules
		SET enabled = 0, completed_at = ?, completion_reason = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND mode = 'temporary' AND completed_at IS NULL
	`, at.UTC().Unix(), reason, at.UTC().Unix(), ruleID, userID)
	if err != nil {
		return false, fmt.Errorf("complete watch rule: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read completed watch rule result: %w", err)
	}
	return rows == 1, nil
}

// CompleteExpiredWatchRules persists expiry before scheduler target selection,
// so process restarts cannot revive a temporary task that has passed its hard
// deadline.
func (s *Store) CompleteExpiredWatchRules(at time.Time) (int64, error) {
	if at.IsZero() {
		return 0, fmt.Errorf("expire watch rules requires time")
	}
	result, err := s.db.Exec(`
		UPDATE watch_rules
		SET enabled = 0, completed_at = ?, completion_reason = 'expired', updated_at = ?
		WHERE mode = 'temporary' AND completed_at IS NULL AND expires_at <= ?
	`, at.UTC().Unix(), at.UTC().Unix(), at.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("expire watch rules: %w", err)
	}
	return result.RowsAffected()
}

// CompleteNotifiedTemporaryWatchRules repairs the narrow crash window between
// durable notification creation and rule completion. It intentionally keys off
// the persisted notification fact rather than an in-memory delivery result.
func (s *Store) CompleteNotifiedTemporaryWatchRules(at time.Time) (int64, error) {
	if at.IsZero() {
		return 0, fmt.Errorf("recover notified watch rules requires time")
	}
	result, err := s.db.Exec(`
		UPDATE watch_rules
		SET enabled = 0, completed_at = ?, completion_reason = 'notified', updated_at = ?
		WHERE mode = 'temporary' AND completed_at IS NULL
		  AND EXISTS (
			SELECT 1 FROM notifications n
			WHERE n.user_id = watch_rules.user_id
			  AND n.device_id = watch_rules.device_id
			  AND n.type IN ('port_idle', 'pile_available')
			  AND n.created_at >= watch_rules.created_at
		  )
	`, at.UTC().Unix(), at.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("recover notified watch rules: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) DeleteWatchRule(userID, ruleID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	ruleID = strings.TrimSpace(ruleID)
	if userID == "" || ruleID == "" {
		return false, fmt.Errorf("delete watch rule requires user and rule")
	}
	result, err := s.db.Exec(`DELETE FROM watch_rules WHERE id = ? AND user_id = ?`, ruleID, userID)
	if err != nil {
		return false, fmt.Errorf("delete watch rule: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (s *Store) DeleteWatchDataForPile(userID, deviceID string) error {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	if userID == "" || deviceID == "" {
		return fmt.Errorf("delete pile watch data requires user and pile")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin pile watch data transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM watch_rules WHERE user_id = ? AND device_id = ?`, userID, deviceID); err != nil {
		return fmt.Errorf("delete pile watch rules: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM watch_refresh_states WHERE user_id = ? AND device_id = ?`, userID, deviceID); err != nil {
		return fmt.Errorf("delete pile watch refresh state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pile watch data transaction: %w", err)
	}
	return nil
}

func (s *Store) SaveNotificationPreference(preference model.NotificationPreference) error {
	if err := validateNotificationPreference(preference); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO notification_preferences(
			user_id, browser_enabled, quiet_hours_enabled,
			quiet_start_minute, quiet_end_minute, timezone, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			browser_enabled = excluded.browser_enabled,
			quiet_hours_enabled = excluded.quiet_hours_enabled,
			quiet_start_minute = excluded.quiet_start_minute,
			quiet_end_minute = excluded.quiet_end_minute,
			timezone = excluded.timezone,
			updated_at = excluded.updated_at
	`,
		preference.UserID,
		preference.BrowserEnabled,
		preference.QuietHoursEnabled,
		preference.QuietStartMinute,
		preference.QuietEndMinute,
		preference.Timezone,
		preference.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("save notification preference: %w", err)
	}
	return nil
}

func (s *Store) LoadNotificationPreference(userID string) (model.NotificationPreference, bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return model.NotificationPreference{}, false, fmt.Errorf("notification preference requires a user")
	}
	var preference model.NotificationPreference
	var browserEnabled, quietHoursEnabled int
	var updatedAt int64
	err := s.db.QueryRow(`
		SELECT user_id, browser_enabled, quiet_hours_enabled,
		       quiet_start_minute, quiet_end_minute, timezone, updated_at
		FROM notification_preferences
		WHERE user_id = ?
	`, userID).Scan(
		&preference.UserID,
		&browserEnabled,
		&quietHoursEnabled,
		&preference.QuietStartMinute,
		&preference.QuietEndMinute,
		&preference.Timezone,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return model.NotificationPreference{}, false, nil
	}
	if err != nil {
		return model.NotificationPreference{}, false, fmt.Errorf("load notification preference: %w", err)
	}
	preference.BrowserEnabled = browserEnabled != 0
	preference.QuietHoursEnabled = quietHoursEnabled != 0
	preference.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return preference, true, nil
}

func (s *Store) SaveNotification(notification model.Notification) error {
	notification = normalizeNotificationOccurrences(notification)
	if err := validateNotification(notification); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO notifications(
			id, user_id, type, severity, title, message, device_id, port_id,
			source_event_id, dedupe_key, read_at, resolved_at,
			occurrence_count, last_occurred_at, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		notification.ID,
		notification.UserID,
		storedNotificationType(notification.Type),
		notification.Severity,
		notification.Title,
		notification.Message,
		notification.DeviceID,
		notification.PortID,
		notification.SourceEventID,
		notification.DedupeKey,
		unixTimeOrNil(notification.ReadAt),
		unixTimeOrNil(notification.ResolvedAt),
		notification.OccurrenceCount,
		notification.LastOccurredAt.UTC().Unix(),
		notification.CreatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("save notification: %w", err)
	}
	return nil
}

// InsertNotificationIfAbsent persists a generated notification once. The
// database's source-event and active-dedupe unique indexes are the authority,
// so concurrent refresh workers cannot deliver the same fact twice.
func (s *Store) InsertNotificationIfAbsent(notification model.Notification) (bool, error) {
	notification = normalizeNotificationOccurrences(notification)
	if err := validateNotification(notification); err != nil {
		return false, err
	}
	result, err := s.db.Exec(`
		INSERT INTO notifications(
			id, user_id, type, severity, title, message, device_id, port_id,
			source_event_id, dedupe_key, read_at, resolved_at,
			occurrence_count, last_occurred_at, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`,
		notification.ID,
		notification.UserID,
		storedNotificationType(notification.Type),
		notification.Severity,
		notification.Title,
		notification.Message,
		notification.DeviceID,
		notification.PortID,
		notification.SourceEventID,
		notification.DedupeKey,
		unixTimeOrNil(notification.ReadAt),
		unixTimeOrNil(notification.ResolvedAt),
		notification.OccurrenceCount,
		notification.LastOccurredAt.UTC().Unix(),
		notification.CreatedAt.UTC().Unix(),
	)
	if err != nil {
		return false, fmt.Errorf("insert notification if absent: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read inserted notification result: %w", err)
	}
	return rows == 1, nil
}

// InsertNotificationWithWxPusherDeliveryIfAbsent commits the in-app
// notification and its optional external delivery as one durable fact. A
// missing, disabled, or unsubscribed binding deliberately leaves no delivery
// row; quiet hours leave a terminal suppressed row for user-visible history.
func (s *Store) InsertNotificationWithWxPusherDeliveryIfAbsent(
	notification model.Notification,
	deliveryID string,
	suppress bool,
) (model.Notification, bool, bool, error) {
	notification = normalizeNotificationOccurrences(notification)
	if err := validateNotification(notification); err != nil {
		return model.Notification{}, false, false, err
	}
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return model.Notification{}, false, false, fmt.Errorf("wxpusher delivery requires an id")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return model.Notification{}, false, false, fmt.Errorf("begin notification outbox: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Exec(`
		INSERT INTO notifications(
			id, user_id, type, severity, title, message, device_id, port_id,
			source_event_id, dedupe_key, read_at, resolved_at,
			occurrence_count, last_occurred_at, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`,
		notification.ID,
		notification.UserID,
		storedNotificationType(notification.Type),
		notification.Severity,
		notification.Title,
		notification.Message,
		notification.DeviceID,
		notification.PortID,
		notification.SourceEventID,
		notification.DedupeKey,
		unixTimeOrNil(notification.ReadAt),
		unixTimeOrNil(notification.ResolvedAt),
		notification.OccurrenceCount,
		notification.LastOccurredAt.UTC().Unix(),
		notification.CreatedAt.UTC().Unix(),
	)
	if err != nil {
		return model.Notification{}, false, false, fmt.Errorf("insert notification outbox fact: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return model.Notification{}, false, false, fmt.Errorf("read notification outbox result: %w", err)
	}
	if rows != 1 {
		if notification.SourceEventID != nil {
			existing, scanErr := scanNotification(tx.QueryRow(`
				SELECT id, user_id, type, severity, title, message, device_id, port_id,
				       source_event_id, dedupe_key, read_at, resolved_at,
				       occurrence_count, last_occurred_at, created_at
				FROM notifications WHERE source_event_id = ?
			`, *notification.SourceEventID))
			if scanErr == nil {
				if err := tx.Commit(); err != nil {
					return model.Notification{}, false, false, fmt.Errorf("commit duplicate notification outbox: %w", err)
				}
				return existing, false, false, nil
			}
			if !errors.Is(scanErr, sql.ErrNoRows) {
				return model.Notification{}, false, false, scanErr
			}
		}
		if notification.DedupeKey != "" {
			updated, scanErr := scanNotification(tx.QueryRow(`
				UPDATE notifications
				SET occurrence_count = occurrence_count + 1,
				    last_occurred_at = MAX(last_occurred_at, ?),
				    severity = ?, title = ?, message = ?, read_at = NULL
				WHERE user_id = ? AND dedupe_key = ? AND resolved_at IS NULL
				RETURNING id, user_id, type, severity, title, message, device_id, port_id,
				          source_event_id, dedupe_key, read_at, resolved_at,
				          occurrence_count, last_occurred_at, created_at
			`, notification.LastOccurredAt.UTC().Unix(), notification.Severity,
				notification.Title, notification.Message, notification.UserID, notification.DedupeKey))
			if scanErr == nil {
				if err := tx.Commit(); err != nil {
					return model.Notification{}, false, false, fmt.Errorf("commit merged notification outbox: %w", err)
				}
				return updated, false, false, nil
			}
			if !errors.Is(scanErr, sql.ErrNoRows) {
				return model.Notification{}, false, false, fmt.Errorf("merge duplicate notification: %w", scanErr)
			}
		}
		if err := tx.Commit(); err != nil {
			return model.Notification{}, false, false, fmt.Errorf("commit duplicate notification outbox: %w", err)
		}
		return model.Notification{}, false, false, nil
	}

	var enabled int
	var eventTypes model.WxPusherEventTypes
	err = tx.QueryRow(`
		SELECT enabled, event_types FROM wxpusher_bindings WHERE user_id = ?
	`, notification.UserID).Scan(&enabled, &eventTypes)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return model.Notification{}, false, false, fmt.Errorf("load wxpusher outbox policy: %w", err)
	}
	shouldQueue := err == nil && enabled != 0 && eventTypes&wxPusherEventMask(notification.Type) != 0
	if shouldQueue {
		status := model.NotificationDeliveryPending
		errorCode := ""
		if suppress {
			status = model.NotificationDeliverySuppressed
			errorCode = "quiet_hours"
		}
		now := notification.CreatedAt.UTC().Truncate(time.Second)
		var nextAttemptAt any = now.Unix()
		if suppress {
			nextAttemptAt = nil
		}
		if _, err := tx.Exec(`
			INSERT INTO notification_deliveries(
				id, notification_id, user_id, channel, status, is_test,
				attempt_count, next_attempt_at, last_error_code, created_at, updated_at
			) VALUES(?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING
		`, deliveryID, notification.ID, notification.UserID, wxPusherChannel, status,
			nextAttemptAt, errorCode, now.Unix(), now.Unix()); err != nil {
			return model.Notification{}, false, false, fmt.Errorf("insert wxpusher outbox delivery: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Notification{}, false, false, fmt.Errorf("commit notification outbox: %w", err)
	}
	return notification, true, shouldQueue, nil
}

func wxPusherEventMask(notificationType model.NotificationType) model.WxPusherEventTypes {
	switch notificationType {
	case model.NotificationPileAvailable:
		return model.WxPusherEventPileAvailable
	case model.NotificationCredentialExpired:
		return model.WxPusherEventCredentialExpired
	case model.NotificationPileOffline:
		return model.WxPusherEventPileOffline
	case model.NotificationPileRecovered:
		return model.WxPusherEventPileRecovered
	default:
		return 0
	}
}

func (s *Store) ResolveActiveNotification(userID, dedupeKey string, at time.Time) (model.Notification, bool, error) {
	userID = strings.TrimSpace(userID)
	dedupeKey = strings.TrimSpace(dedupeKey)
	if userID == "" || dedupeKey == "" || at.IsZero() {
		return model.Notification{}, false, fmt.Errorf("resolve notification requires user, dedupe key, and time")
	}
	resolvedAt := at.UTC().Truncate(time.Second)
	notification, err := scanNotification(s.db.QueryRow(`
		UPDATE notifications
		SET resolved_at = ?
		WHERE user_id = ? AND dedupe_key = ? AND resolved_at IS NULL
		RETURNING id, user_id, type, severity, title, message, device_id, port_id,
		          source_event_id, dedupe_key, read_at, resolved_at,
		          occurrence_count, last_occurred_at, created_at
	`, resolvedAt.Unix(), userID, dedupeKey))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Notification{}, false, nil
	}
	if err != nil {
		return model.Notification{}, false, fmt.Errorf("resolve active notification: %w", err)
	}
	return notification, true, nil
}

func (s *Store) ListNotifications(userID string, limit int) ([]model.Notification, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("notifications require a user")
	}
	if limit <= 0 {
		limit = defaultNotificationListLimit
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.Query(`
		SELECT id, user_id, type, severity, title, message, device_id, port_id,
		       source_event_id, dedupe_key, read_at, resolved_at,
		       occurrence_count, last_occurred_at, created_at
		FROM notifications
		WHERE user_id = ?
		ORDER BY last_occurred_at DESC, id DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]model.Notification, 0)
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		notifications = append(notifications, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return notifications, nil
}

func (s *Store) ListNotificationsPage(query NotificationPageQuery) (model.NotificationPage, error) {
	query.UserID = strings.TrimSpace(query.UserID)
	query.CursorID = strings.TrimSpace(query.CursorID)
	query.Status = strings.TrimSpace(query.Status)
	if query.UserID == "" {
		return model.NotificationPage{}, fmt.Errorf("notifications require a user")
	}
	if query.Limit < 1 || query.Limit > 100 {
		return model.NotificationPage{}, fmt.Errorf("notification page limit is invalid")
	}

	clauses := []string{"user_id = ?"}
	args := []any{query.UserID}
	switch query.Status {
	case "", "all":
	case "unread":
		clauses = append(clauses, "read_at IS NULL")
	case "pending":
		clauses = append(clauses, "type IN (?, ?) AND resolved_at IS NULL")
		args = append(args, model.NotificationCredentialExpired, model.NotificationPileOffline)
	case "resolved":
		clauses = append(clauses, "type IN (?, ?) AND resolved_at IS NOT NULL")
		args = append(args, model.NotificationCredentialExpired, model.NotificationPileOffline)
	default:
		return model.NotificationPage{}, fmt.Errorf("notification status is invalid")
	}
	if query.CursorID != "" {
		var cursorLastOccurredAt int64
		err := s.db.QueryRow(`
			SELECT last_occurred_at FROM notifications WHERE user_id = ? AND id = ?
		`, query.UserID, query.CursorID).Scan(&cursorLastOccurredAt)
		if err == sql.ErrNoRows {
			return model.NotificationPage{}, ErrNotificationCursorNotFound
		}
		if err != nil {
			return model.NotificationPage{}, fmt.Errorf("load notification cursor: %w", err)
		}
		clauses = append(clauses, "(last_occurred_at < ? OR (last_occurred_at = ? AND id < ?))")
		args = append(args, cursorLastOccurredAt, cursorLastOccurredAt, query.CursorID)
	}
	args = append(args, query.Limit+1)
	rows, err := s.db.Query(`
		SELECT id, user_id, type, severity, title, message, device_id, port_id,
		       source_event_id, dedupe_key, read_at, resolved_at,
		       occurrence_count, last_occurred_at, created_at
		FROM notifications
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY last_occurred_at DESC, id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return model.NotificationPage{}, fmt.Errorf("list notification page: %w", err)
	}
	defer rows.Close()

	items := make([]model.Notification, 0, query.Limit+1)
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return model.NotificationPage{}, err
		}
		items = append(items, notification)
	}
	if err := rows.Err(); err != nil {
		return model.NotificationPage{}, fmt.Errorf("iterate notification page: %w", err)
	}
	nextCursor := ""
	if len(items) > query.Limit {
		items = items[:query.Limit]
		nextCursor = items[len(items)-1].ID
	}
	var unreadCount int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL
	`, query.UserID).Scan(&unreadCount); err != nil {
		return model.NotificationPage{}, fmt.Errorf("count unread notifications: %w", err)
	}
	return model.NotificationPage{Items: items, NextCursor: nextCursor, UnreadCount: unreadCount}, nil
}

func (s *Store) LoadNotification(userID, notificationID string) (model.Notification, bool, error) {
	userID = strings.TrimSpace(userID)
	notificationID = strings.TrimSpace(notificationID)
	if userID == "" || notificationID == "" {
		return model.Notification{}, false, fmt.Errorf("notification lookup requires user and notification")
	}
	notification, err := scanNotification(s.db.QueryRow(`
		SELECT id, user_id, type, severity, title, message, device_id, port_id,
		       source_event_id, dedupe_key, read_at, resolved_at,
		       occurrence_count, last_occurred_at, created_at
		FROM notifications
		WHERE user_id = ? AND id = ?
	`, userID, notificationID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Notification{}, false, nil
	}
	if err != nil {
		return model.Notification{}, false, err
	}
	return notification, true, nil
}

func (s *Store) MarkNotificationRead(userID, notificationID string, at time.Time) (model.Notification, bool, error) {
	userID = strings.TrimSpace(userID)
	notificationID = strings.TrimSpace(notificationID)
	if userID == "" || notificationID == "" || at.IsZero() {
		return model.Notification{}, false, fmt.Errorf("mark notification read requires user, notification, and time")
	}
	result, err := s.db.Exec(`
		UPDATE notifications SET read_at = COALESCE(read_at, ?) WHERE user_id = ? AND id = ?
	`, at.UTC().Unix(), userID, notificationID)
	if err != nil {
		return model.Notification{}, false, fmt.Errorf("mark notification read: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return model.Notification{}, false, fmt.Errorf("read mark notification result: %w", err)
	}
	if rows == 0 {
		return model.Notification{}, false, nil
	}
	notification, ok, err := s.LoadNotification(userID, notificationID)
	return notification, ok, err
}

func (s *Store) MarkAllNotificationsRead(userID string, at time.Time) (int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || at.IsZero() {
		return 0, fmt.Errorf("mark all notifications read requires user and time")
	}
	result, err := s.db.Exec(`
		UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL
	`, at.UTC().Unix(), userID)
	if err != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read mark all notifications result: %w", err)
	}
	return rows, nil
}

func (s *Store) DeleteResolvedNotifications(userID string) (int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, fmt.Errorf("delete resolved notifications requires a user")
	}
	result, err := s.db.Exec(`
		DELETE FROM notifications WHERE user_id = ? AND resolved_at IS NOT NULL
	`, userID)
	if err != nil {
		return 0, fmt.Errorf("delete resolved notifications: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read delete resolved notifications result: %w", err)
	}
	return rows, nil
}

// PruneNotifications removes resolved notifications and terminal informational
// notifications older than the retention boundary. Unresolved actionable
// notifications are kept regardless of age so maintenance never hides a user
// action that is still required.
func (s *Store) PruneNotifications(before time.Time) (int64, error) {
	if before.IsZero() {
		return 0, fmt.Errorf("notification retention boundary is required")
	}
	cutoff := before.UTC().Unix()
	resolved, err := s.pruneRowsInBatchesWhere(
		"notifications",
		"resolved_at",
		cutoff,
		"resolved_at IS NOT NULL",
	)
	if err != nil {
		return resolved, err
	}
	informational, err := s.pruneRowsInBatchesWhere(
		"notifications",
		"COALESCE(NULLIF(last_occurred_at, 0), created_at)",
		cutoff,
		"resolved_at IS NULL AND type IN ('port_idle', 'pile_recovered')",
	)
	return resolved + informational, err
}

func (s *Store) SaveWatchRefreshState(state model.WatchRefreshState) error {
	if err := validateWatchRefreshState(state); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO watch_refresh_states(
			user_id, device_id, next_attempt_at, last_attempt_at, last_success_at,
			consecutive_failures, paused_reason, quota_date, quota_used,
			availability_known, had_idle_port, availability_event_id, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, device_id) DO UPDATE SET
			next_attempt_at = excluded.next_attempt_at,
			last_attempt_at = excluded.last_attempt_at,
			last_success_at = excluded.last_success_at,
			consecutive_failures = excluded.consecutive_failures,
			paused_reason = excluded.paused_reason,
			quota_date = excluded.quota_date,
			quota_used = excluded.quota_used,
			updated_at = excluded.updated_at
	`,
		state.UserID,
		state.DeviceID,
		state.NextAttemptAt.UTC().Unix(),
		unixTimeOrNil(state.LastAttemptAt),
		unixTimeOrNil(state.LastSuccessAt),
		state.ConsecutiveFailures,
		state.PausedReason,
		state.QuotaDate,
		state.QuotaUsed,
		state.AvailabilityKnown,
		state.HadIdlePort,
		state.AvailabilityEventID,
		state.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("save watch refresh state: %w", err)
	}
	return nil
}

func (s *Store) LoadWatchRefreshState(userID, deviceID string) (model.WatchRefreshState, bool, error) {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	if userID == "" || deviceID == "" {
		return model.WatchRefreshState{}, false, fmt.Errorf("watch refresh state requires user and pile")
	}
	var state model.WatchRefreshState
	var nextAttemptAt, updatedAt int64
	var lastAttemptAt, lastSuccessAt sql.NullInt64
	var availabilityKnown, hadIdlePort int
	err := s.db.QueryRow(`
		SELECT user_id, device_id, next_attempt_at, last_attempt_at, last_success_at,
		       consecutive_failures, paused_reason, quota_date, quota_used,
		       availability_known, had_idle_port, availability_event_id, updated_at
		FROM watch_refresh_states
		WHERE user_id = ? AND device_id = ?
	`, userID, deviceID).Scan(
		&state.UserID,
		&state.DeviceID,
		&nextAttemptAt,
		&lastAttemptAt,
		&lastSuccessAt,
		&state.ConsecutiveFailures,
		&state.PausedReason,
		&state.QuotaDate,
		&state.QuotaUsed,
		&availabilityKnown,
		&hadIdlePort,
		&state.AvailabilityEventID,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return model.WatchRefreshState{}, false, nil
	}
	if err != nil {
		return model.WatchRefreshState{}, false, fmt.Errorf("load watch refresh state: %w", err)
	}
	state.NextAttemptAt = time.Unix(nextAttemptAt, 0).UTC()
	state.LastAttemptAt = nullableUnixTime(lastAttemptAt)
	state.LastSuccessAt = nullableUnixTime(lastSuccessAt)
	state.AvailabilityKnown = availabilityKnown != 0
	state.HadIdlePort = hadIdlePort != 0
	state.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return state, true, nil
}

func (s *Store) ListWatchRefreshStates(userID string) ([]model.WatchRefreshState, error) {
	userID = strings.TrimSpace(userID)
	query := `
		SELECT user_id, device_id, next_attempt_at, last_attempt_at, last_success_at,
		       consecutive_failures, paused_reason, quota_date, quota_used,
		       availability_known, had_idle_port, availability_event_id, updated_at
		FROM watch_refresh_states
	`
	var (
		rows *sql.Rows
		err  error
	)
	if userID == "" {
		rows, err = s.db.Query(query + ` ORDER BY user_id, device_id`)
	} else {
		rows, err = s.db.Query(query+` WHERE user_id = ? ORDER BY device_id`, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("list watch refresh states: %w", err)
	}
	defer rows.Close()
	states := make([]model.WatchRefreshState, 0)
	for rows.Next() {
		state, err := scanWatchRefreshState(rows)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *Store) DeleteWatchRefreshState(userID, deviceID string) error {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	if userID == "" || deviceID == "" {
		return fmt.Errorf("delete watch refresh state requires user and pile")
	}
	if _, err := s.db.Exec(
		`DELETE FROM watch_refresh_states WHERE user_id = ? AND device_id = ?`,
		userID,
		deviceID,
	); err != nil {
		return fmt.Errorf("delete watch refresh state: %w", err)
	}
	return nil
}

func (s *Store) SavePileAvailabilityState(userID, deviceID string, known, hadIdle bool, eventID int64, at time.Time) error {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	if userID == "" || deviceID == "" || eventID < 0 || at.IsZero() {
		return fmt.Errorf("pile availability state requires user, pile, and time")
	}
	_, err := s.db.Exec(`
		INSERT INTO watch_refresh_states(
			user_id, device_id, next_attempt_at, availability_known,
			had_idle_port, availability_event_id, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, device_id) DO UPDATE SET
			availability_known = excluded.availability_known,
			had_idle_port = excluded.had_idle_port,
			availability_event_id = MAX(watch_refresh_states.availability_event_id, excluded.availability_event_id)
	`, userID, deviceID, at.UTC().Unix(), known, hadIdle, eventID, at.UTC().Unix())
	if err != nil {
		return fmt.Errorf("save pile availability state: %w", err)
	}
	return nil
}

func (s *Store) WatchRefreshQuotaUsed(userID, quotaDate string) (int, error) {
	userID = strings.TrimSpace(userID)
	quotaDate = strings.TrimSpace(quotaDate)
	if userID == "" || quotaDate == "" {
		return 0, fmt.Errorf("watch refresh quota requires user and date")
	}
	var used int
	if err := s.db.QueryRow(`
		SELECT COALESCE(SUM(quota_used), 0)
		FROM watch_refresh_states
		WHERE user_id = ? AND quota_date = ?
	`, userID, quotaDate).Scan(&used); err != nil {
		return 0, fmt.Errorf("load watch refresh quota: %w", err)
	}
	return used, nil
}

func scanWatchRefreshState(scanner interface{ Scan(...any) error }) (model.WatchRefreshState, error) {
	var state model.WatchRefreshState
	var nextAttemptAt, updatedAt int64
	var lastAttemptAt, lastSuccessAt sql.NullInt64
	var availabilityKnown, hadIdlePort int
	if err := scanner.Scan(
		&state.UserID,
		&state.DeviceID,
		&nextAttemptAt,
		&lastAttemptAt,
		&lastSuccessAt,
		&state.ConsecutiveFailures,
		&state.PausedReason,
		&state.QuotaDate,
		&state.QuotaUsed,
		&availabilityKnown,
		&hadIdlePort,
		&state.AvailabilityEventID,
		&updatedAt,
	); err != nil {
		return model.WatchRefreshState{}, fmt.Errorf("scan watch refresh state: %w", err)
	}
	state.NextAttemptAt = time.Unix(nextAttemptAt, 0).UTC()
	state.LastAttemptAt = nullableUnixTime(lastAttemptAt)
	state.LastSuccessAt = nullableUnixTime(lastSuccessAt)
	state.AvailabilityKnown = availabilityKnown != 0
	state.HadIdlePort = hadIdlePort != 0
	state.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return state, nil
}

func validateWatchRule(rule model.WatchRule) error {
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.UserID) == "" || strings.TrimSpace(rule.DeviceID) == "" {
		return fmt.Errorf("watch rule requires id, user, and pile")
	}
	if rule.ActiveWeekdays < 1 || rule.ActiveWeekdays > 127 {
		return fmt.Errorf("watch rule weekdays are invalid")
	}
	if rule.ActiveStartMinute < 0 || rule.ActiveStartMinute >= 24*60 ||
		rule.ActiveEndMinute < 0 || rule.ActiveEndMinute >= 24*60 {
		return fmt.Errorf("watch rule active time is invalid")
	}
	if _, err := time.LoadLocation(rule.Timezone); err != nil {
		return fmt.Errorf("watch rule timezone is invalid")
	}
	if rule.CreatedAt.IsZero() || rule.UpdatedAt.IsZero() {
		return fmt.Errorf("watch rule requires timestamps")
	}
	switch rule.Mode {
	case model.WatchRuleRecurring:
		if rule.ExpiresAt != nil || rule.CompletedAt != nil || rule.CompletionReason != "" || rule.StopAfterNotify {
			return fmt.Errorf("recurring watch rule lifecycle is invalid")
		}
	case model.WatchRuleTemporary:
		if rule.ExpiresAt == nil || !rule.ExpiresAt.After(rule.CreatedAt) || !rule.StopAfterNotify {
			return fmt.Errorf("temporary watch rule expiry is invalid")
		}
		if (rule.CompletedAt == nil) != (rule.CompletionReason == "") {
			return fmt.Errorf("temporary watch rule completion is incomplete")
		}
		if rule.CompletedAt != nil {
			if rule.CompletedAt.Before(rule.CreatedAt) {
				return fmt.Errorf("temporary watch rule completion time is invalid")
			}
			switch rule.CompletionReason {
			case model.WatchCompletionNotified, model.WatchCompletionExpired, model.WatchCompletionCancelled:
			default:
				return fmt.Errorf("temporary watch rule completion reason is invalid")
			}
		}
	default:
		return fmt.Errorf("watch rule mode is invalid")
	}
	return nil
}

func normalizeWatchRuleLifecycle(rule model.WatchRule) model.WatchRule {
	if rule.Mode == "" {
		rule.Mode = model.WatchRuleRecurring
	}
	return rule
}

func validateNotificationPreference(preference model.NotificationPreference) error {
	if strings.TrimSpace(preference.UserID) == "" {
		return fmt.Errorf("notification preference requires a user")
	}
	if preference.QuietStartMinute < 0 || preference.QuietStartMinute >= 24*60 ||
		preference.QuietEndMinute < 0 || preference.QuietEndMinute >= 24*60 {
		return fmt.Errorf("notification quiet time is invalid")
	}
	if _, err := time.LoadLocation(preference.Timezone); err != nil {
		return fmt.Errorf("notification timezone is invalid")
	}
	if preference.UpdatedAt.IsZero() {
		return fmt.Errorf("notification preference requires updated time")
	}
	return nil
}

func validateNotification(notification model.Notification) error {
	if strings.TrimSpace(notification.ID) == "" || strings.TrimSpace(notification.UserID) == "" {
		return fmt.Errorf("notification requires id and user")
	}
	switch notification.Type {
	case model.NotificationPileAvailable, model.NotificationCredentialExpired,
		model.NotificationPileOffline, model.NotificationPileRecovered:
	default:
		return fmt.Errorf("notification type is invalid")
	}
	switch notification.Severity {
	case "info", "warning", "critical":
	default:
		return fmt.Errorf("notification severity is invalid")
	}
	if strings.TrimSpace(notification.Title) == "" || strings.TrimSpace(notification.Message) == "" {
		return fmt.Errorf("notification requires title and message")
	}
	if notification.PortID != nil && *notification.PortID <= 0 {
		return fmt.Errorf("notification port must be positive")
	}
	if notification.Type == model.NotificationPileAvailable &&
		(strings.TrimSpace(notification.DeviceID) == "" || notification.PortID == nil ||
			(notification.SourceEventID == nil && !strings.HasPrefix(notification.DedupeKey, "pile_available_rule:"))) {
		return fmt.Errorf("idle notification requires pile, port, and source event")
	}
	if notification.CreatedAt.IsZero() {
		return fmt.Errorf("notification requires created time")
	}
	return nil
}

// Existing v9 databases store the former port-level type name. The public and
// runtime model is pile-level; keeping this storage alias avoids rewriting
// durable notifications while the API consistently exposes pile_available.
func storedNotificationType(value model.NotificationType) string {
	if value == model.NotificationPileAvailable {
		return "port_idle"
	}
	return string(value)
}

func validateWatchRefreshState(state model.WatchRefreshState) error {
	if strings.TrimSpace(state.UserID) == "" || strings.TrimSpace(state.DeviceID) == "" {
		return fmt.Errorf("watch refresh state requires user and pile")
	}
	if state.NextAttemptAt.IsZero() || state.UpdatedAt.IsZero() {
		return fmt.Errorf("watch refresh state requires timestamps")
	}
	if state.ConsecutiveFailures < 0 || state.QuotaUsed < 0 || state.AvailabilityEventID < 0 {
		return fmt.Errorf("watch refresh state counters cannot be negative")
	}
	return nil
}

func scanNotification(scanner interface{ Scan(...any) error }) (model.Notification, error) {
	var notification model.Notification
	var notificationType string
	var portID, sourceEventID, readAt, resolvedAt sql.NullInt64
	var createdAt, lastOccurredAt int64
	if err := scanner.Scan(
		&notification.ID,
		&notification.UserID,
		&notificationType,
		&notification.Severity,
		&notification.Title,
		&notification.Message,
		&notification.DeviceID,
		&portID,
		&sourceEventID,
		&notification.DedupeKey,
		&readAt,
		&resolvedAt,
		&notification.OccurrenceCount,
		&lastOccurredAt,
		&createdAt,
	); err != nil {
		return model.Notification{}, fmt.Errorf("scan notification: %w", err)
	}
	if notificationType == "port_idle" {
		notification.Type = model.NotificationPileAvailable
	} else {
		notification.Type = model.NotificationType(notificationType)
	}
	if portID.Valid {
		value := int(portID.Int64)
		notification.PortID = &value
	}
	if sourceEventID.Valid {
		value := sourceEventID.Int64
		notification.SourceEventID = &value
	}
	notification.ReadAt = nullableUnixTime(readAt)
	notification.ResolvedAt = nullableUnixTime(resolvedAt)
	notification.LastOccurredAt = time.Unix(lastOccurredAt, 0).UTC()
	notification.CreatedAt = time.Unix(createdAt, 0).UTC()
	return notification, nil
}

func normalizeNotificationOccurrences(notification model.Notification) model.Notification {
	if notification.OccurrenceCount < 1 {
		notification.OccurrenceCount = 1
	}
	if notification.LastOccurredAt.IsZero() {
		notification.LastOccurredAt = notification.CreatedAt
	}
	return notification
}

func unixTimeOrNil(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Unix()
}

func nullableUnixTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	result := time.Unix(value.Int64, 0).UTC()
	return &result
}
