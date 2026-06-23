package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestStoreExposesTenantRepos(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(10*time.Second),
		),
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
	tn, err := s.Tenants.GetBySlug(ctx, "default")
	if err != nil {
		t.Fatalf("default tenant: %v", err)
	}
	if tn.ID != 1 {
		t.Fatalf("default tenant id = %d, want 1", tn.ID)
	}
}

func TestStoreCRUD(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
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
	var periodID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('2024 Q3', '2024-07-01', '2024-09-30', 1)
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	goalID, err := s.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID:      teamID,
		PeriodID:    periodID,
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

	krID, err := s.KRs.CreateKeyResult(ctx, domain.TenantScope{TenantID: 1}, krs.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR 1",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create kr: %v", err)
	}
	if err := s.KRs.UpsertBooleanMeta(ctx, domain.TenantScope{TenantID: 1}, krID, true); err != nil {
		t.Fatalf("update boolean: %v", err)
	}

	goalsList, err := s.Goals.ListGoalsByTeamPeriod(ctx, domain.TenantScope{TenantID: 1}, teamID, periodID)
	if err != nil {
		t.Fatalf("list goals: %v", err)
	}
	if len(goalsList) != 1 {
		t.Fatalf("expected 1 goal got %d", len(goalsList))
	}
	if len(goalsList[0].KeyResults) != 1 {
		t.Fatalf("expected 1 kr got %d", len(goalsList[0].KeyResults))
	}
}

func TestListGoalsByTeamsPeriodIncludesKRDataForSharedGoals(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
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
	var ownerID, sharedTeamID, periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Owner') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("insert owner team: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Shared') RETURNING id`).Scan(&sharedTeamID); err != nil {
		t.Fatalf("insert shared team: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('2025 Q2', '2025-04-01', '2025-06-30', 1)
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	goalID, err := s.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID:      ownerID,
		PeriodID:    periodID,
		Title:       "Shared goal",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "owner",
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO goal_shares (goal_id, team_id, weight, sort_order) VALUES ($1,$2,100,1)`, goalID, sharedTeamID); err != nil {
		t.Fatalf("insert goal share: %v", err)
	}

	krID, err := s.KRs.CreateKeyResult(ctx, domain.TenantScope{TenantID: 1}, krs.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR bool",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create key result: %v", err)
	}
	if err := s.KRs.UpsertBooleanMeta(ctx, domain.TenantScope{TenantID: 1}, krID, true); err != nil {
		t.Fatalf("upsert boolean meta: %v", err)
	}

	goalsByTeam, err := s.Goals.ListGoalsByTeamsPeriod(ctx, domain.TenantScope{TenantID: 1}, periodID, []int64{ownerID, sharedTeamID})
	if err != nil {
		t.Fatalf("list goals by teams period: %v", err)
	}
	for _, teamID := range []int64{ownerID, sharedTeamID} {
		goalsList := goalsByTeam[teamID]
		if len(goalsList) != 1 {
			t.Fatalf("team %d expected exactly one goal, got %d", teamID, len(goalsList))
		}
		if len(goalsList[0].KeyResults) != 1 {
			t.Fatalf("team %d expected one key result, got %d", teamID, len(goalsList[0].KeyResults))
		}
		if goalsList[0].KeyResults[0].Boolean == nil || !goalsList[0].KeyResults[0].Boolean.IsDone {
			t.Fatalf("team %d expected boolean KR meta to be loaded for shared goal", teamID)
		}
	}
}

