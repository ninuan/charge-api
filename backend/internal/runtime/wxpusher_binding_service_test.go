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

func TestWxPusherChannelPreferencesAndRecentDelivery(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherDefaultEventTypes)
	disabled := false
	eventTypes := []model.WxPusherEventType{
		model.WxPusherEventTypePileAvailable,
		model.WxPusherEventTypePileRecovered,
	}
	state, err := manager.UpdateWxPusherChannel(owner.ID, model.WxPusherChannelUpdateRequest{
		Enabled: &disabled, EventTypes: &eventTypes,
	}, true)
	if err != nil || state.Enabled || len(state.EventTypes) != 2 || state.EventTypes[1] != model.WxPusherEventTypePileRecovered {
		t.Fatalf("updated channel=%+v err=%v", state, err)
	}
	duplicate := []model.WxPusherEventType{model.WxPusherEventTypePileAvailable, model.WxPusherEventTypePileAvailable}
	if _, err := manager.UpdateWxPusherChannel(owner.ID, model.WxPusherChannelUpdateRequest{EventTypes: &duplicate}, true); !errors.Is(err, ErrWxPusherPreferenceInvalid) {
		t.Fatalf("duplicate preference error = %v", err)
	}
	if _, err := manager.UpdateWxPusherChannel(owner.ID, model.WxPusherChannelUpdateRequest{}, true); !errors.Is(err, ErrWxPusherPreferenceInvalid) {
		t.Fatalf("empty preference error = %v", err)
	}
}

func TestWxPusherTestDeliveryIsQueuedRateLimitedAndDelivered(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	client := &fakeNotificationDeliveryClient{}
	configureTestNotificationDispatcher(manager, client, &now)
	summary, err := manager.CreateWxPusherTestDelivery(owner.ID, true)
	if err != nil || !summary.IsTest || summary.Status != model.NotificationDeliveryPending ||
		summary.UserState != model.NotificationDeliveryUserQueued || summary.Message != "等待发送" {
		t.Fatalf("test delivery summary=%+v err=%v", summary, err)
	}
	if _, err := manager.CreateWxPusherTestDelivery(owner.ID, true); !errors.Is(err, ErrWxPusherRateLimited) {
		t.Fatalf("repeat test delivery error = %v", err)
	}
	if err := manager.runNotificationDispatcherOnce(context.Background(), now); err != nil {
		t.Fatalf("deliver test message: %v", err)
	}
	if client.sendCalls != 1 || len(client.messages) != 1 || client.messages[0].Summary != "Charge Console 测试消息" ||
		client.messages[0].Content != "微信提醒已连接。以后有空闲口或需要重新登录时，会发到这里。" {
		t.Fatalf("test provider messages=%+v calls=%d", client.messages, client.sendCalls)
	}
	state, err := manager.WxPusherChannelState(owner.ID, true)
	if err != nil || state.LastTestAt == nil || state.LastDelivery != nil || state.LastTestDelivery == nil || !state.LastTestDelivery.IsTest || state.LastTestDelivery.Status != model.NotificationDeliveryAccepted {
		t.Fatalf("channel after test=%+v err=%v", state, err)
	}
	notification := model.Notification{
		ID: "notice_recent", UserID: owner.ID, Type: model.NotificationCredentialExpired,
		Severity: "info", Title: "有空闲", Message: "有空闲", CreatedAt: now.Add(time.Minute),
	}
	if err := manager.repository.SaveNotification(notification); err != nil {
		t.Fatalf("save recent notification: %v", err)
	}
	notificationID := notification.ID
	formal := model.NotificationDelivery{
		ID: "ndl_recent", NotificationID: &notificationID, UserID: owner.ID, Channel: "wxpusher",
		Status: model.NotificationDeliveryProviderSucceeded, CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
	}
	if err := manager.repository.SaveNotificationDelivery(formal); err != nil {
		t.Fatalf("save recent formal delivery: %v", err)
	}
	state, err = manager.WxPusherChannelState(owner.ID, true)
	if err != nil || state.LastDelivery == nil || state.LastDelivery.IsTest || state.LastTestDelivery == nil || !state.LastTestDelivery.IsTest {
		t.Fatalf("separate recent channel deliveries=%+v err=%v", state, err)
	}
}

func TestWxPusherTestDeliveryDailyLimitSurvivesStoredHistory(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	saveTestWxPusherBinding(t, manager, owner.ID, now, true, model.WxPusherAllEventTypes)
	for index := 0; index < wxPusherTestDayMax; index++ {
		if _, err := manager.CreateWxPusherTestDelivery(owner.ID, true); err != nil {
			t.Fatalf("create test %d: %v", index, err)
		}
		now = now.Add(time.Minute)
	}
	if _, err := manager.CreateWxPusherTestDelivery(owner.ID, true); !errors.Is(err, ErrWxPusherRateLimited) {
		t.Fatalf("daily test limit error = %v", err)
	}
}

