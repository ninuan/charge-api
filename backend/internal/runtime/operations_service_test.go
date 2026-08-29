package runtime

import (
	"encoding/json"
	"strings"
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
	expiresAt := now.Add(time.Hour)
	if err := manager.repository.SaveWatchRule(model.WatchRule{
		ID: "operations-watch", UserID: user.ID, DeviceID: "620100000001",
		Mode: model.WatchRuleTemporary, Enabled: true, ActiveWeekdays: 127,
		Timezone: "Asia/Shanghai", ExpiresAt: &expiresAt,
		StopAfterNotify: true,
		CreatedAt:       now, UpdatedAt: now,
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

func TestOperationsStatusCreatesWxPusherSystemIncidentWithoutExposingProviderIdentifiers(t *testing.T) {
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
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "wx-operations-user", Password: "operations-password", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	saveTestWxPusherBinding(t, manager, user.ID, now, true, model.WxPusherDefaultEventTypes)
	manager.notificationDispatcher.mu.Lock()
	manager.notificationDispatcher.client = &fakeNotificationDeliveryClient{}
	manager.notificationDispatcher.running = true
	manager.notificationDispatcher.mu.Unlock()
	for index := 0; index < 3; index++ {
		if err := manager.repository.SaveNotificationDelivery(model.NotificationDelivery{
			ID: "wx-system-failure-" + string(rune('a'+index)), UserID: user.ID,
			Status: model.NotificationDeliveryFailed, IsTest: true, AttemptCount: 1,
			LastErrorCode: "wxpusher_invalid_token",
			CreatedAt:     now.Add(-time.Duration(10-index) * time.Minute),
			UpdatedAt:     now.Add(-time.Duration(3-index) * time.Minute),
		}); err != nil {
			t.Fatalf("SaveNotificationDelivery %d: %v", index, err)
		}
	}
	if err := manager.repository.SaveNotificationDelivery(model.NotificationDelivery{
		ID: "wx-queue-delayed", UserID: user.ID, Status: model.NotificationDeliveryPending,
		IsTest: true, CreatedAt: now.Add(-45 * time.Minute), UpdatedAt: now.Add(-45 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveNotificationDelivery delayed queue: %v", err)
	}
	status, err := manager.OperationsStatus()
	if err != nil {
		t.Fatalf("OperationsStatus: %v", err)
	}
	if status.WxPusher.State != "degraded" || status.WxPusher.ConsecutiveSystemFailures != 3 ||
		status.WxPusher.LastErrorCategory != "configuration" {
		t.Fatalf("unexpected wxpusher operations status: %+v", status.WxPusher)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal operations status: %v", err)
	}
	if strings.Contains(string(encoded), "UID_dispatcher") || strings.Contains(string(encoded), user.ID) {
		t.Fatalf("operations status exposed a WxPusher UID or user identifier: %s", encoded)
	}
	stats, err := manager.AdminStatsResult()
	if err != nil {
		t.Fatalf("AdminStatsResult: %v", err)
	}
	foundSystem, foundQueue := false, false
	for _, issue := range stats.Exceptions {
		if issue.ID == "operations-wxpusher-system" {
			foundSystem = true
			if issue.UserID != "" || issue.DeviceID != "" {
				t.Fatalf("system incident exposed user or provider identity: %+v", issue)
			}
		}
		if issue.ID == "operations-wxpusher-queue" {
			foundQueue = true
		}
	}
	if !foundSystem || !foundQueue {
		t.Fatalf("wxpusher incidents missing: %+v", stats.Exceptions)
	}
	for index := 0; index < 3; index++ {
		id := "wx-system-failure-" + string(rune('a'+index))
		delivery, ok, err := manager.repository.LoadNotificationDelivery(user.ID, id)
		if err != nil || !ok {
			t.Fatalf("LoadNotificationDelivery %s: ok=%v err=%v", id, ok, err)
		}
		delivery.Status = model.NotificationDeliveryProviderSucceeded
		delivery.LastErrorCode = ""
		delivery.AcceptedAt, delivery.ProviderSucceededAt = &now, &now
		delivery.UpdatedAt = now
		if err := manager.repository.SaveNotificationDelivery(delivery); err != nil {
			t.Fatalf("recover delivery %s: %v", id, err)
		}
	}
	delayed, ok, err := manager.repository.LoadNotificationDelivery(user.ID, "wx-queue-delayed")
	if err != nil || !ok {
		t.Fatalf("LoadNotificationDelivery delayed queue: ok=%v err=%v", ok, err)
	}
	delayed.Status, delayed.UpdatedAt = model.NotificationDeliveryCancelled, now
	if err := manager.repository.SaveNotificationDelivery(delayed); err != nil {
		t.Fatalf("recover delayed queue: %v", err)
	}
	recoveredStats, err := manager.AdminStatsResult()
	if err != nil {
		t.Fatalf("AdminStatsResult recovered: %v", err)
	}
	for _, issue := range recoveredStats.Exceptions {
		if issue.ID == "operations-wxpusher-system" || issue.ID == "operations-wxpusher-queue" {
			t.Fatalf("recovered wxpusher incident remained active: %+v", issue)
		}
	}
}
