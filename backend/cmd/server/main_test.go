package main

import "testing"

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

func TestPublicBaseURLConfigFromEnv(t *testing.T) {
	lookup := func(value string) envLookup {
		return func(name string) string {
			if name == "PUBLIC_BASE_URL" {
				return value
			}
			return ""
		}
	}
	if value, err := publicBaseURLFromEnv(lookup(""), false); err != nil || value != "" {
		t.Fatalf("optional empty public URL = %q, %v", value, err)
	}
	if _, err := publicBaseURLFromEnv(lookup(""), true); err == nil {
		t.Fatal("required empty public URL was accepted")
	}
	if value, err := publicBaseURLFromEnv(lookup("https://charge.example.com/"), true); err != nil || value != "https://charge.example.com" {
		t.Fatalf("public URL = %q, %v", value, err)
	}
	for _, invalid := range []string{"http://charge.example.com", "https://user@example.com", "https://example.com/path"} {
		if _, err := publicBaseURLFromEnv(lookup(invalid), true); err == nil {
			t.Fatalf("invalid public URL %q was accepted", invalid)
		}
	}
}
