package securelink

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
)

func TestSignerAddsYYBHMACHeaders(t *testing.T) {
	signer, err := NewSigner([]byte("shared-secret"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	body := []byte(`{"ok":true}`)
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8000/wxapp/getCode?x=1", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	if err := signer.Sign(req, body); err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	timestamp := req.Header.Get("X-YYB-Timestamp")
	nonce := req.Header.Get("X-YYB-Nonce")
	signature := req.Header.Get("X-YYB-Signature")
	if timestamp == "" || nonce == "" || signature == "" {
		t.Fatalf("missing signed headers: timestamp=%q nonce=%q signature=%q", timestamp, nonce, signature)
	}
	if len(nonce) < 32 {
		t.Fatalf("nonce too short: %q", nonce)
	}
	bodyHash := sha256.Sum256(body)
	canonical := http.MethodPost + "\n" + "/wxapp/getCode?x=1" + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(bodyHash[:])
	mac := hmac.New(sha256.New, []byte("shared-secret"))
	_, _ = mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))
	if signature != want {
		t.Fatalf("signature = %q, want %q", signature, want)
	}
}

func TestNewSignerRejectsMissingSecret(t *testing.T) {
	if _, err := NewSigner(nil); err == nil {
		t.Fatalf("NewSigner(nil) expected error")
	}
}

func TestLoopbackBaseURLRejectsUnsafeHosts(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8000", "http://localhost:8000"} {
		if _, err := LoopbackBaseURL(raw); err != nil {
			t.Fatalf("LoopbackBaseURL(%q) error = %v", raw, err)
		}
	}
	for _, raw := range []string{"http://0.0.0.0:8000", "http://192.168.1.10:8000", "https://127.0.0.1:8000", "http://example.com", "http://127.0.0.1"} {
		if _, err := LoopbackBaseURL(raw); err == nil {
			t.Fatalf("LoopbackBaseURL(%q) expected error", raw)
		}
	}
}
