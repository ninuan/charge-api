package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
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
