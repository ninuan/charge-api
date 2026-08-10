package runtime

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/persistence"
)

func TestReminderSchedulerDoesNotRequestWithoutEnabledReminderRules(t *testing.T) {
	var requests int32
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	deleteReminderRulesForUser(t, manager, owner.ID)
	deleteReminderRulesForUser(t, manager, other.ID)
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	if err := manager.repository.SaveWatchRefreshState(model.WatchRefreshState{
		UserID: owner.ID, DeviceID: testBackgroundPileID,
		NextAttemptAt: now, QuotaDate: "2026-08-10", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed orphan watch refresh state: %v", err)
	}

	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("runReminderSchedulerOnce: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("scheduler made %d requests without rules", got)
	}
	states, err := manager.repository.ListWatchRefreshStates("")
	if err != nil || len(states) != 0 {
		t.Fatalf("orphan scheduler states = %+v, err %v", states, err)
	}
}

func TestReminderSchedulerRefreshesOneWholePileAndEnforcesDailyQuota(t *testing.T) {
	var requests int32
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"number":"6201","status":"在线","opennum":10,"used":[3]}`, testBackgroundPileID)
	}))
	deleteReminderRulesForUser(t, manager, other.ID)
	settings := manager.Settings()
	settings.WatchDailyRefreshQuota = 1
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)

	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("first scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("one pile rule produced %d requests, want one whole-pile request", got)
	}
	state, ok, err := manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || !ok {
		t.Fatalf("LoadWatchRefreshState: ok %v, err %v", ok, err)
	}
	if state.QuotaUsed != 1 || state.ConsecutiveFailures != 0 || state.PausedReason != "" ||
		state.LastAttemptAt == nil || state.LastSuccessAt == nil || !state.NextAttemptAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("unexpected successful scheduler state: %+v", state)
	}
	events, err := manager.repository.PortStatusEvents(persistence.PortStatusEventQuery{
		UserID: owner.ID, DeviceID: testBackgroundPileID, Limit: 20,
	})
	if err != nil || len(events) != 10 {
		t.Fatalf("whole-pile history events = %d, err %v; want 10", len(events), err)
	}

	now = now.Add(11 * time.Minute)
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("quota scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("quota exhaustion produced another request; total %d", got)
	}
	state, _, err = manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || state.PausedReason != watchPauseQuota || state.QuotaUsed != 1 {
		t.Fatalf("quota state = %+v, err %v", state, err)
	}
	quotaSkips, err := manager.repository.MetricKindCount("watch_quota_skipped", time.Time{})
	if err != nil || quotaSkips != 1 {
		t.Fatalf("watch_quota_skipped metric = %d, err %v; want 1", quotaSkips, err)
	}
	settings.WatchDailyRefreshQuota = 2
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("increase daily quota: %v", err)
	}
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("quota resume scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("increased quota did not resume immediately; total requests %d", got)
	}
}

func TestReminderSchedulerRuleUpdateWakesFutureInactiveState(t *testing.T) {
	var requests int32
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"status":"在线","opennum":10}`, testBackgroundPileID)
	}))
	deleteReminderRulesForUser(t, manager, other.ID)
	rules, err := manager.WatchRules(owner.ID)
	if err != nil || len(rules) != 1 {
		t.Fatalf("WatchRules owner = %+v, err %v", rules, err)
	}
	start := 9 * 60
	end := 10 * 60
	if _, err := manager.UpdateWatchRule(owner.ID, rules[0].ID, model.WatchRuleUpdateRequest{
		ActiveStartMinute: &start, ActiveEndMinute: &end,
	}); err != nil {
		t.Fatalf("set inactive rule window: %v", err)
	}
	now := time.Date(2026, 8, 10, 2, 30, 0, 0, time.UTC) // Monday 10:30 Asia/Shanghai.
	setReminderTestClock(manager, &now)
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("inactive scheduler cycle: %v", err)
	}
	state, _, err := manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || state.PausedReason != watchPauseInactiveRule || !state.NextAttemptAt.After(now) {
		t.Fatalf("inactive state = %+v, err %v", state, err)
	}
	start = 0
	end = 0
	if _, err := manager.UpdateWatchRule(owner.ID, rules[0].ID, model.WatchRuleUpdateRequest{
		ActiveStartMinute: &start, ActiveEndMinute: &end,
	}); err != nil {
		t.Fatalf("activate rule window: %v", err)
	}
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("activated scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("active rule update produced %d requests, want 1", got)
	}
}