func TestTeamDeleteLifecycleAndVisibility(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
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

	var parentID, childNoGoalsID, teamWithGoalsID, childOfDeletedID, periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Parent', 'unit') RETURNING id`).Scan(&parentID); err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, parent_id) VALUES ('Child no goals', 'team', $1) RETURNING id`, parentID).Scan(&childNoGoalsID); err != nil {
		t.Fatalf("insert child no goals: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, parent_id) VALUES ('Team with goals', 'team', $1) RETURNING id`, parentID).Scan(&teamWithGoalsID); err != nil {
		t.Fatalf("insert team with goals: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, parent_id) VALUES ('Child of deleted', 'team', $1) RETURNING id`, teamWithGoalsID).Scan(&childOfDeletedID); err != nil {
		t.Fatalf("insert child of deleted: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('2024 Q4', '2024-10-01', '2024-12-31', 1)
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	if _, err := s.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID:      teamWithGoalsID,
		PeriodID:    periodID,
		Title:       "Historic goal",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Owner",
	}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	scope := domain.TenantScope{TenantID: 1}
	if hasGoals, err := s.Teams.TeamHasGoals(ctx, scope, teamWithGoalsID); err != nil || !hasGoals {
		t.Fatalf("expected team with goals to be detected, got %v %v", hasGoals, err)
	}
	if hasGoals, err := s.Teams.TeamHasGoals(ctx, scope, childNoGoalsID); err != nil || hasGoals {
		t.Fatalf("expected team without goals to be clean, got %v %v", hasGoals, err)
	}

	if err := s.Teams.HardDeleteTeam(ctx, scope, childNoGoalsID); err != nil {
		t.Fatalf("hard delete no goals: %v", err)
	}
	if _, err := s.Teams.GetTeam(ctx, scope, childNoGoalsID); err == nil {
		t.Fatalf("expected hard-deleted team to be removed")
	}

	if err := s.Teams.SoftDeleteTeam(ctx, scope, teamWithGoalsID); err != nil {
		t.Fatalf("soft delete team with goals: %v", err)
	}
	teamWithGoals, err := s.Teams.GetTeam(ctx, scope, teamWithGoalsID)
	if err != nil {
		t.Fatalf("get soft-deleted team: %v", err)
	}
	if teamWithGoals.DeletedAt == nil {
		t.Fatalf("expected deleted_at to be set")
	}
	childOfDeleted, err := s.Teams.GetTeam(ctx, scope, childOfDeletedID)
	if err != nil {
		t.Fatalf("get reparented child: %v", err)
	}
	if childOfDeleted.ParentID == nil || *childOfDeleted.ParentID != parentID {
		t.Fatalf("expected child to be reparented to original parent, got %+v", childOfDeleted.ParentID)
	}

	deletedTeams, err := s.Teams.ListDeletedTeams(ctx, scope)
	if err != nil {
		t.Fatalf("list deleted teams: %v", err)
	}
	if len(deletedTeams) != 1 || deletedTeams[0].ID != teamWithGoalsID {
		t.Fatalf("expected deleted team list to contain team with goals, got %+v", deletedTeams)
	}

	if err := s.Teams.RestoreTeam(ctx, scope, teamWithGoalsID); err != nil {
		t.Fatalf("restore team: %v", err)
	}
	restored, err := s.Teams.GetTeam(ctx, scope, teamWithGoalsID)
	if err != nil {
		t.Fatalf("get restored team: %v", err)
	}
	if restored.DeletedAt != nil {
		t.Fatalf("expected restored team to be active")
	}
}

func TestKRActivityTimestampsUsedForGoalAndTeamUpdates(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
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
	var ownerID, sharedID, periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Owner last update') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("insert owner team: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Shared last update') RETURNING id`).Scan(&sharedID); err != nil {
		t.Fatalf("insert shared team: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('2026 Q2 last update', '2026-04-01', '2026-06-30', 1)
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	goalID, err := s.Goals.CreateGoal(ctx, domain.TenantScope{TenantID: 1}, goals.GoalInput{
		TeamID:      ownerID,
		PeriodID:    periodID,
		Title:       "Goal with KR activity",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Owner",
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO goal_shares (goal_id, team_id, weight) VALUES ($1, $2, 100)`, goalID, sharedID); err != nil {
		t.Fatalf("insert goal share: %v", err)
	}
	krID, err := s.KRs.CreateKeyResult(ctx, domain.TenantScope{TenantID: 1}, krs.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR for timestamp aggregation",
		Description: "desc",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create key result: %v", err)
	}

	noteTime := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	progressTime := time.Date(2026, 4, 7, 9, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE goals SET updated_at = $1 WHERE id = $2`, time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC), goalID); err != nil {
		t.Fatalf("set goal updated_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE key_results SET updated_at = $1, progress_updated_at = $2 WHERE id = $3`, time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC), time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC), krID); err != nil {
		t.Fatalf("set key result updated_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO key_result_notes (key_result_id, text, author_user_id, updated_at) VALUES ($1, 'latest note', 1, $2)`, krID, noteTime); err != nil {
		t.Fatalf("insert key result note: %v", err)
	}

	goalsList, err := s.Goals.ListGoalsByTeamPeriod(ctx, domain.TenantScope{TenantID: 1}, ownerID, periodID)
	if err != nil {
		t.Fatalf("list goals by team period: %v", err)
	}
	if len(goalsList) != 1 {
		t.Fatalf("expected one goal, got %d", len(goalsList))
	}
	if !goalsList[0].UpdatedAt.Equal(noteTime) {
		t.Fatalf("expected goal updated_at from latest KR note %s, got %s", noteTime, goalsList[0].UpdatedAt)
	}

	if _, err := pool.Exec(ctx, `UPDATE key_results SET updated_at = $1 WHERE id = $2`, time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC), krID); err != nil {
		t.Fatalf("set metadata-only key result updated_at: %v", err)
	}
	updatesAfterMetadataEdit, err := s.Goals.ListTeamLastGoalUpdateInPeriod(ctx, domain.TenantScope{TenantID: 1}, periodID, []int64{ownerID, sharedID})
	if err != nil {
		t.Fatalf("list team last update after metadata edit: %v", err)
	}
	if !updatesAfterMetadataEdit[ownerID].Equal(noteTime) {
		t.Fatalf("expected owner update to ignore metadata edit and remain %s, got %s", noteTime, updatesAfterMetadataEdit[ownerID])
	}

	if _, err := pool.Exec(ctx, `UPDATE key_results SET updated_at = $1, progress_updated_at = $2 WHERE id = $3`, progressTime, progressTime, krID); err != nil {
		t.Fatalf("set newer key result progress_updated_at: %v", err)
	}
	updates, err := s.Goals.ListTeamLastGoalUpdateInPeriod(ctx, domain.TenantScope{TenantID: 1}, periodID, []int64{ownerID, sharedID})
	if err != nil {
		t.Fatalf("list team last update: %v", err)
	}
	if !updates[ownerID].Equal(progressTime) {
		t.Fatalf("expected owner update %s, got %s", progressTime, updates[ownerID])
	}
	if !updates[sharedID].Equal(progressTime) {
		t.Fatalf("expected shared update %s, got %s", progressTime, updates[sharedID])
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
