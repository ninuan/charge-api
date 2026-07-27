package runtime

import (
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

func (m *Manager) RecordAdminAudit(
	actor model.CurrentUser,
	action, targetType, targetID, targetLabel, result, message string,
) error {
	entry := model.AuditEntry{
		ActorID: actor.ID, Actor: actor.Username, Action: action,
		TargetType: targetType, TargetID: targetID,
		TargetLabel: targetLabel, Result: result,
		Message: strings.TrimSpace(message), CreatedAt: time.Now(),
	}
	return m.repository.RecordAudit(entry)
}

func (m *Manager) AuditPage(page, pageSize int) (model.AuditPage, error) {
	return m.repository.ListAudit(page, pageSize)
}

func (m *Manager) Incidents(status, issueType, level string) ([]model.SystemException, error) {
	switch status {
	case "", "open", "acknowledged", "resolved":
	default:
		return nil, fmt.Errorf("invalid incident status")
	}
	return m.repository.ListIncidents(status, issueType, level)
}

func (m *Manager) RecordSystemIncident(issue model.SystemException) error {
	if issue.FirstSeenAt.IsZero() {
		issue.FirstSeenAt = issue.Time
	}
	return m.repository.UpsertIncident(issue)
}

func (m *Manager) UpdateIncident(
	id string,
	req model.IncidentUpdateRequest,
	handler model.CurrentUser,
) (model.SystemException, error) {
	switch req.Status {
	case "open", "acknowledged", "resolved":
	default:
		return model.SystemException{}, fmt.Errorf("invalid incident status")
	}
	if len(strings.TrimSpace(req.Note)) > 500 {
		return model.SystemException{}, fmt.Errorf("处理备注不能超过 500 个字符")
	}
	return m.repository.UpdateIncident(id, req.Status, req.Note, handler.Username, time.Now())
}
