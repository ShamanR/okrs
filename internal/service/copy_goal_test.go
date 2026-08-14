package service

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
)

func TestCopyGoalCopyFlipsTargetStatusAndRecordsCopied(t *testing.T) {
	gf := newGoalFakeStore()
	gf.goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	// target (team 2, period 200) has no status → NoGoals → should flip to Forming.
	svc := newGoalTestService(gf)

	newID, err := svc.CopyGoal(context.Background(), domain.TenantScope{TenantID: 1}, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeCopy,
	}, 7)
	if err != nil {
		t.Fatalf("CopyGoal: %v", err)
	}
	if newID == 0 {
		t.Fatal("expected new id")
	}
	if len(gf.copyGoalCalls) != 1 || gf.copyGoalCalls[0].TargetTeamID != 2 || gf.copyGoalCalls[0].TargetPeriodID != 200 {
		t.Fatalf("CopyGoal store not called correctly: %+v", gf.copyGoalCalls)
	}
	// no source delete on copy
	if len(gf.deleteGoalCalls) != 0 {
		t.Fatalf("copy must not delete source, got %v", gf.deleteGoalCalls)
	}
	// status flip to forming on target
	flipped := false
	for _, c := range gf.setStatusCalls {
		if c.teamID == 2 && c.periodID == 200 && c.status == domain.TeamPeriodStatusForming {
			flipped = true
		}
	}
	if !flipped {
		t.Fatalf("expected target status flip to forming, calls=%+v", gf.setStatusCalls)
	}
}

func TestCopyGoalRejectsClosedTarget(t *testing.T) {
	gf := newGoalFakeStore()
	gf.goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	gf.statuses[[2]int64{2, 200}] = domain.TeamPeriodStatusInProgress
	svc := newGoalTestService(gf)

	_, err := svc.CopyGoal(context.Background(), domain.TenantScope{TenantID: 1}, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeCopy,
	}, 7)
	if !errors.Is(err, ErrPeriodClosed) {
		t.Fatalf("expected ErrPeriodClosed, got %v", err)
	}
	if len(gf.copyGoalCalls) != 0 {
		t.Fatal("must not copy into a closed/in-progress target")
	}
}

func TestCopyGoalMoveDeletesSourceAndRejectsSamePair(t *testing.T) {
	gf := newGoalFakeStore()
	gf.goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	svc := newGoalTestService(gf)
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	// same pair rejected
	if _, err := svc.CopyGoal(ctx, scope, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 1, TargetPeriodID: 100, Mode: CopyGoalModeMove,
	}, 7); !errors.Is(err, ErrTransferTargetSameAsSource) {
		t.Fatalf("expected ErrTransferTargetSameAsSource, got %v", err)
	}

	// real move deletes source
	if _, err := svc.CopyGoal(ctx, scope, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeMove,
	}, 7); err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(gf.deleteGoalCalls) != 1 || gf.deleteGoalCalls[0] != 10 {
		t.Fatalf("move must hard-delete source, got %v", gf.deleteGoalCalls)
	}
}
