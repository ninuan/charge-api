package mocele

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExchangeCodeBuildsAutologinRequestAndCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax/WxPay/Api/autologin" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.URL.Query().Get("r"); got != "/i/device/open?id=device-123" {
			t.Fatalf("r query = %q", got)
		}
		if got := r.URL.Query().Get("code"); got != "wx-code-1" {
			t.Fatalf("code query = %q", got)
		}
		if got := r.URL.Query().Get("state"); got != "1" {
			t.Fatalf("state query = %q", got)
		}
		cookie := r.Header.Get("Cookie")
		for _, want := range []string{"deviceid=device-123", "org=1", "openindex=7"} {
			if !strings.Contains(cookie, want) {
				t.Fatalf("Cookie header %q missing %q", cookie, want)
			}
		}
		http.SetCookie(w, &http.Cookie{Name: "wxopenid", Value: "openid-value"})
		http.SetCookie(w, &http.Cookie{Name: "info", Value: "info-value"})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	result, err := client.ExchangeCode(t.Context(), "device-123", "wx-code-1")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if result.WXOpenID != "openid-value" || result.Info != "info-value" {
		t.Fatalf("CookieResult = %#v", result)
	}
	wantCookie := "deviceid=device-123; org=1; openindex=7; wxopenid=openid-value; info=info-value"
	if result.Cookie != wantCookie {
		t.Fatalf("Cookie = %q, want %q", result.Cookie, wantCookie)
	}
}

func TestExchangeCodeRequiresWXOpenIDAndInfoCookies(t *testing.T) {
	tests := []struct {
		name      string
		setCookie func(http.ResponseWriter)
		wantError string
	}{
		{
			name: "missing wxopenid",
			setCookie: func(w http.ResponseWriter) {
				http.SetCookie(w, &http.Cookie{Name: "info", Value: "info-value"})
			},
			wantError: "missing wxopenid",
		},
		{
			name: "missing info",
			setCookie: func(w http.ResponseWriter) {
				http.SetCookie(w, &http.Cookie{Name: "wxopenid", Value: "openid-value"})
			},
			wantError: "missing info",
		},
		{
			name: "http error",
			setCookie: func(w http.ResponseWriter) {
				http.Error(w, `wxopenid=secret-openid&info=secret-info`, http.StatusBadGateway)
			},
			wantError: "status=502",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.setCookie(w)
			}))
			defer server.Close()
			client := NewClient(Config{BaseURL: server.URL})
			_, err := client.ExchangeCode(t.Context(), "device-123", "wx-code-1")
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.wantError)
			}
			if strings.Contains(err.Error(), "secret-openid") || strings.Contains(err.Error(), "secret-info") {
				t.Fatalf("error leaked sensitive value: %s", err)
			}
		})
	}
}
