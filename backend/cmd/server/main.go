package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"charge-dashboard/internal/api"
	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	appruntime "charge-dashboard/internal/runtime"
)

func main() {
	var (
		listenAddr    = flag.String("listen", ":8080", "http listen address")
		captureDir    = flag.String("capture", "", "optional capture directory; built-in request template is used when empty")
		databasePath  = flag.String("database", "../charge_state.db", "SQLite database path")
		legacyState   = flag.String("state", "../charge_state.json", "legacy JSON state file imported when the database is empty")
		adminPassword = flag.String("admin-password", "", "initial admin password, falls back to CHARGE_ADMIN_PASSWORD")
	)
	flag.Parse()

	absDatabasePath, err := filepath.Abs(*databasePath)
	if err != nil {
		log.Fatalf("resolve database path: %v", err)
	}
	absLegacyState, err := filepath.Abs(*legacyState)
	if err != nil {
		log.Fatalf("resolve legacy state path: %v", err)
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

	cookieKey, err := persistence.DecodeCookieKey(os.Getenv("CHARGE_COOKIE_KEY"))
	if err != nil {
		log.Fatalf("cookie encryption key: %v", err)
	}
	repository, err := persistence.OpenSQLite(absDatabasePath, cookieKey)
	if err != nil {
		log.Fatalf("open state database: %v", err)
	}
	defer repository.Close()

	const minRefreshInterval = 30 * time.Second
	manager, err := appruntime.NewManager(repository, absLegacyState, requests, password, minRefreshInterval)
	if err != nil {
		log.Fatalf("create runtime manager: %v", err)
	}
	if manager.MigratedLegacyJSON() {
		log.Printf("legacy JSON state imported from %s", absLegacyState)
	}
	if manager.InitialAdminPassword() != "" {
		log.Printf("generated initial admin password for admin: %s", manager.InitialAdminPassword())
	}

	sessions := auth.NewPersistentSessionManager(7*24*time.Hour, repository)
	defer sessions.Close()
	server := api.NewServer(manager, sessions, turnstile, auth.NewAuthGuard())
	mux := http.NewServeMux()
	server.Register(mux)
	mux.Handle("/", http.FileServer(http.Dir("../frontend/dist")))
	allowedOrigins := splitCommaSeparated(os.Getenv("CORS_ALLOWED_ORIGINS"))
	rateLimiter := api.NewIPRateLimiter(300, time.Minute)
	handler := api.WithCORS(rateLimiter.Middleware(mux), allowedOrigins)
	httpServer := &http.Server{
		Addr:              *listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}

	log.Printf("server listening on %s", *listenAddr)
	log.Printf("request template loaded from %s", templateSource)
	log.Printf("state database: %s", absDatabasePath)
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server stopped: %v", err)
		}
	case sig := <-signals:
		log.Printf("received %s, shutting down", sig)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
		if err := <-errCh; err != nil && err != http.ErrServerClosed {
			log.Printf("server stopped: %v", err)
		}
	}
}

func splitCommaSeparated(raw string) []string {
	var values []string
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}
