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
	if err := validateWatchRule(rule); err != nil {
		return err
	}
	result, err := s.db.Exec(`
		INSERT INTO watch_rules(
			id, user_id, device_id, port_id, notify_idle, enabled,
			active_weekdays, active_start_minute, active_end_minute, timezone,
			created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			device_id = excluded.device_id,
			port_id = excluded.port_id,
			notify_idle = excluded.notify_idle,
			enabled = excluded.enabled,
			active_weekdays = excluded.active_weekdays,
			active_start_minute = excluded.active_start_minute,
			active_end_minute = excluded.active_end_minute,
			timezone = excluded.timezone,
			updated_at = excluded.updated_at
		WHERE watch_rules.user_id = excluded.user_id
	`,
		rule.ID,
		rule.UserID,
		rule.DeviceID,
		rule.PortID,
		rule.NotifyIdle,
		rule.Enabled,
		rule.ActiveWeekdays,
		rule.ActiveStartMinute,
		rule.ActiveEndMinute,
		rule.Timezone,
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
		SELECT id, user_id, device_id, port_id, notify_idle, enabled,
		       active_weekdays, active_start_minute, active_end_minute, timezone,
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
		var portID sql.NullInt64
		var notifyIdle, enabled int
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&rule.ID,
			&rule.UserID,
			&rule.DeviceID,
			&portID,
			&notifyIdle,
			&enabled,
			&rule.ActiveWeekdays,
			&rule.ActiveStartMinute,
			&rule.ActiveEndMinute,
			&rule.Timezone,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan watch rule: %w", err)
		}
		if portID.Valid {
			port := int(portID.Int64)
			rule.PortID = &port
		}
		rule.NotifyIdle = notifyIdle != 0
		rule.Enabled = enabled != 0
		rule.CreatedAt = time.Unix(createdAt, 0).UTC()
		rule.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watch rules: %w", err)
	}
	return rules, nil
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
	if err := validateNotification(notification); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO notifications(
			id, user_id, type, severity, title, message, device_id, port_id,
			source_event_id, dedupe_key, read_at, resolved_at, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		notification.ID,
		notification.UserID,
		string(notification.Type),
		notification.Severity,
		notification.Title,
		notification.Message,
		notification.DeviceID,
		notification.PortID,
		notification.SourceEventID,
		notification.DedupeKey,
		unixTimeOrNil(notification.ReadAt),
		unixTimeOrNil(notification.ResolvedAt),
		notification.CreatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("save notification: %w", err)
	}
	return nil
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
		       source_event_id, dedupe_key, read_at, resolved_at, created_at
		FROM notifications
		WHERE user_id = ?
		ORDER BY created_at DESC, id DESC
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
	case "resolved":
		clauses = append(clauses, "resolved_at IS NOT NULL")
	default:
		return model.NotificationPage{}, fmt.Errorf("notification status is invalid")
	}
	if query.CursorID != "" {
		var cursorCreatedAt int64
		err := s.db.QueryRow(`
			SELECT created_at FROM notifications WHERE user_id = ? AND id = ?
		`, query.UserID, query.CursorID).Scan(&cursorCreatedAt)
		if err == sql.ErrNoRows {
			return model.NotificationPage{}, ErrNotificationCursorNotFound
		}
		if err != nil {
			return model.NotificationPage{}, fmt.Errorf("load notification cursor: %w", err)
		}
		clauses = append(clauses, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, cursorCreatedAt, cursorCreatedAt, query.CursorID)
	}
	args = append(args, query.Limit+1)
	rows, err := s.db.Query(`
		SELECT id, user_id, type, severity, title, message, device_id, port_id,
		       source_event_id, dedupe_key, read_at, resolved_at, created_at
		FROM notifications
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY created_at DESC, id DESC
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
		       source_event_id, dedupe_key, read_at, resolved_at, created_at
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

func (s *Store) SaveWatchRefreshState(state model.WatchRefreshState) error {
	if err := validateWatchRefreshState(state); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO watch_refresh_states(
			user_id, device_id, next_attempt_at, last_attempt_at, last_success_at,
			consecutive_failures, paused_reason, quota_date, quota_used, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	err := s.db.QueryRow(`
		SELECT user_id, device_id, next_attempt_at, last_attempt_at, last_success_at,
		       consecutive_failures, paused_reason, quota_date, quota_used, updated_at
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
	state.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return state, true, nil
}

func validateWatchRule(rule model.WatchRule) error {
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.UserID) == "" || strings.TrimSpace(rule.DeviceID) == "" {
		return fmt.Errorf("watch rule requires id, user, and pile")
	}
	if rule.PortID != nil && *rule.PortID <= 0 {
		return fmt.Errorf("watch rule port must be positive")
	}
	if rule.NotifyIdle && rule.PortID == nil {
		return fmt.Errorf("idle notification requires a port")
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
	return nil
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
	case model.NotificationPortIdle, model.NotificationCredentialExpired,
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
	if notification.Type == model.NotificationPortIdle &&
		(strings.TrimSpace(notification.DeviceID) == "" || notification.PortID == nil || notification.SourceEventID == nil) {
		return fmt.Errorf("idle notification requires pile, port, and source event")
	}
	if notification.CreatedAt.IsZero() {
		return fmt.Errorf("notification requires created time")
	}
	return nil
}

func validateWatchRefreshState(state model.WatchRefreshState) error {
	if strings.TrimSpace(state.UserID) == "" || strings.TrimSpace(state.DeviceID) == "" {
		return fmt.Errorf("watch refresh state requires user and pile")
	}
	if state.NextAttemptAt.IsZero() || state.UpdatedAt.IsZero() {
		return fmt.Errorf("watch refresh state requires timestamps")
	}
	if state.ConsecutiveFailures < 0 || state.QuotaUsed < 0 {
		return fmt.Errorf("watch refresh state counters cannot be negative")
	}
	return nil
}

func scanNotification(scanner interface{ Scan(...any) error }) (model.Notification, error) {
	var notification model.Notification
	var notificationType string
	var portID, sourceEventID, readAt, resolvedAt sql.NullInt64
	var createdAt int64
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
		&createdAt,
	); err != nil {
		return model.Notification{}, fmt.Errorf("scan notification: %w", err)
	}
	notification.Type = model.NotificationType(notificationType)
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
	notification.CreatedAt = time.Unix(createdAt, 0).UTC()
	return notification, nil
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
