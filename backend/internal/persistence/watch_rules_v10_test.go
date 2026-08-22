package persistence

import (
	"bytes"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestWatchRuleV10LifecycleAndActiveUniqueness(t *testing.T) {
	store, err := OpenSQLite(t.TempDir()+"/watch-v10.db", bytes.Repeat([]byte{0x75}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	user := model.User{ID: "watch-v10-user", Username: "watch", PasswordHash: "hash", Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := store.Save(State{Version: schemaVersion, Users: []model.User{user}, UserStates: map[string]UserState{user.ID: {}}}); err != nil {
		t.Fatalf("Save user: %v", err)
	}
	recurring := model.WatchRule{
		ID: "recurring", UserID: user.ID, DeviceID: "pile-recurring", Enabled: true,
		ActiveWeekdays: 127, Timezone: "Asia/Shanghai", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveWatchRule(recurring); err != nil {
		t.Fatalf("SaveWatchRule recurring compatibility: %v", err)
	}
	rules, err := store.ListWatchRules(user.ID)
	if err != nil || len(rules) != 1 || rules[0].Mode != model.WatchRuleRecurring || rules[0].StopAfterNotify {
		t.Fatalf("recurring rule did not normalize: %+v, err %v", rules, err)
	}
	expiresAt := now.Add(2 * time.Hour)
	completedAt := now.Add(time.Hour)
	completed := model.WatchRule{
		ID: "temporary-completed", UserID: user.ID, DeviceID: "pile-temporary",
		Mode: model.WatchRuleTemporary, Enabled: true, ActiveWeekdays: 127,
		Timezone: "Asia/Shanghai", ExpiresAt: &expiresAt, CompletedAt: &completedAt,
		CompletionReason: model.WatchCompletionNotified, StopAfterNotify: true,
		CreatedAt: now, UpdatedAt: completedAt,
	}
	if err := store.SaveWatchRule(completed); err != nil {
		t.Fatalf("SaveWatchRule completed temporary: %v", err)
	}
	active := completed
	active.ID = "temporary-active"
	active.CompletedAt = nil
	active.CompletionReason = ""
	active.UpdatedAt = now
	if err := store.SaveWatchRule(active); err != nil {
		t.Fatalf("new active rule after completed history: %v", err)
	}
	duplicate := active
	duplicate.ID = "temporary-active-duplicate"
	if err := store.SaveWatchRule(duplicate); err == nil {
		t.Fatal("multiple active rules for one user and pile were accepted")
	}
	invalid := active
	invalid.ID = "temporary-no-expiry"
	invalid.DeviceID = "pile-invalid"
	invalid.ExpiresAt = nil
	if err := store.SaveWatchRule(invalid); err == nil {
		t.Fatal("temporary rule without expiry was accepted")
	}
	if _, err := store.db.Exec(`
		INSERT INTO watch_rules(
			id,user_id,device_id,port_id,notify_idle,mode,enabled,
			active_weekdays,active_start_minute,active_end_minute,timezone,
			expires_at,completed_at,completion_reason,stop_after_notify,created_at,updated_at
		) VALUES('invalid-sql',?,'pile-invalid-sql',NULL,0,'temporary',1,127,0,0,'Asia/Shanghai',NULL,NULL,'',1,?,?)
	`, user.ID, now.Unix(), now.Unix()); err == nil {
		t.Fatal("sqlite lifecycle guard accepted invalid temporary rule")
	}
}

func TestWatchRuleV10CompletionAndExpiryAreIdempotent(t *testing.T) {
	store, err := OpenSQLite(t.TempDir()+"/watch-v10-completion.db", bytes.Repeat([]byte{0x76}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	user := model.User{ID: "watch-completion-user", Username: "watch-completion", PasswordHash: "hash", Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := store.Save(State{Version: schemaVersion, Users: []model.User{user}, UserStates: map[string]UserState{user.ID: {}}}); err != nil {
		t.Fatalf("Save user: %v", err)
	}
	for index, expiry := range []time.Time{now.Add(time.Hour), now.Add(2 * time.Hour)} {
		rule := model.WatchRule{
			ID: "temporary-" + string(rune('a'+index)), UserID: user.ID, DeviceID: "pile-" + string(rune('a'+index)),
			Mode: model.WatchRuleTemporary, Enabled: true, ActiveWeekdays: 127, Timezone: "Asia/Shanghai",
			ExpiresAt: &expiry, StopAfterNotify: true, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.SaveWatchRule(rule); err != nil {
			t.Fatalf("SaveWatchRule %d: %v", index, err)
		}
	}
	completed, err := store.CompleteWatchRule(user.ID, "temporary-b", model.WatchCompletionCancelled, now.Add(30*time.Minute))
	if err != nil || !completed {
		t.Fatalf("CompleteWatchRule = %v, err %v", completed, err)
	}
	completed, err = store.CompleteWatchRule(user.ID, "temporary-b", model.WatchCompletionCancelled, now.Add(31*time.Minute))
	if err != nil || completed {
		t.Fatalf("repeat CompleteWatchRule = %v, err %v", completed, err)
	}
	expired, err := store.CompleteExpiredWatchRules(now.Add(90 * time.Minute))
	if err != nil || expired != 1 {
		t.Fatalf("CompleteExpiredWatchRules = %d, err %v", expired, err)
	}
	rules, err := store.ListWatchRules(user.ID)
	if err != nil || len(rules) != 2 {
		t.Fatalf("ListWatchRules = %+v, err %v", rules, err)
	}
	for _, rule := range rules {
		if rule.CompletedAt == nil || rule.Enabled {
			t.Fatalf("rule not completed: %+v", rule)
		}
	}
}

func TestWatchRuleV10RecoversCompletionFromDurableNotification(t *testing.T) {
	store, err := OpenSQLite(t.TempDir()+"/watch-v10-recovery.db", bytes.Repeat([]byte{0x77}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	user := model.User{ID: "watch-recovery-user", Username: "watch-recovery", PasswordHash: "hash", Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := store.Save(State{Version: schemaVersion, Users: []model.User{user}, UserStates: map[string]UserState{user.ID: {}}}); err != nil {
		t.Fatalf("Save user: %v", err)
	}
	expiresAt := now.Add(2 * time.Hour)
	rule := model.WatchRule{
		ID: "temporary-recovery", UserID: user.ID, DeviceID: "pile-recovery",
		Mode: model.WatchRuleTemporary, Enabled: true, ActiveWeekdays: 127, Timezone: "Asia/Shanghai",
		ExpiresAt: &expiresAt, StopAfterNotify: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveWatchRule(rule); err != nil {
		t.Fatalf("SaveWatchRule: %v", err)
	}
	portID := 1
	if err := store.SaveNotification(model.Notification{
		ID: "notification-recovery", UserID: user.ID, Type: model.NotificationPileAvailable,
		Severity: "info", Title: "充电桩有空闲口", Message: "1 号口空闲。",
		DeviceID: rule.DeviceID, PortID: &portID, DedupeKey: "pile_available_rule:" + rule.ID,
		CreatedAt: now.Add(10 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveNotification: %v", err)
	}
	recovered, err := store.CompleteNotifiedTemporaryWatchRules(now.Add(11 * time.Minute))
	if err != nil || recovered != 1 {
		t.Fatalf("CompleteNotifiedTemporaryWatchRules = %d, err %v", recovered, err)
	}
	rules, err := store.ListWatchRules(user.ID)
	if err != nil || len(rules) != 1 || rules[0].CompletionReason != model.WatchCompletionNotified || rules[0].CompletedAt == nil {
		t.Fatalf("recovered rule = %+v, err %v", rules, err)
	}
}
