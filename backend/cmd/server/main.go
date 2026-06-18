package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charge-dashboard/internal/api"
	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/parser"
	appruntime "charge-dashboard/internal/runtime"
)

func main() {
	var (
		listenAddr    = flag.String("listen", ":8080", "http listen address")
		captureDir    = flag.String("capture", "", "optional capture directory; built-in request template is used when empty")
		statePath     = flag.String("state", "../charge_state.json", "local persisted state file")
		adminPassword = flag.String("admin-password", "", "initial admin password, falls back to CHARGE_ADMIN_PASSWORD")
	)
	flag.Parse()

	absStatePath, err := filepath.Abs(*statePath)
	if err != nil {
		log.Fatalf("resolve state path: %v", err)
	}

	requests := parser.DefaultCaptureRequests()
	templateSource := "built-in request template"
	if *captureDir != "" {
		absCaptureDir, err := filepath.Abs(*captureDir)
		if err != nil {
			log.Fatalf("resolve capture dir: %v", err)
		}
		if _, err := os.Stat(absCaptureDir); err != nil {
			log.Fatalf("capture dir not available: %v", err)
		}
		requests, err = parser.ParseCaptureRequests(absCaptureDir)
		if err != nil {
			log.Fatalf("parse capture requests: %v", err)
		}
		templateSource = absCaptureDir
	}

	password := *adminPassword
	if password == "" {
		password = os.Getenv("CHARGE_ADMIN_PASSWORD")
	}
	turnstileSiteKey := os.Getenv("TURNSTILE_SITE_KEY")
	turnstileSecretKey := os.Getenv("TURNSTILE_SECRET_KEY")
	if (turnstileSiteKey == "") != (turnstileSecretKey == "") {
		log.Fatalf("TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY must be configured together")
	}
	turnstile := auth.NewTurnstileVerifier(
		turnstileSiteKey,
		turnstileSecretKey,
		os.Getenv("TURNSTILE_HOSTNAME"),
	)
	turnstileRequired := strings.EqualFold(os.Getenv("TURNSTILE_REQUIRED"), "true")
	if turnstileRequired && !turnstile.Enabled() {
		log.Fatalf("Turnstile is required but TURNSTILE_SITE_KEY or TURNSTILE_SECRET_KEY is missing")
	}
	if !turnstile.Enabled() {
		log.Printf("warning: Turnstile is disabled; configure TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY in production")
	}

	const minRefreshInterval = 30 * time.Second
	manager, err := appruntime.NewManager(absStatePath, requests, password, minRefreshInterval)
	if err != nil {
		log.Fatalf("create runtime manager: %v", err)
	}
	if manager.InitialAdminPassword() != "" {
		log.Printf("generated initial admin password for admin: %s", manager.InitialAdminPassword())
	}

	sessions := auth.NewSessionManager(24 * time.Hour)
	server := api.NewServer(manager, sessions, turnstile, auth.NewAuthGuard())
	mux := http.NewServeMux()
	server.Register(mux)
	mux.Handle("/", http.FileServer(http.Dir("../frontend/dist")))

	log.Printf("server listening on %s", *listenAddr)
	log.Printf("request template loaded from %s", templateSource)
	log.Printf("state file: %s", absStatePath)
	if err := http.ListenAndServe(*listenAddr, withCORS(mux)); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
