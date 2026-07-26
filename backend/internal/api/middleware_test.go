package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompressionShrinksTextResponses(t *testing.T) {
	payload := strings.Repeat("charge console dashboard payload ", 200)
	handler := WithCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, payload)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/piles", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if !strings.Contains(recorder.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("Vary = %q, want it to include Accept-Encoding", recorder.Header().Get("Vary"))
	}
	if recorder.Body.Len() >= len(payload) {
		t.Fatalf("compressed body is %d bytes, want fewer than the %d byte payload", recorder.Body.Len(), len(payload))
	}

	reader, err := gzip.NewReader(recorder.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read compressed body: %v", err)
	}
	if string(decoded) != payload {
		t.Fatalf("decompressed body does not match the original payload")
	}
}

func TestCompressionSkipsStreamAndUnsupportedClients(t *testing.T) {
	handler := WithCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: snapshot\ndata: {}\n\n")
	}))

	stream := httptest.NewRequest(http.MethodGet, streamPath, nil)
	stream.Header.Set("Accept-Encoding", "gzip")
	streamRecorder := httptest.NewRecorder()
	handler.ServeHTTP(streamRecorder, stream)
	if got := streamRecorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("SSE Content-Encoding = %q, want it left uncompressed", got)
	}

	plain := httptest.NewRequest(http.MethodGet, "/api/piles", nil)
	plainRecorder := httptest.NewRecorder()
	handler.ServeHTTP(plainRecorder, plain)
	if got := plainRecorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("client without Accept-Encoding got %q, want an uncompressed response", got)
	}
}

func TestCompressionLeavesBinaryAndNotModifiedAlone(t *testing.T) {
	binary := WithCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	}))
	request := httptest.NewRequest(http.MethodGet, "/icon.png", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	binary.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("png Content-Encoding = %q, want it left uncompressed", got)
	}

	notModified := WithCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.WriteHeader(http.StatusNotModified)
	}))
	cached := httptest.NewRequest(http.MethodGet, "/_next/static/app.css", nil)
	cached.Header.Set("Accept-Encoding", "gzip")
	cachedRecorder := httptest.NewRecorder()
	notModified.ServeHTTP(cachedRecorder, cached)
	if got := cachedRecorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("304 Content-Encoding = %q, want no body encoding", got)
	}
}

func TestAcceptsGzipHonoursQualityValues(t *testing.T) {
	cases := map[string]bool{
		"":                      false,
		"gzip":                  true,
		"gzip, deflate, br":     true,
		"br;q=1.0, gzip;q=0.8":  true,
		"gzip;q=0":              false,
		"gzip;q=0.0":            false,
		"identity":              false,
		"deflate, gzip;q=0.001": true,
	}
	for header, want := range cases {
		if got := acceptsGzip(header); got != want {
			t.Fatalf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestCacheHeadersSeparateHashedAssetsFromHTMLAndAPI(t *testing.T) {
	handler := WithCacheHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cases := map[string]string{
		"/_next/static/chunks/app.js": "public, max-age=31536000, immutable",
		"/dashboard/index.html":       "no-cache",
		"/":                           "no-cache",
		"/api/piles":                  "no-store",
	}
	for path, want := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if got := recorder.Header().Get("Cache-Control"); got != want {
			t.Fatalf("Cache-Control for %s = %q, want %q", path, got, want)
		}
	}
}

func TestSecurityHeadersAreSetOnEveryResponse(t *testing.T) {
	handler := WithSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))

	expected := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
	}
	for name, want := range expected {
		if got := recorder.Header().Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	policy := recorder.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"frame-ancestors 'none'",
		"object-src 'none'",
		"base-uri 'none'",
		"https://challenges.cloudflare.com",
	} {
		if !strings.Contains(policy, directive) {
			t.Fatalf("CSP %q is missing %q", policy, directive)
		}
	}
	if recorder.Header().Get("Strict-Transport-Security") != "" {
		t.Fatalf("HSTS must not be sent over plaintext http")
	}
}