func TestReminderSchedulerPowerOffWindowClearsFailuresAndSpreadsRestore(t *testing.T) {
	var requests int32
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"number":"6201","status":"在线","opennum":10}`, testBackgroundPileID)
	}))
	now := time.Date(2026, 8, 10, 15, 30, 0, 0, time.UTC) // 23:30 Asia/Shanghai.
	setReminderTestClock(manager, &now)
	settings := manager.Settings()
	settings.BackgroundRemindersEnabled = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("disable reminders during power-off test: %v", err)
	}
	var jitterCalls int32
	manager.reminderScheduler.mu.Lock()
	manager.reminderScheduler.restoreJitter = func(max time.Duration) time.Duration {
		call := atomic.AddInt32(&jitterCalls, 1)
		return time.Duration(call*2) * time.Minute
	}
	manager.reminderScheduler.mu.Unlock()
	for _, userID := range []string{owner.ID, other.ID} {
		if err := manager.repository.SaveWatchRefreshState(model.WatchRefreshState{
			UserID: userID, DeviceID: testBackgroundPileID,
			NextAttemptAt: now, ConsecutiveFailures: 5,
			QuotaDate: "2026-08-10", UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SaveWatchRefreshState: %v", err)
		}
	}

	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("power-off scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("power-off window produced %d requests", got)
	}
	states, err := manager.repository.ListWatchRefreshStates("")
	if err != nil || len(states) != 2 {
		t.Fatalf("power-off states = %+v, err %v", states, err)
	}
	restoreBase := time.Date(2026, 8, 10, 23, 0, 0, 0, time.UTC) // 07:00 next day.
	for index, state := range states {
		if state.PausedReason != watchPausePowerOff || state.ConsecutiveFailures != 0 ||
			state.QuotaUsed != 0 || state.LastAttemptAt != nil ||
			!state.NextAttemptAt.Equal(restoreBase.Add(time.Duration(index+1)*2*time.Minute)) {
			t.Fatalf("power-off state %d = %+v", index, state)
		}
	}
	settings.BackgroundRemindersEnabled = true
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("enable reminders before restore: %v", err)
	}

	// Repeated checks during the same outage preserve the first persisted jitter.
	now = time.Date(2026, 8, 10, 22, 59, 0, 0, time.UTC) // 06:59 next day.
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("pre-restore scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&jitterCalls); got != 2 {
		t.Fatalf("restore jitter was regenerated %d times", got)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("pre-restore cycle produced %d requests", got)
	}

	now = restoreBase.Add(2 * time.Minute)
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("first restore scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("first restore slot produced %d requests, want 1", got)
	}
	now = restoreBase.Add(4 * time.Minute)
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("second restore scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("second restore slot produced total %d requests, want 2", got)
	}
}

func TestReminderSchedulerPersistsFailureBackoffAcrossRestart(t *testing.T) {
	var requests int32
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		http.Error(w, "temporary upstream failure", http.StatusBadGateway)
	}))
	deleteReminderRulesForUser(t, manager, other.ID)
	settings := manager.Settings()
	settings.ScheduledPowerOffEnabled = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("disable scheduled power-off for backoff test: %v", err)
	}
	start := time.Now().UTC().Truncate(time.Second)
	now := start
	setReminderTestClock(manager, &now)

	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("first failure cycle: %v", err)
	}
	state, ok, err := manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || !ok || state.ConsecutiveFailures != 1 || state.QuotaUsed != 1 ||
		state.PausedReason != watchPauseBackoff || !state.NextAttemptAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("first failure state = %+v, ok %v, err %v", state, ok, err)
	}
	now = now.Add(30 * time.Second)
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("backoff skip cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("backoff skip produced total %d requests", got)
	}

	restarted, err := NewManager(manager.repository, "", manager.requests, "", manager.minInterval)
	if err != nil {
		t.Fatalf("restart NewManager: %v", err)
	}
	setReminderTestClock(restarted, &now)
	if err := restarted.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("restart before due cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("restart bypassed persisted backoff; total %d", got)
	}

	now = start.Add(time.Minute)
	if err := restarted.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("second failure cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("due retry total requests = %d, want 2", got)
	}
	state, _, err = restarted.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || state.ConsecutiveFailures != 2 || state.QuotaUsed != 2 ||
		!state.NextAttemptAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("second failure state = %+v, err %v", state, err)
	}
}

func TestReminderSchedulerGlobalSwitchAndLifecycle(t *testing.T) {
	var requests int32
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"status":"在线","opennum":10}`, testBackgroundPileID)
	}))
	deleteReminderRulesForUser(t, manager, other.ID)
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	settings := manager.Settings()
	settings.BackgroundRemindersEnabled = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("disable reminders: %v", err)
	}
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("disabled scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("disabled scheduler produced %d requests", got)
	}
	state, _, err := manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || state.PausedReason != watchPauseGlobalDisabled {
		t.Fatalf("disabled state = %+v, err %v", state, err)
	}
	settings.BackgroundRemindersEnabled = true
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("enable reminders: %v", err)
	}
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("enabled scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("enabled scheduler produced %d requests, want 1", got)
	}

	deleteReminderRulesForUser(t, manager, owner.ID)
	manager.reminderScheduler.mu.Lock()
	manager.reminderScheduler.pollInterval = time.Hour
	manager.reminderScheduler.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.StartReminderScheduler(ctx); err != nil {
		t.Fatalf("StartReminderScheduler: %v", err)
	}
	if err := manager.StartReminderScheduler(ctx); err != ErrReminderSchedulerRunning {
		t.Fatalf("second StartReminderScheduler error = %v", err)
	}
	cancel()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := manager.WaitReminderScheduler(waitCtx); err != nil {
		t.Fatalf("WaitReminderScheduler: %v", err)
	}
}

