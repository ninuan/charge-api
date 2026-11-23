package api

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		"/dashboard/index.html":       "public, no-cache, no-transform",
		"/":                           "public, no-cache, no-transform",
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
		"frame-src 'none'",
		"script-src 'self' https://static.cloudflareinsights.com",
		"connect-src 'self' https://cloudflareinsights.com",
	} {
		if !strings.Contains(policy, directive) {
			t.Fatalf("CSP %q is missing %q", policy, directive)
		}
	}
	if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("CSP %q must not allow unsafe-inline scripts", policy)
	}
	if recorder.Header().Get("Strict-Transport-Security") != "" {
		t.Fatalf("HSTS must not be sent over plaintext http")
	}
}

func TestStaticPolicyHashesInlineScripts(t *testing.T) {
	policy := staticPolicy([]byte(`<html><script>window.__BOOT__=1</script></html>`))
	if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") || !strings.Contains(policy, "'sha256-") {
		t.Fatalf("static policy did not replace inline script allowance: %s", policy)
	}
	if !strings.Contains(policy, "https://static.cloudflareinsights.com") || !strings.Contains(policy, "connect-src 'self' https://cloudflareinsights.com") {
		t.Fatalf("static policy dropped Cloudflare Web Analytics allowance: %s", policy)
	}
}

func TestRequestBodyLimitBeforeDispatch(t *testing.T) {
	for _, route := range []string{"/api/auth/login", "/api/cookie", "/api/stream", "/future-upload"} {
		for _, contentType := range []string{"application/json", "application/x-www-form-urlencoded"} {
			for _, chunked := range []bool{false, true} {
				t.Run(route+contentType+fmt.Sprint(chunked), func(t *testing.T) {
					called := false
					handler := WithRequestBodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
					request := httptest.NewRequest(http.MethodPost, route, strings.NewReader(strings.Repeat("x", int(MaxRequestBodyBytes)+1)))
					request.Header.Set("Content-Type", contentType)
					if chunked {
						request.ContentLength = -1
						request.TransferEncoding = []string{"chunked"}
					}
					recorder := httptest.NewRecorder()
					handler.ServeHTTP(recorder, request)
					if recorder.Code != http.StatusRequestEntityTooLarge || called {
						t.Fatalf("status=%d handler called=%v", recorder.Code, called)
					}
				})
			}
		}
	}
}

func TestRequestBodyLimitPreservesExactBody(t *testing.T) {
	payload := strings.Repeat("x", int(MaxRequestBodyBytes))
	handler := WithRequestBodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != payload {
			t.Fatalf("body changed: length=%d error=%v", len(body), err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(payload)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d", recorder.Code)
	}
}

func TestDecodeJSONRejectsOversizedTrailingWhitespace(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(`{}`+strings.Repeat(" ", 64)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	var target struct{}
	if decodeJSON(recorder, request, 32, &target) || recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d, want 413", recorder.Code)
	}
}

func TestStaticHandlerRoutesAndBlocksDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		"index.html",
		"dashboard/index.html",
		"login/index.html",
		"_next/static/chunks/app.js",
	} {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte("<html><script>console.log(1)</script></html>"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	handler := StaticHandler(dir)
	tests := []struct {
		path   string
		status int
	}{
		{"/", http.StatusOK},
		{"/dashboard/", http.StatusOK},
		{"/dashboard", http.StatusMovedPermanently},
		{"/login", http.StatusMovedPermanently},
		{"/_next/static/chunks/app.js", http.StatusOK},
		{"/_next/static", http.StatusNotFound},
		{"/_next/static/", http.StatusNotFound},
		{"/_next/static/chunks", http.StatusNotFound},
		{"/_next/static/chunks/", http.StatusNotFound},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.status {
			t.Fatalf("GET %s: status=%d, want %d", tc.path, rec.Code, tc.status)
		}
	}
}

func TestStaticHandlerCachesHTMLWithETagAndInvalidatesOnChange(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.html")
	v1 := "<html><script>console.log('v1')</script></html>"
	if err := os.WriteFile(indexPath, []byte(v1), 0o644); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	handler := StaticHandler(dir)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status=%d, want 200", rec1.Code)
	}
	etag1 := rec1.Header().Get("ETag")
	csp1 := rec1.Header().Get("Content-Security-Policy")
	if !strings.HasPrefix(etag1, `W/"`) || !strings.Contains(csp1, "sha256-") {
		t.Fatalf("missing weak ETag or CSP hash: etag=%q csp=%q", etag1, csp1)
	}

	req304 := httptest.NewRequest(http.MethodGet, "/", nil)
	req304.Header.Set("If-None-Match", etag1)
	rec304 := httptest.NewRecorder()
	handler.ServeHTTP(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("conditional status=%d, want 304", rec304.Code)
	}

	v2 := "<html><script>console.log('v2-updated-script')</script></html>"
	if err := os.WriteFile(indexPath, []byte(v2), 0o644); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("updated status=%d, want 200", rec2.Code)
	}
	if rec2.Header().Get("ETag") == etag1 || rec2.Header().Get("Content-Security-Policy") == csp1 {
		t.Fatalf("cache did not invalidate on file update: etag=%q csp=%q", rec2.Header().Get("ETag"), rec2.Header().Get("Content-Security-Policy"))
	}
	if rec2.Body.String() != v2 {
		t.Fatalf("body=%q, want %q", rec2.Body.String(), v2)
	}
}
