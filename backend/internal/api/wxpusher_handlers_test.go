package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/wxpusher"
)

type fakeAPIWxPusherClient struct{}

func (fakeAPIWxPusherClient) CreateQRCode(context.Context, string, time.Duration) (wxpusher.QRCode, error) {
	return wxpusher.QRCode{
		Code:      "provider-secret-code",
		URL:       "https://wxpusher.zjiecode.com/api/qrcode/test",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}, nil
}

func (fakeAPIWxPusherClient) QueryScanUID(context.Context, string) (wxpusher.ScanResult, error) {
	return wxpusher.ScanResult{}, nil
}

func TestWxPusherBindingAPIIsPrivateConfiguredAndUserScoped(t *testing.T) {
	server, manager, sessions := newTestServer(t)
	owner, err := manager.CreateUser(model.UserCreateRequest{
		Username: "wx-api-owner", Password: "password123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	other, err := manager.CreateUser(model.UserCreateRequest{
		Username: "wx-api-other", Password: "password123", Role: model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	ownerSession, _ := sessions.Create(owner.ID)
	otherSession, _ := sessions.Create(other.ID)
	mux := http.NewServeMux()
	server.Register(mux)

	stateResponse := wxPusherAPIRequest(t, mux, ownerSession, http.MethodGet, "/api/notification-channels/wxpusher")
	if stateResponse.Code != http.StatusOK {
		t.Fatalf("unconfigured state=%d body=%s", stateResponse.Code, stateResponse.Body.String())
	}
	var state model.WxPusherChannelState
	if err := json.NewDecoder(stateResponse.Body).Decode(&state); err != nil || state.Configured || state.Bound {
		t.Fatalf("unconfigured state=%+v err=%v", state, err)
	}
	missingTest := wxPusherAPIRequest(t, mux, ownerSession, http.MethodPost, "/api/notification-channels/wxpusher/test/recheck")
	if missingTest.Code != http.StatusNotFound || !strings.Contains(missingTest.Body.String(), "WXPUSHER_TEST_NOT_FOUND") {
		t.Fatalf("missing test recheck=%d body=%s", missingTest.Code, missingTest.Body.String())
	}
	unconfigured := wxPusherAPIRequest(t, mux, ownerSession, http.MethodPost, "/api/notification-channels/wxpusher/bind-sessions")
	if unconfigured.Code != http.StatusServiceUnavailable || !strings.Contains(unconfigured.Body.String(), "WXPUSHER_NOT_CONFIGURED") {
		t.Fatalf("unconfigured create=%d body=%s", unconfigured.Code, unconfigured.Body.String())
	}

	server.wxPusherClient = fakeAPIWxPusherClient{}
	invalidUpdate := wxPusherAPIJSONRequest(t, mux, ownerSession, http.MethodPatch, "/api/notification-channels/wxpusher", `{}`)
	if invalidUpdate.Code != http.StatusBadRequest || !strings.Contains(invalidUpdate.Body.String(), "WXPUSHER_INVALID") {
		t.Fatalf("invalid update=%d body=%s", invalidUpdate.Code, invalidUpdate.Body.String())
	}
	unboundUpdate := wxPusherAPIJSONRequest(t, mux, ownerSession, http.MethodPatch, "/api/notification-channels/wxpusher", `{"enabled":true}`)
	if unboundUpdate.Code != http.StatusConflict || !strings.Contains(unboundUpdate.Body.String(), "WXPUSHER_NOT_BOUND") {
		t.Fatalf("unbound update=%d body=%s", unboundUpdate.Code, unboundUpdate.Body.String())
	}
	unboundTest := wxPusherAPIRequest(t, mux, ownerSession, http.MethodPost, "/api/notification-channels/wxpusher/test")
	if unboundTest.Code != http.StatusConflict || !strings.Contains(unboundTest.Body.String(), "WXPUSHER_NOT_BOUND") {
		t.Fatalf("unbound test=%d body=%s", unboundTest.Code, unboundTest.Body.String())
	}
	created := wxPusherAPIRequest(t, mux, ownerSession, http.MethodPost, "/api/notification-channels/wxpusher/bind-sessions")
	if created.Code != http.StatusCreated || created.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("create=%d cache=%q body=%s", created.Code, created.Header().Get("Cache-Control"), created.Body.String())
	}
	if strings.Contains(created.Body.String(), "provider-secret-code") {
		t.Fatal("provider code leaked through binding API")
	}
	var bindSession model.WxPusherBindSessionView
	if err := json.NewDecoder(created.Body).Decode(&bindSession); err != nil || bindSession.ID == "" || bindSession.QRURL == "" {
		t.Fatalf("created session=%+v err=%v", bindSession, err)
	}
	crossUser := wxPusherAPIRequest(t, mux, otherSession, http.MethodGet, "/api/notification-channels/wxpusher/bind-sessions/"+bindSession.ID)
	if crossUser.Code != http.StatusNotFound {
		t.Fatalf("cross-user poll=%d body=%s", crossUser.Code, crossUser.Body.String())
	}
	deleted := wxPusherAPIRequest(t, mux, ownerSession, http.MethodDelete, "/api/notification-channels/wxpusher")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func wxPusherAPIJSONRequest(t *testing.T, handler http.Handler, session auth.Session, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func wxPusherAPIRequest(t *testing.T, handler http.Handler, session auth.Session, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
