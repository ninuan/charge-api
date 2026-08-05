package runtime

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

func (m *Manager) OperationsStatus() (model.OperationsStatus, error) {
	settings := normalizeRegistrationSettings(m.Settings())
	return m.repository.OperationsStatus(
		settings.StatsRetentionDays,
		settings.PortHistoryRetentionDays,
	)
}

func (m *Manager) RecordHealthCheck(
	service string,
	health model.ServiceHealth,
) (model.ServiceHealth, error) {
	now := time.Now()
	if err := m.repository.RecordHealthCheck(service, health, now); err != nil {
		return health, err
	}
	return m.repository.HealthSummary(service, health, now)
}

func (m *Manager) AdminStats() model.AdminStats {
	stats, _ := m.AdminStatsResult()
	return stats
}

func (m *Manager) AdminStatsResult() (model.AdminStats, error) {
	hourly, err := m.repository.MetricSeries(time.Now().Add(-24*time.Hour), 3600)
	if err != nil {
		return model.AdminStats{}, fmt.Errorf("load hourly admin metrics: %w", err)
	}
	daily, err := m.repository.MetricSeries(time.Now().AddDate(0, 0, -30), 86400)
	if err != nil {
		return model.AdminStats{}, fmt.Errorf("load daily admin metrics: %w", err)
	}
	users := m.ListUsers()
	exceptions := make([]model.SystemException, 0)
	now := time.Now()
	for _, summary := range users {
		if summary.User.Role != model.RoleUser {
			continue
		}
		if len(summary.DeviceIDs) > 0 && summary.Credential.State != model.CredentialHealthy {
			exceptions = append(exceptions, model.SystemException{
				ID:       "credential-" + summary.User.ID,
				UserID:   summary.User.ID,
				Username: summary.User.Username,
				Type:     "credential",
				Level:    credentialIssueLevel(summary.Credential.State),
				Message:  credentialIssueMessage(summary.Credential.State),
				Time:     summary.SnapshotUpdatedAt,
			})
		}
		if hasActiveAuthFailure(summary.Stats) {
			at := *summary.Stats.LastAuthFailureAt
			exceptions = append(exceptions, model.SystemException{ID: "auth-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "cookie_expired", Level: "critical", Message: "远端鉴权失败，Cookie 可能已失效", Time: at})
		}
		if summary.LastRefresh.FailedDevices > 0 {
			exceptions = append(exceptions, model.SystemException{ID: "refresh-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "refresh", Level: "warning", Message: fmt.Sprintf("%d 台设备刷新失败", summary.LastRefresh.FailedDevices), Time: issueTime(summary)})
		}
		if summary.LastRefresh.LastRemoteAt != nil && now.Sub(*summary.LastRefresh.LastRemoteAt) > 24*time.Hour {
			exceptions = append(exceptions, model.SystemException{ID: "stale-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "stale", Level: "warning", Message: "设备数据已超过 24 小时未更新", Time: *summary.LastRefresh.LastRemoteAt})
		}
		if summary.Dashboard.OfflinePorts > 0 {
			exceptions = append(exceptions, model.SystemException{ID: "offline-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "offline", Level: "warning", Message: fmt.Sprintf("%d 个充电口处于离线状态", summary.Dashboard.OfflinePorts), Time: summary.SnapshotUpdatedAt})
		}
		for _, diagnostic := range summary.RecoveryDiagnostics {
			if !isActionableDiagnostic(diagnostic) {
				continue
			}
			exceptions = append(exceptions, model.SystemException{
				ID:       "diagnostic-" + summary.User.ID + "-" + diagnostic.DeviceSuffix + "-" + diagnostic.Code,
				UserID:   summary.User.ID,
				Username: summary.User.Username,
				DeviceID: diagnostic.DeviceSuffix,
				Type:     "operation",
				Level:    "warning",
				Message:  diagnostic.Message,
				Time:     diagnostic.At,
			})
		}
	}
	settings := normalizeRegistrationSettings(m.Settings())
	operations, operationsErr := m.repository.OperationsStatus(
		settings.StatsRetentionDays,
		settings.PortHistoryRetentionDays,
	)
	if operationsErr != nil {
		exceptions = append(exceptions, model.SystemException{
			ID: "operations-database", Username: "系统", Type: "database",
			Level: "critical", Message: "数据库运维检查失败",
			Time: now.Truncate(24 * time.Hour),
		})
	} else if operations.BackupState != "healthy" {
		at := now.Truncate(24 * time.Hour)
		if operations.LastBackupAt != nil {
			at = *operations.LastBackupAt
		}
		exceptions = append(exceptions, model.SystemException{
			ID: "operations-backup", Username: "系统", Type: "backup",
			Level: "warning", Message: operations.BackupMessage, Time: at,
		})
	}
	sort.SliceStable(exceptions, func(i, j int) bool {
		if exceptions[i].Level != exceptions[j].Level {
			return exceptions[i].Level == "critical"
		}
		return exceptions[i].Time.After(exceptions[j].Time)
	})
	if hourly == nil {
		hourly = []model.MetricPoint{}
	}
	if daily == nil {
		daily = []model.MetricPoint{}
	}
	if users == nil {
		users = []model.AdminUserSummary{}
	}
	activeIssueIDs := make(map[string]struct{}, len(exceptions))
	for _, issue := range exceptions {
		activeIssueIDs[issue.ID] = struct{}{}
		if err := m.repository.UpsertIncident(issue); err != nil {
			return model.AdminStats{}, fmt.Errorf("sync admin incident: %w", err)
		}
	}
	persisted, err := m.repository.ListIncidents("", "", "")
	if err != nil {
		return model.AdminStats{}, fmt.Errorf("load admin incidents: %w", err)
	}
	for _, issue := range persisted {
		if _, active := activeIssueIDs[issue.ID]; active || issue.Status == "resolved" || !isDerivedAdminIncident(issue.ID) {
			continue
		}
		if _, err := m.repository.UpdateIncident(
			issue.ID, "resolved", "问题已不再出现，系统自动标记为已解决", "系统", now,
		); err != nil {
			return model.AdminStats{}, fmt.Errorf("resolve inactive admin incident: %w", err)
		}
	}
	persisted, err = m.repository.ListIncidents("", "", "")
	if err != nil {
		return model.AdminStats{}, fmt.Errorf("reload admin incidents: %w", err)
	}
	activeIncidents := make([]model.SystemException, 0, len(persisted))
	for _, issue := range persisted {
		if issue.Status != "resolved" {
			activeIncidents = append(activeIncidents, issue)
		}
	}
	overview := adminOverview(users, hourly, len(activeIncidents))
	return model.AdminStats{
		Overview: overview, Users: users, Hourly: hourly, Daily: daily,
		Exceptions: activeIncidents,
	}, nil
}

func isDerivedAdminIncident(id string) bool {
	for _, prefix := range []string{
		"credential-", "auth-", "refresh-", "stale-", "offline-", "diagnostic-",
		"operations-",
	} {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

func isActionableDiagnostic(diagnostic model.RecoveryDiagnostic) bool {
	switch diagnostic.Code {
	case "pile_identifier_required", "pile_id_invalid", "pile_number_invalid", "pile_fields_invalid", "pile_port_count_invalid",
		"add_pile_failed", "pile_update_failed", "pile_delete_failed", "refresh_failed", "cookie_required", "cookie_too_large",
		"cookie_update_failed", "qr_create_failed", "qr_poll_failed", "qr_confirm_failed", "qr_session_invalid",
		"scan_service_unavailable", "binding_save_failed", "credential_sync_failed", "device_id_invalid", "auth_rate_limited",
		"recovery_unavailable", "binding_missing", "yyb_get_code_failed", "yyb_account_refresh_failed", "yyb_get_code_retry_failed",
		"mocele_autologin_missing_info", "mocele_autologin_missing_wxopenid", "mocele_autologin_failed", "new_cookie_validation_failed", "recovery_failed":
		return true
	default:
		return false
	}
}

func credentialIssueLevel(state model.CredentialState) string {
	if state == model.CredentialExpired {
		return "critical"
	}
	return "warning"
}

func credentialIssueMessage(state model.CredentialState) string {
	switch state {
	case model.CredentialExpired:
		return "扫码登录绑定已失效"
	case model.CredentialSyncFailed:
		return "扫码登录凭据同步失败"
	default:
		return "尚未完成扫码登录绑定"
	}
}

func issueTime(summary model.AdminUserSummary) time.Time {
	if summary.LastRefresh.LastRemoteAt != nil {
		return *summary.LastRefresh.LastRemoteAt
	}
	return summary.SnapshotUpdatedAt
}

func hasActiveAuthFailure(stats model.TrafficStats) bool {
	if stats.AuthFailures == 0 || stats.LastAuthFailureAt == nil {
		return false
	}
	if stats.LastRemoteOKAt == nil {
		return true
	}
	return stats.LastAuthFailureAt.After(*stats.LastRemoteOKAt)
}

func adminOverview(users []model.AdminUserSummary, hourly []model.MetricPoint, issueCount int) model.AdminOverview {
	remote := 0
	remoteOK := 0
	active := map[string]struct{}{}
	managed := 0
	offline := 0
	for _, point := range hourly {
		remote += point.Remote
		remoteOK += point.RemoteOK
	}
	for _, summary := range users {
		managed += len(summary.DeviceIDs)
		offline += summary.Dashboard.OfflinePorts
		if summary.User.Role == model.RoleUser && summary.Stats.LastRequestAt != nil &&
			time.Since(*summary.Stats.LastRequestAt) <= 24*time.Hour {
			active[summary.User.ID] = struct{}{}
		}
	}
	rate := 0.0
	if remote > 0 {
		rate = math.Round(float64(remoteOK)/float64(remote)*1000) / 10
	}
	return model.AdminOverview{
		OpenIssues: issueCount, RemoteSuccessRate: rate,
		ActiveUsers: len(active), ManagedDevices: managed, OfflinePorts: offline,
	}
}

func (m *Manager) recordMetric(userID, kind string) {
	m.recordMetricCount(userID, kind, 1)
}

func (m *Manager) recordMetricCount(userID, kind string, count int) {
	if count <= 0 {
		return
	}
	now := time.Now()
	if kind == "remote_ok" {
		if runtime, err := m.runtimeFor(userID); err == nil {
			runtime.recordRemoteOK(now)
		}
	}
	_ = m.repository.RecordMetricCount(userID, kind, count, now)
}

func (m *Manager) recordRefreshMetrics(userID string, info model.RefreshInfo) {
	m.recordMetricCount(userID, "remote", info.AttemptedDevices)
	m.recordMetricCount(userID, "remote_ok", info.SuccessfulDevices)
	m.recordMetricCount(userID, "remote_failed", info.FailedDevices)
}