func TestReminderSchedulerLimitsGlobalRemoteConcurrency(t *testing.T) {
	var requests int32
	var active int32
	var maxActive int32
	manager, owner, _ := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		current := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)
		for {
			observed := atomic.LoadInt32(&maxActive)
			if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"status":"在线","opennum":10}`, testBackgroundPileID)
	}))
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	pile := manager.runtimes[owner.ID].store.Snapshot().Piles[0]
	third := model.User{
		ID: "background-third", Username: "background-third", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, DeviceLimit: 10, RefreshEnabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	manager.mu.Lock()
	manager.users[third.ID] = third
	manager.runtimes[third.ID] = newUserRuntime(manager.requests, persistence.UserState{
		Piles: []model.Pile{pile}, DeviceIDs: []string{testBackgroundPileID}, Cookie: "sid=third-secret",
	}, manager.minInterval)
	manager.mu.Unlock()
	if err := manager.Save(); err != nil {
		t.Fatalf("Save third scheduler user: %v", err)
	}
	if _, err := manager.CreateWatchRule(third.ID, model.WatchRuleCreateRequest{
		DeviceID: testBackgroundPileID,
	}); err != nil {
		t.Fatalf("CreateWatchRule third user: %v", err)
	}
	setReminderTestClock(manager, &now)

	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("concurrency scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 3 {
		t.Fatalf("scheduler requests = %d, want 3", got)
	}
	if got := atomic.LoadInt32(&maxActive); got != defaultReminderSchedulerConcurrency {
		t.Fatalf("max scheduler concurrency = %d, want %d", got, defaultReminderSchedulerConcurrency)
	}
}

func TestReminderSchedulerChargesQuotaOnlyToSharedFlightLeader(t *testing.T) {
	var requests int32
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"status":"在线","opennum":10}`, testBackgroundPileID)
	}))
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	manager.markBackgroundCredentialsValidated(owner.ID, []model.Pile{{ID: testBackgroundPileID}}, now)
	manager.markBackgroundCredentialsValidated(other.ID, []model.Pile{{ID: testBackgroundPileID}}, now)

	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("shared-flight scheduler cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("shared scheduler flight produced %d requests, want 1", got)
	}
	quotaDate, err := reminderQuotaDate(now, manager.Settings().ScheduledPowerOffTimezone)
	if err != nil {
		t.Fatalf("reminderQuotaDate: %v", err)
	}
	for _, userID := range []string{owner.ID, other.ID} {
		state, ok, err := manager.repository.LoadWatchRefreshState(userID, testBackgroundPileID)
		if err != nil || !ok || state.LastSuccessAt == nil {
			t.Fatalf("shared-flight state for %s = %+v, ok %v, err %v", userID, state, ok, err)
		}
	}
	totalQuota := 0
	for _, userID := range []string{owner.ID, other.ID} {
		used, err := manager.repository.WatchRefreshQuotaUsed(userID, quotaDate)
		if err != nil {
			t.Fatalf("WatchRefreshQuotaUsed(%s): %v", userID, err)
		}
		totalQuota += used
	}
	if totalQuota != 1 {
		t.Fatalf("shared flight consumed %d total quota units, want 1", totalQuota)
	}
}

