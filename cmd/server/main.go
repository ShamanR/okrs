package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"okrs/app"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/platform/entitlements"
	"okrs/internal/store"
	"okrs/internal/store/periods"

	// Register OAuth2 providers via side-effect imports.
	_ "okrs/internal/auth/providers/github"
	_ "okrs/internal/auth/providers/google"
	_ "okrs/internal/auth/providers/keycloak"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// main only translates run's exit code: os.Exit bypasses every deferred call in the
// function it is invoked from, so all cleanup (pool.Close, the graceful-shutdown
// sequence) has to live inside run and finish before main ever calls os.Exit.
func main() {
	os.Exit(run())
}

func run() int {
	var seed bool
	flag.BoolVar(&seed, "seed", false, "seed demo data")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	port := envOrDefault("PORT", "8080")
	zoneName := envOrDefault("TZ", "Asia/Bangkok")
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		logger.Error("invalid timezone", slog.String("tz", zoneName))
		return 1
	}

	databaseURL := envOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable")

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		logger.Error("failed to connect db", slog.String("error", err.Error()))
		return 1
	}
	defer pool.Close()

	if err := runMigrations(databaseURL); err != nil {
		logger.Error("failed to migrate", slog.String("error", err.Error()))
		return 1
	}

	pgstore := store.New(pool)
	if seed {
		now := time.Now().In(zone)
		seedScope := domain.TenantScope{TenantID: 1}
		period, err := pgstore.Periods.FindPeriodForDate(context.Background(), seedScope, now)
		var periodID int64
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				name, startDate, endDate := quarterPeriod(now)
				periodID, err = pgstore.Periods.CreatePeriod(context.Background(), seedScope, periods.PeriodInput{
					Name:      name,
					StartDate: startDate,
					EndDate:   endDate,
				})
				if err != nil {
					logger.Error("failed to create seed period", slog.String("error", err.Error()))
					return 1
				}
			} else {
				logger.Error("failed to resolve seed period", slog.String("error", err.Error()))
				return 1
			}
		} else {
			periodID = period.ID
		}
		if err := pgstore.SeedDemo(context.Background(), periodID); err != nil {
			logger.Error("failed to seed", slog.String("error", err.Error()))
			return 1
		}
		logger.Info("seed data created")
	}

	// OSS feature-gating: every feature is on, every limit is unlimited. A SaaS
	// build registers a billing-backed implementation under a different name.
	entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })

	authCfg := loadAuthConfig()
	logger.Info("auth mode", slog.String("mode", string(authCfg.Mode)),
		slog.Any("providers", authCfg.EnabledProviders))

	// Assemble the box via the public façade on OSS defaults (session resolver, unlimited
	// entitlements, stub no-membership, no control-plane mounts).
	a, err := app.New(app.Config{
		Pool:      pgstore.DB,
		Logger:    logger,
		Zone:      zone,
		Auth:      authCfg,
		AssetsDev: envBool("WEB_ASSETS_DEV"),
	})
	if err != nil {
		logger.Error("failed to assemble app", slog.String("error", err.Error()))
		return 1
	}

	// Graceful shutdown: without catching the signal, http.ListenAndServe either
	// blocks forever or returns straight to os.Exit(1) in main, which skips every
	// defer — including pool.Close() above and a.Close() below. That was harmless in
	// phase 1a (the only subscriber was synchronous), but phase 1b added an async
	// notifications subscriber with a buffer: events still queued at SIGTERM would be
	// lost silently without this.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// serveErr is set only on a genuine listen/serve failure (port in use, bad
	// address, ...), never on a clean shutdown (http.ErrServerClosed). It is written
	// once, from the goroutine below, strictly before that same goroutine calls
	// stop() — and stop() closing ctx.Done() is what unblocks the read below, so the
	// write happens-before the read without any extra synchronization.
	var serveErr error

	addr := fmt.Sprintf(":%s", port)
	srv := &http.Server{Addr: addr, Handler: a.Handler}
	go func() {
		logger.Info("listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", slog.String("error", err.Error()))
			serveErr = err
			stop()
		}
	}()
	<-ctx.Done()

	// Order matters: stop accepting requests, then drain the event bus (so the async
	// notifications subscriber gets to run against a live pool), and only then close
	// the pool via the deferred pool.Close() above. Reversing this loses events to a
	// closed pool. This ordering holds regardless of why we got here — a signal or a
	// failed listener — so a failed bind still exits cleanly, just with exit code 1.
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown did not complete cleanly", slog.String("error", err.Error()))
	}
	if err := a.Close(5 * time.Second); err != nil {
		logger.Warn("event bus did not drain cleanly", slog.String("error", err.Error()))
	}

	if serveErr != nil {
		// pool.Close() still runs: it is deferred above, and a deferred call fires
		// when run() returns, before main() ever sees this exit code.
		return 1
	}
	return 0
}

