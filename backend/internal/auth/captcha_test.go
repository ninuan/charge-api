package auth

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"
	"time"
)

func TestCaptchaGeneratesRasterImageWithoutPlaintextAnswer(t *testing.T) {
	store := NewCaptchaStore()
	challenge, err := store.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	entry := store.challenges[challenge.ID]
	if len(entry.answer) != 5 {
		t.Fatalf("answer length = %d, want 5", len(entry.answer))
	}
	for _, digit := range entry.answer {
		if !strings.ContainsRune(string(captchaAlphabet), digit) {
			t.Fatalf("answer contains unsupported digit %q", digit)
		}
	}

	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(challenge.Image, prefix) {
		t.Fatalf("image prefix = %q, want PNG data URL", challenge.Image[:min(len(challenge.Image), 32)])
	}
	imageBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(challenge.Image, prefix))
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	if bytes.Contains(imageBytes, []byte(entry.answer)) {
		t.Fatal("PNG payload contains the plaintext captcha answer")
	}
	config, err := png.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	if config.Width != captchaWidth || config.Height != captchaHeight {
		t.Fatalf("PNG size = %dx%d, want %dx%d", config.Width, config.Height, captchaWidth, captchaHeight)
	}
}

func TestCaptchaIsSingleUse(t *testing.T) {
	store := NewCaptchaStore()
	challenge, err := store.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	answer := store.challenges[challenge.ID].answer
	if err := store.Verify(challenge.ID, answer); err != nil {
		t.Fatalf("first Verify: %v", err)
	}
	if err := store.Verify(challenge.ID, answer); err == nil {
		t.Fatal("second Verify unexpectedly accepted a consumed challenge")
	}
}

func TestCaptchaWrongAnswerConsumesChallenge(t *testing.T) {
	store := NewCaptchaStore()
	challenge, err := store.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	answer := store.challenges[challenge.ID].answer
	wrongAnswer := "22222"
	if answer == wrongAnswer {
		wrongAnswer = "33333"
	}
	if err := store.Verify(challenge.ID, wrongAnswer); err == nil {
		t.Fatal("wrong answer unexpectedly succeeded")
	}
	if err := store.Verify(challenge.ID, answer); err == nil {
		t.Fatal("challenge remained usable after a wrong answer")
	}
}

func TestCaptchaExpiresAfterTwoMinutes(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	store := NewCaptchaStore()
	store.now = func() time.Time { return now }
	challenge, err := store.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	answer := store.challenges[challenge.ID].answer
	if got := challenge.ExpiresAt.Sub(now); got != 2*time.Minute {
		t.Fatalf("TTL = %s, want 2m", got)
	}
	now = now.Add(2*time.Minute + time.Nanosecond)
	if err := store.Verify(challenge.ID, answer); err == nil {
		t.Fatal("expired challenge unexpectedly succeeded")
	}
}
