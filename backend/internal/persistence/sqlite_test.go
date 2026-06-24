package persistence

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestSQLiteEncryptsCookieAtRestAndLoadsIt(t *testing.T) {
	path := t.TempDir() + "/state.db"
	key := bytes.Repeat([]byte{0x11}, CookieKeySize)
	store, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}

	const cookie = "sid=plain-secret-cookie; wxopenid=private-user"
	now := time.Now()
	state := State{
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
		UserStates: map[string]UserState{
			"user-1": {
				DeviceIDs: []string{"device-1"},
				Cookie:    cookie,
			},
		},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	databaseFiles, err := filepath.Glob(path + "*")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	for _, databaseFile := range databaseFiles {
		databaseBytes, err := os.ReadFile(databaseFile)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", databaseFile, err)
		}
		if bytes.Contains(databaseBytes, []byte(cookie)) || bytes.Contains(databaseBytes, []byte("plain-secret-cookie")) {
			t.Fatalf("%s contains plaintext cookie", databaseFile)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("database permissions = %o, want 600", info.Mode().Perm())
	}

	reopened, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer reopened.Close()
	loaded, ok, err := reopened.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ok || loaded.UserStates["user-1"].Cookie != cookie {
		t.Fatalf("cookie did not round-trip: %+v", loaded.UserStates["user-1"])
	}
}

func TestSQLitePersistsUsageGuideAcknowledgement(t *testing.T) {
	path := t.TempDir() + "/state.db"
	store, err := OpenSQLite(path, bytes.Repeat([]byte{0x12}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	ackAt := now.Add(time.Minute)
	if err := store.Save(State{
		Version: 3,
		Users: []model.User{{
			ID:              "user-1",
			Username:        "alice",
			PasswordHash:    "hash",
			Role:            model.RoleUser,
			Enabled:         true,
			CreatedAt:       now,
			UpdatedAt:       now,
			UsageGuideAckAt: &ackAt,
		}},
		UserStates: map[string]UserState{"user-1": {}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenSQLite(path, bytes.Repeat([]byte{0x12}, CookieKeySize))
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer reopened.Close()
	loaded, ok, err := reopened.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ok || len(loaded.Users) != 1 || loaded.Users[0].UsageGuideAckAt == nil {
		t.Fatalf("usage guide ack missing after reload: %+v", loaded.Users)
	}
	if !loaded.Users[0].UsageGuideAckAt.Equal(ackAt) {
		t.Fatalf("usage guide ack = %s, want %s", loaded.Users[0].UsageGuideAckAt, ackAt)
	}
}

func TestSQLiteRejectsWrongCookieKey(t *testing.T) {
	path := t.TempDir() + "/state.db"
	store, err := OpenSQLite(path, bytes.Repeat([]byte{0x21}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	now := time.Now()
	if err := store.Save(State{
		Version: 2,
		Users: []model.User{{
			ID: "user-1", Username: "alice", PasswordHash: "hash",
			Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now,
		}},
		UserStates: map[string]UserState{
			"user-1": {Cookie: "sid=secret"},
		},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	wrongKeyStore, err := OpenSQLite(path, bytes.Repeat([]byte{0x22}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite with wrong key: %v", err)
	}
	defer wrongKeyStore.Close()
	if _, _, err := wrongKeyStore.Load(); err == nil {
		t.Fatal("expected cookie decryption to fail with the wrong key")
	}
}

func TestDecodeCookieKey(t *testing.T) {
	encoded := "ERERERERERERERERERERERERERERERERERERERERERE="
	key, err := DecodeCookieKey(encoded)
	if err != nil {
		t.Fatalf("DecodeCookieKey: %v", err)
	}
	if len(key) != CookieKeySize {
		t.Fatalf("decoded key length = %d", len(key))
	}
	if _, err := DecodeCookieKey("too-short"); err == nil {
		t.Fatal("expected invalid key error")
	}
}
