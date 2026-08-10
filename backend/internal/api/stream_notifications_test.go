package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

type synchronizedStreamRecorder struct {
	header http.Header
	mu     sync.Mutex
	body   bytes.Buffer
}

func newSynchronizedStreamRecorder() *synchronizedStreamRecorder {
	return &synchronizedStreamRecorder{header: make(http.Header)}
}

func (r *synchronizedStreamRecorder) Header() http.Header { return r.header }
func (r *synchronizedStreamRecorder) WriteHeader(int)     {}
func (r *synchronizedStreamRecorder) Flush()              {}

func (r *synchronizedStreamRecorder) Write(payload []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(payload)
}

func (r *synchronizedStreamRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}

func TestDashboardStreamPublishesUserNotificationEvents(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "stream-notification-user", Password: "password123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	session, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/api/stream", nil).WithContext(ctx)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := newSynchronizedStreamRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.handleStream(recorder, request)
	}()

	waitForStreamText(t, recorder, "event: snapshot")
	if _, err := manager.RecordNotification(model.Notification{
		UserID: user.ID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "登录凭据已失效", Message: "请重新更新凭据", DedupeKey: "stream-test",
	}); err != nil {
		cancel()
		t.Fatalf("RecordNotification: %v", err)
	}
	waitForStreamText(t, recorder, "event: notification")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
	body := recorder.String()
	if !strings.Contains(body, `"type":"credential_expired"`) || strings.Contains(body, "dedupeKey") {
		t.Fatalf("unexpected notification stream payload: %s", body)
	}
}

func waitForStreamText(t *testing.T, recorder *synchronizedStreamRecorder, expected string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(recorder.String(), expected) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("stream did not contain %q: %s", expected, recorder.String())
}
