package runtime

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/charger"
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
	ErrWatchPileLimit           = errors.New("watch pile limit reached")
	ErrWatchPowerOff            = errors.New("watch task blocked by scheduled power-off")
	ErrWatchCredentialExpired   = errors.New("watch credential expired")
	ErrWatchRecurringDisabled   = errors.New("recurring watch rules disabled")
	ErrNotificationQueryInvalid = errors.New("notification query invalid")
	ErrNotificationNotFound     = errors.New("notification not found")
	ErrNotificationInputInvalid = errors.New("notification input invalid")
)

type WatchPowerOffError struct {
	RestoreAt time.Time
}

func (e WatchPowerOffError) Error() string {
	return fmt.Sprintf("当前处于计划断电时段，预计 %s 恢复供电", e.RestoreAt.In(time.FixedZone("UTC+8", 8*60*60)).Format("01月02日 15:04"))
}

func (e WatchPowerOffError) Unwrap() error { return ErrWatchPowerOff }

func (m *Manager) WatchRules(userID string) ([]model.WatchRule, error) {
	if _, err := m.runtimeFor(userID); err != nil {
		return nil, err
	}
	now := m.reminderSchedulerNow().UTC()
	if _, err := m.repository.CompleteExpiredWatchRules(now); err != nil {
		return nil, fmt.Errorf("complete expired watch rules: %w", err)
	}
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return nil, fmt.Errorf("list watch rules: %w", err)
	}
	settings := normalizeRegistrationSettings(m.Settings())
	for index := range rules {
		if err := decorateWatchRule(&rules[index], now, settings, m.repository); err != nil {
			return nil, err
		}
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
	rules, err := m.WatchRules(userID)
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
	if request.Mode == nil {
		mode := model.WatchRuleTemporary
		request.Mode = &mode
	}
	return m.createWatchRule(userID, request, true)
}

func (m *Manager) CreateWatchRuleWithInitialCheck(userID string, request model.WatchRuleCreateRequest) (model.WatchRuleCreateResult, error) {
	if request.Mode == nil {
		mode := model.WatchRuleTemporary
		request.Mode = &mode
	}
	rule, err := m.createWatchRule(userID, request, false)
	if err != nil {
		return model.WatchRuleCreateResult{}, err
	}
	result := model.WatchRuleCreateResult{
		Rule: &rule, IdlePortIDs: []int{}, BackgroundScheduled: true,
		Message: "已开始等待空闲口，有空闲时会通知你。",
	}
	// The opening check is a user-triggered request and must not be satisfied by
	// a stale scheduler cache. Notification delivery is suppressed until we know
	// whether a background task is actually needed.
	m.recordMetric(userID, "watch_initial_check")
	m.invalidateBackgroundPileCache(userID, rule.DeviceID)
	refresh, refreshErr := m.refreshWatchedPileWithDelivery(userID, rule.DeviceID, nil, false)
	if refreshErr != nil || refresh.Skipped {
		if charger.IsAuthExpired(refreshErr) {
			_, _ = m.repository.DeleteWatchRule(userID, rule.ID)
			_ = m.repository.DeleteWatchRefreshState(userID, rule.DeviceID)
			return model.WatchRuleCreateResult{}, ErrWatchCredentialExpired
		}
		attemptedAt := m.reminderSchedulerNow().UTC().Truncate(time.Second)
		nextAttemptAt := attemptedAt.Add(initialReminderFailureBackoff)
		if refresh.NextRetryAt != nil && refresh.NextRetryAt.After(nextAttemptAt) {
			nextAttemptAt = *refresh.NextRetryAt
		}
		failures := 0
		var lastAttemptAt *time.Time
		if refresh.Attempted {
			failures = 1
			lastAttemptAt = &attemptedAt
		}
		if err := m.repository.SaveWatchRefreshState(model.WatchRefreshState{
			UserID: rule.UserID, DeviceID: rule.DeviceID,
			NextAttemptAt: nextAttemptAt, LastAttemptAt: lastAttemptAt,
			ConsecutiveFailures: failures, PausedReason: watchPauseBackoff, UpdatedAt: attemptedAt,
		}); err != nil {
			return model.WatchRuleCreateResult{}, fmt.Errorf("save initial watch retry: %w", err)
		}
		result.Message = "暂时没有更新到充电桩状态，系统稍后会自动重试。"
		m.wakeReminderScheduler()
		return result, nil
	}
	idlePortIDs := pileIdlePortIDs(refresh.Pile)
	if len(idlePortIDs) == 0 {
		state, ok, err := m.repository.LoadWatchRefreshState(userID, rule.DeviceID)
		if err != nil {
			return model.WatchRuleCreateResult{}, fmt.Errorf("load first background watch state: %w", err)
		}
		if ok {
			settings := normalizeRegistrationSettings(m.Settings())
			state.NextAttemptAt = refresh.FetchedAt.Add(time.Duration(settings.WatchRefreshIntervalMinutes) * time.Minute)
			state.UpdatedAt = completedTime(refresh.FetchedAt, m.reminderSchedulerNow())
			if saveErr := m.repository.SaveWatchRefreshState(state); saveErr != nil {
				return model.WatchRuleCreateResult{}, fmt.Errorf("schedule first background watch check: %w", saveErr)
			}
		}
		m.wakeReminderScheduler()
		return result, nil
	}
	// An already-idle pile needs no timer and no synthetic notification. Keep the
	// immediate answer in the response and close the short-lived task history.
	if deleted, err := m.repository.DeleteWatchRule(userID, rule.ID); err != nil || !deleted {
		if err == nil {
			err = ErrWatchRuleNotFound
		}
		return model.WatchRuleCreateResult{}, fmt.Errorf("remove immediately satisfied watch rule: %w", err)
	}
	_ = m.repository.DeleteWatchRefreshState(userID, rule.DeviceID)
	result.Rule = nil
	result.IdlePortIDs = idlePortIDs
	result.BackgroundScheduled = false
	result.Message = "这台充电桩现在已有空闲口，无需开启后台提醒。"
	return result, nil
}

