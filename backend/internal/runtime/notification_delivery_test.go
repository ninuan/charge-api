package runtime

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestIdleTransitionRecoveryCreatesOneDurableNotification(t *testing.T) {
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	deleteReminderRulesForUser(t, manager, other.ID)
	changedAt := time.Now().UTC().Truncate(time.Second).Add(time.Minute)
	ports := make([]model.Port, 0, 10)
	for portID := 1; portID <= 10; portID++ {
		status := model.PortInUse
		ports = append(ports, model.Port{ID: portID, Status: status, UpdatedAt: changedAt})
	}
	pile := model.Pile{ID: testBackgroundPileID, Online: true, UpdatedAt: changedAt, Ports: ports}
	if _, err := manager.repository.RecordPortStatusTransitions(owner.ID, []model.Pile{pile}); err != nil {
		t.Fatalf("record in-use baseline: %v", err)
	}
	if err := manager.repository.SavePileAvailabilityState(owner.ID, pile.ID, true, false, 0, changedAt); err != nil {
		t.Fatalf("save unavailable baseline: %v", err)
	}
	pile.Ports[0].Status = model.PortIdle
	pile.Ports[0].UpdatedAt = changedAt.Add(5 * time.Minute)
	events, err := manager.repository.RecordPortStatusTransitions(owner.ID, []model.Pile{pile})
	if err != nil || len(events) != 1 {
		t.Fatalf("record idle transition = %+v, err %v", events, err)
	}
	manager.runtimes[owner.ID].store.MergeCapturePiles([]model.Pile{pile})
	if err := manager.Save(); err != nil {
		t.Fatalf("persist idle snapshot before recovery: %v", err)
	}

	stream, err := manager.SubscribeNotifications(owner.ID)
	if err != nil {
		t.Fatalf("SubscribeNotifications: %v", err)
	}
	defer manager.UnsubscribeNotifications(owner.ID, stream)
	if err := manager.recoverPendingPileAvailabilityNotifications(100); err != nil {
		t.Fatalf("recover pending pile availability: %v", err)
	}
	select {
	case notification := <-stream:
		if notification.Type != model.NotificationPileAvailable || notification.Title != "充电桩有空闲口" ||
			notification.SourceEventID == nil || *notification.SourceEventID != events[0].ID {
			t.Fatalf("unexpected streamed notification: %+v", notification)
		}
	case <-time.After(time.Second):
		t.Fatal("idle notification was not published")
	}

	restarted, err := NewManager(manager.repository, "", manager.requests, "", manager.minInterval)
	if err != nil {
		t.Fatalf("restart manager: %v", err)
	}
	if err := restarted.recoverPendingPileAvailabilityNotifications(100); err != nil {
		t.Fatalf("repeat availability recovery: %v", err)
	}
	notifications, err := restarted.repository.ListNotifications(owner.ID, 20)
	if err != nil || len(notifications) != 1 || notifications[0].SourceEventID == nil || *notifications[0].SourceEventID != events[0].ID {
		t.Fatalf("durable idle notifications = %+v, err %v", notifications, err)
	}

	// A second port becoming idle while the pile already has an idle port must
	// not create another notification.
	pile.Ports[1].Status = model.PortIdle
	pile.Ports[1].UpdatedAt = changedAt.Add(6 * time.Minute)
	additionalEvents, err := manager.repository.RecordPortStatusTransitions(owner.ID, []model.Pile{pile})
	if err != nil || len(additionalEvents) != 1 {
		t.Fatalf("record additional idle transition = %+v, err %v", additionalEvents, err)
	}
	if err := manager.processPileAvailability(owner.ID, []model.Pile{pile}, additionalEvents); err != nil {
		t.Fatalf("process additional idle transition: %v", err)
	}
	notifications, err = manager.repository.ListNotifications(owner.ID, 20)
	if err != nil || len(notifications) != 1 {
		t.Fatalf("additional idle port repeated notification: %+v, err %v", notifications, err)
	}
	pending, err := manager.repository.UnnotifiedIdleTransitions(100)
	if err != nil || len(pending) != 0 {
		t.Fatalf("processed aggregate events remained on recovery cursor: %+v, err %v", pending, err)
	}

	// Two live manager instances may both recover the same later event during a
	// rolling restart. The database source-event key must still admit one row.
	pile.Ports[0].Status = model.PortInUse
	pile.Ports[0].UpdatedAt = changedAt.Add(7 * time.Minute)
	pile.Ports[1].Status = model.PortInUse
	pile.Ports[1].UpdatedAt = changedAt.Add(7 * time.Minute)
	if _, err := manager.repository.RecordPortStatusTransitions(owner.ID, []model.Pile{pile}); err != nil {
		t.Fatalf("record second in-use transition: %v", err)
	}
	if err := manager.processPileAvailability(owner.ID, []model.Pile{pile}, nil); err != nil {
		t.Fatalf("record second unavailable aggregate: %v", err)
	}
	pile.Ports[0].Status = model.PortIdle
	pile.Ports[0].UpdatedAt = changedAt.Add(8 * time.Minute)
	secondEvents, err := manager.repository.RecordPortStatusTransitions(owner.ID, []model.Pile{pile})
	if err != nil || len(secondEvents) != 1 {
		t.Fatalf("record second idle transition = %+v, err %v", secondEvents, err)
	}
	var wait sync.WaitGroup
	errorsByWorker := make(chan error, 2)
	for _, current := range []*Manager{manager, restarted} {
		wait.Add(1)
		go func(current *Manager) {
			defer wait.Done()
			errorsByWorker <- current.processPileAvailability(owner.ID, []model.Pile{pile}, secondEvents)
		}(current)
	}
	wait.Wait()
	close(errorsByWorker)
	for workerErr := range errorsByWorker {
		if workerErr != nil {
			t.Fatalf("concurrent notification generation: %v", workerErr)
		}
	}
	notifications, err = restarted.repository.ListNotifications(owner.ID, 20)
	if err != nil || len(notifications) != 2 {
		t.Fatalf("concurrent durable notifications = %+v, err %v", notifications, err)
	}
	unchanged, err := manager.repository.RecordPortStatusTransitions(owner.ID, []model.Pile{pile})
	if err != nil || len(unchanged) != 0 {
		t.Fatalf("unchanged idle status created events = %+v, err %v", unchanged, err)
	}
}

