package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHCaptchaVerifierPostsExpectedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("content type = %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		want := map[string]string{
			"secret":   "secret-value",
			"response": "response-token",
			"sitekey":  "site-key",
			"remoteip": "203.0.113.9",
		}
		for key, expected := range want {
			if got := r.PostForm.Get(key); got != expected {
				t.Fatalf("%s = %q, want %q", key, got, expected)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	verifier := newHCaptchaVerifier(" site-key ", " secret-value ", server.URL, server.Client())
	if err := verifier.Verify(context.Background(), " response-token ", "203.0.113.9"); err != nil {
		t.Fatalf("verify hcaptcha: %v", err)
	}
}

func TestHCaptchaVerifierDisabled(t *testing.T) {
	verifier := NewHCaptchaVerifier("", "")
	if verifier.Enabled() {
		t.Fatal("empty verifier must be disabled")
	}
	if err := verifier.Verify(context.Background(), "", ""); err != nil {
		t.Fatalf("disabled verifier returned error: %v", err)
	}
}

func TestHCaptchaVerifierRejectsInvalidAndProviderFailures(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		statusCode  int
		body        string
		unavailable bool
		reason      string
	}{
		{name: "provider rejects token", token: "bad", statusCode: http.StatusOK, body: `{"success":false,"error-codes":["expired-input-response"]}`, reason: "expired-input-response"},
		{name: "sitekey and secret mismatch", token: "token", statusCode: http.StatusOK, body: `{"success":false,"error-codes":["sitekey-secret-mismatch"]}`, unavailable: true, reason: "sitekey-secret-mismatch"},
		{name: "provider rate limited", token: "token", statusCode: http.StatusTooManyRequests, body: `{"success":false}`, unavailable: true, reason: "provider-http-429"},
		{name: "provider unavailable", token: "token", statusCode: http.StatusServiceUnavailable, body: `upstream unavailable`, unavailable: true, reason: "provider-http-503"},
		{name: "invalid json", token: "token", statusCode: http.StatusOK, body: `{not-json`, unavailable: true, reason: "provider-response-invalid-json"},
		{name: "oversized response", token: "token", statusCode: http.StatusOK, body: strings.Repeat("x", hcaptchaBodyMaxBytes+1), unavailable: true, reason: "provider-response-too-large"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			verifier := newHCaptchaVerifier("site", "secret-not-for-errors", server.URL, server.Client())
			err := verifier.Verify(context.Background(), test.token, "")
			if err == nil {
				t.Fatal("expected verification error")
			}
			if strings.Contains(err.Error(), "secret-not-for-errors") || strings.Contains(err.Error(), test.token) {
				t.Fatalf("error leaked credentials: %v", err)
			}
			if got := IsHCaptchaUnavailable(err); got != test.unavailable {
				t.Fatalf("unavailable = %t, want %t: %v", got, test.unavailable, err)
			}
			if !strings.Contains(err.Error(), test.reason) {
				t.Fatalf("error = %q, want safe reason %q", err, test.reason)
			}
		})
	}
}

func TestHCaptchaVerifierRejectsMissingAndOversizedTokensWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	verifier := newHCaptchaVerifier("site", "secret", server.URL, server.Client())
	for _, token := range []string{"", strings.Repeat("x", hcaptchaTokenMaxBytes+1)} {
		if err := verifier.Verify(context.Background(), token, ""); err == nil {
			t.Fatalf("token length %d unexpectedly passed", len(token))
		}
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("provider requests = %d, want 0", got)
	}
}

func TestHCaptchaVerifierHonorsClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 5 * time.Millisecond
	verifier := newHCaptchaVerifier("site", "secret", server.URL, client)
	err := verifier.Verify(context.Background(), "token", "")
	if err == nil || !IsHCaptchaUnavailable(err) || !strings.Contains(err.Error(), "provider-timeout") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestSafeHCaptchaErrorCodesRejectsUntrustedDetails(t *testing.T) {
	got := safeHCaptchaErrorCodes([]string{
		"expired-input-response",
		"expired-input-response",
		"secret=do-not-log",
		strings.Repeat("x", 65),
	})
	if len(got) != 1 || got[0] != "expired-input-response" {
		t.Fatalf("safe codes = %#v", got)
	}
}

func TestHCaptchaVerifierRejectsInvalidEndpoint(t *testing.T) {
	verifier := newHCaptchaVerifier("site", "secret", "://bad-url", http.DefaultClient)
	err := verifier.Verify(context.Background(), "token", "")
	if err == nil || !strings.HasPrefix(err.Error(), "create hcaptcha request:") {
		t.Fatalf("invalid endpoint error = %v", err)
	}
	if strings.Contains(fmt.Sprint(err), "secret") {
		t.Fatalf("invalid endpoint error leaked secret: %v", err)
	}
}
