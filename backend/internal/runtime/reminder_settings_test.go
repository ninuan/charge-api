package runtime

import (
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
)

func TestNewManagerAddsReminderDefaultsToLegacySettings(t *testing.T) {
	repository := testRepository(t)
	now := time.Now().UTC().Truncate(time.Second)
	admin := model.User{
		ID: "admin-1", Username: "admin", PasswordHash: "hash",
		Role: model.RoleAdmin, Enabled: true, DeviceLimit: 10,
		RefreshEnabled: true, CreatedAt: now, UpdatedAt: now,
	}
	legacySettings := model.RegistrationSettings{
		OpenRegistration: true, InviteRequired: true,
		DefaultDeviceLimit: 10, DefaultRefreshEnabled: true,
		StatsRetentionDays: 90, PortHistoryRetentionDays: 90,
	}
	if err := repository.Save(persistence.State{
		Version: stateVersion, Users: []model.User{admin},
		UserStates: map[string]persistence.UserState{admin.ID: {}},
		Settings:   legacySettings,
	}); err != nil {
		t.Fatalf("Save legacy settings: %v", err)
	}

	manager, err := NewManager(
		repository,
		"",
		parser.DefaultCaptureRequests(),
		"admin-password-123",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	settings := manager.Settings()
	assertReminderSettingDefaults(t, settings)

	persisted, ok, err := repository.Load()
	if err != nil {
		t.Fatalf("Load persisted settings: %v", err)
	}
	if !ok {
		t.Fatal("persisted state disappeared")
	}
	assertReminderSettingDefaults(t, persisted.Settings)
}

func TestNormalizeRegistrationSettingsPreservesExplicitlyDisabledReminders(t *testing.T) {
	settings := normalizeRegistrationSettings(model.RegistrationSettings{
		DefaultDeviceLimit: 10, StatsRetentionDays: 90, PortHistoryRetentionDays: 90,
		BackgroundRemindersEnabled:   false,
		RecurringRemindersEnabled:    false,
		WatchRefreshIntervalMinutes:  defaultWatchIntervalMinutes,
		WatchPileLimitPerUser:        defaultWatchPileLimit,
		WatchDailyRefreshQuota:       defaultWatchDailyQuota,
		NotificationRetentionDays:    defaultNotificationDays,
		ScheduledPowerOffEnabled:     false,
		ScheduledPowerOffStartMinute: defaultPowerOffStartMinute,
		ScheduledPowerOffEndMinute:   defaultPowerOffEndMinute,
		ScheduledPowerOffTimezone:    defaultPowerOffTimezone,
		PowerRestoreJitterMinutes:    defaultPowerRestoreJitter,
	})
	if settings.BackgroundRemindersEnabled || settings.RecurringRemindersEnabled || settings.ScheduledPowerOffEnabled {
		t.Fatalf("explicit disabled settings were overwritten: %+v", settings)
	}
}

func assertReminderSettingDefaults(t *testing.T, settings model.RegistrationSettings) {
	t.Helper()
	if !settings.BackgroundRemindersEnabled ||
		!settings.RecurringRemindersEnabled ||
		settings.WatchRefreshIntervalMinutes != 10 ||
		settings.WatchPileLimitPerUser != 5 ||
		settings.WatchDailyRefreshQuota != 480 ||
		settings.NotificationRetentionDays != 90 ||
		!settings.ScheduledPowerOffEnabled ||
		settings.ScheduledPowerOffStartMinute != 23*60 ||
		settings.ScheduledPowerOffEndMinute != 7*60 ||
		settings.ScheduledPowerOffTimezone != "Asia/Shanghai" ||
		settings.PowerRestoreJitterMinutes != 10 {
		t.Fatalf("unexpected reminder defaults: %+v", settings)
	}
}