func TestPowerRestoreAvailabilityEstablishesBaselineWithoutNotification(t *testing.T) {
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	deleteReminderRulesForUser(t, manager, other.ID)
	now := time.Now().UTC().Truncate(time.Second).Add(time.Minute)
	offline := model.PortOffline
	pile := model.Pile{
		ID: testBackgroundPileID, Online: true, UpdatedAt: now,
		Ports: []model.Port{{ID: 1, Status: model.PortIdle, UpdatedAt: now}},
	}
	if err := manager.repository.SavePileAvailabilityState(owner.ID, pile.ID, true, false, 0, now.Add(-time.Minute)); err != nil {
		t.Fatalf("save power-off baseline: %v", err)
	}
	if err := manager.processPileAvailability(owner.ID, []model.Pile{pile}, []model.PortStatusEvent{{
		ID: 1, UserID: owner.ID, DeviceID: pile.ID, PortID: 1,
		FromStatus: &offline, ToStatus: model.PortIdle, ChangedAt: now,
	}}); err != nil {
		t.Fatalf("process power restore availability: %v", err)
	}
	notifications, err := manager.repository.ListNotifications(owner.ID, 20)
	if err != nil || len(notifications) != 0 {
		t.Fatalf("power restore created notifications: %+v, err %v", notifications, err)
	}
	state, ok, err := manager.repository.LoadWatchRefreshState(owner.ID, pile.ID)
	if err != nil || !ok || !state.AvailabilityKnown || !state.HadIdlePort {
		t.Fatalf("power restore baseline = %+v, ok %v, err %v", state, ok, err)
	}
}

