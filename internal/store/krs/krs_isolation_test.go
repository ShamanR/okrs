package krs_test

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/krs"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5"
)

// TestKRsScopedByTenant verifies that KR repository operations are strictly
// isolated by tenant: a request under scope2 cannot read or mutate rows that
// belong to tenant 1.
func TestKRsScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	// Tenant 1 exists from migrations (DEFAULT 1). Create tenant 2.
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("create tenant 2: %v", err)
	}

	scope1 := domain.TenantScope{TenantID: 1}
	scope2 := domain.TenantScope{TenantID: 2}

	repo := krs.NewKRRepository(pool)

	// ---- Seed data under tenant 1 via raw SQL ----

	var teamID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO teams (name, tenant_id) VALUES ('Team1', 1) RETURNING id`,
	).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}

	var periodID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO periods (name, start_date, end_date, tenant_id) VALUES ('Q1', '2025-01-01', '2025-03-31', 1) RETURNING id`,
	).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	var goalID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO goals (team_id, period_id, title, priority, weight, work_type, focus_type, sort_order, tenant_id)
		 VALUES ($1,$2,'Goal','P1',100,'Delivery','STABILITY',1,1) RETURNING id`,
		teamID, periodID,
	).Scan(&goalID); err != nil {
		t.Fatalf("insert goal: %v", err)
	}

	// PROJECT KR (has stages)
	projectKRID, err := repo.CreateKeyResult(ctx, scope1, krs.KeyResultInput{
		GoalID: goalID,
		Title:  "Project KR",
		Weight: 50,
		Kind:   domain.KRKindProject,
	})
	if err != nil {
		t.Fatalf("create project KR: %v", err)
	}

	if err := repo.AddProjectStage(ctx, scope1, krs.ProjectStageInput{
		KeyResultID: projectKRID,
		Title:       "Stage 1",
		Weight:      100,
		SortOrder:   1,
	}); err != nil {
		t.Fatalf("add stage: %v", err)
	}

	// Fetch the stage ID so we can test UpdateProjectStageDone cross-tenant.
	stages, err := repo.ListProjectStages(ctx, scope1, projectKRID)
	if err != nil {
		t.Fatalf("list stages scope1: %v", err)
	}
	if len(stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(stages))
	}
	stageID := stages[0].ID

	// BOOLEAN KR (has boolean meta)
	boolKRID, err := repo.CreateKeyResult(ctx, scope1, krs.KeyResultInput{
		GoalID: goalID,
		Title:  "Boolean KR",
		Weight: 50,
		Kind:   domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create boolean KR: %v", err)
	}
	if err := repo.UpsertBooleanMeta(ctx, scope1, boolKRID, false); err != nil {
		t.Fatalf("upsert boolean meta: %v", err)
	}

	// ---- Cross-tenant read assertions ----

	// GetKeyResult under scope2 must not return tenant-1 rows.
	if _, err := repo.GetKeyResult(ctx, scope2, projectKRID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("scope2 GetKeyResult(projectKRID): expected pgx.ErrNoRows, got %v", err)
	}
	if _, err := repo.GetKeyResult(ctx, scope2, boolKRID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("scope2 GetKeyResult(boolKRID): expected pgx.ErrNoRows, got %v", err)
	}

	// ListKeyResultsByGoal under scope2 returns nothing for tenant-1's goalID.
	krList, err := repo.ListKeyResultsByGoal(ctx, scope2, goalID)
	if err != nil {
		t.Fatalf("scope2 ListKeyResultsByGoal: %v", err)
	}
	if len(krList) != 0 {
		t.Fatalf("scope2 must see 0 KRs for tenant-1 goal, got %d", len(krList))
	}

	// ListProjectStages under scope2 returns nothing for tenant-1's KR.
	crossStages, err := repo.ListProjectStages(ctx, scope2, projectKRID)
	if err != nil {
		t.Fatalf("scope2 ListProjectStages: %v", err)
	}
	if len(crossStages) != 0 {
		t.Fatalf("scope2 must see 0 stages, got %d", len(crossStages))
	}

	// GetBooleanMeta under scope2 must not return tenant-1 data.
	if _, err := repo.GetBooleanMeta(ctx, scope2, boolKRID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("scope2 GetBooleanMeta: expected pgx.ErrNoRows, got %v", err)
	}

	// FindGoalIDByKR under scope2 must not find tenant-1 KR.
	if _, err := repo.FindGoalIDByKR(ctx, scope2, projectKRID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("scope2 FindGoalIDByKR: expected pgx.ErrNoRows, got %v", err)
	}

	// FindGoalIDByStage under scope2 must not find tenant-1 stage.
	if _, err := repo.FindGoalIDByStage(ctx, scope2, stageID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("scope2 FindGoalIDByStage: expected pgx.ErrNoRows, got %v", err)
	}

	// ---- Cross-tenant mutation assertions ----
	// Each mutation under scope2 must be a no-op for tenant-1 data.
	// We verify by re-reading under scope1 afterwards.

	// UpdateProjectStageDone cross-tenant — stage must remain undone.
	if err := repo.UpdateProjectStageDone(ctx, scope2, stageID, true); err != nil {
		t.Fatalf("scope2 UpdateProjectStageDone unexpected error: %v", err)
	}
	stagesAfter, err := repo.ListProjectStages(ctx, scope1, projectKRID)
	if err != nil {
		t.Fatalf("scope1 re-read stages: %v", err)
	}
	if len(stagesAfter) != 1 || stagesAfter[0].IsDone {
		t.Fatalf("scope2 UpdateProjectStageDone must not mutate tenant-1 stage; got %+v", stagesAfter)
	}

	// UpsertBooleanMeta cross-tenant — boolean must remain false.
	if err := repo.UpsertBooleanMeta(ctx, scope2, boolKRID, true); err != nil {
		t.Fatalf("scope2 UpsertBooleanMeta unexpected error: %v", err)
	}
	meta, err := repo.GetBooleanMeta(ctx, scope1, boolKRID)
	if err != nil {
		t.Fatalf("scope1 re-read boolean meta: %v", err)
	}
	if meta.IsDone {
		t.Fatalf("scope2 UpsertBooleanMeta must not mutate tenant-1 boolean; got IsDone=true")
	}

	// DeleteKeyResult cross-tenant — KR must still exist under scope1.
	if err := repo.DeleteKeyResult(ctx, scope2, projectKRID); err != nil {
		t.Fatalf("scope2 DeleteKeyResult unexpected error: %v", err)
	}
	if _, err := repo.GetKeyResult(ctx, scope1, projectKRID); err != nil {
		t.Fatalf("scope1 must still find projectKR after scope2 delete attempt: %v", err)
	}
}