func TestWxPusherTestDeliveryRecheckQueriesExistingRecordWithoutResending(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	createdAt := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	now := createdAt.Add(2 * time.Minute)
	setReminderTestClock(manager, &now)
	saveTestWxPusherBinding(t, manager, owner.ID, createdAt, true, model.WxPusherAllEventTypes)
	client := &fakeNotificationDeliveryClient{statuses: []wxpusher.MessageStatus{{
		SendRecordID: "record-recheck", Status: "发送成功", Succeeded: true,
	}}}
	configureTestNotificationDispatcher(manager, client, &now)
	acceptedAt := createdAt.Add(5 * time.Second)
	delivery := model.NotificationDelivery{
		ID: "ndl_recheck", UserID: owner.ID, Channel: "wxpusher", IsTest: true,
		Status: model.NotificationDeliveryAccepted, ProviderRecordID: "record-recheck",
		AcceptedAt: &acceptedAt, CreatedAt: createdAt, UpdatedAt: acceptedAt,
	}
	if err := manager.repository.SaveNotificationDelivery(delivery); err != nil {
		t.Fatalf("save accepted test delivery: %v", err)
	}

	summary, err := manager.RecheckWxPusherTestDelivery(context.Background(), owner.ID)
	if err != nil {
		t.Fatalf("recheck test delivery: %v", err)
	}
	if summary.Status != model.NotificationDeliveryProviderSucceeded || summary.ProviderSucceededAt == nil ||
		!summary.CreatedAt.Equal(createdAt) || !summary.UpdatedAt.Equal(now) {
		t.Fatalf("rechecked summary=%+v", summary)
	}
	if client.queryCalls != 1 || client.sendCalls != 0 {
		t.Fatalf("provider calls query=%d send=%d", client.queryCalls, client.sendCalls)
	}
}

func TestWxPusherTestDeliveryRecheckKeepsUnconfirmedResultWithoutResending(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	createdAt := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	now := createdAt.Add(12 * time.Minute)
	setReminderTestClock(manager, &now)
	client := &fakeNotificationDeliveryClient{}
	configureTestNotificationDispatcher(manager, client, &now)
	delivery := model.NotificationDelivery{
		ID: "ndl_uncertain", UserID: owner.ID, Channel: "wxpusher", IsTest: true,
		Status: model.NotificationDeliveryUncertain, ProviderRecordID: "record-uncertain",
		LastErrorCode: "provider_status_unknown", AcceptedAt: &createdAt,
		CreatedAt: createdAt, UpdatedAt: createdAt.Add(10 * time.Minute),
	}
	if err := manager.repository.SaveNotificationDelivery(delivery); err != nil {
		t.Fatalf("save uncertain test delivery: %v", err)
	}

	summary, err := manager.RecheckWxPusherTestDelivery(context.Background(), owner.ID)
	if err != nil {
		t.Fatalf("recheck uncertain test delivery: %v", err)
	}
	if summary.Status != model.NotificationDeliveryUncertain || summary.UserState != model.NotificationDeliveryUserSubmitted ||
		summary.Message != "已发送，请检查手机" || summary.ErrorCode != "ambiguous_result" || !summary.UpdatedAt.Equal(now) {
		t.Fatalf("uncertain summary=%+v", summary)
	}
	if client.queryCalls != 1 || client.sendCalls != 0 {
		t.Fatalf("provider calls query=%d send=%d", client.queryCalls, client.sendCalls)
	}
}

type fakeWxPusherBindingClient struct {
	now         *time.Time
	uid         string
	queryCalls  int
	createCalls int
}

func (f *fakeWxPusherBindingClient) CreateQRCode(context.Context, string, time.Duration) (wxpusher.QRCode, error) {
	f.createCalls++
	return wxpusher.QRCode{
		Code: "provider-code", URL: "https://wxpusher.zjiecode.com/api/qrcode/test",
		ExpiresAt: f.now.Add(10 * time.Minute),
	}, nil
}

func (f *fakeWxPusherBindingClient) QueryScanUID(context.Context, string) (wxpusher.ScanResult, error) {
	f.queryCalls++
	return wxpusher.ScanResult{UID: f.uid, Scanned: f.uid != ""}, nil
}

