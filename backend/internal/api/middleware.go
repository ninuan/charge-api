package api

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

// 静态导出无法为内联脚本注入 nonce：next-themes 的防闪烁脚本和 Next 的
// flight 数据都直接内联在导出的 HTML 里，因此 script-src 必须放行 unsafe-inline。
const contentSecurityPolicy = "default-src 'self'; " +
	"base-uri 'none'; " +
	"object-src 'none'; " +
	"frame-ancestors 'none'; " +
	"form-action 'self'; " +
	"img-src 'self' data: https:; " +
	"font-src 'self' data:; " +
	"style-src 'self' 'unsafe-inline'; " +
	"script-src 'self' 'unsafe-inline' https://challenges.cloudflare.com; " +
	"frame-src https://challenges.cloudflare.com; " +
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
			w.Header().Set("Cache-Control", "no-cache")
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
