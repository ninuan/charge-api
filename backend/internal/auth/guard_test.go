package auth

import "testing"

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
