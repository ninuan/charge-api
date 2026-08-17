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
