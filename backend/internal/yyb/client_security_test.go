package yyb

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewClientRejectsUnsafeURLAndMissingSecret(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "http://0.0.0.0:8000", APISecret: []byte("secret")}); err == nil {
		t.Fatalf("NewClient accepted unsafe base URL")
	}
	if _, err := NewClient(Config{BaseURL: "http://127.0.0.1:8000"}); err == nil {
		t.Fatalf("NewClient accepted missing API secret")
	}
}

func TestClientSignsSerializedJSONRequests(t *testing.T) {
	var sawSignature bool
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-YYB-Timestamp") == "" || r.Header.Get("X-YYB-Nonce") == "" || r.Header.Get("X-YYB-Signature") == "" {
			t.Fatalf("missing yyb signature headers")
		}
		if r.URL.Path != "/wxapp/getCode" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["ref"] != "acc-1" || body["app_id"] != "wx-app" {
			t.Fatalf("request body = %#v", body)
		}
		sawSignature = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":0,"msg":"success","data":{"code":"wx-code"}}`)),
		}, nil
	})

	client, err := NewClient(Config{
		BaseURL:    "http://127.0.0.1:8000",
		APISecret:  []byte("secret"),
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	code, err := client.GetCode(t.Context(), "acc-1", "wx-app")
	if err != nil {
		t.Fatalf("GetCode() error = %v", err)
	}
	if code != "wx-code" {
		t.Fatalf("response code = %q", code)
	}
	if !sawSignature {
		t.Fatalf("server did not receive signed request")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClientRedactsUpstreamErrorBody(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader(`{"code":"secret-code","refresh_token":"secret-refresh","message":"failed"}`)),
		}, nil
	})
	client, err := NewClient(Config{
		BaseURL:    "http://127.0.0.1:8000",
		APISecret:  []byte("secret"),
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.GetCode(t.Context(), "acc-1", "wx-app")
	if err == nil {
		t.Fatalf("GetCode() expected error")
	}
	msg := err.Error()
	for _, secret := range []string{"secret-code", "secret-refresh"} {
		if strings.Contains(msg, secret) {
			t.Fatalf("error leaked %q: %s", secret, msg)
		}
	}
	if !strings.Contains(msg, "<redacted:code:len=11>") || !strings.Contains(msg, "<redacted:refresh_token:len=14>") {
		t.Fatalf("error did not contain redaction markers: %s", msg)
	}
}
