package api

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Static HTML adds hashes for its exact inline scripts in StaticHandler.
const contentSecurityPolicy = "default-src 'self'; " +
	"base-uri 'none'; " +
	"object-src 'none'; " +
	"frame-ancestors 'none'; " +
	"form-action 'self'; " +
	"img-src 'self' data: https:; " +
	"font-src 'self' data:; " +
	"style-src 'self' 'unsafe-inline'; " +
	"script-src 'self'; " +
	"frame-src 'none'; " +
	"connect-src 'self'"

func WithSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "same-origin")
		header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		header.Set("Content-Security-Policy", contentSecurityPolicy)
		if requestIsSecure(r) {
			header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func WithCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"):
			w.Header().Set("Cache-Control", "no-store")
		case strings.HasPrefix(r.URL.Path, "/_next/static/"):
			// 文件名带内容 hash，内容变了必然换名，可以永久缓存。
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			// 页面是静态导出内容，可以进入共享缓存，但每次使用前必须重新
			// 验证；no-transform 同时禁止 Cloudflare 在 HTML 中自动注入 RUM。
			w.Header().Set("Cache-Control", "public, no-cache, no-transform")
		}
		next.ServeHTTP(w, r)
	})
}

func WithCompression(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SSE 需要逐事件立即送达，gzip 的缓冲会让推送滞留在管道里。
		if r.URL.Path == streamPath || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		compressor := &gzipResponseWriter{ResponseWriter: w}
		defer compressor.Close()
		next.ServeHTTP(compressor, r)
	})
}

const streamPath = "/api/stream"

var compressiblePrefixes = []string{
	"text/",
	"application/json",
	"application/javascript",
	"application/manifest+json",
	"application/xml",
	"image/svg+xml",
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	wroteHeader bool
	compress    bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	header := w.Header()
	// 只压缩 200：304 无正文，其余状态码的错误页压缩收益也不值当。
	if status == http.StatusOK && header.Get("Content-Encoding") == "" &&
		isCompressibleType(header.Get("Content-Type")) {
		w.compress = true
		header.Del("Content-Length")
		header.Set("Content-Encoding", "gzip")
		w.writer = gzip.NewWriter(w.ResponseWriter)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.compress {
		return w.writer.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *gzipResponseWriter) Flush() {
	if w.writer != nil {
		_ = w.writer.Flush()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *gzipResponseWriter) Close() {
	if w.writer != nil {
		_ = w.writer.Close()
	}
}

func isCompressibleType(contentType string) bool {
	value := strings.ToLower(strings.TrimSpace(contentType))
	if index := strings.IndexByte(value, ';'); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	for _, prefix := range compressiblePrefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		return qualityValue(params) > 0
	}
	return false
}

func qualityValue(params string) float64 {
	for _, param := range strings.Split(params, ";") {
		key, value, found := strings.Cut(param, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "q") {
			continue
		}
		quality, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0
		}
		return quality
	}
	return 1
}

// MaxRequestBodyBytes is the shared ceiling; handlers retain their smaller limits.
const MaxRequestBodyBytes int64 = 64 << 10

// WithRequestBodyLimit validates before dispatch, including chunked bodies on
// routes that do not read a body (such as SSE). Memory per request is bounded.
func WithRequestBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > MaxRequestBodyBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body is too large"})
			return
		}
		if r.Body != nil && r.Body != http.NoBody {
			limited := http.MaxBytesReader(w, r.Body, MaxRequestBodyBytes)
			defer limited.Close()
			body, err := io.ReadAll(limited)
			if err != nil {
				status := http.StatusBadRequest
				message := "invalid request body"
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					status = http.StatusRequestEntityTooLarge
					message = "request body is too large"
				}
				writeJSON(w, status, map[string]string{"error": message})
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		next.ServeHTTP(w, r)
	})
}
