package persistence

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

func (s *Store) RecordAudit(entry model.AuditEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO admin_audit_logs(
			actor_id, actor, action, target_type, target_id, target_label,
			result, message, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.ActorID, entry.Actor, entry.Action, entry.TargetType,
		entry.TargetID, entry.TargetLabel, entry.Result, entry.Message,
		entry.CreatedAt.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("record admin audit: %w", err)
	}
	return nil
}

func (s *Store) ListAudit(page, pageSize int) (model.AuditPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_audit_logs`).Scan(&total); err != nil {
		return model.AuditPage{}, fmt.Errorf("count admin audit: %w", err)
	}
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	rows, err := s.db.Query(`
		SELECT id, actor_id, actor, action, target_type, target_id,
		       target_label, result, message, created_at
		FROM admin_audit_logs
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, pageSize, (page-1)*pageSize)
	if err != nil {
		return model.AuditPage{}, fmt.Errorf("list admin audit: %w", err)
	}
	defer rows.Close()
	items := make([]model.AuditEntry, 0)
	for rows.Next() {
		var entry model.AuditEntry
		var createdAt int64
		if err := rows.Scan(
			&entry.ID, &entry.ActorID, &entry.Actor, &entry.Action,
			&entry.TargetType, &entry.TargetID, &entry.TargetLabel,
			&entry.Result, &entry.Message, &createdAt,
		); err != nil {
			return model.AuditPage{}, fmt.Errorf("scan admin audit: %w", err)
		}
		entry.CreatedAt = time.Unix(0, createdAt)
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return model.AuditPage{}, fmt.Errorf("iterate admin audit: %w", err)
	}
	return model.AuditPage{
		Items: items, Page: page, PageSize: pageSize,
		Total: total, TotalPages: totalPages,
	}, nil
}

func (s *Store) UpsertIncident(issue model.SystemException) error {
	firstSeenAt := issue.FirstSeenAt
	if firstSeenAt.IsZero() {
		firstSeenAt = issue.Time
	}
	_, err := s.db.Exec(`
		INSERT INTO admin_incidents(
			id, user_id, username, device_id, type, level, message, status,
			note, occurrences, handled_by, handled_at, first_seen_at, last_seen_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, 'open', '', 1, '', NULL, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			username=excluded.username,
			level=excluded.level,
			message=excluded.message,
			occurrences=admin_incidents.occurrences +
				CASE WHEN excluded.last_seen_at > admin_incidents.last_seen_at THEN 1 ELSE 0 END,
			status=CASE
				WHEN excluded.last_seen_at > admin_incidents.last_seen_at
					AND admin_incidents.status='resolved' THEN 'open'
				ELSE admin_incidents.status
			END,
			handled_by=CASE
				WHEN excluded.last_seen_at > admin_incidents.last_seen_at
					AND admin_incidents.status='resolved' THEN ''
				ELSE admin_incidents.handled_by
			END,
			handled_at=CASE
				WHEN excluded.last_seen_at > admin_incidents.last_seen_at
					AND admin_incidents.status='resolved' THEN NULL
				ELSE admin_incidents.handled_at
			END,
			last_seen_at=MAX(admin_incidents.last_seen_at, excluded.last_seen_at)
	`,
		issue.ID, issue.UserID, issue.Username, issue.DeviceID, issue.Type,
		issue.Level, issue.Message, firstSeenAt.UnixNano(), issue.Time.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("upsert admin incident: %w", err)
	}
	return nil
}

func (s *Store) ListIncidents(status, issueType, level string) ([]model.SystemException, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, username, device_id, type, level, message, status,
		       note, occurrences, handled_by, handled_at, first_seen_at, last_seen_at
		FROM admin_incidents
		WHERE (?='' OR status=?)
		  AND (?='' OR type=?)
		  AND (?='' OR level=?)
		ORDER BY CASE status WHEN 'open' THEN 0 WHEN 'acknowledged' THEN 1 ELSE 2 END,
		         CASE level WHEN 'critical' THEN 0 ELSE 1 END,
		         last_seen_at DESC
	`, status, status, issueType, issueType, level, level)
	if err != nil {
		return nil, fmt.Errorf("list admin incidents: %w", err)
	}
	defer rows.Close()
	items := make([]model.SystemException, 0)
	for rows.Next() {
		var issue model.SystemException
		var handledAt sql.NullInt64
		var firstSeenAt, lastSeenAt int64
		if err := rows.Scan(
			&issue.ID, &issue.UserID, &issue.Username, &issue.DeviceID,
			&issue.Type, &issue.Level, &issue.Message, &issue.Status,
			&issue.Note, &issue.Occurrences, &issue.HandledBy, &handledAt,
			&firstSeenAt, &lastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan admin incident: %w", err)
		}
		issue.FirstSeenAt = time.Unix(0, firstSeenAt)
		issue.Time = time.Unix(0, lastSeenAt)
		if handledAt.Valid {
			at := time.Unix(0, handledAt.Int64)
			issue.HandledAt = &at
		}
		items = append(items, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin incidents: %w", err)
	}
	return items, nil
}

func (s *Store) UpdateIncident(id, status, note, handler string, at time.Time) (model.SystemException, error) {
	result, err := s.db.Exec(`
		UPDATE admin_incidents
		SET status=?, note=?, handled_by=?, handled_at=?
		WHERE id=?
	`, status, strings.TrimSpace(note), handler, at.UnixNano(), id)
	if err != nil {
		return model.SystemException{}, fmt.Errorf("update admin incident: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return model.SystemException{}, fmt.Errorf("read incident update result: %w", err)
	}
	if changed == 0 {
		return model.SystemException{}, fmt.Errorf("incident not found")
	}
	items, err := s.ListIncidents("", "", "")
	if err != nil {
		return model.SystemException{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return model.SystemException{}, fmt.Errorf("incident not found")
}

func (s *Store) RecordHealthCheck(service string, health model.ServiceHealth, at time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO service_health_checks(service, state, message, checked_at)
		VALUES(?, ?, ?, ?)
	`, service, health.State, health.Message, at.UnixNano())
	if err != nil {
		return fmt.Errorf("record service health: %w", err)
	}
	_, _ = s.db.Exec(
		`DELETE FROM service_health_checks WHERE checked_at < ?`,
		at.Add(-7*24*time.Hour).UnixNano(),
	)
	return nil
}

