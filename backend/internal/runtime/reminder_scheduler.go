package runtime

import (
	"charge-dashboard/internal/security"
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"charge-dashboard/internal/charger"
	"charge-dashboard/internal/model"
)

const (
	defaultReminderSchedulerPollInterval = 30 * time.Second
	defaultReminderSchedulerConcurrency  = 2
	initialReminderFailureBackoff        = time.Minute
	maxReminderFailureBackoff            = 6 * time.Hour
	offlineNotificationMinimumDuration   = 30 * time.Minute
	offlineNotificationMinimumChecks     = 3
)

const (
	watchPauseGlobalDisabled    = "global_disabled"
	watchPausePowerOff          = "scheduled_power_off"
	watchPauseInactiveRule      = "inactive_rule_window"
	watchPauseAccountDisabled   = "account_refresh_disabled"
	watchPauseQuota             = "quota_exhausted"
	watchPauseBackoff           = "request_backoff"
	watchPauseInFlight          = "in_flight"
	watchPauseCredentialExpired = "credential_expired"
	watchPausePileOffline       = "pile_offline_observed"
)

var (
	ErrReminderSchedulerRunning = errors.New("reminder scheduler already running")
	ErrWatchQuotaExceeded       = errors.New("watch refresh quota exceeded")
	errWatchPowerOff            = errors.New("scheduled power-off window")
	errWatchRuleInactive        = errors.New("watch rule inactive")
	errWatchGloballyDisabled    = errors.New("background reminders disabled")
)

type reminderSchedulerCoordinator struct {
	mu             sync.Mutex
	running        bool
	wake           chan struct{}
	done           chan struct{}
	now            func() time.Time
	intervalJitter func(time.Duration) time.Duration
	restoreJitter  func(time.Duration) time.Duration
	pollInterval   time.Duration
	maxConcurrency int
}

type reminderTarget struct {
	userID         string
	deviceID       string
	refreshEnabled bool
	rules          []model.WatchRule
}

type reminderTargetState struct {
	target reminderTarget
	state  model.WatchRefreshState
}

type reminderTargetKey struct {
	userID   string
	deviceID string
}

func (m *Manager) StartReminderScheduler(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("reminder scheduler requires a context")
	}
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	coordinator.initializeLocked()
	if coordinator.running {
		coordinator.mu.Unlock()
		return ErrReminderSchedulerRunning
	}
	coordinator.running = true
	coordinator.wake = make(chan struct{}, 1)
	coordinator.done = make(chan struct{})
	wake := coordinator.wake
	done := coordinator.done
	coordinator.mu.Unlock()

	go m.runReminderScheduler(ctx, wake, done)
	return nil
}

func (m *Manager) WaitReminderScheduler(ctx context.Context) error {
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	done := coordinator.done
	running := coordinator.running
	coordinator.mu.Unlock()
	if !running || done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) wakeReminderScheduler() {
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	wake := coordinator.wake
	running := coordinator.running
	coordinator.mu.Unlock()
	if !running || wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (m *Manager) runReminderScheduler(ctx context.Context, wake <-chan struct{}, done chan struct{}) {
	defer func() {
		coordinator := &m.reminderScheduler
		coordinator.mu.Lock()
		coordinator.running = false
		close(done)
		coordinator.mu.Unlock()
	}()

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			stopAndDrainTimer(timer)
			timer.Reset(0)
		case <-timer.C:
			if err := m.runReminderSchedulerOnce(ctx, m.reminderSchedulerNow()); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("watch scheduler cycle failed: %v", security.SanitizeLogText(err.Error(), 1024))
				m.recordMetric("system", "watch_scheduler_error")
			}
			timer.Reset(m.reminderSchedulerPollInterval())
		}
	}
}

