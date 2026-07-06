package yyb

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientParsesEnvelopeAndDirectYYBResponses(t *testing.T) {
	var seen []string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-YYB-Signature") == "" {
			t.Fatalf("request %s missing signature", r.URL.Path)
		}
		switch r.URL.Path {
		case "/qr":
			if r.URL.RawQuery != "as_base64=true" {
				t.Fatalf("create QR query = %q, want as_base64=true", r.URL.RawQuery)
			}
			return jsonResponse(http.StatusOK, `{"code":0,"msg":"ok","data":{"session_id":"sid-1","image_url":"/qr/sid-1/image","image_base64":"data:image/jpeg;base64,abc","status":"waiting"}}`), nil
		case "/qr/sid-1/poll":
			return jsonResponse(http.StatusOK, `{"session_id":"sid-1","status":"confirmed","message":"ready"}`), nil
		case "/qr/sid-1/confirm":
			return jsonResponse(http.StatusOK, `{"id":12345,"openid":"openid-1","nickname":"Alice","avatar":"https://avatar.example/a.png","status":"alive"}`), nil
		case "/wxapp/getCode":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode getCode request: %v", err)
			}
			if body["ref"] != "12345" || body["app_id"] != "wx-app" {
				t.Fatalf("getCode body = %#v", body)
			}
			return jsonResponse(http.StatusOK, `{"code":0,"msg":"ok","data":{"result":{"code":"wx-code"}}}`), nil
		case "/accounts/refresh":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode refresh request: %v", err)
			}
			if body["ref"] != "12345" {
				t.Fatalf("refresh body = %#v", body)
			}
			return jsonResponse(http.StatusOK, `{"code":0,"msg":"ok","data":{"ref":"12345","status":"alive"}}`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return nil, nil
	})

	client, err := NewClient(Config{BaseURL: "http://127.0.0.1:8000", APISecret: []byte("secret"), HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	qr, err := client.CreateQR(t.Context())
	if err != nil {
		t.Fatalf("CreateQR: %v", err)
	}
	if qr.SessionID != "sid-1" || qr.ImageURL != "/qr/sid-1/image" || qr.ImageBase64 == "" || qr.Status != "waiting" {
		t.Fatalf("QRSession = %#v", qr)
	}

	poll, err := client.PollQR(t.Context(), "sid-1")
	if err != nil {
		t.Fatalf("PollQR: %v", err)
	}
	if poll.Status != "confirmed" || poll.Message != "ready" {
		t.Fatalf("QRPollResult = %#v", poll)
	}

	account, err := client.ConfirmQR(t.Context(), "sid-1")
	if err != nil {
		t.Fatalf("ConfirmQR: %v", err)
	}
	if account.ID != "12345" || account.Ref != "12345" || account.OpenID != "openid-1" || account.Nickname != "Alice" {
		t.Fatalf("YYBAccount = %#v", account)
	}

	code, err := client.GetCode(t.Context(), account.Ref, "wx-app")
	if err != nil {
		t.Fatalf("GetCode: %v", err)
	}
	if code != "wx-code" {
		t.Fatalf("code = %q", code)
	}

	if err := client.RefreshAccount(t.Context(), account.Ref); err != nil {
		t.Fatalf("RefreshAccount: %v", err)
	}

	want := []string{"POST /qr", "GET /qr/sid-1/poll", "POST /qr/sid-1/confirm", "POST /wxapp/getCode", "POST /accounts/refresh"}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("seen paths = %#v", seen)
	}
}

func TestClientRejectsEnvelopeHTTPAndMissingCodeErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "error envelope", code: http.StatusOK, body: `{"code":400,"msg":"bad wxopenid secret-token","data":{}}`},
		{name: "http error", code: http.StatusBadGateway, body: `{"refresh_token":"secret-refresh","msg":"bad"}`},
		{name: "missing code", code: http.StatusOK, body: `{"code":0,"msg":"ok","data":{"result":{}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResponse(tt.code, tt.body), nil
			})
			client, err := NewClient(Config{BaseURL: "http://localhost:8000", APISecret: []byte("secret"), HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, err = client.GetCode(t.Context(), "acc-ref", "wx-app")
			if err == nil {
				t.Fatalf("expected GetCode error")
			}
			if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "secret-refresh") {
				t.Fatalf("error leaked secret: %s", err)
			}
		})
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
