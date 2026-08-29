package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/wxpusher"
)

type fakeNotificationDeliveryClient struct {
	sendCalls  int
	queryCalls int
	sendErr    error
	queryErr   error
	statuses   []wxpusher.MessageStatus
	messages   []wxpusher.Message
}

func (f *fakeNotificationDeliveryClient) Send(_ context.Context, message wxpusher.Message) (wxpusher.SendResult, error) {
	f.sendCalls++
	f.messages = append(f.messages, message)
	if f.sendErr != nil {
		return wxpusher.SendResult{}, f.sendErr
	}
	return wxpusher.SendResult{SendRecordID: "record-1", MessageContentID: "content-1"}, nil
}

func (f *fakeNotificationDeliveryClient) QueryMessageStatus(_ context.Context, _ string) (wxpusher.MessageStatus, error) {
	f.queryCalls++
	if f.queryErr != nil {
		return wxpusher.MessageStatus{}, f.queryErr
	}
	if len(f.statuses) == 0 {
		return wxpusher.MessageStatus{SendRecordID: "record-1", Status: "处理中"}, nil
	}
	status := f.statuses[0]
	f.statuses = f.statuses[1:]
	return status, nil
}

func TestNotificationDispatcherPersistsAcceptedThenProviderSucceededWithoutResend(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	client := &fakeNotificationDeliveryClient{statuses: []wxpusher.MessageStatus{
		{SendRecordID: "record-1", Status: "处理中"},
		{SendRecordID: "record-1", Status: "发送成功", Succeeded: true},
	}}
	configureTestNotificationDispatcher(manager, client, &now)
	notification, inserted, err := manager.recordNotificationOnce(model.Notification{
		UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "登录状态已失效", Message: "请重新登录后继续使用提醒。",
		DedupeKey: "dispatcher-accepted", CreatedAt: now,
	})
	if err != nil || !inserted {
		t.Fatalf("record notification = %+v, inserted %v, err %v", notification, inserted, err)
	}
	if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("send delivery: %v", err)
	}
	delivery := onlyTestDelivery(t, manager, owner.ID)
	if delivery.Status != model.NotificationDeliveryAccepted || delivery.ProviderRecordID != "record-1" || client.sendCalls != 1 {
		t.Fatalf("accepted delivery=%+v sendCalls=%d", delivery, client.sendCalls)
	}
	if len(client.messages) != 1 || client.messages[0].UID != "UID_dispatcher" ||
		client.messages[0].URL != "https://charge.example.com/dashboard?notification="+notification.ID {
		t.Fatalf("unsafe or incomplete provider message: %+v", client.messages)
	}
	delivery.LastErrorCode = "wxpusher_invalid_response"
	if err := manager.repository.SaveNotificationDelivery(delivery); err != nil {
		t.Fatalf("seed stale provider error: %v", err)
	}

	now = now.Add(30 * time.Second)
	if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("first status query: %v", err)
	}
	delivery = onlyTestDelivery(t, manager, owner.ID)
	if client.queryCalls != 1 || client.sendCalls != 1 || delivery.Status != model.NotificationDeliveryAccepted || delivery.LastErrorCode != "" {
		t.Fatalf("pending confirmation resent or finalized early: send=%d query=%d delivery=%+v", client.sendCalls, client.queryCalls, onlyTestDelivery(t, manager, owner.ID))
	}
	now = time.Date(2026, 8, 24, 1, 2, 0, 0, time.UTC)
	if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("final status query: %v", err)
	}
	delivery = onlyTestDelivery(t, manager, owner.ID)
	if delivery.Status != model.NotificationDeliveryProviderSucceeded || delivery.ProviderSucceededAt == nil || client.sendCalls != 1 || client.queryCalls != 2 {
		t.Fatalf("provider confirmation=%+v send=%d query=%d", delivery, client.sendCalls, client.queryCalls)
	}
}

