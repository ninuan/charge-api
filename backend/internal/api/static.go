package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"charge-dashboard/internal/security"
)

var inlineScriptPattern = regexp.MustCompile(`(?is)<script\b[^>]*>(.*?)</script\s*>`)

type cachedHTML struct {
	modTime time.Time
	size    int64
	body    []byte
	csp     string
	etag    string
}

// StaticHandler hashes the same bytes it serves, so a build replacement never
// combines new HTML with a stale policy. Only trusted exported HTML is hashed.
func StaticHandler(directory string) http.Handler {
	root := http.Dir(directory)
	files := http.FileServer(root)
	var cacheMu sync.RWMutex
	htmlCache := make(map[string]cachedHTML)

	loadHTML := func(name string) (cachedHTML, bool, error) {
		file, err := root.Open(name)
		if err != nil {
			return cachedHTML{}, false, nil
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return cachedHTML{}, false, nil
		}
		cacheMu.RLock()
		entry, hit := htmlCache[name]
		cacheMu.RUnlock()
		if !hit || !entry.modTime.Equal(info.ModTime()) || entry.size != info.Size() {
			body, readErr := security.ReadLimited(file, 4<<20)
			if readErr != nil {
				return cachedHTML{}, false, readErr
			}
			sum := sha256.Sum256(body)
			entry = cachedHTML{
				modTime: info.ModTime(),
				size:    info.Size(),
				body:    body,
				csp:     staticPolicy(body),
				etag:    `W/"` + hex.EncodeToString(sum[:12]) + `"`,
			}
			cacheMu.Lock()
			htmlCache[name] = entry
			cacheMu.Unlock()
		}
		return entry, true, nil
	}

	serveNotFound := func(w http.ResponseWriter, r *http.Request, name string) {
		// Never allow a 404 response (for example under /_next/static/) to inherit
		// long-lived immutable cache headers.
		w.Header().Set("Cache-Control", "no-store")
		// Only render the exported 404.html page for frontend HTML/route requests,
		// keeping unknown /api/ paths and missing static assets as plain 404s.
		isNonPagePrefix := name == "/api" || strings.HasPrefix(name, "/api/") ||
			name == "/_next" || strings.HasPrefix(name, "/_next/")
		isRouteRequest := !isNonPagePrefix &&
			(strings.HasSuffix(name, ".html") || !strings.Contains(path.Base(name), "."))
		if isRouteRequest {
			if entry, ok, err := loadHTML("/404.html"); err == nil && ok {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Content-Security-Policy", entry.csp)
				w.WriteHeader(http.StatusNotFound)
				if r.Method != http.MethodHead {
					_, _ = w.Write(entry.body)
				}
				return
			}
		}
		http.NotFound(w, r)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean("/" + r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/") {
			name = path.Join(name, "index.html")
		}
		if !strings.HasSuffix(name, ".html") {
			file, err := root.Open(name)
			if err != nil {
				serveNotFound(w, r, name)
				return
			}
			info, statErr := file.Stat()
			_ = file.Close()
			if statErr != nil {
				serveNotFound(w, r, name)
				return
			}
			if info.IsDir() {
				// Next.js static export (trailingSlash: true) exports routes as
				// <dir>/index.html. Allow directories with index.html while blocking
				// directory listings on asset folders like /_next/static.
				indexFile, indexErr := root.Open(path.Join(name, "index.html"))
				if indexErr != nil {
					serveNotFound(w, r, name)
					return
				}
				indexInfo, indexStatErr := indexFile.Stat()
				_ = indexFile.Close()
				if indexStatErr != nil || !indexInfo.Mode().IsRegular() {
					serveNotFound(w, r, name)
					return
				}
			}
			files.ServeHTTP(w, r)
			return
		}
		entry, ok, err := loadHTML(name)
		if err != nil {
			http.Error(w, "static page unavailable", http.StatusInternalServerError)
			return
		}
		if !ok {
			serveNotFound(w, r, name)
			return
		}
		w.Header().Set("Content-Security-Policy", entry.csp)
		w.Header().Set("ETag", entry.etag)
		http.ServeContent(w, r, name, entry.modTime, bytes.NewReader(entry.body))
	})
}

func staticPolicy(body []byte) string {
	sources := []string{"'self'"}
	seen := map[string]bool{}
	for _, match := range inlineScriptPattern.FindAllSubmatch(body, -1) {
		digest := sha256.Sum256(match[1])
		hash := "'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
		if !seen[hash] {
			sources = append(sources, hash)
			seen[hash] = true
		}
	}
	return strings.Replace(contentSecurityPolicy, "script-src 'self'", "script-src "+strings.Join(sources, " "), 1)
}
