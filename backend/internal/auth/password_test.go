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
