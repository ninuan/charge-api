package persistence

import (
	"bytes"
	"database/sql"
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

	reminder := model.WatchRule{
		ID: "watch-pile", UserID: user.ID, DeviceID: "pile-1",
		Enabled: true, ActiveWeekdays: 127, Timezone: "Asia/Shanghai",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveWatchRule(reminder); err != nil {
		t.Fatalf("SaveWatchRule reminder: %v", err)
	}
	portID := 1
	duplicate := reminder
	duplicate.ID = "watch-pile-duplicate"
	if err := store.SaveWatchRule(duplicate); err == nil {
		t.Fatal("duplicate user/pile watch target was accepted")
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
	if len(rules) != 1 {
		t.Fatalf("watch rule count = %d, want 1", len(rules))
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
		ID: "notification-idle", UserID: user.ID, Type: model.NotificationPileAvailable,
		Severity: "info", Title: "充电桩有空闲口", Message: "充电桩现在有空闲端口",
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
	pendingPage, err := store.ListNotificationsPage(NotificationPageQuery{
		UserID: user.ID, Status: "pending", Limit: 10,
	})
	if err != nil || len(pendingPage.Items) != 1 || pendingPage.Items[0].ID != duplicateCredential.ID {
		t.Fatalf("pending page included informational notifications: %+v, err %v", pendingPage, err)
	}
	marked, ok, err := store.MarkNotificationRead(user.ID, idleNotification.ID, now.Add(3*time.Second))
	if err != nil || !ok || marked.ReadAt == nil {
		t.Fatalf("MarkNotificationRead = %+v, ok %v, err %v", marked, ok, err)
	}
	if marked.Type.RequiresAction() || marked.ResolvedAt != nil {
		t.Fatalf("read informational notification entered action lifecycle: %+v", marked)
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
		AvailabilityKnown: true, HadIdlePort: true, AvailabilityEventID: 42,
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
		loadedRefreshState.LastSuccessAt == nil || !loadedRefreshState.LastSuccessAt.Equal(lastSuccessAt) ||
		!loadedRefreshState.AvailabilityKnown || !loadedRefreshState.HadIdlePort || loadedRefreshState.AvailabilityEventID != 42 {
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

func TestRepeatedActiveNotificationsMergeOccurrenceDetails(t *testing.T) {
	path := t.TempDir() + "/state.db"
	key := bytes.Repeat([]byte{0x72}, CookieKeySize)
	store, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	user := model.User{
		ID: "user-merge", Username: "merge", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Save(State{
		Version: schemaVersion, Users: []model.User{user},
		UserStates: map[string]UserState{user.ID: {}},
	}); err != nil {
		t.Fatalf("Save user: %v", err)
	}

	first := model.Notification{
		ID: "notification-first", UserID: user.ID,
		Type: model.NotificationPileOffline, Severity: "warning",
		Title: "充电桩持续离线", Message: "第一次检查仍然离线",
		DeviceID: "pile-1", DedupeKey: "pile-offline:pile-1", CreatedAt: now,
	}
	stored, inserted, queued, err := store.InsertNotificationWithWxPusherDeliveryIfAbsent(first, "delivery-first", false)
	if err != nil || !inserted || queued || stored.OccurrenceCount != 1 || !stored.LastOccurredAt.Equal(now) {
		t.Fatalf("first notification = %+v, inserted %v, queued %v, err %v", stored, inserted, queued, err)
	}

	second := first
	second.ID = "notification-second"
	second.Message = "第二次检查仍然离线"
	second.CreatedAt = now.Add(5 * time.Minute)
	stored, inserted, queued, err = store.InsertNotificationWithWxPusherDeliveryIfAbsent(second, "delivery-second", false)
	if err != nil || inserted || queued {
		t.Fatalf("merged notification = %+v, inserted %v, queued %v, err %v", stored, inserted, queued, err)
	}
	if stored.ID != first.ID || stored.OccurrenceCount != 2 || stored.Message != second.Message ||
		!stored.LastOccurredAt.Equal(second.CreatedAt) {
		t.Fatalf("unexpected merged occurrence: %+v", stored)
	}

	pending, err := store.ListNotificationsPage(NotificationPageQuery{
		UserID: user.ID, Status: "pending", Limit: 10,
	})
	if err != nil || len(pending.Items) != 1 || pending.Items[0].OccurrenceCount != 2 {
		t.Fatalf("pending notifications = %+v, err %v", pending, err)
	}
	resolvedAt := now.Add(time.Minute)
	if err := store.SaveNotification(model.Notification{
		ID: "notification-recovered", UserID: user.ID,
		Type: model.NotificationPileRecovered, Severity: "info",
		Title: "充电桩恢复在线", Message: "充电桩已经恢复",
		DeviceID: "pile-1", ResolvedAt: &resolvedAt, CreatedAt: resolvedAt,
	}); err != nil {
		t.Fatalf("save legacy resolved informational notification: %v", err)
	}
	resolved, err := store.ListNotificationsPage(NotificationPageQuery{
		UserID: user.ID, Status: "resolved", Limit: 10,
	})
	if err != nil || len(resolved.Items) != 0 {
		t.Fatalf("resolved page included informational notifications = %+v, err %v", resolved, err)
	}
}

func TestPruneNotificationsRemovesOldTerminalRowsAndKeepsActiveProblems(t *testing.T) {
	store, err := OpenSQLite(
		t.TempDir()+"/notifications.db",
		bytes.Repeat([]byte{0x73}, CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	user := model.User{
		ID: "retention-user", Username: "retention-user", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Save(State{
		Version: schemaVersion, Users: []model.User{user},
		UserStates: map[string]UserState{user.ID: {}},
	}); err != nil {
		t.Fatalf("Save user: %v", err)
	}
	boundary := now.Add(-90 * 24 * time.Hour)
	for _, notification := range []model.Notification{
		{ID: "active-old", UserID: user.ID, Type: model.NotificationPileOffline, Severity: "warning", Title: "active", Message: "active", CreatedAt: now.Add(-100 * 24 * time.Hour)},
		{ID: "resolved-old", UserID: user.ID, Type: model.NotificationPileRecovered, Severity: "info", Title: "old", Message: "old", CreatedAt: now.Add(-100 * 24 * time.Hour)},
		{ID: "resolved-boundary", UserID: user.ID, Type: model.NotificationPileRecovered, Severity: "info", Title: "boundary", Message: "boundary", CreatedAt: now.Add(-90 * 24 * time.Hour)},
		{ID: "resolved-new", UserID: user.ID, Type: model.NotificationPileRecovered, Severity: "info", Title: "new", Message: "new", CreatedAt: now.Add(-2 * 24 * time.Hour)},
		{ID: "informational-old", UserID: user.ID, Type: model.NotificationPileRecovered, Severity: "info", Title: "old info", Message: "old info", CreatedAt: now.Add(-100 * 24 * time.Hour)},
		{ID: "informational-boundary", UserID: user.ID, Type: model.NotificationPileRecovered, Severity: "info", Title: "boundary info", Message: "boundary info", CreatedAt: boundary},
		{ID: "informational-new", UserID: user.ID, Type: model.NotificationPileRecovered, Severity: "info", Title: "new info", Message: "new info", CreatedAt: now.Add(-2 * 24 * time.Hour)},
	} {
		if err := store.SaveNotification(notification); err != nil {
			t.Fatalf("SaveNotification %s: %v", notification.ID, err)
		}
	}
	for id, resolvedAt := range map[string]time.Time{
		"resolved-old":      boundary.Add(-time.Second),
		"resolved-boundary": boundary,
		"resolved-new":      now.Add(-time.Hour),
	} {
		if _, err := store.db.Exec(`UPDATE notifications SET resolved_at=? WHERE id=?`, resolvedAt.Unix(), id); err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
	}
	deleted, err := store.PruneNotifications(boundary)
	if err != nil || deleted != 2 {
		t.Fatalf("PruneNotifications deleted %d, err %v; want 2", deleted, err)
	}
	rows, err := store.ListNotifications(user.ID, 10)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	remaining := make(map[string]bool, len(rows))
	for _, row := range rows {
		remaining[row.ID] = true
	}
	if remaining["resolved-old"] || remaining["informational-old"] ||
		!remaining["active-old"] || !remaining["resolved-boundary"] ||
		!remaining["resolved-new"] || !remaining["informational-boundary"] ||
		!remaining["informational-new"] {
		t.Fatalf("unexpected retained notifications: %+v", remaining)
	}
}

func TestLegacyFavoritesAndPortRulesNormalizeToOnePileReminder(t *testing.T) {
	path := t.TempDir() + "/legacy-watch.db"
	key := bytes.Repeat([]byte{0x72}, CookieKeySize)
	store, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	user := model.User{
		ID: "legacy-user", Username: "legacy-user", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Save(State{
		Version: schemaVersion, Users: []model.User{user},
		UserStates: map[string]UserState{user.ID: {}},
	}); err != nil {
		t.Fatalf("Save legacy user: %v", err)
	}
	statements := []string{
		`DELETE FROM metadata WHERE key='watch_rules_pile_level'`,
		`DROP INDEX IF EXISTS watch_rules_user_pile_unique_idx`,
		`DROP INDEX IF EXISTS watch_rules_user_active_pile_unique_idx`,
		`DROP INDEX IF EXISTS watch_rules_enabled_pile_idx`,
		`CREATE UNIQUE INDEX watch_rules_user_target_unique_idx ON watch_rules(user_id, device_id, COALESCE(port_id, 0))`,
		`INSERT INTO watch_rules(id,user_id,device_id,port_id,notify_idle,enabled,active_weekdays,active_start_minute,active_end_minute,timezone,created_at,updated_at) VALUES('favorite','legacy-user','pile-1',NULL,0,1,127,0,0,'Asia/Shanghai',1,1)`,
		`INSERT INTO watch_rules(id,user_id,device_id,port_id,notify_idle,enabled,active_weekdays,active_start_minute,active_end_minute,timezone,created_at,updated_at) VALUES('reminder-old','legacy-user','pile-1',1,1,1,127,0,0,'Asia/Shanghai',2,2)`,
		`INSERT INTO watch_rules(id,user_id,device_id,port_id,notify_idle,enabled,active_weekdays,active_start_minute,active_end_minute,timezone,created_at,updated_at) VALUES('reminder-new','legacy-user','pile-1',2,1,1,31,480,1320,'Asia/Shanghai',3,3)`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatalf("seed legacy watch data: %v\n%s", err, statement)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close legacy store: %v", err)
	}

	reopened, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("reopen legacy store: %v", err)
	}
	defer reopened.Close()
	rules, err := reopened.ListWatchRules(user.ID)
	if err != nil || len(rules) != 1 || rules[0].ID != "reminder-new" || rules[0].ActiveWeekdays != 31 {
		t.Fatalf("normalized pile rules = %+v, err %v", rules, err)
	}
	var portID sql.NullInt64
	var notifyIdle int
	if err := reopened.db.QueryRow(`SELECT port_id, notify_idle FROM watch_rules WHERE id='reminder-new'`).Scan(&portID, &notifyIdle); err != nil {
		t.Fatalf("read normalized physical rule: %v", err)
	}
	if portID.Valid || notifyIdle != 0 {
		t.Fatalf("legacy target fields were not cleared: port=%v notify=%d", portID, notifyIdle)
	}
	if _, ok, err := reopened.metadata("watch_rules_pile_level"); err != nil || !ok {
		t.Fatalf("pile normalization marker missing: ok %v, err %v", ok, err)
	}
}
