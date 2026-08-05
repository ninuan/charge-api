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

// RecordPortStatusTransitions compares fresh remote piles with the latest
// persisted status for every port and records only baselines or real changes.
// The latest-state reads and inserts share one transaction so concurrent
// refreshes cannot manufacture duplicate transitions.
func (s *Store) RecordPortStatusTransitions(userID string, piles []model.Pile) ([]model.PortStatusEvent, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("port status transitions require a user")
	}
	if len(piles) == 0 {
		return []model.PortStatusEvent{}, nil
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("begin port status transition transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	latestStatement, err := tx.Prepare(`
		SELECT to_status, changed_at
		FROM port_status_events
		WHERE user_id = ? AND device_id = ? AND port_id = ?
		ORDER BY changed_at DESC, id DESC
		LIMIT 1
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare latest port status query: %w", err)
	}
	defer latestStatement.Close()

	insertStatement, err := tx.Prepare(`
		INSERT INTO port_status_events(
			user_id, device_id, port_id, from_status, to_status, changed_at,
			used_seconds, remaining_text, source
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'remote')
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare port status transition insert: %w", err)
	}
	defer insertStatement.Close()

	observedAt := time.Now().UTC()
	seen := make(map[string]struct{})
	events := make([]model.PortStatusEvent, 0)
	for _, pile := range piles {
		deviceID := strings.TrimSpace(pile.ID)
		if deviceID == "" {
			return nil, fmt.Errorf("port status transition requires a device")
		}
		for _, port := range pile.Ports {
			key := fmt.Sprintf("%s\x00%d", deviceID, port.ID)
			if _, duplicate := seen[key]; duplicate {
				return nil, fmt.Errorf("duplicate port status transition for device %s port %d", deviceID, port.ID)
			}
			seen[key] = struct{}{}
			if port.ID <= 0 {
				return nil, fmt.Errorf("port status transition requires a positive port id")
			}
			if !validPortStatus(port.Status) {
				return nil, fmt.Errorf("invalid port status %q", port.Status)
			}
			if port.UsedSeconds < 0 {
				return nil, fmt.Errorf("port status transition used seconds cannot be negative")
			}

			var previousStatus string
			var previousChangedAt int64
			err := latestStatement.QueryRow(userID, deviceID, port.ID).Scan(&previousStatus, &previousChangedAt)
			hasPrevious := err == nil
			if err != nil && err != sql.ErrNoRows {
				return nil, fmt.Errorf("query latest port status transition: %w", err)
			}
			if hasPrevious && previousStatus == string(port.Status) {
				continue
			}

			changedAt := port.UpdatedAt
			if changedAt.IsZero() {
				changedAt = pile.UpdatedAt
			}
			if changedAt.IsZero() {
				changedAt = observedAt
			}
			changedAt = changedAt.UTC().Truncate(time.Second)
			if hasPrevious && changedAt.Unix() < previousChangedAt {
				changedAt = time.Unix(previousChangedAt, 0).UTC()
			}

			var fromStatus any
			var previous *model.PortStatus
			if hasPrevious {
				status := model.PortStatus(previousStatus)
				previous = &status
				fromStatus = previousStatus
			}
			result, err := insertStatement.Exec(
				userID,
				deviceID,
				port.ID,
				fromStatus,
				string(port.Status),
				changedAt.Unix(),
				port.UsedSeconds,
				port.RemainingText,
			)
			if err != nil {
				return nil, fmt.Errorf("insert port status transition: %w", err)
			}
			id, err := result.LastInsertId()
			if err != nil {
				return nil, fmt.Errorf("read port status transition id: %w", err)
			}
			events = append(events, model.PortStatusEvent{
				ID: id, UserID: userID, DeviceID: deviceID, PortID: port.ID,
				FromStatus: previous, ToStatus: port.Status, ChangedAt: changedAt,
				UsedSeconds: port.UsedSeconds, RemainingText: port.RemainingText, Source: "remote",
			})
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit port status transitions: %w", err)
	}
	return events, nil
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

// PortStatusEventsForAnalysis returns the latest event before the requested
// window for each selected port plus every event inside the window. The former
// lets the analytics layer establish the state at the range boundary without
// treating an unknown interval as idle.
func (s *Store) PortStatusEventsForAnalysis(query PortStatusEventQuery, eventLimit int) ([]model.PortStatusEvent, bool, error) {
	if strings.TrimSpace(query.UserID) == "" || strings.TrimSpace(query.DeviceID) == "" {
		return nil, false, fmt.Errorf("port status analysis requires user and device")
	}
	if query.Since.IsZero() || query.Until.IsZero() || !query.Since.Before(query.Until) {
		return nil, false, fmt.Errorf("port status analysis requires a valid time range")
	}
	if eventLimit <= 0 {
		return nil, false, fmt.Errorf("port status analysis requires a positive event limit")
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin port status analysis transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	portClause := ""
	beforeArgs := []any{query.UserID, query.DeviceID, query.Since.Unix()}
	windowArgs := []any{query.UserID, query.DeviceID, query.Since.Unix(), query.Until.Unix()}
	if query.PortID != nil {
		portClause = " AND port_id = ?"
		beforeArgs = append(beforeArgs, *query.PortID)
		windowArgs = append(windowArgs, *query.PortID)
	}

	beforeRows, err := tx.Query(`
		SELECT id, user_id, device_id, port_id, from_status, to_status,
		       changed_at, used_seconds, remaining_text, source
		FROM (
			SELECT *, ROW_NUMBER() OVER (
				PARTITION BY port_id ORDER BY changed_at DESC, id DESC
			) AS position
			FROM port_status_events
			WHERE user_id = ? AND device_id = ? AND changed_at < ?`+portClause+`
		)
		WHERE position = 1
		ORDER BY port_id
	`, beforeArgs...)
	if err != nil {
		return nil, false, fmt.Errorf("query port status range baselines: %w", err)
	}
	events, err := scanPortStatusEvents(beforeRows, 0)
	beforeRows.Close()
	if err != nil {
		return nil, false, err
	}

	windowArgs = append(windowArgs, eventLimit+1)
	windowRows, err := tx.Query(`
		SELECT id, user_id, device_id, port_id, from_status, to_status,
		       changed_at, used_seconds, remaining_text, source
		FROM port_status_events
		WHERE user_id = ? AND device_id = ?
		  AND changed_at >= ? AND changed_at < ?`+portClause+`
		ORDER BY changed_at, id
		LIMIT ?
	`, windowArgs...)
	if err != nil {
		return nil, false, fmt.Errorf("query port status analysis range: %w", err)
	}
	windowEvents, err := scanPortStatusEvents(windowRows, eventLimit+1)
	windowRows.Close()
	if err != nil {
		return nil, false, err
	}
	truncated := len(windowEvents) > eventLimit
	if truncated {
		windowEvents = windowEvents[:eventLimit]
	}
	events = append(events, windowEvents...)

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit port status analysis transaction: %w", err)
	}
	return events, truncated, nil
}

func (s *Store) PortStatusEventStarts(userID, deviceID string) (map[int]time.Time, error) {
	rows, err := s.db.Query(`
		SELECT port_id, MIN(changed_at)
		FROM port_status_events
		WHERE user_id = ? AND device_id = ?
		GROUP BY port_id
		ORDER BY port_id
	`, userID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("query port status event starts: %w", err)
	}
	defer rows.Close()
	starts := make(map[int]time.Time)
	for rows.Next() {
		var portID int
		var changedAt int64
		if err := rows.Scan(&portID, &changedAt); err != nil {
			return nil, fmt.Errorf("scan port status event start: %w", err)
		}
		starts[portID] = time.Unix(changedAt, 0).UTC()
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate port status event starts: %w", err)
	}
	return starts, nil
}

func scanPortStatusEvents(rows *sql.Rows, capacity int) ([]model.PortStatusEvent, error) {
	if capacity < 0 {
		capacity = 0
	}
	events := make([]model.PortStatusEvent, 0, capacity)
	for rows.Next() {
		event, err := scanPortStatusEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate port status events: %w", err)
	}
	return events, nil
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
	if err := s.ensurePortHistoryRetentionAnchors(before); err != nil {
		return 0, err
	}
	return s.pruneRowsInBatches(
		"port_status_events",
		"changed_at",
		before.Unix(),
	)
}

func (s *Store) ensurePortHistoryRetentionAnchors(before time.Time) error {
	cutoff := before.Unix()
	_, err := s.db.Exec(`
		WITH ranked AS (
			SELECT user_id, device_id, port_id, to_status, used_seconds,
			       remaining_text,
			       ROW_NUMBER() OVER (
				   PARTITION BY user_id, device_id, port_id
				   ORDER BY changed_at DESC, id DESC
			   ) AS position
			FROM port_status_events
			WHERE changed_at < ?
		)
		INSERT INTO port_status_events(
			user_id, device_id, port_id, from_status, to_status, changed_at,
			used_seconds, remaining_text, source
		)
		SELECT ranked.user_id, ranked.device_id, ranked.port_id, NULL,
		       ranked.to_status, ?, ranked.used_seconds, ranked.remaining_text,
		       'retention'
		FROM ranked
		WHERE ranked.position = 1
		  AND NOT EXISTS (
			SELECT 1 FROM port_status_events current
			WHERE current.user_id = ranked.user_id
			  AND current.device_id = ranked.device_id
			  AND current.port_id = ranked.port_id
			  AND current.changed_at = ?
		  )
	`, cutoff, cutoff, cutoff)
	if err != nil {
		return fmt.Errorf("create port history retention anchors: %w", err)
	}
	return nil
}
