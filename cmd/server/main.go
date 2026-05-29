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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"okrs/internal/auth"
	httpserver "okrs/internal/http"
	"okrs/internal/store"
	"okrs/internal/store/grants"
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

func main() {
	var seed bool
	flag.BoolVar(&seed, "seed", false, "seed demo data")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	port := envOrDefault("PORT", "8080")
	zoneName := envOrDefault("TZ", "Asia/Bangkok")
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		logger.Error("invalid timezone", slog.String("tz", zoneName))
		os.Exit(1)
	}

	databaseURL := envOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/okrs?sslmode=disable")

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		logger.Error("failed to connect db", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	if err := runMigrations(databaseURL); err != nil {
		logger.Error("failed to migrate", slog.String("error", err.Error()))
		os.Exit(1)
	}

	pgstore := store.New(pool)
	if seed {
		now := time.Now().In(zone)
		period, err := pgstore.Periods.FindPeriodForDate(context.Background(), now)
		var periodID int64
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				name, startDate, endDate := quarterPeriod(now)
				periodID, err = pgstore.Periods.CreatePeriod(context.Background(), periods.PeriodInput{
					Name:      name,
					StartDate: startDate,
					EndDate:   endDate,
				})
				if err != nil {
					logger.Error("failed to create seed period", slog.String("error", err.Error()))
					os.Exit(1)
				}
			} else {
				logger.Error("failed to resolve seed period", slog.String("error", err.Error()))
				os.Exit(1)
			}
		} else {
			periodID = period.ID
		}
		if err := pgstore.SeedDemo(context.Background(), periodID); err != nil {
			logger.Error("failed to seed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("seed data created")
	}

	grantsCache := grants.NewGrantsCache(pgstore.Grants)

	authCfg := loadAuthConfig()
	authMgr, err := auth.NewManager(authCfg, pgstore, grantsCache)
	if err != nil {
		logger.Error("failed to init auth", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("auth mode", slog.String("mode", string(authCfg.Mode)),
		slog.Any("providers", authCfg.EnabledProviders))

	server, err := httpserver.NewServer(pgstore, grantsCache, logger, zone, authMgr)
	if err != nil {
		logger.Error("failed to start", slog.String("error", err.Error()))
		os.Exit(1)
	}

	addr := fmt.Sprintf(":%s", port)
	logger.Info("listening", slog.String("addr", addr))
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		logger.Error("server stopped", slog.String("error", err.Error()))
		os.Exit(1)
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
