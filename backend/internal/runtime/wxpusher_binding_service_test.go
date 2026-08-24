package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/wxpusher"
)

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
