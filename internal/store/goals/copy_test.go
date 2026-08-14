package goals_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/testutil"
)

var copyScope = domain.TenantScope{TenantID: 1}

// Enum values are constructed via type conversion from their stored string form
// (priority "P0".."P3"; work_type "Delivery"/"Discovery"; focus UPPER_SNAKE) to avoid
// depending on exact Go constant names — see specs/020-domain-model.md.

func TestCopyGoalDuplicatesStructure(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	krRepo := krs.NewKRRepository(pool)
	repo := goals.NewGoalRepository(pool, krRepo)

	var srcTeam, dstTeam, period int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Src') RETURNING id`).Scan(&srcTeam)
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Dst') RETURNING id`).Scan(&dstTeam)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('P','2026-01-01','2026-12-31') RETURNING id`).Scan(&period)

	srcGoal, err := repo.CreateGoal(ctx, copyScope, goals.GoalInput{
		TeamID: srcTeam, PeriodID: period, Title: "Src goal", Description: "d",
		Priority: domain.Priority("P1"), Weight: 40, WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("STABILITY"),
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	krID, err := krRepo.CreateKeyResult(ctx, copyScope, krs.KeyResultInput{
		GoalID: srcGoal, Title: "KR", Description: "kd", Weight: 100, Kind: domain.KRKindNumerical,
	})
	if err != nil {
		t.Fatalf("CreateKeyResult: %v", err)
	}
	if err := krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 55, Unit: "%",
	}); err != nil {
		t.Fatalf("UpsertNumericalMeta: %v", err)
	}

	newID, err := repo.CopyGoal(ctx, copyScope, goals.CopyGoalInput{
		SourceGoalID: srcGoal, TargetTeamID: dstTeam, TargetPeriodID: period,
		WithProgress: false, WithComments: false,
	})
	if err != nil {
		t.Fatalf("CopyGoal: %v", err)
	}
	if newID == srcGoal {
		t.Fatal("expected a new goal id, got the source id")
	}

	got, err := repo.GetGoal(ctx, copyScope, newID)
	if err != nil {
		t.Fatalf("GetGoal(new): %v", err)
	}
	if got.TeamID != dstTeam || got.PeriodID != period {
		t.Fatalf("target mismatch: team=%d period=%d", got.TeamID, got.PeriodID)
	}
	if got.Title != "Src goal" || got.Weight != 40 || got.Priority != domain.Priority("P1") {
		t.Fatalf("fields not copied: %+v", got)
	}
	if len(got.KeyResults) != 1 {
		t.Fatalf("expected 1 KR, got %d", len(got.KeyResults))
	}
	kr := got.KeyResults[0]
	if kr.Kind != domain.KRKindNumerical || kr.Numerical == nil {
		t.Fatalf("KR kind/meta not copied: %+v", kr)
	}
	// WithProgress=false → current reset to start_value (0).
	if kr.Numerical.CurrentValue != 0 || kr.Numerical.TargetValue != 100 {
		t.Fatalf("progress not reset: current=%v target=%v", kr.Numerical.CurrentValue, kr.Numerical.TargetValue)
	}
}

func TestCopyGoalCarriesProgressNotesAndComments(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	krRepo := krs.NewKRRepository(pool)
	repo := goals.NewGoalRepository(pool, krRepo)

	var team, period int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&team)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('P','2026-01-01','2026-12-31') RETURNING id`).Scan(&period)

	srcGoal, _ := repo.CreateGoal(ctx, copyScope, goals.GoalInput{
		TeamID: team, PeriodID: period, Title: "G", Priority: domain.Priority("P2"), Weight: 10,
		WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("STABILITY"),
	})
	krID, _ := krRepo.CreateKeyResult(ctx, copyScope, krs.KeyResultInput{GoalID: srcGoal, Title: "KR", Weight: 100, Kind: domain.KRKindNumerical})
	krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{KeyResultID: krID, StartValue: 0, TargetValue: 100, CurrentValue: 70, Unit: "%"})
	krRepo.UpsertKeyResultNote(ctx, copyScope, krID, "note text", 1)
	// A task + a reply.
	var taskID int64
	pool.QueryRow(ctx, `INSERT INTO goal_comments (goal_id, text, author_user_id, tenant_id) VALUES ($1,'task',1,1) RETURNING id`, srcGoal).Scan(&taskID)
	pool.Exec(ctx, `INSERT INTO goal_comments (goal_id, parent_id, text, author_user_id, tenant_id) VALUES ($1,$2,'reply',1,1)`, srcGoal, taskID)

	newID, err := repo.CopyGoal(ctx, copyScope, goals.CopyGoalInput{
		SourceGoalID: srcGoal, TargetTeamID: team, TargetPeriodID: period, WithProgress: true, WithComments: true,
	})
	if err != nil {
		t.Fatalf("CopyGoal: %v", err)
	}
	got, _ := repo.GetGoal(ctx, copyScope, newID)
	if got.KeyResults[0].Numerical.CurrentValue != 70 {
		t.Fatalf("progress not carried: %v", got.KeyResults[0].Numerical.CurrentValue)
	}
	if got.KeyResults[0].Note == nil || got.KeyResults[0].Note.Text != "note text" {
		t.Fatalf("note not carried: %+v", got.KeyResults[0].Note)
	}
	if len(got.Comments) != 1 || got.Comments[0].Text != "task" {
		t.Fatalf("comment task not carried: %+v", got.Comments)
	}
	if len(got.Comments[0].Replies) != 1 || got.Comments[0].Replies[0].Text != "reply" {
		t.Fatalf("reply not carried: %+v", got.Comments)
	}
}

func TestCopyGoalDeleteSourceRemovesOriginal(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	krRepo := krs.NewKRRepository(pool)
	repo := goals.NewGoalRepository(pool, krRepo)

	var srcTeam, dstTeam, period int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Src') RETURNING id`).Scan(&srcTeam)
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Dst') RETURNING id`).Scan(&dstTeam)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('P','2026-01-01','2026-12-31') RETURNING id`).Scan(&period)

	srcGoal, _ := repo.CreateGoal(ctx, copyScope, goals.GoalInput{
		TeamID: srcTeam, PeriodID: period, Title: "Src", Priority: domain.Priority("P1"), Weight: 10,
		WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("STABILITY"),
	})
	krRepo.CreateKeyResult(ctx, copyScope, krs.KeyResultInput{GoalID: srcGoal, Title: "KR", Weight: 100, Kind: domain.KRKindBoolean})

	newID, err := repo.CopyGoal(ctx, copyScope, goals.CopyGoalInput{
		SourceGoalID: srcGoal, TargetTeamID: dstTeam, TargetPeriodID: period, DeleteSource: true,
	})
	if err != nil {
		t.Fatalf("CopyGoal(move): %v", err)
	}
	// Copy exists.
	if _, err := repo.GetGoal(ctx, copyScope, newID); err != nil {
		t.Fatalf("copy should exist: %v", err)
	}
	// Source is gone (and its KRs cascade).
	if _, err := repo.GetGoal(ctx, copyScope, srcGoal); err == nil {
		t.Fatal("source goal should have been deleted by DeleteSource")
	}
	var krCount int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM key_results WHERE goal_id=$1`, srcGoal).Scan(&krCount)
	if krCount != 0 {
		t.Fatalf("source KRs should cascade-delete, got %d", krCount)
	}
}
