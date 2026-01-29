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
	"time"

	httpserver "okrs/internal/http"
	"okrs/internal/store"

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
		period, err := pgstore.FindPeriodForDate(context.Background(), now)
		var periodID int64
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				name, startDate, endDate := quarterPeriod(now)
				periodID, err = pgstore.CreatePeriod(context.Background(), store.PeriodInput{
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

	server, err := httpserver.NewServer(pgstore, logger, zone)
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
