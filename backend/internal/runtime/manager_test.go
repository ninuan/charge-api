package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"charge-dashboard/internal/mocele"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	"charge-dashboard/internal/yyb"
)

func TestUpdateUserKeepsAtLeastOneEnabledAdmin(t *testing.T) {
	admin := model.User{
		ID:        "admin-1",
		Username:  "admin",
		Role:      model.RoleAdmin,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	manager := &Manager{
		repository: testRepository(t),
		users:      map[string]model.User{admin.ID: admin},
		runtimes:   map[string]*UserRuntime{},
	}

	disabled := false
	if _, err := manager.UpdateUser(admin.ID, model.UserUpdateRequest{Enabled: &disabled}); err == nil {
		t.Fatal("expected disabling the last enabled admin to fail")
	}

	userRole := model.RoleUser
	if _, err := manager.UpdateUser(admin.ID, model.UserUpdateRequest{Role: &userRole}); err == nil {
		t.Fatal("expected demoting the last enabled admin to fail")
	}
}

func TestConcurrentSaveProducesValidState(t *testing.T) {
	repository := testRepository(t)
	manager := &Manager{
		repository: repository,
		users: map[string]model.User{
			"admin-1": {
				ID:        "admin-1",
				Username:  "admin",
				Role:      model.RoleAdmin,
				Enabled:   true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
		runtimes: map[string]*UserRuntime{},
	}

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- manager.Save()
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Save failed: %v", err)
		}
	}
	state, ok, err := repository.Load()
	if err != nil {
		t.Fatalf("load saved state: %v", err)
	}
	if !ok || len(state.Users) != 1 || state.Users[0].ID != "admin-1" {
		t.Fatalf("unexpected saved state: %+v", state)
	}
}

func TestAddPileEnforcesPerUserDeviceLimit(t *testing.T) {
	runtime := newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{}, 30*time.Second)
	for i := 0; i < maxDevicesPerUser; i++ {
		if err := runtime.client.AddDevice(fmt.Sprintf("device-%d", i)); err != nil {
			t.Fatalf("seed device: %v", err)
		}
	}
	manager := &Manager{
		repository: testRepository(t),
		users: map[string]model.User{
			"user-1": {ID: "user-1", Username: "alice", Role: model.RoleUser, Enabled: true},
		},
		runtimes: map[string]*UserRuntime{"user-1": runtime},
	}

	_, err := manager.AddPile("user-1", model.PileUpsertRequest{ID: "device-over-limit"})
	if err == nil {
		t.Fatal("expected device limit error")
	}
}