func TestCredentialExpiryNotifiesOncePausesAndResumesAfterValidation(t *testing.T) {
	var requests int32
	var valid atomic.Bool
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		if !valid.Load() {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"number":"6201","status":"在线","opennum":10}`, testBackgroundPileID)
	}))
	deleteReminderRulesForUser(t, manager, other.ID)
	settings := manager.Settings()
	settings.ScheduledPowerOffEnabled = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("disable power window: %v", err)
	}
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)

	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("expired credential cycle: %v", err)
	}
	state, ok, err := manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || !ok || state.PausedReason != watchPauseCredentialExpired {
		t.Fatalf("credential pause state = %+v, ok %v, err %v", state, ok, err)
	}
	notifications, err := manager.repository.ListNotifications(owner.ID, 20)
	if err != nil || len(notifications) != 1 || notifications[0].Type != model.NotificationCredentialExpired || notifications[0].ResolvedAt != nil {
		t.Fatalf("credential notifications = %+v, err %v", notifications, err)
	}
	now = now.Add(time.Hour)
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("paused credential cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("credential pause allowed %d requests, want one", got)
	}

	valid.Store(true)
	if _, err := manager.UpdateCookie(owner.ID, "sid=fresh-secret"); err != nil {
		t.Fatalf("validate replacement credential: %v", err)
	}
	state, _, err = manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || state.PausedReason == watchPauseCredentialExpired {
		t.Fatalf("credential state did not resume: %+v, err %v", state, err)
	}
	notifications, err = manager.repository.ListNotifications(owner.ID, 20)
	if err != nil || len(notifications) != 1 || notifications[0].ResolvedAt == nil {
		t.Fatalf("credential notification was not resolved: %+v, err %v", notifications, err)
	}
}

func TestOfflineNotificationWaitsForPowerRestoreThreeChecksAndThirtyMinutes(t *testing.T) {
	var requests int32
	var online atomic.Bool
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		status := "离线"
		if online.Load() {
			status = "在线"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"number":"6201","status":%q,"opennum":10}`, testBackgroundPileID, status)
	}))
	deleteReminderRulesForUser(t, manager, other.ID)
	now := time.Date(2026, 8, 10, 15, 30, 0, 0, time.UTC) // 23:30 Asia/Shanghai.
	setReminderTestClock(manager, &now)

	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("power-off cycle: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("scheduled outage made %d requests", got)
	}
	if notifications, err := manager.repository.ListNotifications(owner.ID, 20); err != nil || len(notifications) != 0 {
		t.Fatalf("scheduled outage notifications = %+v, err %v", notifications, err)
	}

	restoredAt := time.Date(2026, 8, 10, 23, 0, 0, 0, time.UTC) // 07:00 next day.
	for _, elapsed := range []time.Duration{0, 10 * time.Minute, 20 * time.Minute} {
		now = restoredAt.Add(elapsed)
		if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
			t.Fatalf("offline check at %v: %v", elapsed, err)
		}
	}
	state, _, stateErr := manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if stateErr != nil || state.PausedReason != watchPausePileOffline || state.ConsecutiveFailures != 3 {
		t.Fatalf("pre-threshold offline state = %+v, err %v", state, stateErr)
	}
	if notifications, err := manager.repository.ListNotifications(owner.ID, 20); err != nil || len(notifications) != 0 {
		t.Fatalf("early offline notifications = %+v, err %v", notifications, err)
	}
	now = restoredAt.Add(30 * time.Minute)
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("offline threshold check: %v", err)
	}
	state, _, stateErr = manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if stateErr != nil || state.ConsecutiveFailures != 4 {
		t.Fatalf("threshold offline state = %+v, err %v", state, stateErr)
	}
	notifications, err := manager.repository.ListNotifications(owner.ID, 20)
	if err != nil || len(notifications) != 1 || notifications[0].Type != model.NotificationPileOffline ||
		notifications[0].Title != "充电桩持续离线" || notifications[0].ResolvedAt != nil {
		t.Fatalf("offline threshold notifications = %+v, err %v", notifications, err)
	}

	online.Store(true)
	now = restoredAt.Add(40 * time.Minute)
	if err := manager.runReminderSchedulerOnce(context.Background(), now); err != nil {
		t.Fatalf("online recovery check: %v", err)
	}
	notifications, err = manager.repository.ListNotifications(owner.ID, 20)
	if err != nil || len(notifications) != 1 || notifications[0].ResolvedAt == nil {
		t.Fatalf("offline notification was not resolved: %+v, err %v", notifications, err)
	}
	state, _, err = manager.repository.LoadWatchRefreshState(owner.ID, testBackgroundPileID)
	if err != nil || state.PausedReason != "" || state.ConsecutiveFailures != 0 {
		t.Fatalf("online recovery state = %+v, err %v", state, err)
	}
}
