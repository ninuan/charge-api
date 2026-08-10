package runtime

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/persistence"
)

const (
	defaultWatchTimezone        = "Asia/Shanghai"
	defaultQuietStartMinute     = 22 * 60
	defaultQuietEndMinute       = 8 * 60
	defaultNotificationPageSize = 20
	maxNotificationPageSize     = 100
)

var (
	ErrWatchRuleInvalid         = errors.New("watch rule invalid")
	ErrWatchTargetNotFound      = errors.New("watch target not found")
	ErrWatchRuleNotFound        = errors.New("watch rule not found")
	ErrWatchRuleConflict        = errors.New("watch rule conflict")
	ErrWatchRuleLimit           = errors.New("watch rule limit reached")
	ErrWatchPileLimit           = errors.New("watch pile limit reached")
	ErrNotificationQueryInvalid = errors.New("notification query invalid")
	ErrNotificationNotFound     = errors.New("notification not found")
	ErrNotificationInputInvalid = errors.New("notification input invalid")
)

func (m *Manager) WatchRules(userID string) ([]model.WatchRule, error) {
	if _, err := m.runtimeFor(userID); err != nil {
		return nil, err
	}
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return nil, fmt.Errorf("list watch rules: %w", err)
	}
	return rules, nil
}

func (m *Manager) WatchOverview(userID string) (model.WatchOverview, error) {
	if _, err := m.runtimeFor(userID); err != nil {
		return model.WatchOverview{}, err
	}
	user, ok := m.User(userID)
	if !ok {
		return model.WatchOverview{}, ErrWatchTargetNotFound
	}
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return model.WatchOverview{}, fmt.Errorf("list watch rules for overview: %w", err)
	}
	settings := normalizeRegistrationSettings(m.Settings())
	quotaDate, err := reminderQuotaDate(time.Now(), settings.ScheduledPowerOffTimezone)
	if err != nil {
		return model.WatchOverview{}, err
	}
	quotaUsed, err := m.repository.WatchRefreshQuotaUsed(userID, quotaDate)
	if err != nil {
		return model.WatchOverview{}, fmt.Errorf("load watch quota for overview: %w", err)
	}
	return model.WatchOverview{
		RuleCount: len(rules), RuleLimit: settings.WatchRuleLimitPerUser,
		ReminderPileCount: activeReminderPileCount(rules), ReminderPileLimit: settings.WatchPileLimitPerUser,
		DailyQuotaUsed: quotaUsed, DailyQuotaLimit: settings.WatchDailyRefreshQuota, QuotaDate: quotaDate,
		RefreshIntervalMinutes:       settings.WatchRefreshIntervalMinutes,
		BackgroundRemindersEnabled:   settings.BackgroundRemindersEnabled,
		AccountRefreshEnabled:        user.RefreshEnabled,
		ScheduledPowerOffEnabled:     settings.ScheduledPowerOffEnabled,
		ScheduledPowerOffStartMinute: settings.ScheduledPowerOffStartMinute,
		ScheduledPowerOffEndMinute:   settings.ScheduledPowerOffEndMinute,
		ScheduledPowerOffTimezone:    settings.ScheduledPowerOffTimezone,
	}, nil
}

