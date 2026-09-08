package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"path"
	"regexp"
	"strings"

	"charge-dashboard/internal/security"
)

var inlineScriptPattern = regexp.MustCompile(`(?is)<script\b[^>]*>(.*?)</script\s*>`)

// StaticHandler hashes the same bytes it serves, so a build replacement never
// combines new HTML with a stale policy. Only trusted exported HTML is hashed.
func StaticHandler(directory string) http.Handler {
	root := http.Dir(directory)
	files := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean("/" + r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/") {
			name = path.Join(name, "index.html")
		}
		if !strings.HasSuffix(name, ".html") {
			files.ServeHTTP(w, r)
			return
		}
		file, err := root.Open(name)
		if err != nil {
			files.ServeHTTP(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		body, err := security.ReadLimited(file, 4<<20)
		if err != nil {
			http.Error(w, "static page unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Security-Policy", staticPolicy(body))
		http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(body))
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
