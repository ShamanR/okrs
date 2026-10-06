package goals_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/testutil"
)

// TestBoardReportsCurrentValueFromStartUntilCheckIn covers the reads behind the team board —
// the ones that showed the defect. Both batch readers build the numerical measure themselves,
// so both are exercised: ListGoalsByTeamPeriod (loadKRsForGoals) and ListGoalsByTeamsPeriod.
func TestBoardReportsCurrentValueFromStartUntilCheckIn(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	krRepo := krs.NewKRRepository(pool)
	repo := goals.NewGoalRepository(pool, krRepo)

	var team, period int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&team)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('P','2026-01-01','2026-12-31') RETURNING id`).Scan(&period)

	goalID, err := repo.CreateGoal(ctx, copyScope, goals.GoalInput{
		TeamID: team, PeriodID: period, Title: "G", Priority: domain.Priority("P1"), Weight: 100,
		WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("STABILITY"),
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	krID, err := krRepo.CreateKeyResult(ctx, copyScope, krs.KeyResultInput{
		GoalID: goalID, Title: "KR", Weight: 100, Kind: domain.KRKindNumerical,
	})
	if err != nil {
		t.Fatalf("CreateKeyResult: %v", err)
	}
	if err := krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 0, TargetValue: 200, Unit: "%",
	}); err != nil {
		t.Fatalf("UpsertNumericalMeta: %v", err)
	}

	// assertBoard checks every board read path at once: they must not disagree.
	assertBoard := func(what string, wantCurrent float64, wantProgress int) {
		t.Helper()
		single, err := repo.ListGoalsByTeamPeriod(ctx, copyScope, team, period)
		if err != nil {
			t.Fatalf("%s: ListGoalsByTeamPeriod: %v", what, err)
		}
		batch, err := repo.ListGoalsByTeamsPeriod(ctx, copyScope, period, []int64{team})
		if err != nil {
			t.Fatalf("%s: ListGoalsByTeamsPeriod: %v", what, err)
		}
		if len(single) != 1 || len(single[0].KeyResults) != 1 {
			t.Fatalf("%s: unexpected board shape: %+v", what, single)
		}
		if len(batch[team]) != 1 || len(batch[team][0].KeyResults) != 1 {
			t.Fatalf("%s: unexpected batch board shape: %+v", what, batch)
		}
		for name, kr := range map[string]domain.KeyResult{
			"ListGoalsByTeamPeriod":  single[0].KeyResults[0],
			"ListGoalsByTeamsPeriod": batch[team][0].KeyResults[0],
		} {
			if kr.Numerical == nil {
				t.Fatalf("%s via %s: expected numerical meta", what, name)
			}
			if kr.Numerical.CurrentValue != wantCurrent {
				t.Fatalf("%s via %s: expected current %v, got %v", what, name, wantCurrent, kr.Numerical.CurrentValue)
			}
			if got := progress.ForKR(kr); got != wantProgress {
				t.Fatalf("%s via %s: expected progress %d%%, got %d%%", what, name, wantProgress, got)
			}
		}
	}

	assertBoard("just created", 0, 0)

	// The defect: editing the start before the first check-in used to keep reporting 0.
	if err := krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 100, TargetValue: 200, Unit: "%",
	}); err != nil {
		t.Fatalf("UpsertNumericalMeta (edit): %v", err)
	}
	assertBoard("start edited before the first check-in", 100, 0)

	if err := krRepo.UpdateNumericalCurrent(ctx, copyScope, krID, 150); err != nil {
		t.Fatalf("UpdateNumericalCurrent: %v", err)
	}
	assertBoard("after the check-in", 150, 50)

	// Editing the definition after a check-in must leave the progress alone.
	if err := krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{
		KeyResultID: krID, StartValue: 0, TargetValue: 300, Unit: "%",
	}); err != nil {
		t.Fatalf("UpsertNumericalMeta (edit after check-in): %v", err)
	}
	assertBoard("start edited after the check-in", 150, 50)
}

// TestCopyGoalWithoutProgressLeavesCurrentValueUnset pins that a copy made without progress
// behaves like a freshly created KR: editing its start value moves the reported current value.
func TestCopyGoalWithoutProgressLeavesCurrentValueUnset(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	krRepo := krs.NewKRRepository(pool)
	repo := goals.NewGoalRepository(pool, krRepo)

	var team, period int64
	pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('T') RETURNING id`).Scan(&team)
	pool.QueryRow(ctx, `INSERT INTO periods (name,start_date,end_date) VALUES ('P','2026-01-01','2026-12-31') RETURNING id`).Scan(&period)

	srcGoal, _ := repo.CreateGoal(ctx, copyScope, goals.GoalInput{
		TeamID: team, PeriodID: period, Title: "G", Priority: domain.Priority("P1"), Weight: 100,
		WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("STABILITY"),
	})
	srcKR, _ := krRepo.CreateKeyResult(ctx, copyScope, krs.KeyResultInput{GoalID: srcGoal, Title: "KR", Weight: 100, Kind: domain.KRKindNumerical})
	if err := krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{KeyResultID: srcKR, StartValue: 0, TargetValue: 100, Unit: "%"}); err != nil {
		t.Fatalf("UpsertNumericalMeta: %v", err)
	}
	if err := krRepo.UpdateNumericalCurrent(ctx, copyScope, srcKR, 55); err != nil {
		t.Fatalf("UpdateNumericalCurrent: %v", err)
	}

	newID, err := repo.CopyGoal(ctx, copyScope, goals.CopyGoalInput{
		SourceGoalID: srcGoal, TargetTeamID: team, TargetPeriodID: period, WithProgress: false,
	})
	if err != nil {
		t.Fatalf("CopyGoal: %v", err)
	}
	got, err := repo.GetGoal(ctx, copyScope, newID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	copiedKR := got.KeyResults[0]
	var raw *float64
	if err := pool.QueryRow(ctx, `SELECT current_value FROM key_results WHERE id=$1`, copiedKR.ID).Scan(&raw); err != nil {
		t.Fatalf("select current_value: %v", err)
	}
	if raw != nil {
		t.Fatalf("expected the copy to carry no current value, got %v", *raw)
	}

	var stamp *time.Time
	if err := pool.QueryRow(ctx, `SELECT progress_updated_at FROM key_results WHERE id=$1`, copiedKR.ID).Scan(&stamp); err != nil {
		t.Fatalf("select progress_updated_at: %v", err)
	}
	if stamp != nil {
		t.Fatalf("expected the copy to carry no progress timestamp, got %v", stamp)
	}

	// And it behaves like a new KR: the edited start value is what reads report.
	if err := krRepo.UpsertNumericalMeta(ctx, copyScope, krs.NumericalMetaInput{KeyResultID: copiedKR.ID, StartValue: 20, TargetValue: 100, Unit: "%"}); err != nil {
		t.Fatalf("UpsertNumericalMeta (copy edit): %v", err)
	}
	after, err := repo.GetGoal(ctx, copyScope, newID)
	if err != nil {
		t.Fatalf("GetGoal (after edit): %v", err)
	}
	if got := after.KeyResults[0].Numerical.CurrentValue; got != 20 {
		t.Fatalf("expected the edited start value 20, got %v", got)
	}
}
