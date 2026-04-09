package v1

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func newAPIV1TestRouter(service *service.Service) *chi.Mux {
	handler := NewHandler(service)
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) {
		r.Get("/hierarchy", handler.HandleHierarchy())
		r.Get("/periods", handler.HandlePeriods())
		r.Get("/teams/{teamID}", handler.HandleTeam())
		r.Get("/teams/{teamID}/okrs", handler.HandleTeamOKRs())
		r.Get("/teams/{teamID}/overview", handler.HandleTeamOverview())
		r.Post("/teams/{teamID}/status", handler.HandleUpdateTeamPeriodStatus())
		r.Get("/goals/{goalID}", handler.HandleGoal())
		r.Post("/goals/{goalID}/share", handler.HandleShareGoal())
		r.Post("/goals/{goalID}/weight", handler.HandleUpdateGoalWeight())
		r.Post("/goals/{goalID}/comments", handler.HandleAddGoalComment())
		r.Post("/goals/{goalID}", handler.HandleUpdateGoal())
		r.Post("/goals/{goalID}/key-results", handler.HandleCreateKeyResult())
		r.Post("/goals/{goalID}/move-up", handler.HandleMoveGoalUp())
		r.Post("/goals/{goalID}/move-down", handler.HandleMoveGoalDown())
		r.Post("/krs/{krID}/progress/percent", handler.HandleUpdatePercentProgress())
		r.Post("/krs/{krID}/progress/boolean", handler.HandleUpdateBooleanProgress())
		r.Post("/krs/{krID}/progress/project", handler.HandleUpdateProjectProgress())
		r.Post("/krs/{krID}/comments", handler.HandleAddKRComment())
		r.Post("/krs/{krID}", handler.HandleUpdateKeyResult())
		r.Post("/krs/{krID}/move-up", handler.HandleMoveKeyResultUp())
		r.Post("/krs/{krID}/move-down", handler.HandleMoveKeyResultDown())
		RegisterMethodNotAllowed(r)
	})
	return router
}

func runMigrations(databaseURL string) error {
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
	return nil
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

func requireDockerOrSkip(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
}
