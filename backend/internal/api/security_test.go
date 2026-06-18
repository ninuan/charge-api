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
