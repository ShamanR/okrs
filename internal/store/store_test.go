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

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

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

	goalID, err := s.CreateGoal(ctx, GoalInput{
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

	goals, err := s.ListGoalsByTeamPeriod(ctx, teamID, periodID)
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

func TestListTeamOverviewStatsWorkBalance(t *testing.T) {
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
	var teamID, periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Team A') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('2025 Q1', '2025-01-01', '2025-03-31', 1)
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	if _, err := s.CreateGoal(ctx, GoalInput{
		TeamID:      teamID,
		PeriodID:    periodID,
		Title:       "Discovery goal",
		Description: "d",
		Priority:    domain.PriorityP1,
		Weight:      50,
		WorkType:    domain.WorkTypeDiscovery,
		FocusType:   domain.FocusStability,
		OwnerText:   "owner",
	}); err != nil {
		t.Fatalf("create discovery goal: %v", err)
	}
	if _, err := s.CreateGoal(ctx, GoalInput{
		TeamID:      teamID,
		PeriodID:    periodID,
		Title:       "Delivery goal",
		Description: "d",
		Priority:    domain.PriorityP2,
		Weight:      50,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusSpeedEfficiency,
		OwnerText:   "owner",
	}); err != nil {
		t.Fatalf("create delivery goal: %v", err)
	}

	stats, err := s.ListTeamOverviewStats(ctx, periodID, []int64{teamID})
	if err != nil {
		t.Fatalf("list overview stats: %v", err)
	}
	item, ok := stats[teamID]
	if !ok {
		t.Fatalf("expected stats for team %d", teamID)
	}
	if item.Discovery != 1 || item.Delivery != 1 {
		t.Fatalf("expected work balance discovery=1, delivery=1; got %+v", item)
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

	goalID, err := s.CreateGoal(ctx, GoalInput{
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

	krID, err := s.CreateKeyResult(ctx, KeyResultInput{
		GoalID:      goalID,
		Title:       "KR bool",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create key result: %v", err)
	}
	if err := s.UpsertBooleanMeta(ctx, krID, true); err != nil {
		t.Fatalf("upsert boolean meta: %v", err)
	}

	goalsByTeam, err := s.ListGoalsByTeamsPeriod(ctx, periodID, []int64{ownerID, sharedTeamID})
	if err != nil {
		t.Fatalf("list goals by teams period: %v", err)
	}
	for _, teamID := range []int64{ownerID, sharedTeamID} {
		goals := goalsByTeam[teamID]
		if len(goals) != 1 {
			t.Fatalf("team %d expected exactly one goal, got %d", teamID, len(goals))
		}
		if len(goals[0].KeyResults) != 1 {
			t.Fatalf("team %d expected one key result, got %d", teamID, len(goals[0].KeyResults))
		}
		if goals[0].KeyResults[0].Boolean == nil || !goals[0].KeyResults[0].Boolean.IsDone {
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
	if _, err := s.CreateGoal(ctx, GoalInput{
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

	if hasGoals, err := s.TeamHasGoals(ctx, teamWithGoalsID); err != nil || !hasGoals {
		t.Fatalf("expected team with goals to be detected, got %v %v", hasGoals, err)
	}
	if hasGoals, err := s.TeamHasGoals(ctx, childNoGoalsID); err != nil || hasGoals {
		t.Fatalf("expected team without goals to be clean, got %v %v", hasGoals, err)
	}

	if err := s.HardDeleteTeam(ctx, childNoGoalsID); err != nil {
		t.Fatalf("hard delete no goals: %v", err)
	}
	if _, err := s.GetTeam(ctx, childNoGoalsID); err == nil {
		t.Fatalf("expected hard-deleted team to be removed")
	}

	if err := s.SoftDeleteTeam(ctx, teamWithGoalsID); err != nil {
		t.Fatalf("soft delete team with goals: %v", err)
	}
	teamWithGoals, err := s.GetTeam(ctx, teamWithGoalsID)
	if err != nil {
		t.Fatalf("get soft-deleted team: %v", err)
	}
	if teamWithGoals.DeletedAt == nil {
		t.Fatalf("expected deleted_at to be set")
	}
	childOfDeleted, err := s.GetTeam(ctx, childOfDeletedID)
	if err != nil {
		t.Fatalf("get reparented child: %v", err)
	}
	if childOfDeleted.ParentID == nil || *childOfDeleted.ParentID != parentID {
		t.Fatalf("expected child to be reparented to original parent, got %+v", childOfDeleted.ParentID)
	}

	deletedTeams, err := s.ListDeletedTeams(ctx)
	if err != nil {
		t.Fatalf("list deleted teams: %v", err)
	}
	if len(deletedTeams) != 1 || deletedTeams[0].ID != teamWithGoalsID {
		t.Fatalf("expected deleted team list to contain team with goals, got %+v", deletedTeams)
	}

	if err := s.RestoreTeam(ctx, teamWithGoalsID); err != nil {
		t.Fatalf("restore team: %v", err)
	}
	restored, err := s.GetTeam(ctx, teamWithGoalsID)
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

	goalID, err := s.CreateGoal(ctx, GoalInput{
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
	krID, err := s.CreateKeyResult(ctx, KeyResultInput{
		GoalID:      goalID,
		Title:       "KR for timestamp aggregation",
		Description: "desc",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create key result: %v", err)
	}

	commentTime := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	progressTime := time.Date(2026, 4, 7, 9, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE goals SET updated_at = $1 WHERE id = $2`, time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC), goalID); err != nil {
		t.Fatalf("set goal updated_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE key_results SET updated_at = $1, progress_updated_at = $2 WHERE id = $3`, time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC), time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC), krID); err != nil {
		t.Fatalf("set key result updated_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO key_result_comments (key_result_id, text, author_user_id, created_at) VALUES ($1, 'latest comment', 1, $2)`, krID, commentTime); err != nil {
		t.Fatalf("insert key result comment: %v", err)
	}

	goals, err := s.ListGoalsByTeamPeriod(ctx, ownerID, periodID)
	if err != nil {
		t.Fatalf("list goals by team period: %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("expected one goal, got %d", len(goals))
	}
	if !goals[0].UpdatedAt.Equal(commentTime) {
		t.Fatalf("expected goal updated_at from latest KR comment %s, got %s", commentTime, goals[0].UpdatedAt)
	}

	if _, err := pool.Exec(ctx, `UPDATE key_results SET updated_at = $1 WHERE id = $2`, time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC), krID); err != nil {
		t.Fatalf("set metadata-only key result updated_at: %v", err)
	}
	updatesAfterMetadataEdit, err := s.ListTeamLastGoalUpdateInPeriod(ctx, periodID, []int64{ownerID, sharedID})
	if err != nil {
		t.Fatalf("list team last update after metadata edit: %v", err)
	}
	if !updatesAfterMetadataEdit[ownerID].Equal(commentTime) {
		t.Fatalf("expected owner update to ignore metadata edit and remain %s, got %s", commentTime, updatesAfterMetadataEdit[ownerID])
	}

	if _, err := pool.Exec(ctx, `UPDATE key_results SET updated_at = $1, progress_updated_at = $2 WHERE id = $3`, progressTime, progressTime, krID); err != nil {
		t.Fatalf("set newer key result progress_updated_at: %v", err)
	}
	updates, err := s.ListTeamLastGoalUpdateInPeriod(ctx, periodID, []int64{ownerID, sharedID})
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