func TestWxPusherBindingIsUserScopedAndUIDIsUnique(t *testing.T) {
	manager, owner, other := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	client := &fakeWxPusherBindingClient{now: &now, uid: "UID_shared_receiver"}

	ownerSession, err := manager.CreateWxPusherBindSession(context.Background(), owner.ID, client)
	if err != nil {
		t.Fatalf("create owner session: %v", err)
	}
	now = now.Add(10 * time.Second)
	ownerResult, err := manager.PollWxPusherBindSession(context.Background(), owner.ID, ownerSession.ID, client)
	if err != nil || ownerResult.Status != "bound" {
		t.Fatalf("owner bind result=%+v err=%v", ownerResult, err)
	}
	state, err := manager.WxPusherChannelState(owner.ID, true)
	if err != nil || !state.Bound || state.MaskedUID == client.uid {
		t.Fatalf("owner channel state=%+v err=%v", state, err)
	}

	otherSession, err := manager.CreateWxPusherBindSession(context.Background(), other.ID, client)
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	now = now.Add(10 * time.Second)
	otherResult, err := manager.PollWxPusherBindSession(context.Background(), other.ID, otherSession.ID, client)
	if err != nil || otherResult.Status != "failed" || otherResult.ErrorCode != "invalid_uid" {
		t.Fatalf("duplicate uid result=%+v err=%v", otherResult, err)
	}
	otherState, err := manager.WxPusherChannelState(other.ID, true)
	if err != nil || otherState.Bound {
		t.Fatalf("other channel state=%+v err=%v", otherState, err)
	}
}

func TestWxPusherPollingIntervalAndRestartRecovery(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	client := &fakeWxPusherBindingClient{now: &now}
	session, err := manager.CreateWxPusherBindSession(context.Background(), owner.ID, client)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	now = now.Add(5 * time.Second)
	if _, err := manager.PollWxPusherBindSession(context.Background(), owner.ID, session.ID, client); err != nil {
		t.Fatalf("early poll: %v", err)
	}
	if client.queryCalls != 0 {
		t.Fatalf("provider queried before nextPollAt: %d", client.queryCalls)
	}

	restarted, err := NewManager(manager.repository, "", parser.DefaultCaptureRequests(), "", 30*time.Second)
	if err != nil {
		t.Fatalf("restart manager: %v", err)
	}
	setReminderTestClock(restarted, &now)
	if _, err := restarted.PollWxPusherBindSession(context.Background(), owner.ID, session.ID, client); err != nil {
		t.Fatalf("restart early poll: %v", err)
	}
	if client.queryCalls != 0 {
		t.Fatalf("restart bypassed persisted poll interval: %d", client.queryCalls)
	}

	now = now.Add(5 * time.Second)
	waiting, err := restarted.PollWxPusherBindSession(context.Background(), owner.ID, session.ID, client)
	if err != nil || waiting.Status != "waiting_scan" || client.queryCalls != 1 {
		t.Fatalf("due poll=%+v calls=%d err=%v", waiting, client.queryCalls, err)
	}
	if _, err := restarted.PollWxPusherBindSession(context.Background(), owner.ID, session.ID, client); err != nil {
		t.Fatalf("repeat poll: %v", err)
	}
	if client.queryCalls != 1 {
		t.Fatalf("same timestamp caused duplicate provider poll: %d", client.queryCalls)
	}
}

func TestWxPusherQRExpiresAndCreateIsRateLimited(t *testing.T) {
	manager, owner, _ := newWatchTestManager(t)
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	setReminderTestClock(manager, &now)
	client := &fakeWxPusherBindingClient{now: &now}
	session, err := manager.CreateWxPusherBindSession(context.Background(), owner.ID, client)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := manager.CreateWxPusherBindSession(context.Background(), owner.ID, client); !errors.Is(err, ErrWxPusherBindSessionActive) {
		t.Fatalf("active session error=%v", err)
	}
	now = now.Add(time.Second)
	if err := manager.repository.CompleteWxPusherBindSession(owner.ID, session.ID, now); err != nil {
		t.Fatalf("complete session: %v", err)
	}
	if _, err := manager.CreateWxPusherBindSession(context.Background(), owner.ID, client); !errors.Is(err, ErrWxPusherRateLimited) {
		t.Fatalf("minute rate limit error=%v", err)
	}
	now = now.Add(10*time.Minute - time.Second)
	expired, err := manager.PollWxPusherBindSession(context.Background(), owner.ID, session.ID, client)
	if err != nil || expired.Status != "expired" || client.queryCalls != 0 {
		t.Fatalf("expired result=%+v calls=%d err=%v", expired, client.queryCalls, err)
	}
}