func loadAuthConfig() auth.Config {
	cfg := auth.DefaultConfig()

	if mode := os.Getenv("AUTH_MODE"); mode != "" {
		cfg.Mode = auth.Mode(mode)
	}
	if providers := os.Getenv("AUTH_ENABLED_PROVIDERS"); providers != "" {
		cfg.EnabledProviders = splitTrimmed(providers, ",")
	}
	if cookie := os.Getenv("AUTH_SESSION_COOKIE_NAME"); cookie != "" {
		cfg.SessionCookie = cookie
	}
	if ttl := os.Getenv("AUTH_SESSION_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil {
			cfg.SessionTTL = d
		}
	}
	if base := os.Getenv("AUTH_BASE_URL"); base != "" {
		cfg.BaseURL = base
	}
	if policy := os.Getenv("AUTH_DEFAULT_NEW_USER_POLICY"); policy != "" {
		cfg.NewUserPolicy = auth.NewUserPolicy(policy)
	}
	if nodeID := os.Getenv("AUTH_DEFAULT_NODE_ID"); nodeID != "" {
		if id, err := strconv.ParseInt(nodeID, 10, 64); err == nil {
			cfg.DefaultNodeID = id
		}
	}
	cfg.ProvisioningToken = os.Getenv("PROVISIONING_TOKEN")
	cfg.BootstrapSystemAdmin = os.Getenv("BOOTSTRAP_SYSTEM_ADMIN")

	// Google
	cfg.Google.ClientID = os.Getenv("AUTH_GOOGLE_CLIENT_ID")
	cfg.Google.ClientSecret = os.Getenv("AUTH_GOOGLE_CLIENT_SECRET")
	cfg.Google.RedirectURL = os.Getenv("AUTH_GOOGLE_REDIRECT_URL")

	// GitHub
	cfg.GitHub.ClientID = os.Getenv("AUTH_GITHUB_CLIENT_ID")
	cfg.GitHub.ClientSecret = os.Getenv("AUTH_GITHUB_CLIENT_SECRET")
	cfg.GitHub.RedirectURL = os.Getenv("AUTH_GITHUB_REDIRECT_URL")

	// Keycloak
	cfg.Keycloak.IssuerURL = os.Getenv("AUTH_KEYCLOAK_ISSUER_URL")
	cfg.Keycloak.ClientID = os.Getenv("AUTH_KEYCLOAK_CLIENT_ID")
	cfg.Keycloak.ClientSecret = os.Getenv("AUTH_KEYCLOAK_CLIENT_SECRET")
	cfg.Keycloak.RedirectURL = os.Getenv("AUTH_KEYCLOAK_REDIRECT_URL")

	return cfg
}

func splitTrimmed(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func envOrDefault(key, def string) string {
	value := os.Getenv(key)
	if value == "" {
		return def
	}
	return value
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func runMigrations(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	migrationsPath, err := resolveMigrationsPath()
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func resolveMigrationsPath() (string, error) {
	baseDir, err := os.Getwd()
	if err != nil {
		executable, execErr := os.Executable()
		if execErr != nil {
			return "", err
		}
		baseDir = filepath.Dir(executable)
	}
	absPath, err := filepath.Abs(filepath.Join(baseDir, "migrations"))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(absPath), nil
}

func quarterPeriod(now time.Time) (string, time.Time, time.Time) {
	year := now.Year()
	quarter := ((int(now.Month()) - 1) / 3) + 1
	startMonth := time.Month((quarter-1)*3 + 1)
	start := time.Date(year, startMonth, 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 3, -1)
	return fmt.Sprintf("%d Q%d", year, quarter), start, end
}
