package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestAuthGuardLocksAfterFailures(t *testing.T) {
	guard := NewAuthGuard()
	for i := 0; i < 4; i++ {
		locked, _ := guard.RecordFailure("203.0.113.1", "alice")
		if locked {
			t.Fatalf("locked too early after %d failures", i+1)
		}
	}
	locked, retryAfter := guard.RecordFailure("203.0.113.1", "alice")
	if !locked || retryAfter <= 0 {
		t.Fatalf("expected lock after fifth failure")
	}

	if locked, _ := guard.Locked("203.0.113.1", "alice"); !locked {
		t.Fatalf("expected account or IP to remain locked")
	}
}

func TestAuthGuardRequiresCaptchaAfterTwoFailures(t *testing.T) {
	guard := NewAuthGuard()
	if guard.RequiresCaptcha("203.0.113.1", "alice") {
		t.Fatal("captcha required before any failures")
	}
	guard.RecordFailure("203.0.113.1", "alice")
	if guard.RequiresCaptcha("203.0.113.1", "alice") {
		t.Fatal("captcha required after only one failure")
	}
	guard.RecordFailure("203.0.113.1", "alice")
	if !guard.RequiresCaptcha("203.0.113.1", "alice") {
		t.Fatal("captcha not required after two failures")
	}

	guard.RecordSuccess("203.0.113.1", "alice")
	if guard.RequiresCaptcha("203.0.113.1", "alice") {
		t.Fatal("captcha requirement remained after successful login")
	}
}

func TestAuthGuardDoesNotLockOtherIPsByUsername(t *testing.T) {
	guard := NewAuthGuard()
	for i := 0; i < 5; i++ {
		guard.RecordFailure("198.51.100.10", "admin")
	}
	if locked, _ := guard.Locked("198.51.100.10", "admin"); !locked {
		t.Fatal("attacker IP should be locked after five failures")
	}
	if locked, _ := guard.Locked("203.0.113.99", "admin"); locked {
		t.Fatal("legitimate IP must not be globally locked out by another IP guessing the username")
	}
	if !guard.RequiresCaptcha("203.0.113.99", "admin") {
		t.Fatal("distributed password guesses against a username should still require CAPTCHA")
	}
}

func TestAuthGuardEvictsUnlockedEntriesBeforeLockedOnes(t *testing.T) {
	guard := NewAuthGuard()
	now := time.Now()
	guard.mu.Lock()
	// Add an actively locked entry with an older WindowStart than all unlocked entries.
	guard.failures["ip:locked"] = &guardEntry{
		Count:        5,
		WindowStart:  now.Add(-10 * time.Minute),
		BlockedUntil: now.Add(15 * time.Minute),
	}
	for i := 0; i < maxAuthGuardEntries-1; i++ {
		guard.failures[fmt.Sprintf("ip:unlocked-%d", i)] = &guardEntry{
			Count:       1,
			WindowStart: now.Add(-time.Duration(i+1) * time.Millisecond),
		}
	}
	guard.ensureCapacityLocked(guard.failures, now)
	_, stillLocked := guard.failures["ip:locked"]
	guard.mu.Unlock()

	if !stillLocked {
		t.Fatal("actively locked entry must not be evicted while unlocked entries exist")
	}
}
