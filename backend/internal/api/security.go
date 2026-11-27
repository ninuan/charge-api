package api

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxRateLimiterEntries = 10_000

type IPRateLimiter struct {
	mu          sync.Mutex
	limit       int
	window      time.Duration
	entries     map[string]rateEntry
	lastCleanup time.Time
}

type rateEntry struct {
	count       int
	windowStart time.Time
}

func NewIPRateLimiter(limit int, window time.Duration) *IPRateLimiter {
	return &IPRateLimiter{
		limit:       limit,
		window:      window,
		entries:     make(map[string]rateEntry),
		lastCleanup: time.Now(),
	}
}

func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		allowed, retryAfter := l.allow(clientIP(r), time.Now())
		if !allowed {
			writeRateLimit(w, retryAfter, "API 请求过于频繁，请稍后再试")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *IPRateLimiter) allow(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastCleanup) >= 10*time.Minute {
		for key, entry := range l.entries {
			if now.Sub(entry.windowStart) >= l.window {
				delete(l.entries, key)
			}
		}
		l.lastCleanup = now
	}

	entry, exists := l.entries[ip]
	if !exists && len(l.entries) >= maxRateLimiterEntries {
		for key, item := range l.entries {
			if now.Sub(item.windowStart) >= l.window {
				delete(l.entries, key)
			}
		}
		l.lastCleanup = now
		if len(l.entries) >= maxRateLimiterEntries {
			var oldestKey string
			var oldestStart time.Time
			for key, item := range l.entries {
				if oldestKey == "" || item.windowStart.Before(oldestStart) {
					oldestKey = key
					oldestStart = item.windowStart
				}
			}
			if oldestKey != "" {
				delete(l.entries, oldestKey)
			}
		}
	}
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= l.window {
		entry = rateEntry{windowStart: now}
	}
	entry.count++
	l.entries[ip] = entry
	if entry.count > l.limit {
		return false, entry.windowStart.Add(l.window).Sub(now)
	}
	return true, 0
}

func WithCORS(next http.Handler, allowedOrigins []string) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if normalized := normalizeOrigin(origin); normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(allowed) > 0 {
			w.Header().Add("Vary", "Origin")
		}
		rawOrigin := strings.TrimSpace(r.Header.Get("Origin"))
		origin := normalizeOrigin(rawOrigin)
		if rawOrigin != "" && origin == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin is not allowed"})
			return
		}
		_, explicitlyAllowed := allowed[origin]
		if isUnsafeMethod(r.Method) {
			fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
			if fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
				if origin == "" || !explicitlyAllowed {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site request is not allowed"})
					return
				}
			}
			if origin == "" {
				if rawReferer := strings.TrimSpace(r.Header.Get("Referer")); rawReferer != "" {
					refOrigin := normalizeRefererOrigin(rawReferer)
					_, refAllowed := allowed[refOrigin]
					if refOrigin == "" || (!sameOrigin(r, refOrigin) && !refAllowed) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin is not allowed"})
						return
					}
				}
			}
		}
		if origin == "" {
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if !sameOrigin(r, origin) && !explicitlyAllowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin is not allowed"})
			return
		}
		if explicitlyAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func normalizeRefererOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func normalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		(parsed.Path != "" && parsed.Path != "/") ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func sameOrigin(r *http.Request, origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	scheme := "http"
	if requestIsSecure(r) {
		scheme = "https"
	}
	return strings.EqualFold(parsed.Scheme, scheme) && strings.EqualFold(parsed.Host, r.Host)
}
