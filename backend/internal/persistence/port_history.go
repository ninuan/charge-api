package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

const defaultPortStatusEventLimit = 1000

type PortStatusEventQuery struct {
	UserID   string
	DeviceID string
	PortID   *int
	Since    time.Time
	Until    time.Time
	Limit    int
}

func (s *Store) RecordPortStatusEvents(events []model.PortStatusEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin port status event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statement, err := tx.Prepare(`
		INSERT INTO port_status_events(
			user_id, device_id, port_id, from_status, to_status, changed_at,
			used_seconds, remaining_text, source
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare port status event insert: %w", err)
	}
	defer statement.Close()

	for _, event := range events {
		if err := validatePortStatusEvent(event); err != nil {
			return err
		}
		var fromStatus any
		if event.FromStatus != nil {
			fromStatus = string(*event.FromStatus)
		}
		source := strings.TrimSpace(event.Source)
		if source == "" {
			source = "remote"
		}
		if _, err := statement.Exec(
			event.UserID,
			event.DeviceID,
			event.PortID,
			fromStatus,
			string(event.ToStatus),
			event.ChangedAt.Unix(),
			event.UsedSeconds,
			event.RemainingText,
			source,
		); err != nil {
			return fmt.Errorf("insert port status event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit port status events: %w", err)
	}
	return nil
}

func validatePortStatusEvent(event model.PortStatusEvent) error {
	if strings.TrimSpace(event.UserID) == "" || strings.TrimSpace(event.DeviceID) == "" {
		return fmt.Errorf("port status event requires user and device")
	}
	if event.PortID <= 0 {
		return fmt.Errorf("port status event requires a positive port id")
	}
	if event.ChangedAt.IsZero() {
		return fmt.Errorf("port status event requires changed time")
	}
	if event.UsedSeconds < 0 {
		return fmt.Errorf("port status event used seconds cannot be negative")
	}
	if event.FromStatus != nil && !validPortStatus(*event.FromStatus) {
		return fmt.Errorf("invalid previous port status %q", *event.FromStatus)
	}
	if !validPortStatus(event.ToStatus) {
		return fmt.Errorf("invalid port status %q", event.ToStatus)
	}
	return nil
}

func validPortStatus(status model.PortStatus) bool {
	switch status {
	case model.PortIdle, model.PortInUse, model.PortOffline:
		return true
	default:
		return false
	}
}

func (s *Store) LatestPortStatuses(userID, deviceID string) (map[int]model.PortStatusEvent, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, device_id, port_id, from_status, to_status,
		       changed_at, used_seconds, remaining_text, source
		FROM (
			SELECT *, ROW_NUMBER() OVER (
				PARTITION BY port_id ORDER BY changed_at DESC, id DESC
			) AS position
			FROM port_status_events
			WHERE user_id = ? AND device_id = ?
		)
		WHERE position = 1
		ORDER BY port_id
	`, userID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("query latest port statuses: %w", err)
	}
	defer rows.Close()

	result := make(map[int]model.PortStatusEvent)
	for rows.Next() {
		event, err := scanPortStatusEvent(rows)
		if err != nil {
			return nil, err
		}
		result[event.PortID] = event
	}
	return result, rows.Err()
}

func (s *Store) PortStatusEvents(query PortStatusEventQuery) ([]model.PortStatusEvent, error) {
	clauses := []string{"user_id = ?", "device_id = ?"}
	args := []any{query.UserID, query.DeviceID}
	if query.PortID != nil {
		clauses = append(clauses, "port_id = ?")
		args = append(args, *query.PortID)
	}
	if !query.Since.IsZero() {
		clauses = append(clauses, "changed_at >= ?")
		args = append(args, query.Since.Unix())
	}
	if !query.Until.IsZero() {
		clauses = append(clauses, "changed_at < ?")
		args = append(args, query.Until.Unix())
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultPortStatusEventLimit
	}
	args = append(args, limit)

	rows, err := s.db.Query(`
		SELECT id, user_id, device_id, port_id, from_status, to_status,
		       changed_at, used_seconds, remaining_text, source
		FROM port_status_events
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY changed_at, id
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query port status events: %w", err)
	}
	defer rows.Close()

	result := make([]model.PortStatusEvent, 0)
	for rows.Next() {
		event, err := scanPortStatusEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPortStatusEvent(row rowScanner) (model.PortStatusEvent, error) {
	var event model.PortStatusEvent
	var fromStatus sql.NullString
	var toStatus string
	var changedAt int64
	if err := row.Scan(
		&event.ID,
		&event.UserID,
		&event.DeviceID,
		&event.PortID,
		&fromStatus,
		&toStatus,
		&changedAt,
		&event.UsedSeconds,
		&event.RemainingText,
		&event.Source,
	); err != nil {
		return event, fmt.Errorf("scan port status event: %w", err)
	}
	if fromStatus.Valid {
		status := model.PortStatus(fromStatus.String)
		event.FromStatus = &status
	}
	event.ToStatus = model.PortStatus(toStatus)
	event.ChangedAt = time.Unix(changedAt, 0).UTC()
	return event, nil
}

func (s *Store) DeletePortStatusEvents(userID, deviceID string) (int64, error) {
	result, err := s.db.Exec(
		`DELETE FROM port_status_events WHERE user_id = ? AND device_id = ?`,
		userID,
		deviceID,
	)
	if err != nil {
		return 0, fmt.Errorf("delete port status events: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) PrunePortStatusEvents(before time.Time) (int64, error) {
	result, err := s.db.Exec(
		`DELETE FROM port_status_events WHERE changed_at < ?`,
		before.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("prune port status events: %w", err)
	}
	return result.RowsAffected()
}