func TestAddPileResolvesDeviceIDFromNumber(t *testing.T) {
	const (
		number = "61034278"
		longID = "2601201412385560001"
	)
	var postedID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/i/cnum":
			if got := r.URL.Query().Get("n"); got != number {
				t.Fatalf("cnum number = %q, want %q", got, number)
			}
			w.Header().Set("Location", "/i/device/opening?id="+longID+"&i=1")
			w.WriteHeader(http.StatusFound)
		case "/action/i/api/devicewithnumbers":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse form body: %v", err)
			}
			postedID = values.Get("id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":%q,"number":%q,"name":"远端名称","status":"在线","opennum":10}`, longID, number)
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	requests := []parser.CaptureRequest{{
		Name:   "template",
		URL:    server.URL + "/action/i/api/devicewithnumbers",
		Method: http.MethodPost,
		Body:   "id=YOUR_DEVICE_LONG_ID",
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
	}}
	manager := &Manager{
		repository: testRepository(t),
		requests:   requests,
		users: map[string]model.User{
			"user-1": {
				ID:             "user-1",
				Username:       "alice",
				Role:           model.RoleUser,
				Enabled:        true,
				DeviceLimit:    10,
				RefreshEnabled: true,
			},
		},
		runtimes: map[string]*UserRuntime{
			"user-1": newUserRuntime(requests, persistence.UserState{}, 30*time.Second),
		},
		settings: model.RegistrationSettings{DefaultDeviceLimit: 10, DefaultRefreshEnabled: true},
		invites:  map[string]model.InviteCode{},
	}

	pile, err := manager.AddPile("user-1", model.PileUpsertRequest{
		Number: number,
		Name:   "松园 3 号楼",
	})
	if err != nil {
		t.Fatalf("AddPile: %v", err)
	}
	if pile.ID != longID || pile.Number != number {
		t.Fatalf("unexpected pile identity: %+v", pile)
	}
	if postedID != longID {
		t.Fatalf("status request id = %q, want %q", postedID, longID)
	}
}

func TestAddPileWithYYBRetriesWithSyncedCookieWhenAuthExpired(t *testing.T) {
	const (
		number = "61034278"
		longID = "2601201412385560001"
	)
	var statusCalls int
	var cookies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/i/cnum":
			if got := r.URL.Query().Get("n"); got != number {
				t.Fatalf("cnum number = %q, want %q", got, number)
			}
			w.Header().Set("Location", "/i/device/opening?id="+longID+"&i=1")
			w.WriteHeader(http.StatusFound)
		case "/action/i/api/devicewithnumbers":
			statusCalls++
			cookies = append(cookies, r.Header.Get("Cookie"))
			if !strings.Contains(r.Header.Get("Cookie"), "wxopenid=open") {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"err":"cookie expired"}`))
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse form body: %v", err)
			}
			if got := values.Get("id"); got != longID {
				t.Fatalf("status request id = %q, want %q", got, longID)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":%q,"number":%q,"name":"远端名称","status":"在线","opennum":10}`, longID, number)
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	requests := []parser.CaptureRequest{{
		Name:   "template",
		URL:    server.URL + "/action/i/api/devicewithnumbers",
		Method: http.MethodPost,
		Body:   "id=YOUR_DEVICE_LONG_ID",
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Cookie":       "sid=expired",
		},
	}}
	manager := &Manager{
		repository: testRepository(t),
		requests:   requests,
		users: map[string]model.User{
			"user-1": {
				ID:             "user-1",
				Username:       "alice",
				Role:           model.RoleUser,
				Enabled:        true,
				DeviceLimit:    10,
				RefreshEnabled: true,
			},
		},
		runtimes: map[string]*UserRuntime{
			"user-1": newUserRuntime(requests, persistence.UserState{}, 30*time.Second),
		},
		settings: model.RegistrationSettings{DefaultDeviceLimit: 10, DefaultRefreshEnabled: true},
		invites:  map[string]model.InviteCode{},
	}
	if _, err := manager.SaveYYBBinding("user-1", yyb.YYBAccount{Ref: "ref-1", OpenID: "openid-1"}); err != nil {
		t.Fatalf("SaveYYBBinding: %v", err)
	}
	yybClient := &fakeYYBClient{codes: []string{"wx-code"}}
	moceleClient := &fakeMoceleClient{cookie: "deviceid=" + longID + "; org=1; openindex=7; wxopenid=open; info=info"}

	pile, err := manager.AddPileWithYYB("user-1", model.PileUpsertRequest{Number: number}, yybClient, moceleClient)
	if err != nil {
		t.Fatalf("AddPileWithYYB: %v", err)
	}
	if pile.ID != longID || pile.Number != number {
		t.Fatalf("unexpected pile: %+v", pile)
	}
	if statusCalls != 2 {
		t.Fatalf("status calls = %d, want 2", statusCalls)
	}
	if moceleClient.deviceID != longID || moceleClient.code != "wx-code" {
		t.Fatalf("mocele call = device %q code %q", moceleClient.deviceID, moceleClient.code)
	}
	if len(cookies) != 2 || strings.Contains(cookies[0], "wxopenid=open") || !strings.Contains(cookies[1], "wxopenid=open") {
		t.Fatalf("cookies = %#v", cookies)
	}
}

