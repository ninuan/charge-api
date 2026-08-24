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

func TestWxPusherClientConfigFromEnv(t *testing.T) {
	if client, err := wxPusherClientFromEnv(func(string) string { return "" }); err != nil || client != nil {
		t.Fatalf("empty env client=%v err=%v, want nil nil", client, err)
	}
	if _, err := wxPusherClientFromEnv(func(name string) string {
		if name == "WXPUSHER_BASE_URL" {
			return "http://127.0.0.1:8089"
		}
		return ""
	}); err == nil {
		t.Fatal("expected missing app token error")
	}
	client, err := wxPusherClientFromEnv(func(name string) string {
		switch name {
		case "WXPUSHER_APP_TOKEN":
			return "AT_test"
		case "WXPUSHER_BASE_URL":
			return "http://127.0.0.1:8089"
		default:
			return ""
		}
	})
	if err != nil || client == nil {
		t.Fatalf("configured client=%v err=%v", client, err)
	}
}
