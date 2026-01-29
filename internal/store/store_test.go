package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"okrs/internal/domain"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestStoreCRUD(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer func() { _ = container.Terminate(ctx) }()

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := runMigrations(dbURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	s := New(pool)
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('QA') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}

	goalID, err := s.CreateGoal(ctx, GoalInput{
		TeamID:      teamID,
		Year:        2024,
		Quarter:     3,
		Title:       "Ship something",
		Description: "Testing",
		Priority:    domain.PriorityP1,
		Weight:      50,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "QA",
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}

	krID, err := s.CreateKeyResult(ctx, KeyResultInput{
		GoalID:      goalID,
		Title:       "KR 1",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	if err := s.UpsertBooleanMeta(ctx, krID, true); err != nil {
		t.Fatalf("update boolean: %v", err)
	}

	goals, err := s.ListGoalsByTeamQuarter(ctx, teamID, 2024, 3)
	if err != nil {
		t.Fatalf("list goals: %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("expected 1 goal got %d", len(goals))
	}
	if len(goals[0].KeyResults) != 1 {
		t.Fatalf("expected 1 kr got %d", len(goals[0].KeyResults))
	}
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
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