func TestAddPileWithYYBRetriesWhenRemoteReturnsLoginScript(t *testing.T) {
	const (
		number = "61034278"
		longID = "2601201412385560001"
	)
	var statusCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/i/cnum":
			w.Header().Set("Location", "/i/device/opening?id="+longID+"&i=1")
			w.WriteHeader(http.StatusFound)
		case "/action/i/api/devicewithnumbers":
			statusCalls++
			if !strings.Contains(r.Header.Get("Cookie"), "wxopenid=open") {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`alert("请重新打开页面")`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":%q,"number":%q,"name":"远端名称","status":"在线","opennum":10}`, longID, number)
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	requests := []parser.CaptureRequest{{
		Name:   "template",
		URL:    server.URL + "/action/i/api/devicewithnumbers",
		Method: http.MethodPost,
		Body:   "id=YOUR_DEVICE_LONG_ID",
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
	}}
	manager := &Manager{
		repository: testRepository(t),
		requests:   requests,
		users: map[string]model.User{
			"user-1": {ID: "user-1", Username: "alice", Role: model.RoleUser, Enabled: true, DeviceLimit: 10, RefreshEnabled: true},
		},
		runtimes: map[string]*UserRuntime{
			"user-1": newUserRuntime(requests, persistence.UserState{}, 30*time.Second),
		},
		settings: model.RegistrationSettings{DefaultDeviceLimit: 10, DefaultRefreshEnabled: true},
		invites:  map[string]model.InviteCode{},
	}
	if _, err := manager.SaveYYBBinding("user-1", yyb.YYBAccount{Ref: "ref-1", OpenID: "openid-1"}); err != nil {
		t.Fatalf("SaveYYBBinding: %v", err)
	}
	yybClient := &fakeYYBClient{codes: []string{"wx-code"}}
	moceleClient := &fakeMoceleClient{cookie: "deviceid=" + longID + "; org=1; openindex=7; wxopenid=open; info=info"}

	pile, err := manager.AddPileWithYYB("user-1", model.PileUpsertRequest{Number: number}, yybClient, moceleClient)
	if err != nil {
		t.Fatalf("AddPileWithYYB: %v", err)
	}
	if pile.ID != longID {
		t.Fatalf("pile.ID = %q, want %q", pile.ID, longID)
	}
	if statusCalls != 2 {
		t.Fatalf("status calls = %d, want 2", statusCalls)
	}
}

func TestRegistrationPolicySupportsPublicAndInviteEntryPoints(t *testing.T) {
	manager, err := NewManager(
		testRepository(t),
		"",
		parser.DefaultCaptureRequests(),
		"admin-password-123",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	settings := manager.Settings()
	settings.OpenRegistration = true
	settings.InviteRequired = true
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("enable public and invite registration: %v", err)
	}
	if _, err := manager.RegisterUser("public-user", "password123", ""); err != nil {
		t.Fatalf("public registration should not require an invite: %v", err)
	}

	settings.OpenRegistration = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("switch to invite-only registration: %v", err)
	}
	invite, err := manager.CreateInvite("INVITE-ONLY", nil)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if _, err := manager.RegisterUser("invited-user", "password123", invite.Code); err != nil {
		t.Fatalf("invite registration failed: %v", err)
	}
	if _, err := manager.RegisterUser("missing-invite", "password123", ""); err == nil {
		t.Fatal("invite-only registration accepted a missing invite")
	}

	settings.InviteRequired = false
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("disable all registration: %v", err)
	}
	if _, err := manager.RegisterUser("closed-user", "password123", ""); err == nil {
		t.Fatal("registration succeeded while both entry points were disabled")
	}
}