func (m *Manager) CreateWatchRule(userID string, request model.WatchRuleCreateRequest) (model.WatchRule, error) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	now := time.Now().UTC().Truncate(time.Second)
	rule := model.WatchRule{
		ID: randomID("wtr"), UserID: strings.TrimSpace(userID),
		DeviceID: strings.TrimSpace(request.DeviceID), PortID: request.PortID,
		NotifyIdle: request.NotifyIdle, Enabled: true, ActiveWeekdays: 127,
		ActiveStartMinute: 0, ActiveEndMinute: 0, Timezone: defaultWatchTimezone,
		CreatedAt: now, UpdatedAt: now,
	}
	if request.Enabled != nil {
		rule.Enabled = *request.Enabled
	}
	if request.ActiveWeekdays != nil {
		rule.ActiveWeekdays = *request.ActiveWeekdays
	}
	if request.ActiveStartMinute != nil {
		rule.ActiveStartMinute = *request.ActiveStartMinute
	}
	if request.ActiveEndMinute != nil {
		rule.ActiveEndMinute = *request.ActiveEndMinute
	}
	if request.Timezone != nil {
		rule.Timezone = strings.TrimSpace(*request.Timezone)
	}
	if err := m.validateWatchRuleTarget(rule); err != nil {
		return model.WatchRule{}, err
	}
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return model.WatchRule{}, fmt.Errorf("list watch rules before create: %w", err)
	}
	settings := normalizeRegistrationSettings(m.Settings())
	if len(rules) >= settings.WatchRuleLimitPerUser {
		return model.WatchRule{}, ErrWatchRuleLimit
	}
	for _, existing := range rules {
		if sameWatchTarget(existing, rule) {
			return model.WatchRule{}, ErrWatchRuleConflict
		}
	}
	if activeReminderPileCount(append(rules, rule)) > settings.WatchPileLimitPerUser {
		return model.WatchRule{}, ErrWatchPileLimit
	}
	if err := m.repository.SaveWatchRule(rule); err != nil {
		return model.WatchRule{}, fmt.Errorf("save watch rule: %w", err)
	}
	m.wakeReminderScheduler()
	return rule, nil
}

func (m *Manager) UpdateWatchRule(userID, ruleID string, request model.WatchRuleUpdateRequest) (model.WatchRule, error) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	if strings.TrimSpace(ruleID) == "" || watchUpdateEmpty(request) {
		return model.WatchRule{}, ErrWatchRuleInvalid
	}
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return model.WatchRule{}, fmt.Errorf("list watch rules before update: %w", err)
	}
	position := -1
	for index := range rules {
		if rules[index].ID == ruleID {
			position = index
			break
		}
	}
	if position < 0 {
		return model.WatchRule{}, ErrWatchRuleNotFound
	}
	rule := rules[position]
	if request.NotifyIdle != nil {
		rule.NotifyIdle = *request.NotifyIdle
	}
	if request.Enabled != nil {
		rule.Enabled = *request.Enabled
	}
	if request.ActiveWeekdays != nil {
		rule.ActiveWeekdays = *request.ActiveWeekdays
	}
	if request.ActiveStartMinute != nil {
		rule.ActiveStartMinute = *request.ActiveStartMinute
	}
	if request.ActiveEndMinute != nil {
		rule.ActiveEndMinute = *request.ActiveEndMinute
	}
	if request.Timezone != nil {
		rule.Timezone = strings.TrimSpace(*request.Timezone)
	}
	rule.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := m.validateWatchRuleTarget(rule); err != nil {
		return model.WatchRule{}, err
	}
	rules[position] = rule
	settings := normalizeRegistrationSettings(m.Settings())
	if activeReminderPileCount(rules) > settings.WatchPileLimitPerUser {
		return model.WatchRule{}, ErrWatchPileLimit
	}
	if err := m.repository.SaveWatchRule(rule); err != nil {
		return model.WatchRule{}, fmt.Errorf("update watch rule: %w", err)
	}
	m.wakeReminderScheduler()
	return rule, nil
}

func (m *Manager) DeleteWatchRule(userID, ruleID string) error {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()
	deleted, err := m.repository.DeleteWatchRule(userID, ruleID)
	if err != nil {
		return fmt.Errorf("delete watch rule: %w", err)
	}
	if !deleted {
		return ErrWatchRuleNotFound
	}
	m.wakeReminderScheduler()
	return nil
}

func (m *Manager) NotificationPreference(userID string) (model.NotificationPreference, error) {
	if _, err := m.runtimeFor(userID); err != nil {
		return model.NotificationPreference{}, err
	}
	preference, ok, err := m.repository.LoadNotificationPreference(userID)
	if err != nil {
		return model.NotificationPreference{}, fmt.Errorf("load notification preference: %w", err)
	}
	if ok {
		return preference, nil
	}
	return defaultNotificationPreference(userID, time.Now()), nil
}

