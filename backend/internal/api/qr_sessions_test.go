package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/yyb"
)

func TestQRBindingOnboarding(t *testing.T) {
	for _, status := range []string{"pending", "scanned", "expired", "cancelled", "unknown", "authorized", "confirmed"} {
		t.Run(status, func(t *testing.T) {
			server, manager, sessions := newTestServer(t)
			user, err := manager.CreateUser(model.UserCreateRequest{Username: "qr-user", Password: "password123", Role: model.RoleUser})
			if err != nil {
				t.Fatal(err)
			}
			login, err := sessions.Create(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeAPIYYBClient{pollStatus: status}
			server.SetYYBIntegration(client, &fakeAPIMoceleClient{})
			mux := http.NewServeMux()
			server.Register(mux)
			call := func(method, path string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, nil)
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: login.Token})
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				return rec
			}
			rec := call("GET", "/api/session/yyb-binding")
			if !strings.Contains(rec.Body.String(), `"scanEnabled":true`) || !strings.Contains(rec.Body.String(), `"bound":false`) {
				t.Fatal(rec.Body.String())
			}
			// Knowledge of a sidecar session ID does not grant access to it.
			if got := call("POST", "/api/session/yyb-qr/sid-1/confirm"); got.Code != 404 {
				t.Fatal(got.Code)
			}
			server.rememberQR("foreign", "another-user")
			for _, action := range []string{"poll", "confirm"} {
				method := "GET"
				if action == "confirm" {
					method = "POST"
				}
				if got := call(method, "/api/session/yyb-qr/foreign/"+action); got.Code != 404 {
					t.Fatal(got.Code)
				}
			}
			if got := call("POST", "/api/session/yyb-qr"); got.Code != 200 {
				t.Fatal(got.Body.String())
			}
			confirmed := call("POST", "/api/session/yyb-qr/sid-1/confirm")
			ready := status == "authorized" || status == "confirmed"
			if !ready {
				if confirmed.Code != 409 || client.confirmCalls != 0 {
					t.Fatalf("premature confirmation: %d calls=%d", confirmed.Code, client.confirmCalls)
				}
				binding, _ := manager.YYBBinding(user.ID)
				if binding != nil {
					t.Fatal("saved unauthorized binding")
				}
				return
			}
			if confirmed.Code != 200 {
				t.Fatal(confirmed.Body.String())
			}
			var result map[string]any
			if err := json.Unmarshal(confirmed.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["bound"] != true || result["sessionId"] != "sid-1" || result["syncState"] != "not_needed" {
				t.Fatal(result)
			}
			// Lost response recovery is linked to this session, not a pre-existing binding.
			if got := call("GET", "/api/session/yyb-qr/sid-1/poll"); !strings.Contains(got.Body.String(), `"status":"saved"`) {
				t.Fatal(got.Body.String())
			}
			if got := call("POST", "/api/session/yyb-qr/sid-1/confirm"); got.Code != 200 || client.confirmCalls != 1 {
				t.Fatalf("not idempotent: %d calls=%d", got.Code, client.confirmCalls)
			}
			call("DELETE", "/api/session/yyb-binding")
			if got := call("GET", "/api/session/yyb-qr/sid-1/poll"); got.Code != 404 {
				t.Fatal("unbound session reused")
			}
		})
	}
}

func TestQRRegistryExpiryAndDuplicateOwnership(t *testing.T) {
	server, _, _ := newTestServer(t)
	if !server.rememberQR("session", "a") || server.rememberQR("session", "b") {
		t.Fatal("duplicate session transferred")
	}
	entry := server.findQR("session", "a")
	if entry == nil || server.findQR("session", "b") != nil {
		t.Fatal("ownership")
	}
	entry.expires = time.Now().Add(-time.Second)
	if server.findQR("session", "a") != nil {
		t.Fatal("expired accepted")
	}
	if !server.rememberQR("session", "b") {
		t.Fatal("expired not pruned")
	}
}

func TestAddPileRescanErrorsAreSpecific(t *testing.T) {
	for _, err := range []error{yyb.ErrAccountExpired, yyb.ErrAccountUnknown, yyb.ErrAccountRecoveryFailed, errors.New("upstream unavailable")} {
		rec := httptest.NewRecorder()
		writeAddPileError(rec, err)
		needsScan := errors.Is(err, yyb.ErrAccountExpired) || errors.Is(err, yyb.ErrAccountUnknown)
		if strings.Contains(rec.Body.String(), "YYB_RESCAN_REQUIRED") != needsScan {
			t.Fatal(rec.Body.String())
		}
	}
}

func TestQRConfirmKeepsSavedBindingWhenSyncFails(t *testing.T) {
	server, manager, sessions, user := newTestServerWithDevice(t, "2601201412385560001")
	login, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A failed upstream exchange occurs after the binding has already been saved.
	server.SetYYBIntegration(&fakeAPIYYBClient{code: "test-code"}, &fakeAPIMoceleClient{exchangeErr: errors.New("upstream unavailable")})
	server.rememberQR("sid-1", user.ID)
	req := httptest.NewRequest("POST", "/api/session/yyb-qr/sid-1/confirm", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: login.Token})
	rec := httptest.NewRecorder()
	server.handleYYBQR(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"syncState":"failed"`) || !strings.Contains(rec.Body.String(), `"bound":true`) {
		t.Fatal(rec.Body.String())
	}
	binding, err := manager.YYBBinding(user.ID)
	if err != nil || binding == nil {
		t.Fatalf("binding lost: %v", err)
	}
}

type blockedQRClient struct {
	fakeAPIYYBClient
	entered chan struct{}
	release chan struct{}
}

func (c *blockedQRClient) ConfirmQR(ctx context.Context, id string) (yyb.YYBAccount, error) {
	if id == "first" {
		close(c.entered)
		select {
		case <-c.release:
		case <-ctx.Done():
			return yyb.YYBAccount{}, ctx.Err()
		}
	}
	return c.fakeAPIYYBClient.ConfirmQR(ctx, id)
}

func TestQRConfirmationExcludesConcurrentBindingMutations(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	user, err := manager.CreateUser(model.UserCreateRequest{Username: "concurrent-qr", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	login, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	client := &blockedQRClient{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(client.release)
	server.SetYYBIntegration(client, &fakeAPIMoceleClient{})
	server.rememberQR("first", user.ID)
	server.rememberQR("second", user.ID)
	mux := http.NewServeMux()
	server.Register(mux)
	call := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: login.Token})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- call("POST", "/api/session/yyb-qr/first/confirm") }()
	select {
	case <-client.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("confirmation did not start")
	}
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/session/yyb-qr/second/confirm"},
		{"DELETE", "/api/session/yyb-binding"},
	} {
		if got := call(tc.method, tc.path); got.Code != 409 {
			t.Errorf("concurrent %s: status=%d body=%s", tc.path, got.Code, got.Body.String())
		}
	}
	client.release <- struct{}{}
	select {
	case got := <-done:
		if got.Code != 200 {
			t.Fatal(got.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("confirmation did not finish")
	}
	// A successful newer binding invalidates old QR confirmation evidence.
	if got := call("POST", "/api/session/yyb-qr/second/confirm"); got.Code != 404 {
		t.Errorf("obsolete session accepted: %d", got.Code)
	}
	if got := call("DELETE", "/api/session/yyb-binding"); got.Code != 204 {
		t.Fatal(got.Body.String())
	}
	binding, err := manager.YYBBinding(user.ID)
	if err != nil || binding != nil {
		t.Fatalf("deleted binding resurrected: %+v %v", binding, err)
	}
}