func TestCreateInviteGeneratesRandomCode(t *testing.T) {
	manager, err := NewManager(
		testRepository(t),
		"",
		parser.DefaultCaptureRequests(),
		"admin-password-123",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	first, err := manager.CreateInvite("", nil)
	if err != nil {
		t.Fatalf("CreateInvite first: %v", err)
	}
	second, err := manager.CreateInvite("", nil)
	if err != nil {
		t.Fatalf("CreateInvite second: %v", err)
	}
	if !strings.HasPrefix(first.Code, "CHG-") || len(first.Code) < 12 {
		t.Fatalf("unexpected generated invite format: %q", first.Code)
	}
	if first.Code == second.Code {
		t.Fatalf("generated duplicate invite codes: %q", first.Code)
	}
}

func TestAdminStatsClearsAuthExceptionAfterLaterRemoteSuccess(t *testing.T) {
	manager := &Manager{
		repository: testRepository(t),
		users: map[string]model.User{
			"user-1": {ID: "user-1", Username: "alice", Role: model.RoleUser, Enabled: true},
		},
		runtimes: map[string]*UserRuntime{
			"user-1": newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{Cookie: "sid=valid"}, 30*time.Second),
		},
	}
	runtime := manager.runtimes["user-1"]
	runtime.recordFailure(true)
	manager.recordMetric("user-1", "cookie_error")
	time.Sleep(time.Millisecond)
	manager.recordMetric("user-1", "remote_ok")

	stats := manager.AdminStats()
	for _, exception := range stats.Exceptions {
		if exception.UserID == "user-1" && exception.Type == "cookie_expired" {
			t.Fatalf("auth exception should be cleared after later remote success: %+v", stats.Exceptions)
		}
	}
}

func TestAdminStatsJSONUsesEmptyArrays(t *testing.T) {
	manager, err := NewManager(
		testRepository(t),
		"",
		parser.DefaultCaptureRequests(),
		"admin-password-123",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	body, err := json.Marshal(manager.AdminStats())
	if err != nil {
		t.Fatalf("marshal admin stats: %v", err)
	}
	for _, forbidden := range []string{
		`"users":null`,
		`"hourly":null`,
		`"daily":null`,
		`"exceptions":null`,
		`"deviceIds":null`,
	} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("admin stats JSON contains %s: %s", forbidden, body)
		}
	}
}

