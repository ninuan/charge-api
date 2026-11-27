package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCORSRejectsUnknownAndNullOrigins(t *testing.T) {
	handler := WithCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), []string{"http://127.0.0.1:5173"})

	for _, origin := range []string{"https://evil.example", "null", "not-an-origin"} {
		request := httptest.NewRequest(http.MethodPost, "http://api.example/api/refresh", nil)
		request.Header.Set("Origin", origin)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("origin %q returned %d, want 403", origin, recorder.Code)
		}
	}
}

func TestCORSAllowsConfiguredPreflight(t *testing.T) {
	handler := WithCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("preflight should not reach application handler")
	}), []string{"http://127.0.0.1:5173"})
	request := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:8080/api/auth/me", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight returned %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestIPRateLimiter(t *testing.T) {
	limiter := NewIPRateLimiter(2, time.Minute)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for attempt := 1; attempt <= 3; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "http://api.example/api/auth/me", nil)
		request.RemoteAddr = "203.0.113.10:1234"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if attempt <= 2 && recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d returned %d", attempt, recorder.Code)
		}
		if attempt == 3 && recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("limited attempt returned %d, want 429", recorder.Code)
		}
	}
}

func TestCSRFBlocksCrossSiteAndSameSiteBodylessPosts(t *testing.T) {
	handler := WithCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), nil)

	for _, fetchSite := range []string{"cross-site", "same-site"} {
		req := httptest.NewRequest(http.MethodPost, "https://charge.example.com/api/admin/users/u1/reset-password", nil)
		req.Header.Set("Sec-Fetch-Site", fetchSite)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("Sec-Fetch-Site=%q returned %d, want 403", fetchSite, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "https://charge.example.com/api/session/cookie/join-pool", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Origin", "https://charge.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("same-origin POST returned %d, want 200", rec.Code)
	}
}

func TestClientIPAndMultiHopForwardedProto(t *testing.T) {
	// Non-IP rightmost X-Forwarded-For entry must fail closed to RemoteAddr instead of walking leftward.
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/auth/me", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.99, unknown")
	if got := clientIP(req); got != "127.0.0.1" {
		t.Fatalf("clientIP with non-IP rightmost hop = %q, want 127.0.0.1", got)
	}

	// Explicit TRUSTED_PROXY_HEADER=x-forwarded-for ignores attacker-spoofed X-Real-IP.
	t.Setenv("TRUSTED_PROXY_HEADER", "x-forwarded-for")
	req2 := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/auth/me", nil)
	req2.RemoteAddr = "127.0.0.1:54321"
	req2.Header.Set("X-Real-IP", "1.1.1.1")
	req2.Header.Set("X-Forwarded-For", "2001:0db8::0001")
	if got := clientIP(req2); got != "2001:db8::1" {
		t.Fatalf("clientIP with TRUSTED_PROXY_HEADER=x-forwarded-for = %q, want canonical 2001:db8::1", got)
	}

	// Multi-hop X-Forwarded-Proto inspects rightmost hop.
	protoHTTPS := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/", nil)
	protoHTTPS.RemoteAddr = "127.0.0.1:54321"
	protoHTTPS.Header.Set("X-Forwarded-Proto", "https, https")
	if !requestIsSecure(protoHTTPS) {
		t.Fatal("expected multi-hop 'https, https' to be secure")
	}
	protoDowngraded := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/", nil)
	protoDowngraded.RemoteAddr = "127.0.0.1:54321"
	protoDowngraded.Header.Set("X-Forwarded-Proto", "https, http")
	if requestIsSecure(protoDowngraded) {
		t.Fatal("expected multi-hop 'https, http' to be insecure")
	}
}

func TestWithCompressionSkipsHeadAndSupportsUnwrap(t *testing.T) {
	headHandler := WithCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	}))
	headReq := httptest.NewRequest(http.MethodHead, "/dashboard/", nil)
	headReq.Header.Set("Accept-Encoding", "gzip")
	headRec := httptest.NewRecorder()
	headHandler.ServeHTTP(headRec, headReq)
	if headRec.Body.Len() != 0 || headRec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("HEAD response must not be gzip-encoded or carry body: len=%d encoding=%q", headRec.Body.Len(), headRec.Header().Get("Content-Encoding"))
	}

	unwrapHandler := WithCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(interface{ Unwrap() http.ResponseWriter }); !ok {
			t.Fatal("gzipResponseWriter must implement Unwrap()")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	getReq := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	getReq.Header.Set("Accept-Encoding", "gzip")
	unwrapHandler.ServeHTTP(httptest.NewRecorder(), getReq)
}