func (m *Manager) UpdateNotificationPreference(userID string, request model.NotificationPreferenceUpdateRequest) (model.NotificationPreference, error) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	if notificationPreferenceUpdateEmpty(request) {
		return model.NotificationPreference{}, ErrNotificationInputInvalid
	}
	if _, err := m.runtimeFor(userID); err != nil {
		return model.NotificationPreference{}, err
	}
	preference, ok, err := m.repository.LoadNotificationPreference(userID)
	if err != nil {
		return model.NotificationPreference{}, fmt.Errorf("load notification preference before update: %w", err)
	}
	if !ok {
		preference = defaultNotificationPreference(userID, time.Now())
	}
	if request.BrowserEnabled != nil {
		preference.BrowserEnabled = *request.BrowserEnabled
	}
	if request.QuietHoursEnabled != nil {
		preference.QuietHoursEnabled = *request.QuietHoursEnabled
	}
	if request.QuietStartMinute != nil {
		preference.QuietStartMinute = *request.QuietStartMinute
	}
	if request.QuietEndMinute != nil {
		preference.QuietEndMinute = *request.QuietEndMinute
	}
	if request.Timezone != nil {
		preference.Timezone = strings.TrimSpace(*request.Timezone)
	}
	preference.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if !validMinute(preference.QuietStartMinute) || !validMinute(preference.QuietEndMinute) ||
		!validTimezoneName(preference.Timezone) {
		return model.NotificationPreference{}, ErrNotificationInputInvalid
	}
	if _, err := time.LoadLocation(preference.Timezone); err != nil {
		return model.NotificationPreference{}, ErrNotificationInputInvalid
	}
	if err := m.repository.SaveNotificationPreference(preference); err != nil {
		return model.NotificationPreference{}, fmt.Errorf("save notification preference: %w", err)
	}
	return preference, nil
}

func (m *Manager) Notifications(userID, cursor, status string, limit int) (model.NotificationPage, error) {
	if _, err := m.runtimeFor(userID); err != nil {
		return model.NotificationPage{}, err
	}
	cursor = strings.TrimSpace(cursor)
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		status = "all"
	}
	if limit == 0 {
		limit = defaultNotificationPageSize
	}
	if limit < 1 || limit > maxNotificationPageSize ||
		(status != "all" && status != "unread" && status != "resolved") ||
		(cursor != "" && !validOpaqueID(cursor)) {
		return model.NotificationPage{}, ErrNotificationQueryInvalid
	}
	page, err := m.repository.ListNotificationsPage(persistence.NotificationPageQuery{
		UserID: userID, CursorID: cursor, Status: status, Limit: limit,
	})
	if errors.Is(err, persistence.ErrNotificationCursorNotFound) {
		return model.NotificationPage{}, ErrNotificationQueryInvalid
	}
	if err != nil {
		return model.NotificationPage{}, fmt.Errorf("list notifications: %w", err)
	}
	return page, nil
}

func (m *Manager) MarkNotificationRead(userID, notificationID string) (model.Notification, error) {
	if !validOpaqueID(notificationID) {
		return model.Notification{}, ErrNotificationNotFound
	}
	notification, ok, err := m.repository.MarkNotificationRead(userID, notificationID, time.Now())
	if err != nil {
		return model.Notification{}, fmt.Errorf("mark notification read: %w", err)
	}
	if !ok {
		return model.Notification{}, ErrNotificationNotFound
	}
	return notification, nil
}

func (m *Manager) MarkAllNotificationsRead(userID string) (int64, error) {
	if _, err := m.runtimeFor(userID); err != nil {
		return 0, err
	}
	count, err := m.repository.MarkAllNotificationsRead(userID, time.Now())
	if err != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", err)
	}
	return count, nil
}

func (m *Manager) DeleteResolvedNotifications(userID string) (int64, error) {
	if _, err := m.runtimeFor(userID); err != nil {
		return 0, err
	}
	count, err := m.repository.DeleteResolvedNotifications(userID)
	if err != nil {
		return 0, fmt.Errorf("delete resolved notifications: %w", err)
	}
	return count, nil
}