func TestNewManagerMigratesLegacyJSONAndRemovesPlaintextCookie(t *testing.T) {
	dir := t.TempDir()
	legacyPath := dir + "/charge_state.json"
	now := time.Now()
	legacyState := persistence.State{
		Version: 2,
		Users: []model.User{{
			ID:           "user-1",
			Username:     "alice",
			PasswordHash: "hash",
			Role:         model.RoleUser,
			Enabled:      true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
		UserStates: map[string]persistence.UserState{
			"user-1": {
				DeviceIDs: []string{"device-1"},
				Cookie:    "sid=legacy-plaintext-cookie",
			},
		},
	}
	body, err := json.Marshal(legacyState)
	if err != nil {
		t.Fatalf("marshal legacy state: %v", err)
	}
	if err := os.WriteFile(legacyPath, body, 0600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	repository, err := persistence.OpenSQLite(
		dir+"/state.db",
		bytes.Repeat([]byte{0x33}, persistence.CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer repository.Close()

	manager, err := NewManager(repository, legacyPath, parser.DefaultCaptureRequests(), "", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !manager.MigratedLegacyJSON() {
		t.Fatal("expected legacy JSON migration")
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("plaintext legacy JSON still exists: %v", err)
	}
	archive, err := os.ReadFile(legacyPath + ".migrated")
	if err != nil {
		t.Fatalf("read migration archive: %v", err)
	}
	if bytes.Contains(archive, []byte("legacy-plaintext-cookie")) {
		t.Fatal("sanitized migration archive contains plaintext cookie")
	}

	loaded, ok, err := repository.Load()
	if err != nil {
		t.Fatalf("load migrated database: %v", err)
	}
	if !ok || !bytes.Contains(
		[]byte(loaded.UserStates["user-1"].Cookie),
		[]byte("sid=legacy-plaintext-cookie"),
	) {
		t.Fatalf("legacy cookie was not migrated: %+v", loaded.UserStates["user-1"])
	}
}

func testRepository(t *testing.T) *persistence.Store {
	t.Helper()
	repository, err := persistence.OpenSQLite(
		t.TempDir()+"/state.db",
		bytes.Repeat([]byte{0x42}, persistence.CookieKeySize),
	)
	if err != nil {
		t.Fatalf("open test repository: %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close test repository: %v", err)
		}
	})
	return repository
}

func TestSaveAndClearYYBBinding(t *testing.T) {
	manager := testYYBManager(t)
	account := yyb.YYBAccount{
		Ref:      "ref-1",
		OpenID:   "openid-1",
		Nickname: "Alice",
		Avatar:   "https://avatar.example/a.png",
		Status:   "ignored",
	}

	binding, err := manager.SaveYYBBinding("user-1", account)
	if err != nil {
		t.Fatalf("SaveYYBBinding: %v", err)
	}
	if binding.Ref != "ref-1" || binding.OpenID != "openid-1" || binding.Status != "alive" || binding.LastError != "" || binding.BoundAt.IsZero() {
		t.Fatalf("binding = %#v", binding)
	}

	loaded, err := manager.YYBBinding("user-1")
	if err != nil {
		t.Fatalf("YYBBinding: %v", err)
	}
	if loaded == nil || loaded.Ref != "ref-1" || loaded.Status != "alive" {
		t.Fatalf("loaded binding = %#v", loaded)
	}

	if err := manager.ClearYYBBinding("user-1"); err != nil {
		t.Fatalf("ClearYYBBinding: %v", err)
	}
	cleared, err := manager.YYBBinding("user-1")
	if err != nil {
		t.Fatalf("YYBBinding after clear: %v", err)
	}
	if cleared != nil {
		t.Fatalf("binding was not cleared: %#v", cleared)
	}
}

func TestSyncCookieFromYYBGetCodeSuccess(t *testing.T) {
	manager := testYYBManager(t)
	if _, err := manager.SaveYYBBinding("user-1", yyb.YYBAccount{Ref: "ref-1", OpenID: "openid-1", Nickname: "Alice"}); err != nil {
		t.Fatalf("SaveYYBBinding: %v", err)
	}
	yybClient := &fakeYYBClient{codes: []string{"wx-code-1"}}
	moceleClient := &fakeMoceleClient{cookie: "deviceid=device-1; wxopenid=openid; info=info"}

	if _, err := manager.SyncCookieFromYYB("user-1", "device-1", yybClient, moceleClient); err != nil {
		t.Fatalf("SyncCookieFromYYB: %v", err)
	}
	if len(yybClient.getCodeCalls) != 1 || yybClient.getCodeCalls[0] != "ref-1|wx9cbffc15d3cb7739" {
		t.Fatalf("getCodeCalls = %#v", yybClient.getCodeCalls)
	}
	if yybClient.refreshCalls != 0 {
		t.Fatalf("refreshCalls = %d", yybClient.refreshCalls)
	}
	if moceleClient.deviceID != "device-1" || moceleClient.code != "wx-code-1" {
		t.Fatalf("mocele call = device %q code %q", moceleClient.deviceID, moceleClient.code)
	}
	binding, _ := manager.YYBBinding("user-1")
	if binding == nil || binding.Status != "alive" || binding.LastError != "" || binding.LastCheckedAt == nil {
		t.Fatalf("binding after sync = %#v", binding)
	}
}

func TestSyncCookieFromYYBRefreshesThenRetriesGetCode(t *testing.T) {
	manager := testYYBManager(t)
	if _, err := manager.SaveYYBBinding("user-1", yyb.YYBAccount{Ref: "ref-1", OpenID: "openid-1"}); err != nil {
		t.Fatalf("SaveYYBBinding: %v", err)
	}
	yybClient := &fakeYYBClient{errors: []error{fmt.Errorf("expired")}, codes: []string{"wx-code-2"}}
	moceleClient := &fakeMoceleClient{cookie: "deviceid=device-1; wxopenid=openid; info=info"}

	if _, err := manager.SyncCookieFromYYB("user-1", "device-1", yybClient, moceleClient); err != nil {
		t.Fatalf("SyncCookieFromYYB: %v", err)
	}
	if yybClient.refreshCalls != 1 || len(yybClient.getCodeCalls) != 2 {
		t.Fatalf("refresh=%d getCodeCalls=%#v", yybClient.refreshCalls, yybClient.getCodeCalls)
	}
}

func TestSyncCookieFromYYBMarksBindingExpiredWhenRefreshRetryFails(t *testing.T) {
	manager := testYYBManager(t)
	if _, err := manager.SaveYYBBinding("user-1", yyb.YYBAccount{Ref: "ref-1", OpenID: "openid-1"}); err != nil {
		t.Fatalf("SaveYYBBinding: %v", err)
	}
	yybClient := &fakeYYBClient{errors: []error{fmt.Errorf("expired once"), fmt.Errorf("expired twice")}}
	moceleClient := &fakeMoceleClient{}

	_, err := manager.SyncCookieFromYYB("user-1", "device-1", yybClient, moceleClient)
	if err == nil {
		t.Fatalf("expected sync error")
	}
	if yybClient.refreshCalls != 1 || len(yybClient.getCodeCalls) != 2 {
		t.Fatalf("refresh=%d getCodeCalls=%#v", yybClient.refreshCalls, yybClient.getCodeCalls)
	}
	if moceleClient.calls != 0 {
		t.Fatalf("mocele should not be called")
	}
	binding, _ := manager.YYBBinding("user-1")
	if binding == nil || binding.Status != "expired" || binding.LastError == "" || binding.LastCheckedAt == nil {
		t.Fatalf("binding after failure = %#v", binding)
	}
}

func TestSyncCookieFromYYBRequiresBinding(t *testing.T) {
	manager := testYYBManager(t)
	_, err := manager.SyncCookieFromYYB("user-1", "device-1", &fakeYYBClient{}, &fakeMoceleClient{})
	if err == nil || !strings.Contains(err.Error(), "yyb binding") {
		t.Fatalf("expected missing binding error, got %v", err)
	}
}

func testYYBManager(t *testing.T) *Manager {
	t.Helper()
	manager := &Manager{
		repository: testRepository(t),
		users: map[string]model.User{
			"user-1": {ID: "user-1", Username: "alice", Role: model.RoleUser, Enabled: true, RefreshEnabled: true, DeviceLimit: 10},
		},
		runtimes: map[string]*UserRuntime{
			"user-1": newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{}, 30*time.Second),
		},
		settings: model.RegistrationSettings{DefaultDeviceLimit: 10, DefaultRefreshEnabled: true},
		invites:  map[string]model.InviteCode{},
	}
	return manager
}

type fakeYYBClient struct {
	codes        []string
	errors       []error
	getCodeCalls []string
	refreshCalls int
}

func (f *fakeYYBClient) GetCode(ctx context.Context, ref string, appID string) (string, error) {
	f.getCodeCalls = append(f.getCodeCalls, ref+"|"+appID)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		return "", err
	}
	if len(f.codes) == 0 {
		return "", fmt.Errorf("missing fake code")
	}
	code := f.codes[0]
	f.codes = f.codes[1:]
	return code, nil
}

func (f *fakeYYBClient) RefreshAccount(ctx context.Context, ref string) error {
	f.refreshCalls++
	return nil
}

type fakeMoceleClient struct {
	cookie   string
	deviceID string
	code     string
	calls    int
}

func (f *fakeMoceleClient) ExchangeCode(ctx context.Context, deviceID string, code string) (mocele.CookieResult, error) {
	f.calls++
	f.deviceID = deviceID
	f.code = code
	return mocele.CookieResult{Cookie: f.cookie, WXOpenID: "openid", Info: "info"}, nil
}

func TestFirstDeviceIDReturnsUserDevice(t *testing.T) {
	manager := testYYBManager(t)
	runtime := manager.runtimes["user-1"]
	if err := runtime.client.AddDevice("device-1"); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	id, ok, err := manager.FirstDeviceID("user-1")
	if err != nil {
		t.Fatalf("FirstDeviceID: %v", err)
	}
	if !ok || id != "device-1" {
		t.Fatalf("FirstDeviceID = %q %v", id, ok)
	}
}
