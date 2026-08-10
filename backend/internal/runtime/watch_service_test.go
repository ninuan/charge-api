package runtime

import (
	"errors"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
)

const (
	testWatchPileOne = "2601201412385560101"
	testWatchPileTwo = "2601201412385560102"
)

func TestWatchRulesValidatePileOwnershipConflictAndUpdates(t *testing.T) {
	manager, owner, other := newWatchTestManager(t)

	rule, err := manager.CreateWatchRule(owner.ID, model.WatchRuleCreateRequest{
		DeviceID: testWatchPileOne,
	})
	if err != nil {
		t.Fatalf("CreateWatchRule: %v", err)
	}
	if !rule.Enabled || rule.DeviceID != testWatchPileOne {
		t.Fatalf("unexpected whole-pile defaults: %+v", rule)
	}
	if _, err := manager.CreateWatchRule(owner.ID, model.WatchRuleCreateRequest{
		DeviceID: testWatchPileOne,
	}); !errors.Is(err, ErrWatchRuleConflict) {
		t.Fatalf("duplicate target error = %v", err)
	}
	if _, err := manager.CreateWatchRule(owner.ID, model.WatchRuleCreateRequest{
		DeviceID: "2601201412385560999",
	}); !errors.Is(err, ErrWatchTargetNotFound) {
		t.Fatalf("unowned pile error = %v", err)
	}

	disabled := false
	start := 420
	end := 1380
	updated, err := manager.UpdateWatchRule(owner.ID, rule.ID, model.WatchRuleUpdateRequest{
		Enabled: &disabled, ActiveStartMinute: &start, ActiveEndMinute: &end,
	})
	if err != nil {
		t.Fatalf("UpdateWatchRule: %v", err)
	}
	if updated.Enabled || updated.ActiveStartMinute != 420 || updated.ActiveEndMinute != 1380 {
		t.Fatalf("watch rule update did not apply: %+v", updated)
	}
	if _, err := manager.UpdateWatchRule(other.ID, rule.ID, model.WatchRuleUpdateRequest{
		Enabled: &disabled,
	}); !errors.Is(err, ErrWatchRuleNotFound) {
		t.Fatalf("cross-user update error = %v", err)
	}
	if err := manager.DeleteWatchRule(other.ID, rule.ID); !errors.Is(err, ErrWatchRuleNotFound) {
		t.Fatalf("cross-user delete error = %v", err)
	}
	if err := manager.DeletePile(owner.ID, testWatchPileOne); err != nil {
		t.Fatalf("DeletePile with watch rule: %v", err)
	}
	rules, err := manager.WatchRules(owner.ID)
	if err != nil || len(rules) != 0 {
		t.Fatalf("watch rules remained after pile deletion: %+v, err %v", rules, err)
	}
}

func TestWatchRulesEnforceWholePileLimit(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	settings := manager.Settings()
	settings.WatchPileLimitPerUser = 1
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("UpdateSettings pile limit: %v", err)
	}
	if _, err := manager.CreateWatchRule(owner.ID, model.WatchRuleCreateRequest{
		DeviceID: testWatchPileOne,
	}); err != nil {
		t.Fatalf("create first pile reminder: %v", err)
	}
	if _, err := manager.CreateWatchRule(owner.ID, model.WatchRuleCreateRequest{
		DeviceID: testWatchPileTwo,
	}); !errors.Is(err, ErrWatchPileLimit) {
		t.Fatalf("pile limit error = %v", err)
	}
}