func (m *Manager) runReminderSchedulerOnce(ctx context.Context, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now = now.UTC()
	if _, err := m.repository.CompleteExpiredWatchRules(now); err != nil {
		return fmt.Errorf("complete expired reminder tasks: %w", err)
	}
	if _, err := m.repository.CompleteNotifiedTemporaryWatchRules(now); err != nil {
		return fmt.Errorf("recover notified reminder tasks: %w", err)
	}
	if err := m.maybeRunRetentionMaintenance(now); err != nil {
		return fmt.Errorf("run retention maintenance: %w", err)
	}
	if err := m.recoverPendingPileAvailabilityNotifications(defaultPortStatusEventRecoveryLimit); err != nil {
		return fmt.Errorf("recover pile availability notifications: %w", err)
	}
	settings := normalizeRegistrationSettings(m.Settings())
	targets, err := m.reminderTargets()
	if err != nil {
		return err
	}
	if err := m.reconcileReminderStates(targets); err != nil {
		return err
	}

	due := make([]reminderTargetState, 0)
	selectedUsers := make(map[string]struct{})
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, runnable, err := m.prepareReminderTarget(target, settings, now)
		if err != nil {
			return err
		}
		if !runnable {
			continue
		}
		// One target per user per cycle makes the per-user quota reservation
		// deterministic while still allowing different users to use the two
		// global workers concurrently.
		if _, selected := selectedUsers[target.userID]; selected {
			continue
		}
		selectedUsers[target.userID] = struct{}{}
		due = append(due, reminderTargetState{target: target, state: state})
	}
	if len(due) == 0 {
		return nil
	}

	workers := m.reminderSchedulerConcurrency()
	semaphore := make(chan struct{}, workers)
	errCh := make(chan error, len(due))
	var wait sync.WaitGroup

launchLoop:
	for _, item := range due {
		if err := ctx.Err(); err != nil {
			break
		}
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			break launchLoop
		}
		wait.Add(1)
		go func(item reminderTargetState) {
			defer wait.Done()
			defer func() { <-semaphore }()
			if err := m.executeReminderTarget(item, now); err != nil {
				errCh <- err
			}
		}(item)
	}
	wait.Wait()
	close(errCh)
	for err := range errCh {
		return err
	}
	return ctx.Err()
}

func (m *Manager) reminderSchedulerRunning() bool {
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.running
}

func (m *Manager) reminderTargets() ([]reminderTarget, error) {
	type schedulerUser struct {
		id             string
		refreshEnabled bool
	}
	m.mu.RLock()
	users := make([]schedulerUser, 0, len(m.users))
	for _, user := range m.users {
		if user.Enabled && user.Role == model.RoleUser {
			users = append(users, schedulerUser{id: user.ID, refreshEnabled: user.RefreshEnabled})
		}
	}
	m.mu.RUnlock()
	sort.Slice(users, func(i, j int) bool { return users[i].id < users[j].id })

	now := m.reminderSchedulerNow()
	targets := make([]reminderTarget, 0)
	for _, user := range users {
		rules, err := m.repository.ListWatchRules(user.id)
		if err != nil {
			return nil, fmt.Errorf("list scheduler watch rules for user %s: %w", user.id, err)
		}
		byPile := make(map[string][]model.WatchRule)
		for _, rule := range rules {
			if rule.Mode == model.WatchRuleTemporary && rule.Enabled &&
				rule.CompletedAt == nil && rule.ExpiresAt != nil && rule.ExpiresAt.After(now) {
				byPile[rule.DeviceID] = append(byPile[rule.DeviceID], rule)
			}
		}
		deviceIDs := make([]string, 0, len(byPile))
		for deviceID := range byPile {
			deviceIDs = append(deviceIDs, deviceID)
		}
		sort.Strings(deviceIDs)
		for _, deviceID := range deviceIDs {
			targets = append(targets, reminderTarget{
				userID: user.id, deviceID: deviceID, refreshEnabled: user.refreshEnabled,
				rules: byPile[deviceID],
			})
		}
	}
	return targets, nil
}

