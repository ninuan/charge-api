package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charge-dashboard/internal/persistence"
)

func TestSessionManagerLimitsAndRevokesUserSessions(t *testing.T) {
	manager := NewSessionManager(time.Hour)
	defer manager.Close()

	var first Session
	for i := 0; i < defaultMaxSessionsPerUser+1; i++ {
		session, err := manager.Create("user-1")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if i == 0 {
			first = session
		}
		time.Sleep(time.Millisecond)
	}
	if _, ok := manager.Get(first.Token); ok {
		t.Fatal("expected oldest session to be evicted")
	}

	current, err := manager.Create("user-2")
	if err != nil {
		t.Fatalf("Create second user: %v", err)
	}
	if err := manager.DeleteUser("user-1"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, ok := manager.Get(current.Token); !ok {
		t.Fatal("revoking one user should not affect another user")
	}
}

func TestSessionManagerRemovesExpiredSession(t *testing.T) {
	manager := NewSessionManager(time.Millisecond)
	defer manager.Close()
	session, err := manager.Create("user-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, ok := manager.Get(session.Token); ok {
		t.Fatal("expected expired session to be rejected")
	}
}

func TestPersistentSessionSurvivesManagerRestartWithoutStoringRawToken(t *testing.T) {
	path := t.TempDir() + "/state.db"
	store, err := persistence.OpenSQLite(path, bytes.Repeat([]byte{0x66}, persistence.CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	first := NewPersistentSessionManager(time.Hour, store)
	session, err := first.Create("user-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	first.Close()

	second := NewPersistentSessionManager(time.Hour, store)
	defer second.Close()
	loaded, ok := second.Get(session.Token)
	if !ok || loaded.UserID != "user-1" {
		t.Fatalf("persistent session not restored: %+v", loaded)
	}

	files, err := filepath.Glob(path + "*")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if bytes.Contains(body, []byte(session.Token)) {
			t.Fatalf("%s contains raw session token", file)
		}
	}
}
