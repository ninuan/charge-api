package persistence

import (
	"bytes"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestWatchNotificationPersistenceRoundTripAndConstraints(t *testing.T) {
	path := t.TempDir() + "/state.db"
	key := bytes.Repeat([]byte{0x71}, CookieKeySize)
	store, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	user := model.User{
		ID: "user-1", Username: "alice", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Save(State{
		Version: schemaVersion, Users: []model.User{user},
		UserStates: map[string]UserState{user.ID: {}},
	}); err != nil {
		t.Fatalf("Save user: %v", err)
	}

	favorite := model.WatchRule{
		ID: "watch-pile", UserID: user.ID, DeviceID: "pile-1",
		Enabled: true, ActiveWeekdays: 127, Timezone: "Asia/Shanghai",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveWatchRule(favorite); err != nil {
		t.Fatalf("SaveWatchRule favorite: %v", err)
	}
	portID := 1
	reminder := model.WatchRule{
		ID: "watch-port", UserID: user.ID, DeviceID: "pile-1", PortID: &portID,
		NotifyIdle: true, Enabled: true, ActiveWeekdays: 127,
		ActiveStartMinute: 420, ActiveEndMinute: 1380, Timezone: "Asia/Shanghai",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveWatchRule(reminder); err != nil {
		t.Fatalf("SaveWatchRule reminder: %v", err)
	}
	duplicate := reminder
	duplicate.ID = "watch-port-duplicate"
	if err := store.SaveWatchRule(duplicate); err == nil {
		t.Fatal("duplicate user/pile/port watch target was accepted")
	}
	invalidPileReminder := favorite
	invalidPileReminder.ID = "invalid-pile-reminder"
	invalidPileReminder.NotifyIdle = true
	if err := store.SaveWatchRule(invalidPileReminder); err == nil {
		t.Fatal("pile-level idle reminder without a port was accepted")
	}
	foreignUpdate := reminder
	foreignUpdate.UserID = "user-2"
	if err := store.SaveWatchRule(foreignUpdate); err == nil {
		t.Fatal("watch rule ownership change was accepted")
	}
	rules, err := store.ListWatchRules(user.ID)
	if err != nil {
		t.Fatalf("ListWatchRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("watch rule count = %d, want 2", len(rules))
	}

	preference := model.NotificationPreference{
		UserID: user.ID, BrowserEnabled: true, QuietHoursEnabled: true,
		QuietStartMinute: 1320, QuietEndMinute: 480,
		Timezone: "Asia/Shanghai", UpdatedAt: now,
	}
	if err := store.SaveNotificationPreference(preference); err != nil {
		t.Fatalf("SaveNotificationPreference: %v", err)
	}
	loadedPreference, ok, err := store.LoadNotificationPreference(user.ID)
	if err != nil {
		t.Fatalf("LoadNotificationPreference: %v", err)
	}
	if !ok || !loadedPreference.BrowserEnabled || loadedPreference.QuietStartMinute != 1320 {
		t.Fatalf("notification preference did not round-trip: %+v", loadedPreference)
	}

	if err := store.RecordPortStatusEvents([]model.PortStatusEvent{{
		UserID: user.ID, DeviceID: "pile-1", PortID: portID,
		ToStatus: model.PortIdle, ChangedAt: now, Source: "remote",
	}}); err != nil {
		t.Fatalf("RecordPortStatusEvents: %v", err)
	}
	var sourceEventID int64
	if err := store.db.QueryRow(`SELECT id FROM port_status_events WHERE user_id=?`, user.ID).Scan(&sourceEventID); err != nil {
		t.Fatalf("read source event: %v", err)
	}
	idleNotification := model.Notification{
		ID: "notification-idle", UserID: user.ID, Type: model.NotificationPortIdle,
		Severity: "info", Title: "充电口已空闲", Message: "1 号端口现在可以使用",
		DeviceID: "pile-1", PortID: &portID, SourceEventID: &sourceEventID,
		CreatedAt: now,
	}
	if err := store.SaveNotification(idleNotification); err != nil {
		t.Fatalf("SaveNotification idle: %v", err)
	}
	duplicateNotification := idleNotification
	duplicateNotification.ID = "notification-idle-duplicate"
	if err := store.SaveNotification(duplicateNotification); err == nil {
		t.Fatal("duplicate source event notification was accepted")
	}
	if _, err := store.db.Exec(`DELETE FROM port_status_events WHERE id=?`, sourceEventID); err != nil {
		t.Fatalf("delete retained notification source event: %v", err)
	}
	var retainedSourceEventID any
	if err := store.db.QueryRow(`SELECT source_event_id FROM notifications WHERE id=?`, idleNotification.ID).Scan(&retainedSourceEventID); err != nil {
		t.Fatalf("read notification after source event cleanup: %v", err)
	}
	if retainedSourceEventID != nil {
		t.Fatalf("notification source event was not cleared: %v", retainedSourceEventID)
	}
	credentialNotification := model.Notification{
		ID: "notification-credential", UserID: user.ID,
		Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "凭据已失效", Message: "请重新扫码登录",
		DedupeKey: "credential-expired", CreatedAt: now.Add(time.Second),
	}
	if err := store.SaveNotification(credentialNotification); err != nil {
		t.Fatalf("SaveNotification credential: %v", err)
	}
	duplicateCredential := credentialNotification
	duplicateCredential.ID = "notification-credential-duplicate"
	if err := store.SaveNotification(duplicateCredential); err == nil {
		t.Fatal("duplicate active notification dedupe key was accepted")
	}
	if _, err := store.db.Exec(`UPDATE notifications SET resolved_at=? WHERE id=?`, now.Add(2*time.Second).Unix(), credentialNotification.ID); err != nil {
		t.Fatalf("resolve credential notification: %v", err)
	}
	if err := store.SaveNotification(duplicateCredential); err != nil {
		t.Fatalf("SaveNotification after previous notification resolved: %v", err)
	}
	notifications, err := store.ListNotifications(user.ID, 10)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(notifications) != 3 || notifications[0].ID != duplicateCredential.ID {
		t.Fatalf("notifications did not round-trip in order: %+v", notifications)
	}
	page, err := store.ListNotificationsPage(NotificationPageQuery{
		UserID: user.ID, Status: "all", Limit: 2,
	})
	if err != nil {
		t.Fatalf("ListNotificationsPage: %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" || page.UnreadCount != 3 {
		t.Fatalf("unexpected first notification page: %+v", page)
	}
	nextPage, err := store.ListNotificationsPage(NotificationPageQuery{
		UserID: user.ID, CursorID: page.NextCursor, Status: "all", Limit: 2,
	})
	if err != nil {
		t.Fatalf("ListNotificationsPage next: %v", err)
	}
	if len(nextPage.Items) != 1 || nextPage.NextCursor != "" {
		t.Fatalf("unexpected next notification page: %+v", nextPage)
	}
	marked, ok, err := store.MarkNotificationRead(user.ID, idleNotification.ID, now.Add(3*time.Second))
	if err != nil || !ok || marked.ReadAt == nil {
		t.Fatalf("MarkNotificationRead = %+v, ok %v, err %v", marked, ok, err)
	}
	updated, err := store.MarkAllNotificationsRead(user.ID, now.Add(4*time.Second))
	if err != nil || updated != 2 {
		t.Fatalf("MarkAllNotificationsRead updated %d, err %v", updated, err)
	}
	resolvedPage, err := store.ListNotificationsPage(NotificationPageQuery{
		UserID: user.ID, Status: "resolved", Limit: 10,
	})
	if err != nil || len(resolvedPage.Items) != 1 || resolvedPage.UnreadCount != 0 {
		t.Fatalf("resolved notification page = %+v, err %v", resolvedPage, err)
	}
	deletedResolved, err := store.DeleteResolvedNotifications(user.ID)
	if err != nil || deletedResolved != 1 {
		t.Fatalf("DeleteResolvedNotifications deleted %d, err %v", deletedResolved, err)
	}

	lastAttemptAt := now.Add(3 * time.Minute)
	lastSuccessAt := now.Add(2 * time.Minute)
	refreshState := model.WatchRefreshState{
		UserID: user.ID, DeviceID: "pile-1",
		NextAttemptAt: now.Add(10 * time.Minute), LastAttemptAt: &lastAttemptAt,
		LastSuccessAt: &lastSuccessAt, ConsecutiveFailures: 2,
		PausedReason: "backoff", QuotaDate: "2026-08-09", QuotaUsed: 12,
		UpdatedAt: now.Add(3 * time.Minute),
	}
	if err := store.SaveWatchRefreshState(refreshState); err != nil {
		t.Fatalf("SaveWatchRefreshState: %v", err)
	}
	loadedRefreshState, ok, err := store.LoadWatchRefreshState(user.ID, "pile-1")
	if err != nil {
		t.Fatalf("LoadWatchRefreshState: %v", err)
	}
	if !ok || loadedRefreshState.QuotaUsed != 12 || loadedRefreshState.ConsecutiveFailures != 2 ||
		loadedRefreshState.LastSuccessAt == nil || !loadedRefreshState.LastSuccessAt.Equal(lastSuccessAt) {
		t.Fatalf("watch refresh state did not round-trip: %+v", loadedRefreshState)
	}
	secondRefreshState := refreshState
	secondRefreshState.DeviceID = "pile-2"
	secondRefreshState.QuotaUsed = 5
	if err := store.SaveWatchRefreshState(secondRefreshState); err != nil {
		t.Fatalf("SaveWatchRefreshState second pile: %v", err)
	}
	states, err := store.ListWatchRefreshStates(user.ID)
	if err != nil || len(states) != 2 {
		t.Fatalf("ListWatchRefreshStates = %+v, err %v", states, err)
	}
	quotaUsed, err := store.WatchRefreshQuotaUsed(user.ID, "2026-08-09")
	if err != nil || quotaUsed != 17 {
		t.Fatalf("WatchRefreshQuotaUsed = %d, err %v; want 17", quotaUsed, err)
	}
	if err := store.DeleteWatchRefreshState(user.ID, "pile-2"); err != nil {
		t.Fatalf("DeleteWatchRefreshState: %v", err)
	}
	if err := store.DeleteWatchDataForPile(user.ID, "pile-1"); err != nil {
		t.Fatalf("DeleteWatchDataForPile: %v", err)
	}
	if rules, err := store.ListWatchRules(user.ID); err != nil || len(rules) != 0 {
		t.Fatalf("pile watch rules after delete = %+v, err %v", rules, err)
	}
	if _, ok, err := store.LoadWatchRefreshState(user.ID, "pile-1"); err != nil || ok {
		t.Fatalf("pile watch refresh state remained after delete: ok %v, err %v", ok, err)
	}
	if err := store.SaveWatchRule(reminder); err != nil {
		t.Fatalf("restore watch rule for reopen/cascade checks: %v", err)
	}
	if err := store.SaveWatchRefreshState(refreshState); err != nil {
		t.Fatalf("restore watch refresh state for reopen/cascade checks: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	reopened, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	if rules, err := reopened.ListWatchRules(user.ID); err != nil || len(rules) != 1 {
		t.Fatalf("watch rules after reopen = %+v, err %v", rules, err)
	}

	if _, err := reopened.db.Exec(`DELETE FROM users WHERE id=?`, user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	for _, table := range []string{
		"watch_rules", "notification_preferences", "notifications", "watch_refresh_states",
	} {
		var count int
		if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE user_id=?`, user.ID).Scan(&count); err != nil {
			t.Fatalf("count cascaded %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows after user deletion", table, count)
		}
	}
}