func TestReminderTimeWindowsHandleWeekdaysAndCrossMidnight(t *testing.T) {
	mondayDaytime := model.WatchRule{
		ActiveWeekdays: 1, ActiveStartMinute: 9 * 60, ActiveEndMinute: 10 * 60,
		Timezone: "Asia/Shanghai",
	}
	monday0930 := time.Date(2026, 8, 10, 1, 30, 0, 0, time.UTC)
	active, _, err := reminderRuleActiveAt(mondayDaytime, monday0930)
	if err != nil || !active {
		t.Fatalf("Monday daytime active = %v, err %v", active, err)
	}
	monday1030 := monday0930.Add(time.Hour)
	active, next, err := reminderRuleActiveAt(mondayDaytime, monday1030)
	if err != nil || active || next.In(time.FixedZone("UTC+8", 8*3600)).Weekday() != time.Monday {
		t.Fatalf("Monday daytime next = %v, active %v, err %v", next, active, err)
	}

	mondayOvernight := model.WatchRule{
		ActiveWeekdays: 1, ActiveStartMinute: 23 * 60, ActiveEndMinute: 7 * 60,
		Timezone: "Asia/Shanghai",
	}
	tuesday0630 := time.Date(2026, 8, 10, 22, 30, 0, 0, time.UTC)
	active, _, err = reminderRuleActiveAt(mondayOvernight, tuesday0630)
	if err != nil || !active {
		t.Fatalf("Monday overnight at Tuesday 06:30 active = %v, err %v", active, err)
	}
	tuesday0730 := tuesday0630.Add(time.Hour)
	active, _, err = reminderRuleActiveAt(mondayOvernight, tuesday0730)
	if err != nil || active {
		t.Fatalf("Monday overnight at Tuesday 07:30 active = %v, err %v", active, err)
	}
}

func deleteReminderRulesForUser(t *testing.T, manager *Manager, userID string) {
	t.Helper()
	rules, err := manager.WatchRules(userID)
	if err != nil {
		t.Fatalf("WatchRules(%s): %v", userID, err)
	}
	for _, rule := range rules {
		if err := manager.DeleteWatchRule(userID, rule.ID); err != nil {
			t.Fatalf("DeleteWatchRule(%s): %v", userID, err)
		}
	}
}

func setReminderTestClock(manager *Manager, now *time.Time) {
	manager.reminderScheduler.mu.Lock()
	manager.reminderScheduler.now = func() time.Time { return *now }
	manager.reminderScheduler.intervalJitter = func(base time.Duration) time.Duration { return base }
	manager.reminderScheduler.restoreJitter = func(max time.Duration) time.Duration { return 0 }
	manager.reminderScheduler.mu.Unlock()
	manager.backgroundRefresh.mu.Lock()
	manager.backgroundRefresh.now = func() time.Time { return *now }
	manager.backgroundRefresh.mu.Unlock()
}