func completedTime(preferred, fallback time.Time) time.Time {
	if preferred.IsZero() {
		preferred = fallback
	}
	return preferred.UTC().Truncate(time.Second)
}

func (m *Manager) createWatchRule(userID string, request model.WatchRuleCreateRequest, wakeScheduler bool) (model.WatchRule, error) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	now := m.reminderSchedulerNow().UTC().Truncate(time.Second)
	rule := model.WatchRule{
		ID: randomID("wtr"), UserID: strings.TrimSpace(userID),
		DeviceID: strings.TrimSpace(request.DeviceID),
		Enabled:  true, ActiveWeekdays: 127,
		ActiveStartMinute: 0, ActiveEndMinute: 0, Timezone: defaultWatchTimezone,
		CreatedAt: now, UpdatedAt: now,
	}
	if request.Mode == nil {
		return model.WatchRule{}, ErrWatchRuleInvalid
	}
	rule.Mode = *request.Mode
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
	settings := normalizeRegistrationSettings(m.Settings())
	switch rule.Mode {
	case model.WatchRuleTemporary:
		if request.Enabled != nil && !*request.Enabled || request.ActiveWeekdays != nil ||
			request.ActiveStartMinute != nil || request.ActiveEndMinute != nil || request.Timezone != nil {
			return model.WatchRule{}, ErrWatchRuleInvalid
		}
		rule.Enabled = true
		rule.ActiveWeekdays = 127
		rule.ActiveStartMinute = 0
		rule.ActiveEndMinute = 0
		rule.Timezone = settings.ScheduledPowerOffTimezone
		expiresAt, err := temporaryWatchExpiry(now, request.Duration, settings)
		if err != nil {
			return model.WatchRule{}, err
		}
		rule.ExpiresAt = &expiresAt
		rule.StopAfterNotify = true
	case model.WatchRuleRecurring:
		return model.WatchRule{}, ErrWatchRecurringDisabled
	default:
		return model.WatchRule{}, ErrWatchRuleInvalid
	}
	if err := m.validateWatchRuleTarget(rule); err != nil {
		return model.WatchRule{}, err
	}
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return model.WatchRule{}, fmt.Errorf("list watch rules before create: %w", err)
	}
	for _, existing := range rules {
		if existing.Enabled && existing.CompletedAt == nil && sameWatchTarget(existing, rule) {
			return model.WatchRule{}, ErrWatchRuleConflict
		}
	}
	if activeReminderPileCount(rules) >= settings.WatchPileLimitPerUser {
		return model.WatchRule{}, ErrWatchPileLimit
	}
	if err := m.repository.SaveWatchRule(rule); err != nil {
		return model.WatchRule{}, fmt.Errorf("save watch rule: %w", err)
	}
	if wakeScheduler {
		m.wakeReminderScheduler()
	}
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
	if rule.Mode == model.WatchRuleRecurring {
		return model.WatchRule{}, ErrWatchRecurringDisabled
	}
	now := m.reminderSchedulerNow().UTC().Truncate(time.Second)
	if rule.Mode == model.WatchRuleTemporary {
		if rule.CompletedAt != nil {
			return model.WatchRule{}, ErrWatchRuleNotFound
		}
		if rule.ExpiresAt == nil || !rule.ExpiresAt.After(now) {
			_, _ = m.repository.CompleteWatchRule(userID, rule.ID, model.WatchCompletionExpired, now)
			return model.WatchRule{}, ErrWatchRuleNotFound
		}
		if request.Enabled != nil || request.ActiveWeekdays != nil || request.ActiveStartMinute != nil ||
			request.ActiveEndMinute != nil || request.Timezone != nil ||
			(request.Cancel != nil && !*request.Cancel) ||
			(request.Cancel != nil && request.Duration != nil) {
			return model.WatchRule{}, ErrWatchRuleInvalid
		}
		if request.Cancel != nil {
			completed, err := m.repository.CompleteWatchRule(userID, rule.ID, model.WatchCompletionCancelled, now)
			if err != nil {
				return model.WatchRule{}, fmt.Errorf("cancel watch rule: %w", err)
			}
			if !completed {
				return model.WatchRule{}, ErrWatchRuleNotFound
			}
			rule.Enabled = false
			rule.CompletedAt = &now
			rule.CompletionReason = model.WatchCompletionCancelled
			rule.UpdatedAt = now
			_ = m.repository.DeleteWatchRefreshState(userID, rule.DeviceID)
			m.wakeReminderScheduler()
			return rule, nil
		}
		if request.Duration == nil {
			return model.WatchRule{}, ErrWatchRuleInvalid
		}
		expiresAt, err := temporaryWatchExpiry(now, request.Duration, normalizeRegistrationSettings(m.Settings()))
		if err != nil || !expiresAt.After(*rule.ExpiresAt) {
			return model.WatchRule{}, ErrWatchRuleInvalid
		}
		rule.ExpiresAt = &expiresAt
		rule.UpdatedAt = now
		if err := m.repository.SaveWatchRule(rule); err != nil {
			return model.WatchRule{}, fmt.Errorf("extend watch rule: %w", err)
		}
		m.wakeReminderScheduler()
		return rule, nil
	}
	return model.WatchRule{}, ErrWatchRuleInvalid
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
		(status != "all" && status != "unread" && status != "pending" && status != "resolved") ||
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
	suppressed, err := m.wxPusherQuietAt(notification.UserID, notification.CreatedAt)
	if err != nil {
		return model.Notification{}, fmt.Errorf("load notification delivery preference: %w", err)
	}
	stored, inserted, queued, err := m.repository.InsertNotificationWithWxPusherDeliveryIfAbsent(
		notification,
		randomID("ndl"),
		suppressed,
	)
	if err != nil {
		return model.Notification{}, fmt.Errorf("record notification: %w", err)
	}
	if stored.ID == "" {
		return model.Notification{}, fmt.Errorf("record notification: duplicate notification")
	}
	if inserted || stored.OccurrenceCount > 1 {
		m.notificationHub.publish(stored)
	}
	if queued && !suppressed {
		m.wakeNotificationDispatcher()
	}
	return stored, nil
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
	return nil
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
		if rule.Mode == model.WatchRuleTemporary && rule.Enabled && rule.CompletedAt == nil {
			piles[rule.DeviceID] = struct{}{}
		}
	}
	return len(piles)
}