func TestNotificationDispatcherStopsConfirmingAfterProviderStatusTimeout(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	client := &fakeNotificationDeliveryClient{statuses: []wxpusher.MessageStatus{
		{SendRecordID: "record-1", ProviderCode: 1, Status: "等待发送"},
		{SendRecordID: "record-1", ProviderCode: 1, Status: "等待发送"},
		{SendRecordID: "record-1", ProviderCode: 1, Status: "等待发送"},
	}}
	configureTestNotificationDispatcher(manager, client, &now)
	if _, inserted, err := manager.recordNotificationOnce(model.Notification{
		UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "登录状态已失效", Message: "请重新登录。", DedupeKey: "provider-status-timeout", CreatedAt: now,
	}); err != nil || !inserted {
		t.Fatalf("record notification: inserted=%v err=%v", inserted, err)
	}
	if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("send delivery: %v", err)
	}
	for _, elapsed := range []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute} {
		now = time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC).Add(elapsed)
		if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
			t.Fatalf("query at %s: %v", elapsed, err)
		}
	}
	delivery := onlyTestDelivery(t, manager, owner.ID)
	if delivery.Status != model.NotificationDeliveryUncertain || delivery.LastErrorCode != "provider_status_unknown" ||
		delivery.NextAttemptAt != nil || client.sendCalls != 1 || client.queryCalls != 3 {
		t.Fatalf("timed out confirmation=%+v send=%d query=%d", delivery, client.sendCalls, client.queryCalls)
	}
	if delivery.UserState() != model.NotificationDeliveryUserSubmitted {
		t.Fatalf("accepted delivery lost its submitted user state: %+v", delivery)
	}
}

func TestNotificationDispatcherFailureDoesNotRemoveInAppNotification(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	client := &fakeNotificationDeliveryClient{sendErr: errors.New("provider failed")}
	configureTestNotificationDispatcher(manager, client, &now)
	notification, inserted, err := manager.recordNotificationOnce(model.Notification{
		UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "登录状态已失效", Message: "请重新登录。", DedupeKey: "dispatcher-failure", CreatedAt: now,
	})
	if err != nil || !inserted {
		t.Fatalf("record notification: inserted=%v err=%v", inserted, err)
	}
	if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("run failed delivery: %v", err)
	}
	if delivery := onlyTestDelivery(t, manager, owner.ID); delivery.Status != model.NotificationDeliveryFailed {
		t.Fatalf("failed delivery state = %+v", delivery)
	}
	if stored, found, err := manager.repository.LoadNotification(owner.ID, notification.ID); err != nil || !found || stored.ID != notification.ID {
		t.Fatalf("in-app notification lost: %+v found=%v err=%v", stored, found, err)
	}
}

func TestNotificationOutboxSkipsDisabledTypesAndSuppressesQuietHours(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherEventPileAvailable)
	if _, inserted, err := manager.recordNotificationOnce(model.Notification{
		UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "凭据失效", Message: "请重新登录。", DedupeKey: "disabled-type", CreatedAt: now,
	}); err != nil || !inserted {
		t.Fatalf("record disabled type: inserted=%v err=%v", inserted, err)
	}
	if deliveries, err := manager.repository.ListNotificationDeliveries(owner.ID, 10); err != nil || len(deliveries) != 0 {
		t.Fatalf("disabled type created deliveries: %+v err=%v", deliveries, err)
	}

	now = time.Date(2026, 8, 24, 15, 30, 0, 0, time.UTC) // 23:30 Asia/Shanghai
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	if _, inserted, err := manager.recordNotificationOnce(model.Notification{
		UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "凭据失效", Message: "请重新登录。", DedupeKey: "quiet-hours", CreatedAt: now,
	}); err != nil || !inserted {
		t.Fatalf("record quiet notification: inserted=%v err=%v", inserted, err)
	}
	delivery := onlyTestDelivery(t, manager, owner.ID)
	if delivery.Status != model.NotificationDeliverySuppressed || delivery.LastErrorCode != "quiet_hours" {
		t.Fatalf("quiet delivery = %+v", delivery)
	}
}

