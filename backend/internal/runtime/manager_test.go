package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
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

func TestCreateUserRollsBackMemoryWhenSaveFails(t *testing.T) {
	brokenRepo := testRepository(t)
	if err := brokenRepo.Close(); err != nil {
		t.Fatalf("close broken repo: %v", err)
	}
	manager := &Manager{
		repository: brokenRepo,
		users:      map[string]model.User{},
		runtimes:   map[string]*UserRuntime{},
		settings: model.RegistrationSettings{
			DefaultDeviceLimit:    defaultDeviceLimit,
			DefaultRefreshEnabled: true,
		},
	}

	if _, err := manager.CreateUser(model.UserCreateRequest{
		Username: "rollback-user",
		Password: "password123",
		Role:     model.RoleUser,
	}); err == nil {
		t.Fatal("expected CreateUser to fail when repository is closed")
	}
	if len(manager.users) != 0 || len(manager.runtimes) != 0 {
		t.Fatalf("expected failed CreateUser to roll back memory, got users=%d runtimes=%d", len(manager.users), len(manager.runtimes))
	}

	manager.repository = testRepository(t)
	created, err := manager.CreateUser(model.UserCreateRequest{
		Username: "rollback-user",
		Password: "password123",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("retry CreateUser after storage recovery failed: %v", err)
	}
	if created.Username != "rollback-user" {
		t.Fatalf("created username = %q, want rollback-user", created.Username)
	}
}

func TestResetUserPasswordRequiresChangeOnNextLogin(t *testing.T) {
	manager := &Manager{
		repository: testRepository(t),
		users:      map[string]model.User{},
		runtimes:   map[string]*UserRuntime{},
		settings: model.RegistrationSettings{
			DefaultDeviceLimit:    defaultDeviceLimit,
			DefaultRefreshEnabled: true,
		},
	}
	user, err := manager.CreateUser(model.UserCreateRequest{
		Username: "reset-user",
		Password: "password123",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	temporaryPassword, err := manager.ResetUserPassword(user.ID)
	if err != nil {
		t.Fatalf("ResetUserPassword: %v", err)
	}
	if len(temporaryPassword) < 8 || temporaryPassword == "password123" {
		t.Fatal("temporary password did not satisfy reset requirements")
	}
	if _, err := manager.Authenticate(user.Username, "password123"); err == nil {
		t.Fatal("old password remains valid after reset")
	}
	current, err := manager.Authenticate(user.Username, temporaryPassword)
	if err != nil {
		t.Fatalf("Authenticate with temporary password: %v", err)
	}
	if !current.MustChangePassword {
		t.Fatal("temporary-password login did not require a password change")
	}
	state, ok, err := manager.repository.Load()
	if err != nil {
		t.Fatalf("Load persisted reset state: %v", err)
	}
	if !ok {
		t.Fatal("reset state was not persisted")
	}
	persistedMustChange := false
	for _, persistedUser := range state.Users {
		if persistedUser.ID == user.ID {
			persistedMustChange = persistedUser.MustChangePassword
			break
		}
	}
	if !persistedMustChange {
		t.Fatal("persisted user did not retain the password-change requirement")
	}

	changed, err := manager.ChangePassword(user.ID, temporaryPassword, "replacement-password-123")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if changed.MustChangePassword {
		t.Fatal("password-change requirement was not cleared")
	}
	current, err = manager.Authenticate(user.Username, "replacement-password-123")
	if err != nil {
		t.Fatalf("Authenticate with replacement password: %v", err)
	}
	if current.MustChangePassword {
		t.Fatal("replacement-password login still requires another password change")
	}
}

func TestInitialPasswordFileIsRemovedAfterAdminChangesPassword(t *testing.T) {
	repository := testRepository(t)
	manager, err := NewManager(repository, "", nil, "initial-password-123", time.Minute)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	passwordFile := t.TempDir() + "/initial-admin-password.txt"
	if err := os.WriteFile(passwordFile, []byte("initial-password-123\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.ConfigureInitialPasswordFile(passwordFile); err != nil {
		t.Fatalf("ConfigureInitialPasswordFile: %v", err)
	}
	adminID := ""
	for _, user := range manager.ListUsers() {
		adminID = user.User.ID
	}
	if adminID == "" {
		t.Fatal("admin user was not created")
	}
	if _, err := manager.ChangePassword(adminID, "initial-password-123", "replacement-password-123"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := os.Stat(passwordFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("initial password file still exists: %v", err)
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
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestNewManagerNormalizesRetentionAndPrunesExpiredData(t *testing.T) {
	repository := testRepository(t)
	now := time.Now().UTC().Truncate(time.Second)
	user := model.User{
		ID: "user-retention", Username: "retention-user", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
		DeviceLimit: 10, RefreshEnabled: true,
	}
	if err := repository.Save(persistence.State{
		Version:    3,
		Users:      []model.User{user},
		UserStates: map[string]persistence.UserState{user.ID: {}},
		Settings: model.RegistrationSettings{
			OpenRegistration: true, DefaultDeviceLimit: 10,
			DefaultRefreshEnabled: true, StatsRetentionDays: 30,
		},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	old := now.AddDate(0, 0, -100)
	recent := now.AddDate(0, 0, -2)
	if err := repository.RecordMetric("user-retention", "request", old); err != nil {
		t.Fatalf("RecordMetric old: %v", err)
	}
	if err := repository.RecordMetric("user-retention", "request", recent); err != nil {
		t.Fatalf("RecordMetric recent: %v", err)
	}
	if err := repository.RecordPortStatusEvents([]model.PortStatusEvent{
		{UserID: user.ID, DeviceID: "device-1", PortID: 1, ToStatus: model.PortIdle, ChangedAt: old, Source: "remote"},
		{UserID: user.ID, DeviceID: "device-1", PortID: 1, ToStatus: model.PortInUse, ChangedAt: recent, Source: "remote"},
	}); err != nil {
		t.Fatalf("RecordPortStatusEvents: %v", err)
	}

	manager, err := NewManager(
		repository,
		"",
		parser.DefaultCaptureRequests(),
		"admin-password-123",
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := manager.Settings().PortHistoryRetentionDays; got != defaultHistoryRetentionDays {
		t.Fatalf("history retention = %d, want %d", got, defaultHistoryRetentionDays)
	}
	operations, err := manager.OperationsStatus()
	if err != nil {
		t.Fatalf("OperationsStatus: %v", err)
	}
	if operations.MetricRows != 1 || operations.PortHistoryRows != 2 {
		t.Fatalf("startup retention did not prune expired rows: %+v", operations)
	}

	settings := manager.Settings()
	settings.PortHistoryRetentionDays = 0
	if err := manager.UpdateSettings(settings); err == nil {
		t.Fatal("UpdateSettings accepted zero history retention")
	}
	settings.PortHistoryRetentionDays = 1
	if err := manager.UpdateSettings(settings); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	operations, err = manager.OperationsStatus()
	if err != nil {
		t.Fatalf("OperationsStatus after update: %v", err)
	}
	if operations.PortHistoryRows != 1 || operations.PortHistoryRetentionDays != 1 ||
		operations.PortHistoryOldestAt == nil ||
		operations.PortHistoryOldestAt.Before(time.Now().Add(-25*time.Hour)) {
		t.Fatalf("updated retention was not applied: %+v", operations)
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

func TestAdminStatsIncludesLatestActionableOperationDiagnostic(t *testing.T) {
	manager := testDiagnosticManager(t)

	manager.RecordOperationDiagnostic("user-1", "update_cookie", "cookie_update_failed", "2601201412385560001", 403)

	stats := manager.AdminStats()
	issue, ok := findException(stats.Exceptions, "user-1", "operation")
	if !ok {
		t.Fatalf("missing operation diagnostic: %#v", stats.Exceptions)
	}
	if issue.Message != "更新登录凭据失败，请检查凭据后重试" || issue.DeviceID != "0001" {
		t.Fatalf("unexpected operation diagnostic: %#v", issue)
	}
}

func TestAdminCredentialStatesAndIssues(t *testing.T) {
	const (
		secretCookie = "sid=valid-secret"
		secretOpenID = "secret-openid-123"
		secretRef    = "secret-ref-123"
	)
	expiredAt := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		state     persistence.UserState
		wantState model.CredentialState
		wantIssue bool
	}{
		{
			name:      "unused user",
			state:     persistence.UserState{},
			wantState: model.CredentialUnbound,
			wantIssue: false,
		},
		{
			name: "retired yyb binding without personal cookie is unbound",
			state: persistence.UserState{
				YYBBinding: &model.YYBBinding{Ref: secretRef, OpenID: secretOpenID, Status: "alive"},
			},
			wantState: model.CredentialUnbound,
			wantIssue: false,
		},
		{
			name: "device without credential",
			state: persistence.UserState{
				DeviceIDs: []string{"2601201412385560001"},
			},
			wantState: model.CredentialUnbound,
			wantIssue: false,
		},
		{
			name: "healthy manual cookie",
			state: persistence.UserState{
				DeviceIDs: []string{"2601201412385560001"},
				Cookie:    secretCookie,
			},
			wantState: model.CredentialHealthy,
			wantIssue: false,
		},
		{
			name: "retired yyb binding sync error does not raise admin incident",
			state: persistence.UserState{
				DeviceIDs: []string{"2601201412385560001"},
				YYBBinding: &model.YYBBinding{
					Ref: secretRef, OpenID: secretOpenID, Status: "alive", LastError: "secret-upstream-error",
				},
			},
			wantState: model.CredentialUnbound,
			wantIssue: false,
		},
		{
			name: "expired personal cookie marked expired without flooding admin incidents",
			state: persistence.UserState{
				DeviceIDs: []string{"2601201412385560001"},
				Cookie:    secretCookie,
				Stats: model.TrafficStats{
					AuthFailures:      1,
					LastAuthFailureAt: &expiredAt,
				},
			},
			wantState: model.CredentialExpired,
			wantIssue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := model.User{
				ID: "user-1", Username: "alice", Role: model.RoleUser, Enabled: true,
				CreatedAt: time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC),
			}
			manager := &Manager{
				repository: testRepository(t),
				users:      map[string]model.User{user.ID: user},
				runtimes: map[string]*UserRuntime{
					user.ID: newUserRuntime(parser.DefaultCaptureRequests(), tt.state, 30*time.Second),
				},
			}

			stats := manager.AdminStats()
			if len(stats.Users) != 1 {
				t.Fatalf("users = %d, want 1", len(stats.Users))
			}
			summary := stats.Users[0]
			if summary.Credential.State != tt.wantState {
				t.Fatalf("credential state = %q, want %q", summary.Credential.State, tt.wantState)
			}
			if got := hasExceptionType(stats.Exceptions, user.ID, "credential"); got != tt.wantIssue {
				t.Fatalf("credential issue = %v, want %v: %+v", got, tt.wantIssue, stats.Exceptions)
			}
			body, err := json.Marshal(stats)
			if err != nil {
				t.Fatalf("marshal stats: %v", err)
			}
			for _, secret := range []string{secretCookie, secretOpenID, secretRef, "secret-upstream-error"} {
				if bytes.Contains(body, []byte(secret)) {
					t.Fatalf("admin stats leaked %q: %s", secret, body)
				}
			}
		})
	}
}

func TestAdminStatsAutoResolvesLegacyAuthIncidentsAndTracksPublicPool(t *testing.T) {
	repo := testRepository(t)
	now := time.Now().UTC()
	if err := repo.UpsertIncident(model.SystemException{
		ID: "auth-user-1", UserID: "user-1", Username: "ABC",
		Type: "cookie_expired", Level: "critical",
		Message: "远端鉴权失败，Cookie 可能已失效", Time: now.Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed legacy auth incident: %v", err)
	}
	user := model.User{
		ID: "user-1", Username: "ABC", Role: model.RoleUser, Enabled: true,
		CreatedAt: now.Add(-48 * time.Hour),
	}
	manager := &Manager{
		repository: repo,
		users:      map[string]model.User{user.ID: user},
		runtimes: map[string]*UserRuntime{
			user.ID: newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{
				Cookie: "wxopenid=legacy; info=old",
				Stats:  model.TrafficStats{AuthFailures: 3, LastAuthFailureAt: &now},
			}, 30*time.Second),
		},
	}

	stats, err := manager.AdminStatsResult()
	if err != nil {
		t.Fatalf("AdminStatsResult: %v", err)
	}
	foundPool := false
	for _, issue := range stats.Exceptions {
		if issue.ID == "auth-user-1" {
			t.Fatalf("legacy auth incident should be auto-resolved, still active: %+v", issue)
		}
		if issue.ID == "operations-public-pool" && issue.Level == "critical" {
			foundPool = true
		}
	}
	if !foundPool {
		t.Fatalf("expected operations-public-pool critical incident when pool is empty: %+v", stats.Exceptions)
	}
}

func TestListUsersPageFiltersByNewCredentialModes(t *testing.T) {
	repo := testRepository(t)
	now := time.Now().UTC()
	manager := &Manager{
		repository: repo,
		settings: model.RegistrationSettings{
			DefaultDeviceLimit:           defaultDeviceLimit,
			DefaultRefreshEnabled:        true,
			FreeDailyRefreshQuota:        3,
			SharedPileCacheTTLSeconds:    60,
			RequirePoolShareForUnlimited: true,
		},
		users: map[string]model.User{
			"admin-1":   {ID: "admin-1", Username: "admin", Role: model.RoleAdmin, Enabled: true, CreatedAt: now},
			"free-1":    {ID: "free-1", Username: "free-user", Role: model.RoleUser, Enabled: true, CreatedAt: now},
			"unjoin-1":  {ID: "unjoin-1", Username: "unjoined-user", Role: model.RoleUser, Enabled: true, CreatedAt: now},
			"pool-1":    {ID: "pool-1", Username: "pool-user", Role: model.RoleUser, Enabled: true, CreatedAt: now},
			"expired-1": {ID: "expired-1", Username: "expired-user", Role: model.RoleUser, Enabled: true, CreatedAt: now},
		},
		runtimes: map[string]*UserRuntime{
			"admin-1":  newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{}, 30*time.Second),
			"free-1":   newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{}, 30*time.Second),
			"unjoin-1": newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{Cookie: "wxopenid=unjoined; info=ok"}, 30*time.Second),
			"pool-1":   newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{Cookie: "wxopenid=pooled; info=ok"}, 30*time.Second),
			"expired-1": newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{
				Cookie: "wxopenid=expired; info=bad",
				Stats:  model.TrafficStats{AuthFailures: 1, LastAuthFailureAt: &now},
			}, 30*time.Second),
		},
	}
	if _, err := repo.UpsertUserPublicPoolCookie("pool-1", "用户互助 · pool-user", "wxopenid=pooled; info=ok"); err != nil {
		t.Fatalf("UpsertUserPublicPoolCookie: %v", err)
	}

	assertMatch := func(filter string, want []string) {
		t.Helper()
		page := manager.ListUsersPage(model.AdminUserListQuery{
			Page: 1, PageSize: 20, Account: "all", Credential: filter, Health: "all",
		})
		got := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			got = append(got, item.User.Username)
		}
		if len(got) != len(want) {
			t.Fatalf("filter %q got %v, want %v", filter, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("filter %q got %v, want %v", filter, got, want)
			}
		}
	}

	assertMatch("all", []string{"admin", "expired-user", "free-user", "pool-user", "unjoined-user"})
	assertMatch("unbound", []string{"free-user"})
	assertMatch("healthy", []string{"pool-user"})
	assertMatch("pool_unjoined", []string{"unjoined-user"})
	assertMatch("expired", []string{"expired-user"})
}

func TestAdminIssuesUseActualFailureTime(t *testing.T) {
	failedAt := time.Date(2026, 7, 9, 8, 30, 0, 0, time.UTC)
	user := model.User{
		ID: "user-1", Username: "alice", Role: model.RoleUser, Enabled: true,
		CreatedAt: failedAt.Add(-time.Hour),
	}
	state := persistence.UserState{
		Refresh: model.RefreshInfo{
			LastRemoteAt:  &failedAt,
			FailedDevices: 1,
		},
	}
	manager := &Manager{
		repository: testRepository(t),
		users:      map[string]model.User{user.ID: user},
		runtimes: map[string]*UserRuntime{
			user.ID: newUserRuntime(parser.DefaultCaptureRequests(), state, 30*time.Second),
		},
	}

	stats := manager.AdminStats()
	issue, ok := findException(stats.Exceptions, user.ID, "refresh")
	if !ok {
		t.Fatalf("missing refresh issue: %+v", stats.Exceptions)
	}
	if !issue.Time.Equal(failedAt) {
		t.Fatalf("issue time = %v, want %v", issue.Time, failedAt)
	}

	recoveredAt := failedAt.Add(time.Minute)
	manager.runtimes[user.ID].store.SetRefreshInfo(model.RefreshInfo{
		LastRemoteAt:  &recoveredAt,
		FailedDevices: 0,
	})
	stats = manager.AdminStats()
	if hasExceptionType(stats.Exceptions, user.ID, "refresh") {
		t.Fatalf("refresh issue remained after recovery: %+v", stats.Exceptions)
	}
}

func hasExceptionType(exceptions []model.SystemException, userID, issueType string) bool {
	_, ok := findException(exceptions, userID, issueType)
	return ok
}

func findException(exceptions []model.SystemException, userID, issueType string) (model.SystemException, bool) {
	for _, exception := range exceptions {
		if exception.UserID == userID && exception.Type == issueType {
			return exception, true
		}
	}
	return model.SystemException{}, false
}

func TestAdminOverviewUsesPerDeviceRemoteSuccessRate(t *testing.T) {
	repository := testRepository(t)
	manager := &Manager{
		repository: repository,
		users: map[string]model.User{
			"user-1": {ID: "user-1", Username: "alice", Role: model.RoleUser, Enabled: true},
		},
		runtimes: map[string]*UserRuntime{
			"user-1": newUserRuntime(parser.DefaultCaptureRequests(), persistence.UserState{}, 30*time.Second),
		},
	}

	now := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 4; i++ {
		if err := repository.RecordMetric("user-1", "remote", now); err != nil {
			t.Fatalf("RecordMetric remote: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := repository.RecordMetric("user-1", "remote_ok", now); err != nil {
			t.Fatalf("RecordMetric remote_ok: %v", err)
		}
	}
	if err := repository.RecordMetric("user-1", "remote_failed", now); err != nil {
		t.Fatalf("RecordMetric remote_failed: %v", err)
	}

	stats := manager.AdminStats()
	if stats.Overview.RemoteSuccessRate != 75 {
		t.Fatalf("success rate = %v, want 75", stats.Overview.RemoteSuccessRate)
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
			PasswordHash: "legacy-password-hash",
			Role:         model.RoleUser,
			Enabled:      true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
		Invites: []model.InviteCode{{
			ID: "invite-1", Code: "legacy-invite-secret", Enabled: true, CreatedAt: now,
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
	for _, secret := range []string{"legacy-password-hash", "legacy-invite-secret", "passwordHash", `"code":"legacy`} {
		if bytes.Contains(archive, []byte(secret)) {
			t.Fatalf("sanitized migration archive contains %q", secret)
		}
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

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func TestRecoveryDiagnosticsKeepNewestTwentyAndPersist(t *testing.T) {
	manager := testDiagnosticManager(t)
	for index := 0; index < 25; index++ {
		manager.RecordOperationDiagnostic("user-1", "refresh", "refresh_failed", "2601201412385560001", 502)
	}
	diagnostics, err := manager.RecoveryDiagnostics("user-1")
	if err != nil {
		t.Fatalf("RecoveryDiagnostics: %v", err)
	}
	if len(diagnostics) != 20 {
		t.Fatalf("diagnostics count = %d, want 20", len(diagnostics))
	}
	state, ok, err := manager.repository.Load()
	if err != nil || !ok {
		t.Fatalf("Load = %v, %v", ok, err)
	}
	if got := len(state.UserStates["user-1"].RecoveryDiagnostics); got != 20 {
		t.Fatalf("persisted diagnostics count = %d, want 20", got)
	}
	assertDiagnosticsDoNotLeak(t, diagnostics, "2601201412385560001")
	if diagnostics[0].DeviceSuffix != "0001" || diagnostics[0].Message != "刷新设备状态失败，请稍后重试" {
		t.Fatalf("diagnostic was not sanitized: %#v", diagnostics[0])
	}
}

func TestRecordOperationDiagnosticSanitizesAndPersists(t *testing.T) {
	manager := testDiagnosticManager(t)

	manager.RecordOperationDiagnostic("user-1", "add_pile", "add_pile_failed", "2601201412385560001", 502)

	diagnostics, err := manager.RecoveryDiagnostics("user-1")
	if err != nil {
		t.Fatalf("RecoveryDiagnostics: %v", err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	got := diagnostics[0]
	if got.Operation != "add_pile" || got.Code != "add_pile_failed" || got.Message != "添加充电桩时未能完成远端校验" || got.DeviceSuffix != "0001" || got.StatusCode != 502 {
		t.Fatalf("unexpected diagnostic: %#v", got)
	}
	assertDiagnosticsDoNotLeak(t, diagnostics, "2601201412385560001")

	state, ok, err := manager.repository.Load()
	if err != nil || !ok {
		t.Fatalf("Load = %v, %v", ok, err)
	}
	if persisted := state.UserStates["user-1"].RecoveryDiagnostics; len(persisted) != 1 || persisted[0].Operation != "add_pile" {
		t.Fatalf("persisted diagnostics = %#v", persisted)
	}
}

func assertDiagnosticsDoNotLeak(t *testing.T, diagnostics []model.RecoveryDiagnostic, secrets ...string) {
	t.Helper()
	body, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	for _, secret := range secrets {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("diagnostics leaked %q: %s", secret, body)
		}
	}
}

func testDiagnosticManager(t *testing.T) *Manager {
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
