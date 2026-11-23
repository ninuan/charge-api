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

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean("/" + r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/") {
			name = path.Join(name, "index.html")
		}
		if !strings.HasSuffix(name, ".html") {
			file, err := root.Open(name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			info, statErr := file.Stat()
			_ = file.Close()
			if statErr != nil {
				http.NotFound(w, r)
				return
			}
			if info.IsDir() {
				// Next.js static export (trailingSlash: true) exports routes as
				// <dir>/index.html. Allow directories with index.html while blocking
				// directory listings on asset folders like /_next/static.
				indexFile, indexErr := root.Open(path.Join(name, "index.html"))
				if indexErr != nil {
					http.NotFound(w, r)
					return
				}
				indexInfo, indexStatErr := indexFile.Stat()
				_ = indexFile.Close()
				if indexStatErr != nil || !indexInfo.Mode().IsRegular() {
					http.NotFound(w, r)
					return
				}
			}
			files.ServeHTTP(w, r)
			return
		}
		file, err := root.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		cacheMu.RLock()
		entry, hit := htmlCache[name]
		cacheMu.RUnlock()
		if !hit || !entry.modTime.Equal(info.ModTime()) || entry.size != info.Size() {
			body, readErr := security.ReadLimited(file, 4<<20)
			if readErr != nil {
				http.Error(w, "static page unavailable", http.StatusInternalServerError)
				return
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
