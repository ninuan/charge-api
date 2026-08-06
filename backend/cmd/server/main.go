package main

import (
	"context"
	"flag"
	"fmt"
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
	"charge-dashboard/internal/mocele"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	appruntime "charge-dashboard/internal/runtime"
	"charge-dashboard/internal/version"
	"charge-dashboard/internal/yyb"
)

type envLookup func(string) string

func yybClientFromEnv(lookup envLookup) (*yyb.Client, error) {
	baseURL := strings.TrimSpace(lookup("YYB_BASE_URL"))
	if baseURL == "" {
		return nil, nil
	}
	secret := strings.TrimSpace(lookup("YYB_API_SECRET"))
	if secret == "" {
		return nil, fmt.Errorf("YYB_API_SECRET is required when YYB_BASE_URL is set")
	}
	return yyb.NewClient(yyb.Config{BaseURL: baseURL, APISecret: []byte(secret)})
}

func moceleClientFromEnv(lookup envLookup) *mocele.Client {
	return mocele.NewClient(mocele.Config{
		BaseURL:   strings.TrimSpace(lookup("MOCELE_BASE_URL")),
		Org:       strings.TrimSpace(lookup("MOCELE_ORG")),
		OpenIndex: strings.TrimSpace(lookup("MOCELE_OPENINDEX")),
	})
}

func devForceAuthExpiredEnabled(lookup envLookup) bool {
	return lookup("CHARGE_LOCAL_DEV") == "1" && strings.EqualFold(strings.TrimSpace(lookup("CHARGE_DEV_FORCE_AUTH_EXPIRED")), "true")
}

func main() {
	var (
		// 默认只监听回环：后端不带 TLS，手工启动时不该裸露到公网；
		// 需要对外时用 -listen 显式指定（部署文档即如此）。
		listenAddr = flag.String("listen", "127.0.0.1:8080", "http listen address")
		captureDir = flag.String("capture", "", "optional capture directory; built-in request template is used when empty")
		// 默认状态与 dev.sh/make dev 同源（.local/ 已忽略提交，由 make reset-local 管理），
		// 避免手工裸跑在仓库根散落状态文件；生产环境仍由 systemd 显式传参。
		databasePath  = flag.String("database", "../.local/charge_state.db", "SQLite database path")
		legacyState   = flag.String("state", "../.local/charge_state.json", "legacy JSON state file imported when the database is empty")
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

	yybClient, err := yybClientFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("configure yyb client: %v", err)
	}
	if yybClient != nil {
		log.Printf("yyb sidecar integration enabled")
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
	if initialPassword := manager.InitialAdminPassword(); initialPassword != "" {
		// 日志会进 journald 并常被采集外送，初始密码只落一次性的 0600 文件。
		passwordPath := filepath.Join(filepath.Dir(absDatabasePath), "initial-admin-password.txt")
		if err := os.WriteFile(passwordPath, []byte(initialPassword+"\n"), 0o600); err != nil {
			log.Fatalf("write initial admin password to %s: %v", passwordPath, err)
		}
		log.Printf("generated initial admin password for admin, saved to %s (delete it after first login)", passwordPath)
	}

	sessions := auth.NewPersistentSessionManager(7*24*time.Hour, repository)
	defer sessions.Close()
	server := api.NewServer(manager, sessions, turnstile, auth.NewAuthGuard())
	if yybClient != nil {
		server.SetYYBIntegration(yybClient, moceleClientFromEnv(os.Getenv))
	}
	if devForceAuthExpiredEnabled(os.Getenv) {
		server.EnableDevForceAuthExpired()
		log.Printf("local development: next refresh will simulate an expired login credential")
	}
	mux := http.NewServeMux()
	server.Register(mux)
	mux.Handle("/", http.FileServer(http.Dir("../frontend/dist")))
	allowedOrigins := splitCommaSeparated(os.Getenv("CORS_ALLOWED_ORIGINS"))
	rateLimiter := api.NewIPRateLimiter(300, time.Minute)
	content := api.WithCacheHeaders(api.WithCompression(mux))
	handler := api.WithSecurityHeaders(api.WithCORS(rateLimiter.Middleware(content), allowedOrigins))
	httpServer := &http.Server{
		Addr:              *listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}

	log.Printf("Charge Console %s listening on %s", version.Current, *listenAddr)
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