func (m *Manager) reconcileReminderStates(targets []reminderTarget) error {
	active := make(map[reminderTargetKey]struct{}, len(targets))
	for _, target := range targets {
		active[reminderTargetKey{userID: target.userID, deviceID: target.deviceID}] = struct{}{}
	}
	states, err := m.repository.ListWatchRefreshStates("")
	if err != nil {
		return err
	}
	for _, state := range states {
		if _, ok := active[reminderTargetKey{userID: state.UserID, deviceID: state.DeviceID}]; ok {
			continue
		}
		if err := m.repository.DeleteWatchRefreshState(state.UserID, state.DeviceID); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) prepareReminderTarget(
	target reminderTarget,
	settings model.RegistrationSettings,
	now time.Time,
) (model.WatchRefreshState, bool, error) {
	state, ok, err := m.repository.LoadWatchRefreshState(target.userID, target.deviceID)
	if err != nil {
		return model.WatchRefreshState{}, false, err
	}
	if !ok {
		state = model.WatchRefreshState{
			UserID: target.userID, DeviceID: target.deviceID,
			NextAttemptAt: now, UpdatedAt: now,
		}
	}
	quotaDate, err := reminderQuotaDate(now, settings.ScheduledPowerOffTimezone)
	if err != nil {
		return model.WatchRefreshState{}, false, err
	}
	quotaReset := state.QuotaDate != quotaDate
	if quotaReset {
		state.QuotaDate = quotaDate
		state.QuotaUsed = 0
	}

	inPowerOff, restoreAt, err := scheduledPowerOffWindow(now, settings)
	if err != nil {
		return model.WatchRefreshState{}, false, err
	}
	if inPowerOff {
		if state.PausedReason == watchPauseCredentialExpired {
			changed := state.ConsecutiveFailures != 0 || quotaReset
			state.ConsecutiveFailures = 0
			if !changed {
				return state, false, nil
			}
			state.UpdatedAt = now
			return state, false, m.repository.SaveWatchRefreshState(state)
		}
		powerChanged := state.PausedReason != watchPausePowerOff || state.NextAttemptAt.Before(restoreAt)
		if powerChanged {
			state.NextAttemptAt = restoreAt.Add(m.reminderRestoreJitter(
				time.Duration(settings.PowerRestoreJitterMinutes) * time.Minute,
			))
		}
		state.PausedReason = watchPausePowerOff
		changed := powerChanged || state.ConsecutiveFailures != 0 || quotaReset
		state.ConsecutiveFailures = 0
		if !changed {
			return state, false, nil
		}
		state.UpdatedAt = now
		return state, false, m.repository.SaveWatchRefreshState(state)
	}
	if state.PausedReason == watchPausePowerOff && !settings.ScheduledPowerOffEnabled {
		state.PausedReason = ""
		state.NextAttemptAt = now
	}
	if state.PausedReason == watchPausePowerOff && state.NextAttemptAt.After(now) {
		if quotaReset {
			state.UpdatedAt = now
			return state, false, m.repository.SaveWatchRefreshState(state)
		}
		return state, false, nil
	}
	if state.PausedReason == watchPauseCredentialExpired {
		if quotaReset {
			state.UpdatedAt = now
			return state, false, m.repository.SaveWatchRefreshState(state)
		}
		return state, false, nil
	}

	switch {
	case !settings.BackgroundRemindersEnabled:
		next := state.NextAttemptAt
		if state.PausedReason != watchPauseGlobalDisabled {
			next = now
		}
		if state.PausedReason == watchPauseGlobalDisabled && !quotaReset {
			return state, false, nil
		}
		return state, false, m.pauseReminderState(&state, watchPauseGlobalDisabled, next, now)
	case !target.refreshEnabled:
		next := state.NextAttemptAt
		if state.PausedReason != watchPauseAccountDisabled {
			next = now
		}
		if state.PausedReason == watchPauseAccountDisabled && !quotaReset {
			return state, false, nil
		}
		return state, false, m.pauseReminderState(&state, watchPauseAccountDisabled, next, now)
	}

	active, nextActive, err := reminderRulesActiveAt(target.rules, now)
	if err != nil {
		return model.WatchRefreshState{}, false, err
	}
	if !active {
		if state.PausedReason == watchPauseInactiveRule && state.NextAttemptAt.Equal(nextActive) && !quotaReset {
			return state, false, nil
		}
		return state, false, m.pauseReminderState(&state, watchPauseInactiveRule, nextActive, now)
	}
	if state.PausedReason == watchPauseInactiveRule ||
		state.PausedReason == watchPauseGlobalDisabled ||
		state.PausedReason == watchPauseAccountDisabled {
		state.PausedReason = ""
		state.NextAttemptAt = now
	}
	if state.PausedReason == watchPauseQuota {
		used, err := m.repository.WatchRefreshQuotaUsed(target.userID, quotaDate)
		if err != nil {
			return model.WatchRefreshState{}, false, err
		}
		if used < settings.WatchDailyRefreshQuota {
			state.PausedReason = ""
			state.NextAttemptAt = now
		}
	}
	if state.NextAttemptAt.After(now) {
		if quotaReset {
			state.UpdatedAt = now
			return state, false, m.repository.SaveWatchRefreshState(state)
		}
		return state, false, nil
	}
	used, err := m.repository.WatchRefreshQuotaUsed(target.userID, quotaDate)
	if err != nil {
		return model.WatchRefreshState{}, false, err
	}
	if used >= settings.WatchDailyRefreshQuota {
		next, err := nextReminderQuotaDay(now, settings.ScheduledPowerOffTimezone)
		if err != nil {
			return model.WatchRefreshState{}, false, err
		}
		if state.PausedReason == watchPauseQuota && state.NextAttemptAt.Equal(next) && !quotaReset {
			return state, false, nil
		}
		m.recordMetric(target.userID, "watch_quota_skipped")
		return state, false, m.pauseReminderState(&state, watchPauseQuota, next, now)
	}
	return state, true, nil
}

func (m *Manager) executeReminderTarget(
	item reminderTargetState,
	cycleNow time.Time,
) error {
	state := item.state
	previousPauseReason := state.PausedReason
	previousConsecutiveFailures := state.ConsecutiveFailures
	previousLastSuccessAt := cloneTimePointer(state.LastSuccessAt)
	result, refreshErr := m.refreshWatchedPile(item.target.userID, item.target.deviceID, func() error {
		now := m.reminderSchedulerNow().UTC()
		latestSettings := normalizeRegistrationSettings(m.Settings())
		inPowerOff, _, err := scheduledPowerOffWindow(now, latestSettings)
		if err != nil {
			return err
		}
		if inPowerOff {
			return errWatchPowerOff
		}
		if !latestSettings.BackgroundRemindersEnabled {
			return errWatchGloballyDisabled
		}
		user, ok := m.User(item.target.userID)
		if !ok || !user.RefreshEnabled {
			return ErrWatchRefreshNotEnabled
		}
		latestRules, err := m.currentReminderPileRules(item.target.userID, item.target.deviceID)
		if err != nil {
			return err
		}
		if len(latestRules) == 0 {
			return ErrWatchRefreshNotEnabled
		}
		active, _, err := reminderRulesActiveAt(latestRules, now)
		if err != nil {
			return err
		}
		if !active {
			return errWatchRuleInactive
		}
		quotaDate, err := reminderQuotaDate(now, latestSettings.ScheduledPowerOffTimezone)
		if err != nil {
			return err
		}
		used, err := m.repository.WatchRefreshQuotaUsed(item.target.userID, quotaDate)
		if err != nil {
			return err
		}
		if used >= latestSettings.WatchDailyRefreshQuota {
			return ErrWatchQuotaExceeded
		}
		if state.QuotaDate != quotaDate {
			state.QuotaDate = quotaDate
			state.QuotaUsed = 0
		}
		state.QuotaUsed++
		attemptedAt := now
		state.LastAttemptAt = &attemptedAt
		state.NextAttemptAt = now.Add(time.Duration(latestSettings.WatchRefreshIntervalMinutes) * time.Minute)
		state.PausedReason = watchPauseInFlight
		state.UpdatedAt = now
		return m.repository.SaveWatchRefreshState(state)
	})
	now := m.reminderSchedulerNow().UTC()

	switch {
	case errors.Is(refreshErr, ErrWatchQuotaExceeded):
		latestSettings := normalizeRegistrationSettings(m.Settings())
		next, err := nextReminderQuotaDay(now, latestSettings.ScheduledPowerOffTimezone)
		if err != nil {
			return err
		}
		m.recordMetric(item.target.userID, "watch_quota_skipped")
		return m.pauseReminderState(&state, watchPauseQuota, next, now)
	case errors.Is(refreshErr, errWatchGloballyDisabled):
		return m.pauseReminderState(&state, watchPauseGlobalDisabled, now, now)
	case errors.Is(refreshErr, errWatchPowerOff):
		latestSettings := normalizeRegistrationSettings(m.Settings())
		_, restoreAt, err := scheduledPowerOffWindow(now, latestSettings)
		if err != nil {
			return err
		}
		state.ConsecutiveFailures = 0
		return m.pauseReminderState(
			&state,
			watchPausePowerOff,
			restoreAt.Add(m.reminderRestoreJitter(time.Duration(latestSettings.PowerRestoreJitterMinutes)*time.Minute)),
			now,
		)
	case errors.Is(refreshErr, errWatchRuleInactive), errors.Is(refreshErr, ErrWatchRefreshNotEnabled):
		user, ok := m.User(item.target.userID)
		if !ok {
			return m.repository.DeleteWatchRefreshState(item.target.userID, item.target.deviceID)
		}
		if !user.RefreshEnabled {
			return m.pauseReminderState(&state, watchPauseAccountDisabled, now, now)
		}
		latestRules, err := m.currentReminderPileRules(item.target.userID, item.target.deviceID)
		if err != nil {
			return err
		}
		if len(latestRules) == 0 {
			return m.repository.DeleteWatchRefreshState(item.target.userID, item.target.deviceID)
		}
		active, next, err := reminderRulesActiveAt(latestRules, now)
		if err != nil {
			return err
		}
		if active {
			return m.repository.DeleteWatchRefreshState(item.target.userID, item.target.deviceID)
		}
		return m.pauseReminderState(&state, watchPauseInactiveRule, next, now)
	case charger.IsAuthExpired(refreshErr):
		return m.notifyCredentialExpired(item.target.userID, now)
	case result.Skipped:
		next := now.Add(initialReminderFailureBackoff)
		if result.NextRetryAt != nil && result.NextRetryAt.After(next) {
			next = *result.NextRetryAt
		}
		return m.pauseReminderState(&state, watchPauseBackoff, next, now)
	case refreshErr != nil:
		if result.Attempted {
			state.ConsecutiveFailures++
		}
		next := now.Add(reminderFailureBackoff(state.ConsecutiveFailures))
		if result.NextRetryAt != nil && result.NextRetryAt.After(next) {
			next = *result.NextRetryAt
		}
		return m.pauseReminderState(&state, watchPauseBackoff, next, now)
	case !result.Pile.Online:
		// The request reservation temporarily marks the row in-flight. Restore
		// the persisted offline streak before evaluating this successful check.
		state.PausedReason = previousPauseReason
		state.ConsecutiveFailures = previousConsecutiveFailures
		state.LastSuccessAt = previousLastSuccessAt
		return m.recordOfflineReminderResult(&state, item.target, now)
	default:
		if _, _, err := m.resolveNotification(
			item.target.userID,
			offlineNotificationDedupeKey(item.target.deviceID),
			now,
		); err != nil {
			return fmt.Errorf("resolve pile offline notification: %w", err)
		}
		succeededAt := result.FetchedAt
		if succeededAt.IsZero() {
			succeededAt = cycleNow
		}
		state.LastSuccessAt = &succeededAt
		state.ConsecutiveFailures = 0
		state.PausedReason = ""
		latestSettings := normalizeRegistrationSettings(m.Settings())
		base := time.Duration(latestSettings.WatchRefreshIntervalMinutes) * time.Minute
		state.NextAttemptAt = now.Add(m.reminderIntervalJitter(base))
		state.UpdatedAt = now
		return m.repository.SaveWatchRefreshState(state)
	}
}

const defaultPortStatusEventRecoveryLimit = 1000

func (m *Manager) recordOfflineReminderResult(
	state *model.WatchRefreshState,
	target reminderTarget,
	now time.Time,
) error {
	if state.PausedReason != watchPausePileOffline || state.LastSuccessAt == nil {
		firstObservedAt := now.UTC()
		state.LastSuccessAt = &firstObservedAt
		state.ConsecutiveFailures = 1
	} else {
		state.ConsecutiveFailures++
	}
	state.PausedReason = watchPausePileOffline

	if state.ConsecutiveFailures >= offlineNotificationMinimumChecks &&
		now.Sub(*state.LastSuccessAt) >= offlineNotificationMinimumDuration {
		pileLabel := strings.TrimSpace(m.notificationPileLabel(target.userID, target.deviceID))
		if _, _, err := m.recordNotificationOnce(model.Notification{
			UserID: target.userID, Type: model.NotificationPileOffline, Severity: "warning",
			Title:     "充电桩无法连接",
			Message:   pileLabel + "在正常供电时段内多次无法连接，请稍后再试。",
			DeviceID:  target.deviceID,
			DedupeKey: offlineNotificationDedupeKey(target.deviceID), CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("record pile offline notification: %w", err)
		}
	}

	settings := normalizeRegistrationSettings(m.Settings())
	base := time.Duration(settings.WatchRefreshIntervalMinutes) * time.Minute
	state.NextAttemptAt = now.Add(m.reminderIntervalJitter(base))
	state.UpdatedAt = now
	return m.repository.SaveWatchRefreshState(*state)
}

func (m *Manager) currentReminderPileRules(userID, deviceID string) ([]model.WatchRule, error) {
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return nil, fmt.Errorf("list current reminder rules: %w", err)
	}
	current := make([]model.WatchRule, 0)
	now := m.reminderSchedulerNow()
	for _, rule := range rules {
		if rule.DeviceID == deviceID && rule.Mode == model.WatchRuleTemporary &&
			rule.Enabled && rule.CompletedAt == nil && rule.ExpiresAt != nil && rule.ExpiresAt.After(now) {
			current = append(current, rule)
		}
	}
	return current, nil
}

func (m *Manager) pauseReminderState(state *model.WatchRefreshState, reason string, next, now time.Time) error {
	state.PausedReason = reason
	state.NextAttemptAt = next.UTC()
	state.UpdatedAt = now.UTC()
	return m.repository.SaveWatchRefreshState(*state)
}

func reminderRulesActiveAt(rules []model.WatchRule, now time.Time) (bool, time.Time, error) {
	var earliest time.Time
	for _, rule := range rules {
		active, next, err := reminderRuleActiveAt(rule, now)
		if err != nil {
			return false, time.Time{}, err
		}
		if active {
			return true, now.UTC(), nil
		}
		if earliest.IsZero() || next.Before(earliest) {
			earliest = next
		}
	}
	if earliest.IsZero() {
		return false, now.UTC().Add(24 * time.Hour), nil
	}
	return false, earliest.UTC(), nil
}

func reminderRuleActiveAt(rule model.WatchRule, now time.Time) (bool, time.Time, error) {
	if rule.CompletedAt != nil {
		return false, now.UTC().Add(24 * time.Hour), nil
	}
	if rule.Mode == model.WatchRuleTemporary {
		if rule.ExpiresAt == nil || !rule.ExpiresAt.After(now) {
			return false, now.UTC().Add(24 * time.Hour), nil
		}
		return true, now.UTC(), nil
	}
	location, err := time.LoadLocation(rule.Timezone)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("load watch rule timezone: %w", err)
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	todaySelected := reminderWeekdaySelected(rule.ActiveWeekdays, local.Weekday())
	active := false
	switch {
	case rule.ActiveStartMinute == rule.ActiveEndMinute:
		active = todaySelected
	case rule.ActiveStartMinute < rule.ActiveEndMinute:
		active = todaySelected && minute >= rule.ActiveStartMinute && minute < rule.ActiveEndMinute
	default:
		previousDay := local.AddDate(0, 0, -1).Weekday()
		active = (todaySelected && minute >= rule.ActiveStartMinute) ||
			(reminderWeekdaySelected(rule.ActiveWeekdays, previousDay) && minute < rule.ActiveEndMinute)
	}
	if active {
		return true, now.UTC(), nil
	}

	localMidnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	for dayOffset := 0; dayOffset <= 7; dayOffset++ {
		day := localMidnight.AddDate(0, 0, dayOffset)
		if !reminderWeekdaySelected(rule.ActiveWeekdays, day.Weekday()) {
			continue
		}
		minuteOfDay := rule.ActiveStartMinute
		if rule.ActiveStartMinute == rule.ActiveEndMinute {
			minuteOfDay = 0
		}
		candidate := day.Add(time.Duration(minuteOfDay) * time.Minute)
		if candidate.After(local) {
			return false, candidate.UTC(), nil
		}
	}
	return false, localMidnight.AddDate(0, 0, 7).UTC(), nil
}

func reminderWeekdaySelected(mask int, weekday time.Weekday) bool {
	// Bit 0 is Monday and bit 6 is Sunday.
	index := (int(weekday) + 6) % 7
	return mask&(1<<index) != 0
}

func scheduledPowerOffWindow(now time.Time, settings model.RegistrationSettings) (bool, time.Time, error) {
	if !settings.ScheduledPowerOffEnabled {
		return false, time.Time{}, nil
	}
	location, err := time.LoadLocation(settings.ScheduledPowerOffTimezone)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("load scheduled power-off timezone: %w", err)
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	start := settings.ScheduledPowerOffStartMinute
	end := settings.ScheduledPowerOffEndMinute
	if start == end {
		return false, time.Time{}, fmt.Errorf("scheduled power-off start and end cannot match")
	}
	if start < end {
		if minute >= start && minute < end {
			return true, midnight.Add(time.Duration(end) * time.Minute).UTC(), nil
		}
		return false, time.Time{}, nil
	}
	if minute >= start {
		return true, midnight.AddDate(0, 0, 1).Add(time.Duration(end) * time.Minute).UTC(), nil
	}
	if minute < end {
		return true, midnight.Add(time.Duration(end) * time.Minute).UTC(), nil
	}
	return false, time.Time{}, nil
}

func reminderQuotaDate(now time.Time, timezone string) (string, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("load reminder quota timezone: %w", err)
	}
	return now.In(location).Format("2006-01-02"), nil
}

