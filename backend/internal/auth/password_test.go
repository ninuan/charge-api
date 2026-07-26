package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestArgon2Password(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	valid, needsUpgrade := VerifyPassword("correct-horse-battery", hash)
	if !valid || needsUpgrade {
		t.Fatalf("expected valid Argon2id password without upgrade")
	}
	if valid, _ := VerifyPassword("wrong-password", hash); valid {
		t.Fatalf("wrong password unexpectedly validated")
	}
}

func TestLegacySHA256NeedsUpgrade(t *testing.T) {
	salt := []byte("1234567890abcdef")
	sum := sha256.Sum256(append(salt, []byte("legacy-password")...))
	encoded := "sha256$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(sum[:])

	valid, needsUpgrade := VerifyPassword("legacy-password", encoded)
	if !valid || !needsUpgrade {
		t.Fatalf("expected legacy password to validate and require upgrade")
	}
}

func TestWrappedLegacySHA256(t *testing.T) {
	salt := []byte("1234567890abcdef")
	sum := sha256.Sum256(append(salt, []byte("legacy-password")...))
	encoded := "sha256$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(sum[:])

	wrapped, err := WrapLegacySHA256(encoded)
	if err != nil {
		t.Fatalf("WrapLegacySHA256: %v", err)
	}
	if IsLegacySHA256(wrapped) {
		t.Fatalf("wrapped hash still looks like a bare legacy hash: %q", wrapped)
	}

	valid, needsUpgrade := VerifyPassword("legacy-password", wrapped)
	if !valid || !needsUpgrade {
		t.Fatalf("expected wrapped legacy password to validate and require upgrade")
	}
	if valid, _ := VerifyPassword("wrong-password", wrapped); valid {
		t.Fatalf("wrong password unexpectedly validated against wrapped hash")
	}
}

func TestWrapLegacySHA256RejectsOtherFormats(t *testing.T) {
	hash, err := HashPassword("modern-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := WrapLegacySHA256(hash); err == nil {
		t.Fatalf("expected wrapping a non-legacy hash to fail")
	}
}
