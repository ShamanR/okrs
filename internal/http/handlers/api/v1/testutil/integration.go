package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"okrs/internal/auth"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	apigoals "okrs/internal/http/handlers/api/v1/goals"
	apihierarhy "okrs/internal/http/handlers/api/v1/hierarhy"
	apikrs "okrs/internal/http/handlers/api/v1/krs"
	apiperiods "okrs/internal/http/handlers/api/v1/periods"
	apiteams "okrs/internal/http/handlers/api/v1/teams"
	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func NewAPIV1Router(service *service.Service) *chi.Mux {
	return NewAPIV1RouterWithScope(service, nil)
}

// NewAPIV1RouterWithScope returns a test router with a fixed scope injected into every request context.
// Pass nil for unrestricted access (admin), an empty slice for no access, or a specific list of team IDs.
func NewAPIV1RouterWithScope(svc *service.Service, allowedTeamIDs []int64) *chi.Mux {
	hierarchyHandler := apihierarhy.New(svc)
	periodsHandler := apiperiods.New(svc)
	teamsHandler := apiteams.New(svc)
	goalsHandler := apigoals.New(svc)
	krsHandler := apikrs.New(svc)

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithAllowedTeamIDs(r.Context(), allowedTeamIDs)
			ctx = auth.WithTenant(ctx, &domain.Tenant{ID: 1})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Route("/api/v1", func(r chi.Router) {
		apihierarhy.RegisterRoutes(r, hierarchyHandler)
		apiperiods.RegisterRoutes(r, periodsHandler)
		apiteams.RegisterRoutes(r, teamsHandler)
		apigoals.RegisterRoutes(r, goalsHandler)
		apikrs.RegisterRoutes(r, krsHandler)
		v1.RegisterMethodNotAllowed(r)
	})
	return router
}

func RunMigrations(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
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
	// Migration 032 drops the transitional tenant_id DEFAULT 1 so a forgotten tenant_id fails
	// in production. Integration fixtures are single-tenant and insert rows with raw SQL that
	// omits tenant_id; restore the default so those rows land in tenant 1.
	for _, tbl := range []string{
		"teams", "periods", "goals", "goal_shares", "team_period_statuses",
		"user_hierarchy_grants", "key_results", "goal_comments", "key_result_notes",
	} {
		if _, err := db.ExecContext(ctx, "ALTER TABLE "+tbl+" ALTER COLUMN tenant_id SET DEFAULT 1"); err != nil {
			return err
		}
	}
	return nil
}

func RequireDockerOrSkip(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
}

func resolveMigrationsPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "migrations"), nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found (start dir: %s)", dir)
		}
		dir = parent
	}
}