func TestNotificationPreferencesAndInboxAreUserScoped(t *testing.T) {
	manager, owner, other := newWatchTestManager(t)
	preference, err := manager.NotificationPreference(owner.ID)
	if err != nil {
		t.Fatalf("NotificationPreference: %v", err)
	}
	if preference.BrowserEnabled || !preference.QuietHoursEnabled ||
		preference.QuietStartMinute != 22*60 || preference.QuietEndMinute != 8*60 {
		t.Fatalf("unexpected default preference: %+v", preference)
	}
	browserEnabled := true
	quietStart := 23 * 60
	updatedPreference, err := manager.UpdateNotificationPreference(owner.ID, model.NotificationPreferenceUpdateRequest{
		BrowserEnabled: &browserEnabled, QuietStartMinute: &quietStart,
	})
	if err != nil {
		t.Fatalf("UpdateNotificationPreference: %v", err)
	}
	if !updatedPreference.BrowserEnabled || updatedPreference.QuietStartMinute != 23*60 {
		t.Fatalf("notification preference update did not apply: %+v", updatedPreference)
	}
	invalidMinute := 24 * 60
	if _, err := manager.UpdateNotificationPreference(owner.ID, model.NotificationPreferenceUpdateRequest{
		QuietEndMinute: &invalidMinute,
	}); !errors.Is(err, ErrNotificationInputInvalid) {
		t.Fatalf("invalid notification preference error = %v", err)
	}

	first, err := manager.RecordNotification(model.Notification{
		UserID: owner.ID, Type: model.NotificationCredentialExpired,
		Severity: "warning", Title: "凭据失效", Message: "请重新扫码",
		DedupeKey: "credential-expired",
	})
	if err != nil {
		t.Fatalf("RecordNotification: %v", err)
	}
	resolvedAt := time.Now().UTC().Truncate(time.Second)
	if _, err := manager.RecordNotification(model.Notification{
		UserID: owner.ID, Type: model.NotificationPileRecovered,
		Severity: "info", Title: "充电桩已恢复", Message: "充电桩已恢复在线",
		DeviceID: testWatchPileOne, ResolvedAt: &resolvedAt,
	}); err != nil {
		t.Fatalf("RecordNotification resolved: %v", err)
	}
	page, err := manager.Notifications(owner.ID, "", "all", 1)
	if err != nil {
		t.Fatalf("Notifications: %v", err)
	}
	if len(page.Items) != 1 || page.NextCursor == "" || page.UnreadCount != 2 {
		t.Fatalf("unexpected notification page: %+v", page)
	}
	if _, err := manager.Notifications(owner.ID, "missing-cursor", "all", 20); !errors.Is(err, ErrNotificationQueryInvalid) {
		t.Fatalf("invalid cursor error = %v", err)
	}
	marked, err := manager.MarkNotificationRead(owner.ID, first.ID)
	if err != nil || marked.ReadAt == nil {
		t.Fatalf("MarkNotificationRead = %+v, err %v", marked, err)
	}
	if _, err := manager.MarkNotificationRead(other.ID, first.ID); !errors.Is(err, ErrNotificationNotFound) {
		t.Fatalf("cross-user mark read error = %v", err)
	}
	updated, err := manager.MarkAllNotificationsRead(owner.ID)
	if err != nil || updated != 1 {
		t.Fatalf("MarkAllNotificationsRead updated %d, err %v", updated, err)
	}
	deleted, err := manager.DeleteResolvedNotifications(owner.ID)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteResolvedNotifications deleted %d, err %v", deleted, err)
	}
}

func newWatchTestManager(t *testing.T) (*Manager, model.User, model.User) {
	t.Helper()
	repository := testRepository(t)
	now := time.Now().UTC().Truncate(time.Second)
	owner := model.User{
		ID: "watch-owner", Username: "watch-owner", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, DeviceLimit: 10, RefreshEnabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	other := model.User{
		ID: "watch-other", Username: "watch-other", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, DeviceLimit: 10, RefreshEnabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	piles := []model.Pile{
		{ID: testWatchPileOne, Number: "60101", OpenNum: 2, Online: true, Ports: []model.Port{{ID: 1}, {ID: 2}}},
		{ID: testWatchPileTwo, Number: "60102", OpenNum: 1, Online: true, Ports: []model.Port{{ID: 1}}},
	}
	settings := normalizeRegistrationSettings(model.RegistrationSettings{
		DefaultDeviceLimit: 10, DefaultRefreshEnabled: true,
		StatsRetentionDays: 90, PortHistoryRetentionDays: 90,
	})
	settings.WatchPileLimitPerUser = 1
	if err := repository.Save(persistence.State{
		Version: stateVersion, Users: []model.User{owner, other},
		UserStates: map[string]persistence.UserState{
			owner.ID: {Piles: piles, DeviceIDs: []string{testWatchPileOne, testWatchPileTwo}},
			other.ID: {},
		},
		Settings: settings,
	}); err != nil {
		t.Fatalf("Save watch fixture: %v", err)
	}
	manager, err := NewManager(repository, "", parser.DefaultCaptureRequests(), "", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return manager, owner, other
}