func nextReminderQuotaDay(now time.Time, timezone string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("load reminder quota timezone: %w", err)
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location).UTC(), nil
}

func reminderFailureBackoff(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := initialReminderFailureBackoff
	for index := 1; index < failures && delay < maxReminderFailureBackoff; index++ {
		delay *= 2
		if delay >= maxReminderFailureBackoff {
			return maxReminderFailureBackoff
		}
	}
	return delay
}

func (c *reminderSchedulerCoordinator) initializeLocked() {
	if c.now == nil {
		c.now = time.Now
	}
	if c.intervalJitter == nil {
		c.intervalJitter = defaultReminderIntervalJitter
	}
	if c.restoreJitter == nil {
		c.restoreJitter = randomReminderDuration
	}
	if c.pollInterval <= 0 {
		c.pollInterval = defaultReminderSchedulerPollInterval
	}
	if c.maxConcurrency <= 0 {
		c.maxConcurrency = defaultReminderSchedulerConcurrency
	}
}

func (m *Manager) reminderSchedulerNow() time.Time {
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.initializeLocked()
	return coordinator.now()
}

func (m *Manager) reminderSchedulerPollInterval() time.Duration {
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.initializeLocked()
	return coordinator.pollInterval
}

func (m *Manager) reminderSchedulerConcurrency() int {
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.initializeLocked()
	return coordinator.maxConcurrency
}

func (m *Manager) reminderIntervalJitter(base time.Duration) time.Duration {
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.initializeLocked()
	return coordinator.intervalJitter(base)
}

func (m *Manager) reminderRestoreJitter(max time.Duration) time.Duration {
	coordinator := &m.reminderScheduler
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.initializeLocked()
	return coordinator.restoreJitter(max)
}

func defaultReminderIntervalJitter(base time.Duration) time.Duration {
	span := base / 10
	if span <= 0 {
		return base
	}
	return base - span + randomReminderDuration(2*span)
}

func randomReminderDuration(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(max)+1))
	if err != nil {
		return 0
	}
	return time.Duration(value.Int64())
}

func stopAndDrainTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
