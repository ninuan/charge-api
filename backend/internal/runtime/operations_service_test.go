package runtime

import (
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
)

func TestOperationsStatusSummarizesReminderSchedulerAndDegradesAfterRepeatedFailures(t *testing.T) {
	manager, err := NewManager(
		testRepository(t),
		"",
		parser.DefaultCaptureRequests(),
		"admin-password-123",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	settings := manager.Settings()
	settings.ScheduledPowerOffEnabled = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("disable power-off window: %v", err)
	}
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "operations-user", Password: "operations-password", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := manager.repository.SaveWatchRule(model.WatchRule{
		ID: "operations-watch", UserID: user.ID, DeviceID: "620100000001",
		Enabled: true, ActiveWeekdays: 127, Timezone: "Asia/Shanghai",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveWatchRule: %v", err)
	}
	if err := manager.repository.SaveWatchRefreshState(model.WatchRefreshState{
		UserID: user.ID, DeviceID: "620100000001", NextAttemptAt: now.Add(time.Minute),
		QuotaDate: now.Format("2006-01-02"), UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveWatchRefreshState: %v", err)
	}
	for kind, count := range map[string]int{
		"watch_remote": 4, "watch_remote_ok": 3, "watch_remote_failed": 1,
		"watch_cache": 2, "watch_coalesced": 1,
	} {
		if err := manager.repository.RecordMetricCount(user.ID, kind, count, now); err != nil {
			t.Fatalf("RecordMetricCount %s: %v", kind, err)
		}
	}
	manager.reminderScheduler.mu.Lock()
	manager.reminderScheduler.running = true
	manager.reminderScheduler.mu.Unlock()

	status, err := manager.OperationsStatus()
	if err != nil {
		t.Fatalf("OperationsStatus: %v", err)
	}
	if status.Reminders.State != "healthy" || status.Reminders.TrackedPiles != 1 ||
		status.Reminders.RemoteSuccessRate24Hours != 75 || status.Reminders.CacheHits24Hours != 2 ||
		status.Reminders.Coalesced24Hours != 1 || status.NotificationRetentionDays != 90 {
		t.Fatalf("unexpected reminder operations status: %+v", status.Reminders)
	}
	if err := manager.repository.RecordMetricCount(user.ID, "watch_remote", 3, time.Now()); err != nil {
		t.Fatalf("RecordMetricCount repeated attempts: %v", err)
	}
	if err := manager.repository.RecordMetricCount(user.ID, "watch_remote_failed", 3, time.Now()); err != nil {
		t.Fatalf("RecordMetricCount repeated failures: %v", err)
	}
	status, err = manager.OperationsStatus()
	if err != nil {
		t.Fatalf("OperationsStatus degraded: %v", err)
	}
	if status.Reminders.State != "degraded" {
		t.Fatalf("repeated reminder failures did not degrade operations: %+v", status.Reminders)
	}
	stats, err := manager.AdminStatsResult()
	if err != nil {
		t.Fatalf("AdminStatsResult: %v", err)
	}
	found := false
	for _, issue := range stats.Exceptions {
		if issue.ID == "operations-reminder" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("reminder degradation missing from admin incidents: %+v", stats.Exceptions)
	}
}
