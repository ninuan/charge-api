package api

import (
	"testing"

	"charge-dashboard/internal/model"
)

func TestSettingsAuditMessageNamesChangedPolicyGroups(t *testing.T) {
	previous := model.RegistrationSettings{
		OpenRegistration: true, DefaultDeviceLimit: 10,
		StatsRetentionDays: 90, PortHistoryRetentionDays: 90, NotificationRetentionDays: 90,
		BackgroundRemindersEnabled: true, RecurringRemindersEnabled: false,
		WatchRefreshIntervalMinutes: 10,
		WatchPileLimitPerUser:       5, WatchDailyRefreshQuota: 480,
		ScheduledPowerOffEnabled: true, ScheduledPowerOffStartMinute: 23 * 60,
		ScheduledPowerOffEndMinute: 7 * 60, ScheduledPowerOffTimezone: "Asia/Shanghai",
		PowerRestoreJitterMinutes: 10,
	}
	next := previous
	next.OpenRegistration = false
	next.NotificationRetentionDays = 120
	next.WatchRefreshIntervalMinutes = 12
	next.ScheduledPowerOffStartMinute = 22 * 60
	if got := settingsAuditMessage(previous, next); got != "变更：注册策略、数据保留、提醒调度、计划断电" {
		t.Fatalf("settingsAuditMessage = %q", got)
	}
	if got := settingsAuditMessage(previous, previous); got != "设置内容未发生变化" {
		t.Fatalf("unchanged settingsAuditMessage = %q", got)
	}
}