func TestNotificationDispatcherClassifiesProviderFailures(t *testing.T) {
	tests := []struct {
		name        string
		providerErr error
		wantStatus  model.NotificationDeliveryStatus
		wantDelay   time.Duration
	}{
		{
			name:        "rate limited is retryable",
			providerErr: &wxpusher.Error{Code: wxpusher.ErrorRateLimited, Operation: "send_message", Retryable: true, HTTPStatus: 429, RetryAfter: 2 * time.Minute},
			wantStatus:  model.NotificationDeliveryRetryWait,
			wantDelay:   2 * time.Minute,
		},
		{
			name:        "known server failure is retryable",
			providerErr: &wxpusher.Error{Code: wxpusher.ErrorProviderUnavailable, Operation: "send_message", Retryable: true, HTTPStatus: 503},
			wantStatus:  model.NotificationDeliveryRetryWait,
		},
		{
			name:        "network result is ambiguous",
			providerErr: &wxpusher.Error{Code: wxpusher.ErrorProviderUnavailable, Operation: "send_message", Retryable: true},
			wantStatus:  model.NotificationDeliveryUncertain,
		},
		{
			name:        "timeout result is ambiguous",
			providerErr: &wxpusher.Error{Code: wxpusher.ErrorProviderTimeout, Operation: "send_message", Retryable: true},
			wantStatus:  model.NotificationDeliveryUncertain,
		},
		{
			name:        "invalid token is permanent",
			providerErr: &wxpusher.Error{Code: wxpusher.ErrorInvalidToken, Operation: "send_message"},
			wantStatus:  model.NotificationDeliveryFailed,
		},
		{
			name:        "invalid uid is permanent",
			providerErr: &wxpusher.Error{Code: wxpusher.ErrorInvalidUID, Operation: "send_message"},
			wantStatus:  model.NotificationDeliveryFailed,
		},
		{
			name:        "recipient rejection is permanent",
			providerErr: &wxpusher.Error{Code: wxpusher.ErrorRecipientRejected, Operation: "send_message"},
			wantStatus:  model.NotificationDeliveryFailed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, owner, _ := newWatchTestManager(t)
			now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
			saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
			client := &fakeNotificationDeliveryClient{sendErr: test.providerErr}
			configureTestNotificationDispatcher(manager, client, &now)
			if _, inserted, err := manager.recordNotificationOnce(model.Notification{
				UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
				Title: "凭据失效", Message: "请重新登录。", DedupeKey: "error-classification", CreatedAt: now,
			}); err != nil || !inserted {
				t.Fatalf("record notification: inserted=%v err=%v", inserted, err)
			}
			if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
				t.Fatalf("run dispatcher: %v", err)
			}
			delivery := onlyTestDelivery(t, manager, owner.ID)
			if delivery.Status != test.wantStatus || delivery.LastErrorMessage != "" {
				t.Fatalf("classified delivery = %+v, want %s", delivery, test.wantStatus)
			}
			if test.wantStatus == model.NotificationDeliveryRetryWait {
				wantDelay := test.wantDelay
				if wantDelay == 0 {
					wantDelay = time.Minute
				}
				if delivery.NextAttemptAt == nil || !delivery.NextAttemptAt.Equal(now.Add(wantDelay)) {
					t.Fatalf("retry schedule = %+v, want %s", delivery, wantDelay)
				}
			}
		})
	}
}

func TestNotificationDispatcherStopsAfterThreeKnownFailedAttempts(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	client := &fakeNotificationDeliveryClient{sendErr: &wxpusher.Error{
		Code: wxpusher.ErrorProviderUnavailable, Operation: "send_message", Retryable: true, HTTPStatus: 503,
	}}
	configureTestNotificationDispatcher(manager, client, &now)
	if _, inserted, err := manager.recordNotificationOnce(model.Notification{
		UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "凭据失效", Message: "请重新登录。", DedupeKey: "retry-limit", CreatedAt: now,
	}); err != nil || !inserted {
		t.Fatalf("record notification: inserted=%v err=%v", inserted, err)
	}
	for _, advance := range []time.Duration{0, time.Minute, 5 * time.Minute} {
		now = now.Add(advance)
		if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
			t.Fatalf("run retry at %s: %v", now, err)
		}
	}
	delivery := onlyTestDelivery(t, manager, owner.ID)
	if delivery.Status != model.NotificationDeliveryFailed || delivery.AttemptCount != 3 || delivery.NextAttemptAt != nil || client.sendCalls != 3 {
		t.Fatalf("retry limit delivery=%+v calls=%d", delivery, client.sendCalls)
	}
}

