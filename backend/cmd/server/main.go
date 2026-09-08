package main

import (
	"charge-dashboard/internal/security"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
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
	"charge-dashboard/internal/wxpusher"
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

func wxPusherClientFromEnv(lookup envLookup) (*wxpusher.Client, error) {
	token := strings.TrimSpace(lookup("WXPUSHER_APP_TOKEN"))
	baseURL := strings.TrimSpace(lookup("WXPUSHER_BASE_URL"))
	if token == "" {
		if baseURL != "" {
			return nil, fmt.Errorf("WXPUSHER_APP_TOKEN is required when WXPUSHER_BASE_URL is set")
		}
		return nil, nil
	}
	return wxpusher.NewClient(wxpusher.Config{AppToken: token, BaseURL: baseURL})
}

func publicBaseURLFromEnv(lookup envLookup, required bool) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(lookup("PUBLIC_BASE_URL")), "/")
	if raw == "" {
		if required {
			return "", fmt.Errorf("PUBLIC_BASE_URL is required when WXPUSHER_APP_TOKEN is set")
		}
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return "", fmt.Errorf("PUBLIC_BASE_URL must be an origin without path, query, or credentials")
	}
	if parsed.Scheme != "https" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if parsed.Scheme != "http" || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return "", fmt.Errorf("PUBLIC_BASE_URL must use HTTPS or loopback HTTP")
		}
	}
	return raw, nil
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
		log.Fatalf("resolve database path: %v", security.SanitizeLogText(err.Error(), 1024))
	}
	absLegacyState, err := filepath.Abs(*legacyState)
	if err != nil {
		log.Fatalf("resolve legacy state path: %v", security.SanitizeLogText(err.Error(), 1024))
	}

	requests := parser.DefaultCaptureRequests()
	templateSource := "built-in request template"
	if *captureDir != "" {
		absCaptureDir, err := filepath.Abs(*captureDir)
		if err != nil {
			log.Fatalf("resolve capture dir: %v", security.SanitizeLogText(err.Error(), 1024))
		}
		if _, err := os.Stat(absCaptureDir); err != nil {
			log.Fatalf("capture dir not available: %v", security.SanitizeLogText(err.Error(), 1024))
		}
		requests, err = parser.ParseCaptureRequests(absCaptureDir)
		if err != nil {
			log.Fatalf("parse capture requests: %v", security.SanitizeLogText(err.Error(), 1024))
		}
		templateSource = "configured capture directory"
	}

	password := *adminPassword
	if password == "" {
		password = os.Getenv("CHARGE_ADMIN_PASSWORD")
	}
	yybClient, err := yybClientFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("configure yyb client: %v", security.SanitizeLogText(err.Error(), 1024))
	}
	if yybClient != nil {
		log.Printf("yyb sidecar integration enabled")
	}
	wxPusherClient, err := wxPusherClientFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("configure wxpusher client: %v", security.SanitizeLogText(err.Error(), 1024))
	}
	if wxPusherClient != nil {
		log.Printf("wxpusher integration enabled")
	}
	publicBaseURL, err := publicBaseURLFromEnv(os.Getenv, wxPusherClient != nil)
	if err != nil {
		log.Fatalf("configure public base url: %v", security.SanitizeLogText(err.Error(), 1024))
	}

	cookieKey, err := persistence.DecodeCookieKey(os.Getenv("CHARGE_COOKIE_KEY"))
	if err != nil {
		log.Fatalf("cookie encryption key: %v", security.SanitizeLogText(err.Error(), 1024))
	}
	repository, err := persistence.OpenSQLite(absDatabasePath, cookieKey)
	if err != nil {
		log.Fatalf("open state database: %v", security.SanitizeLogText(err.Error(), 1024))
	}
	defer repository.Close()

	const minRefreshInterval = 30 * time.Second
	manager, err := appruntime.NewManager(repository, absLegacyState, requests, password, minRefreshInterval)
	if err != nil {
		log.Fatalf("create runtime manager: %v", security.SanitizeLogText(err.Error(), 1024))
	}
	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	if err := manager.StartReminderScheduler(schedulerCtx); err != nil {
		log.Fatalf("start reminder scheduler: %v", security.SanitizeLogText(err.Error(), 1024))
	}
	dispatcherStarted := false
	if wxPusherClient != nil {
		if err := manager.StartNotificationDispatcher(schedulerCtx, wxPusherClient, publicBaseURL); err != nil {
			log.Fatalf("start notification dispatcher: %v", security.SanitizeLogText(err.Error(), 1024))
		}
		dispatcherStarted = true
	}
	defer stopScheduler()
	if manager.MigratedLegacyJSON() {
		log.Print("legacy JSON state imported")
	}
	if initialPassword := manager.InitialAdminPassword(); initialPassword != "" {
		// 日志会进 journald 并常被采集外送，初始密码只落一次性的 0600 文件。
		passwordPath := filepath.Join(filepath.Dir(absDatabasePath), "initial-admin-password.txt")
		if err := os.WriteFile(passwordPath, []byte(initialPassword+"\n"), 0o600); err != nil {
			log.Fatalf("write initial admin password: %v", security.SanitizeLogText(err.Error(), 1024))
		}
		log.Print("generated initial admin password; stored in initial-admin-password.txt in the database directory")
	}
	if err := manager.ConfigureInitialPasswordFile(filepath.Join(filepath.Dir(absDatabasePath), "initial-admin-password.txt")); err != nil {
		log.Fatal("initialize password file lifecycle: check the file type and permissions in the database directory")
	}

	sessions := auth.NewPersistentSessionManager(7*24*time.Hour, repository)
	defer sessions.Close()
	server := api.NewServer(manager, sessions, auth.NewAuthGuard())
	if yybClient != nil {
		server.SetYYBIntegration(yybClient, moceleClientFromEnv(os.Getenv))
	}
	if wxPusherClient != nil {
		server.SetWxPusherIntegration(wxPusherClient)
	}
	if devForceAuthExpiredEnabled(os.Getenv) {
		server.EnableDevForceAuthExpired()
		log.Printf("local development: next refresh will simulate an expired login credential")
	}
	mux := http.NewServeMux()
	server.Register(mux)
	mux.Handle("/", api.StaticHandler("../frontend/dist"))
	allowedOrigins := splitCommaSeparated(os.Getenv("CORS_ALLOWED_ORIGINS"))
	rateLimiter := api.NewIPRateLimiter(300, time.Minute)
	content := api.WithCacheHeaders(api.WithCompression(mux))
	handler := api.WithSecurityHeaders(api.WithCORS(rateLimiter.Middleware(api.WithRequestBodyLimit(content)), allowedOrigins))
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
	log.Print("state database opened")
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		stopScheduler()
		waitCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if waitErr := manager.WaitReminderScheduler(waitCtx); waitErr != nil {
			log.Printf("reminder scheduler shutdown failed: %v", security.SanitizeLogText(waitErr.Error(), 1024))
		}
		if dispatcherStarted {
			if waitErr := manager.WaitNotificationDispatcher(waitCtx); waitErr != nil {
				log.Printf("notification dispatcher shutdown failed: %v", security.SanitizeLogText(waitErr.Error(), 1024))
			}
		}
		cancel()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server stopped: %v", security.SanitizeLogText(err.Error(), 1024))
		}
	case sig := <-signals:
		log.Printf("received %s, shutting down", sig)
		stopScheduler()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", security.SanitizeLogText(err.Error(), 1024))
		}
		if err := <-errCh; err != nil && err != http.ErrServerClosed {
			log.Printf("server stopped: %v", security.SanitizeLogText(err.Error(), 1024))
		}
		if err := manager.WaitReminderScheduler(shutdownCtx); err != nil {
			log.Printf("reminder scheduler shutdown failed: %v", security.SanitizeLogText(err.Error(), 1024))
		}
		if dispatcherStarted {
			if err := manager.WaitNotificationDispatcher(shutdownCtx); err != nil {
				log.Printf("notification dispatcher shutdown failed: %v", security.SanitizeLogText(err.Error(), 1024))
			}
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