func (s *Store) HealthSummary(service string, current model.ServiceHealth, now time.Time) (model.ServiceHealth, error) {
	rows, err := s.db.Query(`
		SELECT state, checked_at
		FROM service_health_checks
		WHERE service=? AND checked_at>=?
		ORDER BY checked_at ASC
	`, service, now.Add(-24*time.Hour).UnixNano())
	if err != nil {
		return current, fmt.Errorf("query service health history: %w", err)
	}
	defer rows.Close()
	total := 0
	healthy := 0
	failures := 0
	var lastRecoveredAt *time.Time
	var previous model.HealthState
	for rows.Next() {
		var state model.HealthState
		var checkedAt int64
		if err := rows.Scan(&state, &checkedAt); err != nil {
			return current, fmt.Errorf("scan service health history: %w", err)
		}
		total++
		if state == model.HealthHealthy {
			healthy++
			failures = 0
			if previous != "" && previous != model.HealthHealthy {
				at := time.Unix(0, checkedAt)
				lastRecoveredAt = &at
			}
		} else {
			failures++
		}
		previous = state
	}
	if err := rows.Err(); err != nil {
		return current, fmt.Errorf("iterate service health history: %w", err)
	}
	current.ConsecutiveFailures = failures
	current.LastRecoveredAt = lastRecoveredAt
	if total > 0 {
		current.Availability24Hours = math.Round(float64(healthy)/float64(total)*1000) / 10
	}
	return current, nil
}

func (s *Store) OperationsStatus(metricRetentionDays, portHistoryRetentionDays int) (model.OperationsStatus, error) {
	result := model.OperationsStatus{
		MetricRetentionDays:      metricRetentionDays,
		PortHistoryRetentionDays: portHistoryRetentionDays,
		CheckedAt:                time.Now(),
		BackupState:              "unavailable",
		BackupMessage:            "尚未发现数据库备份；请检查定时备份任务。",
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return result, fmt.Errorf("stat database: %w", err)
	}
	result.DatabaseSizeBytes = info.Size()
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM metrics`).Scan(&result.MetricRows); err != nil {
		return result, fmt.Errorf("count metrics: %w", err)
	}
	var oldestHistory, newestHistory sql.NullInt64
	if err := s.db.QueryRow(`
		SELECT COUNT(*), MIN(changed_at), MAX(changed_at)
		FROM port_status_events
	`).Scan(&result.PortHistoryRows, &oldestHistory, &newestHistory); err != nil {
		return result, fmt.Errorf("summarize port history: %w", err)
	}
	if oldestHistory.Valid {
		at := time.Unix(oldestHistory.Int64, 0)
		result.PortHistoryOldestAt = &at
	}
	if newestHistory.Valid {
		at := time.Unix(newestHistory.Int64, 0)
		result.PortHistoryNewestAt = &at
	}
	if err := s.db.QueryRow(`PRAGMA quick_check`).Scan(&result.IntegrityResult); err != nil {
		return result, fmt.Errorf("check database integrity: %w", err)
	}

	backupDir := strings.TrimSpace(os.Getenv("CHARGE_BACKUP_DIR"))
	if backupDir == "" {
		backupDir = filepath.Join(filepath.Dir(s.path), "backups")
	}
	matches, _ := filepath.Glob(filepath.Join(backupDir, "charge_state-*.db.gz"))
	sort.Strings(matches)
	if len(matches) == 0 {
		return result, nil
	}
	latest := matches[len(matches)-1]
	backupInfo, err := os.Stat(latest)
	if err != nil {
		return result, nil
	}
	at := backupInfo.ModTime()
	result.LastBackupAt = &at
	result.LastBackupSizeBytes = backupInfo.Size()
	result.BackupState = "healthy"
	result.BackupMessage = "已发现最近数据库备份。"
	if time.Since(at) > 48*time.Hour {
		result.BackupState = "degraded"
		result.BackupMessage = "最近数据库备份已超过 48 小时，请检查定时任务。"
	}
	return result, nil
}