func TestNotificationDispatcherProcessesOneSendAtATime(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	client := &fakeNotificationDeliveryClient{}
	configureTestNotificationDispatcher(manager, client, &now)
	for _, key := range []string{"one", "two"} {
		if _, inserted, err := manager.recordNotificationOnce(model.Notification{
			UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
			Title: "凭据失效", Message: "请重新登录。", DedupeKey: key, CreatedAt: now,
		}); err != nil || !inserted {
			t.Fatalf("record %s: inserted=%v err=%v", key, inserted, err)
		}
	}
	if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	if client.sendCalls != 1 {
		t.Fatalf("one dispatcher cycle sent %d messages", client.sendCalls)
	}
}

func TestAcceptedDeliveryResumesStatusQueryAfterManagerRestart(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	firstClient := &fakeNotificationDeliveryClient{}
	configureTestNotificationDispatcher(manager, firstClient, &now)
	if _, inserted, err := manager.recordNotificationOnce(model.Notification{
		UserID: owner.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "凭据失效", Message: "请重新登录。", DedupeKey: "restart", CreatedAt: now,
	}); err != nil || !inserted {
		t.Fatalf("record restart notification: inserted=%v err=%v", inserted, err)
	}
	if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("initial send: %v", err)
	}
	restarted, err := NewManager(manager.repository, "", parser.DefaultCaptureRequests(), "", 30*time.Second)
	if err != nil {
		t.Fatalf("restart manager: %v", err)
	}
	now = now.Add(30 * time.Second)
	restartClient := &fakeNotificationDeliveryClient{statuses: []wxpusher.MessageStatus{{Succeeded: true, Status: "发送成功"}}}
	configureTestNotificationDispatcher(restarted, restartClient, &now)
	if err := restarted.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("restart status query: %v", err)
	}
	if restartClient.sendCalls != 0 || restartClient.queryCalls != 1 || onlyTestDelivery(t, restarted, owner.ID).Status != model.NotificationDeliveryProviderSucceeded {
		t.Fatalf("restart resent accepted delivery: send=%d query=%d delivery=%+v", restartClient.sendCalls, restartClient.queryCalls, onlyTestDelivery(t, restarted, owner.ID))
	}
}

func TestNotificationDispatcherLifecycle(t *testing.T) {
	manager, _, _ := newWatchTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeNotificationDeliveryClient{}
	if err := manager.StartNotificationDispatcher(ctx, client, "https://charge.example.com"); err != nil {
		t.Fatalf("start dispatcher: %v", err)
	}
	if err := manager.StartNotificationDispatcher(ctx, client, "https://charge.example.com"); !errors.Is(err, ErrNotificationDispatcherRunning) {
		t.Fatalf("second start error = %v", err)
	}
	cancel()
	waitCtx, stopWait := context.WithTimeout(context.Background(), time.Second)
	defer stopWait()
	if err := manager.WaitNotificationDispatcher(waitCtx); err != nil {
		t.Fatalf("wait dispatcher: %v", err)
	}
}

func configureTestNotificationDispatcher(manager *Manager, client NotificationDeliveryClient, now *time.Time) {
	manager.notificationDispatcher.client = client
	manager.notificationDispatcher.publicBaseURL = "https://charge.example.com"
	manager.notificationDispatcher.now = func() time.Time { return *now }
}

func saveTestWxPusherBinding(t *testing.T, manager *Manager, userID string, now time.Time, enabled bool, eventTypes model.WxPusherEventTypes) {
	t.Helper()
	if err := manager.repository.SaveWxPusherBinding(model.WxPusherBinding{
		UserID: userID, UID: "UID_dispatcher", Enabled: enabled, EventTypes: eventTypes,
		BoundAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save wxpusher binding: %v", err)
	}
}

func onlyTestDelivery(t *testing.T, manager *Manager, userID string) model.NotificationDelivery {
	t.Helper()
	deliveries, err := manager.repository.ListNotificationDeliveries(userID, 10)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("delivery count=%d deliveries=%+v err=%v", len(deliveries), deliveries, err)
	}
	return deliveries[0]
}