// RecordNotification is the internal notification entry point used by refresh
// and credential services. User-facing HTTP handlers never expose creation.
func (m *Manager) RecordNotification(notification model.Notification) (model.Notification, error) {
	m.notificationMu.Lock()
	defer m.notificationMu.Unlock()
	if _, err := m.runtimeFor(notification.UserID); err != nil {
		return model.Notification{}, err
	}
	if notification.ID == "" {
		notification.ID = randomID("ntf")
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now().UTC().Truncate(time.Second)
	}
	if err := m.repository.SaveNotification(notification); err != nil {
		return model.Notification{}, fmt.Errorf("record notification: %w", err)
	}
	m.notificationHub.publish(notification)
	return notification, nil
}

func (m *Manager) validateWatchRuleTarget(rule model.WatchRule) error {
	if !validWatchDeviceID(rule.DeviceID) || rule.UserID == "" ||
		rule.ActiveWeekdays < 1 || rule.ActiveWeekdays > 127 ||
		!validMinute(rule.ActiveStartMinute) || !validMinute(rule.ActiveEndMinute) ||
		!validTimezoneName(rule.Timezone) {
		return ErrWatchRuleInvalid
	}
	if _, err := time.LoadLocation(rule.Timezone); err != nil {
		return ErrWatchRuleInvalid
	}
	if rule.NotifyIdle && rule.PortID == nil {
		return ErrWatchRuleInvalid
	}
	if rule.PortID != nil && *rule.PortID <= 0 {
		return ErrWatchRuleInvalid
	}
	runtime, err := m.runtimeFor(rule.UserID)
	if err != nil {
		return ErrWatchTargetNotFound
	}
	owned := false
	for _, deviceID := range runtime.client.DeviceIDs() {
		if deviceID == rule.DeviceID {
			owned = true
			break
		}
	}
	if !owned {
		return ErrWatchTargetNotFound
	}
	if rule.PortID == nil {
		return nil
	}
	for _, pile := range runtime.store.Snapshot().Piles {
		if pile.ID != rule.DeviceID {
			continue
		}
		for _, port := range pile.Ports {
			if port.ID == *rule.PortID {
				return nil
			}
		}
		break
	}
	return ErrWatchTargetNotFound
}

func defaultNotificationPreference(userID string, now time.Time) model.NotificationPreference {
	return model.NotificationPreference{
		UserID: userID, BrowserEnabled: false, QuietHoursEnabled: true,
		QuietStartMinute: defaultQuietStartMinute, QuietEndMinute: defaultQuietEndMinute,
		Timezone: defaultWatchTimezone, UpdatedAt: now.UTC().Truncate(time.Second),
	}
}

func activeReminderPileCount(rules []model.WatchRule) int {
	piles := make(map[string]struct{})
	for _, rule := range rules {
		if rule.Enabled && rule.NotifyIdle {
			piles[rule.DeviceID] = struct{}{}
		}
	}
	return len(piles)
}

func sameWatchTarget(left, right model.WatchRule) bool {
	if left.UserID != right.UserID || left.DeviceID != right.DeviceID {
		return false
	}
	if left.PortID == nil || right.PortID == nil {
		return left.PortID == nil && right.PortID == nil
	}
	return *left.PortID == *right.PortID
}

func validWatchDeviceID(value string) bool {
	if len(value) < 6 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validOpaqueID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("_.-", character) {
			continue
		}
		return false
	}
	return true
}

func validMinute(value int) bool {
	return value >= 0 && value < 24*60
}

func watchUpdateEmpty(request model.WatchRuleUpdateRequest) bool {
	return request.NotifyIdle == nil && request.Enabled == nil && request.ActiveWeekdays == nil &&
		request.ActiveStartMinute == nil && request.ActiveEndMinute == nil && request.Timezone == nil
}

func notificationPreferenceUpdateEmpty(request model.NotificationPreferenceUpdateRequest) bool {
	return request.BrowserEnabled == nil && request.QuietHoursEnabled == nil &&
		request.QuietStartMinute == nil && request.QuietEndMinute == nil && request.Timezone == nil
}
