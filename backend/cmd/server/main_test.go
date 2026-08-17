package main

import "testing"

func TestYYBClientConfigFromEnv(t *testing.T) {
	if client, err := yybClientFromEnv(func(string) string { return "" }); err != nil || client != nil {
		t.Fatalf("empty env client=%v err=%v, want nil nil", client, err)
	}

	_, err := yybClientFromEnv(func(name string) string {
		if name == "YYB_BASE_URL" {
			return "http://127.0.0.1:8000"
		}
		return ""
	})
	if err == nil {
		t.Fatalf("expected missing YYB_API_SECRET error")
	}

	_, err = yybClientFromEnv(func(name string) string {
		switch name {
		case "YYB_BASE_URL":
			return "http://0.0.0.0:8000"
		case "YYB_API_SECRET":
			return "secret"
		default:
			return ""
		}
	})
	if err == nil {
		t.Fatalf("expected unsafe YYB_BASE_URL error")
	}

	client, err := yybClientFromEnv(func(name string) string {
		switch name {
		case "YYB_BASE_URL":
			return "http://127.0.0.1:8000"
		case "YYB_API_SECRET":
			return "secret"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("yybClientFromEnv() error = %v", err)
	}
	if client == nil {
		t.Fatalf("client is nil")
	}
}

func TestHCaptchaVerifierConfigFromEnv(t *testing.T) {
	lookup := func(values map[string]string) envLookup {
		return func(name string) string { return values[name] }
	}

	verifier, err := hcaptchaVerifierFromEnv(lookup(nil))
	if err != nil || verifier == nil || verifier.Enabled() {
		t.Fatalf("empty env verifier=%v enabled=%v err=%v", verifier, verifier != nil && verifier.Enabled(), err)
	}

	for _, values := range []map[string]string{
		{"HCAPTCHA_SITE_KEY": "site"},
		{"HCAPTCHA_SECRET_KEY": "secret"},
		{"HCAPTCHA_REQUIRED": "true"},
		{"TURNSTILE_REQUIRED": "true"},
	} {
		if _, err := hcaptchaVerifierFromEnv(lookup(values)); err == nil {
			t.Fatalf("config %#v unexpectedly succeeded", values)
		}
	}

	verifier, err = hcaptchaVerifierFromEnv(lookup(map[string]string{
		"HCAPTCHA_REQUIRED":   "true",
		"HCAPTCHA_SITE_KEY":   " site ",
		"HCAPTCHA_SECRET_KEY": " secret ",
	}))
	if err != nil || !verifier.Enabled() || verifier.SiteKey() != "site" {
		t.Fatalf("enabled verifier=%v err=%v", verifier, err)
	}
}

func TestDevForceAuthExpiredRequiresLocalDevMode(t *testing.T) {
	lookup := func(values map[string]string) envLookup {
		return func(name string) string { return values[name] }
	}
	if devForceAuthExpiredEnabled(lookup(map[string]string{"CHARGE_DEV_FORCE_AUTH_EXPIRED": "true"})) {
		t.Fatal("force auth expiry must remain disabled outside local development mode")
	}
	if !devForceAuthExpiredEnabled(lookup(map[string]string{
		"CHARGE_LOCAL_DEV":              "1",
		"CHARGE_DEV_FORCE_AUTH_EXPIRED": "true",
	})) {
		t.Fatal("force auth expiry should be enabled for local development")
	}
}
