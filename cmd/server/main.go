package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
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
	"okrs/internal/platform/logging"
	"okrs/internal/store"
	"okrs/internal/store/periods"
	"okrs/notifychannel"
	"okrs/notifychannel/mattermost"

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

// Разбор флагов живёт здесь, а не в runWith: flag.BoolVar регистрирует флаг в
// глобальном наборе процесса, и повторный вызов паникует — тест вызывает runWith
// несколько раз.
func run() int {
	var seed bool
	flag.BoolVar(&seed, "seed", false, "seed demo data")
	flag.Parse()
	return runWith(os.Stdout, seed)
}

// runWith принимает поток вывода логов параметром, чтобы тест мог наблюдать записи
// жизненного цикла: в остальном это и есть run.
func runWith(logOutput io.Writer, seed bool) int {
	// Конфигурация логирования читается только здесь, в composition root:
	// app.New намеренно не трогает окружение, иначе его нельзя было бы собрать
	// в тесте без переменных среды.
	logger := logging.New(logging.Config{
		Level:   os.Getenv("LOG_LEVEL"),
		Format:  os.Getenv("LOG_FORMAT"),
		Service: envOrDefault("SERVICE_NAME", logging.DefaultService),
		Env:     envOrDefault("ENV", logging.DefaultEnv),
		Output:  logOutput,
	})
	logger.Info("starting", slog.String(logging.KeyEvent, logging.EventAppStart))

	port := envOrDefault("PORT", "8080")
	zoneName := envOrDefault("TZ", "Asia/Bangkok")
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		logger.Error("invalid timezone",
			slog.String(logging.KeyEvent, logging.EventAppStart),
			slog.String("tz", zoneName),
			slog.String("err", err.Error()))
		return 1
	}

	databaseURL := envOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable")

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		logger.Error("failed to connect db",
			slog.String(logging.KeyEvent, logging.EventAppStart),
			slog.String("err", err.Error()))
		return 1
	}
	defer pool.Close()

	if err := runMigrations(databaseURL); err != nil {
		logger.Error("failed to migrate",
			slog.String(logging.KeyEvent, logging.EventMigration),
			slog.String("err", err.Error()))
		return 1
	}
	logger.Info("migrations applied", slog.String(logging.KeyEvent, logging.EventMigration))

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
					logger.Error("failed to create seed period",
						slog.String(logging.KeyEvent, logging.EventAppStart),
						slog.String("err", err.Error()))
					return 1
				}
			} else {
				logger.Error("failed to resolve seed period",
					slog.String(logging.KeyEvent, logging.EventAppStart),
					slog.String("err", err.Error()))
				return 1
			}
		} else {
			periodID = period.ID
		}
		if err := pgstore.SeedDemo(context.Background(), periodID); err != nil {
			logger.Error("failed to seed",
				slog.String(logging.KeyEvent, logging.EventAppStart),
				slog.String("err", err.Error()))
			return 1
		}
		logger.Info("seed data created", slog.String(logging.KeyEvent, logging.EventAppStart))
	}

	// OSS feature-gating: every feature is on, every limit is unlimited. A SaaS
	// build registers a billing-backed implementation under a different name.
	entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })

	authCfg := loadAuthConfig()
	logger.Info("auth mode",
		slog.String(logging.KeyEvent, logging.EventAppStart),
		slog.String("mode", string(authCfg.Mode)),
		slog.Any("providers", authCfg.EnabledProviders))

	// Assemble the box via the public façade on OSS defaults (session resolver, unlimited
	// entitlements, stub no-membership, no control-plane mounts).
	a, err := app.New(app.Config{
		Pool:      pgstore.DB,
		Logger:    logger,
		Zone:      zone,
		Auth:      authCfg,
		AssetsDev: envBool("WEB_ASSETS_DEV"),
		// Channels are assembled here, next to main, and not registered from a package
		// init: that is the point of the seam — another build can swap this list for its
		// own without the application knowing the channel's package exists.
		NotificationChannels:  []notifychannel.Channel{mattermost.Channel()},
		NotificationSecretKey: os.Getenv("NOTIFICATIONS_SECRET_KEY"),
	})
	if err != nil {
		logger.Error("failed to assemble app",
			slog.String(logging.KeyEvent, logging.EventAppStart),
			slog.String("err", err.Error()))
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
		logger.Info("listening",
			slog.String(logging.KeyEvent, logging.EventAppReady),
			slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed",
				slog.String(logging.KeyEvent, logging.EventAppStart),
				slog.String("addr", addr),
				slog.String("err", err.Error()))
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
	shutdownSequence(logger, srv.Shutdown, a.Close)

	if serveErr != nil {
		// pool.Close() still runs: it is deferred above, and a deferred call fires
		// when run() returns, before main() ever sees this exit code.
		logger.Error("shutdown complete after serve failure",
			slog.String(logging.KeyEvent, logging.EventAppShutdown),
			slog.String("err", serveErr.Error()))
		return 1
	}
	logger.Info("shutdown complete", slog.String(logging.KeyEvent, logging.EventAppShutdown))
	return 0
}

const (
	httpShutdownTimeout = 15 * time.Second
	busDrainTimeout     = 5 * time.Second
)

// shutdownSequence останавливает приём запросов, затем дренит event bus, логируя
// исход каждого шага.
//
// Вынесено из runWith отдельной функцией с внедрёнными шагами, потому что иначе
// последовательность остановки проверялась бы только живым запуском: тест передаёт
// сюда заглушки и наблюдает записи, которых требует спецификация.
func shutdownSequence(logger *slog.Logger, stopServing func(context.Context) error, drainBus func(time.Duration) error) {
	logger.Info("shutdown signal received", slog.String(logging.KeyEvent, logging.EventAppShutdown))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := stopServing(shutdownCtx); err != nil {
		logger.Warn("http shutdown did not complete cleanly",
			slog.String(logging.KeyEvent, logging.EventAppShutdown),
			slog.String("err", err.Error()))
	} else {
		logger.Info("http server stopped serving", slog.String(logging.KeyEvent, logging.EventAppShutdown))
	}

	if err := drainBus(busDrainTimeout); err != nil {
		logger.Warn("event bus did not drain cleanly",
			slog.String(logging.KeyEvent, logging.EventAppShutdown),
			slog.String("err", err.Error()))
	} else {
		logger.Info("event bus drained", slog.String(logging.KeyEvent, logging.EventAppShutdown))
	}
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