func decorateWatchRule(rule *model.WatchRule, now time.Time, settings model.RegistrationSettings, repository *persistence.Store) error {
	if rule == nil || rule.Mode != model.WatchRuleTemporary || !rule.Enabled || rule.CompletedAt != nil {
		return nil
	}
	next := now.UTC()
	state, ok, err := repository.LoadWatchRefreshState(rule.UserID, rule.DeviceID)
	if err != nil {
		return fmt.Errorf("load watch rule schedule: %w", err)
	}
	if ok && state.NextAttemptAt.After(next) {
		next = state.NextAttemptAt
	}
	rule.NextCheckAt = &next
	if rule.ExpiresAt == nil || !rule.ExpiresAt.After(next) {
		return nil
	}
	interval := time.Duration(settings.WatchRefreshIntervalMinutes) * time.Minute
	if interval <= 0 {
		return nil
	}
	remaining := rule.ExpiresAt.Sub(next)
	rule.EstimatedRemainingChecks = int((remaining + interval - 1) / interval)
	return nil
}

func sameWatchTarget(left, right model.WatchRule) bool {
	return left.UserID == right.UserID && left.DeviceID == right.DeviceID
}

func temporaryWatchExpiry(
	now time.Time,
	duration *model.WatchTemporaryDuration,
	settings model.RegistrationSettings,
) (time.Time, error) {
	inPowerOff, restoreAt, err := scheduledPowerOffWindow(now, settings)
	if err != nil {
		return time.Time{}, ErrWatchRuleInvalid
	}
	if inPowerOff {
		return time.Time{}, WatchPowerOffError{RestoreAt: restoreAt}
	}
	selected := model.WatchDurationTwoHours
	if duration != nil {
		selected = *duration
	}
	var expiresAt time.Time
	switch selected {
	case model.WatchDurationOneHour:
		expiresAt = now.Add(time.Hour)
	case model.WatchDurationTwoHours:
		expiresAt = now.Add(2 * time.Hour)
	case model.WatchDurationFourHours:
		expiresAt = now.Add(4 * time.Hour)
	case model.WatchDurationUntilPowerOff:
		if !settings.ScheduledPowerOffEnabled {
			return time.Time{}, ErrWatchRuleInvalid
		}
	default:
		return time.Time{}, ErrWatchRuleInvalid
	}
	if settings.ScheduledPowerOffEnabled {
		cutoff, err := nextScheduledPowerOffStart(now, settings)
		if err != nil {
			return time.Time{}, ErrWatchRuleInvalid
		}
		if selected == model.WatchDurationUntilPowerOff || cutoff.Before(expiresAt) {
			expiresAt = cutoff
		}
	}
	if !expiresAt.After(now) {
		return time.Time{}, ErrWatchRuleInvalid
	}
	return expiresAt.UTC().Truncate(time.Second), nil
}

func nextScheduledPowerOffStart(now time.Time, settings model.RegistrationSettings) (time.Time, error) {
	location, err := time.LoadLocation(settings.ScheduledPowerOffTimezone)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(location)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	for dayOffset := 0; dayOffset <= 1; dayOffset++ {
		candidate := midnight.AddDate(0, 0, dayOffset).Add(
			time.Duration(settings.ScheduledPowerOffStartMinute) * time.Minute,
		)
		if candidate.After(local) {
			return candidate.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("next scheduled power-off start not found")
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
	return request.Enabled == nil && request.Duration == nil && request.Cancel == nil && request.ActiveWeekdays == nil &&
		request.ActiveStartMinute == nil && request.ActiveEndMinute == nil && request.Timezone == nil
}

func notificationPreferenceUpdateEmpty(request model.NotificationPreferenceUpdateRequest) bool {
	return request.BrowserEnabled == nil && request.QuietHoursEnabled == nil &&
		request.QuietStartMinute == nil && request.QuietEndMinute == nil && request.Timezone == nil
}
