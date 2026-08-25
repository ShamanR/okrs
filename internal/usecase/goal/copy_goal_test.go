package goal

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
)

func TestCopyGoalCopyFlipsTargetStatusAndRecordsCopied(t *testing.T) {
	gf := servicetest.NewGoalStore()
	gf.Goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	// target (team 2, period 200) has no status → NoGoals → should flip to Forming.
	svc := newGoalTestService(gf)

	newID, err := svc.Copy(context.Background(), domain.TenantScope{TenantID: 1}, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeCopy,
	}, 7)
	if err != nil {
		t.Fatalf("CopyGoal: %v", err)
	}
	if newID == 0 {
		t.Fatal("expected new id")
	}
	if len(gf.CopyGoalCalls) != 1 || gf.CopyGoalCalls[0].TargetTeamID != 2 || gf.CopyGoalCalls[0].TargetPeriodID != 200 {
		t.Fatalf("CopyGoal store not called correctly: %+v", gf.CopyGoalCalls)
	}
	// no source delete on copy
	if len(gf.DeleteGoalCalls) != 0 {
		t.Fatalf("copy must not delete source, got %v", gf.DeleteGoalCalls)
	}
	// status flip to forming on target
	flipped := false
	for _, c := range gf.SetStatusCalls {
		if c.TeamID == 2 && c.PeriodID == 200 && c.Status == domain.TeamPeriodStatusForming {
			flipped = true
		}
	}
	if !flipped {
		t.Fatalf("expected target status flip to forming, calls=%+v", gf.SetStatusCalls)
	}
}

func TestCopyGoalRejectsClosedTarget(t *testing.T) {
	gf := servicetest.NewGoalStore()
	gf.Goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	gf.Statuses[[2]int64{2, 200}] = domain.TeamPeriodStatusInProgress
	svc := newGoalTestService(gf)

	_, err := svc.Copy(context.Background(), domain.TenantScope{TenantID: 1}, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeCopy,
	}, 7)
	if !errors.Is(err, domain.ErrPeriodClosed) {
		t.Fatalf("expected domain.ErrPeriodClosed, got %v", err)
	}
	if len(gf.CopyGoalCalls) != 0 {
		t.Fatal("must not copy into a closed/in-progress target")
	}
}

func TestCopyGoalMoveDeletesSourceAndRejectsSamePair(t *testing.T) {
	gf := servicetest.NewGoalStore()
	gf.Goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	svc := newGoalTestService(gf)
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	// same pair rejected
	if _, err := svc.Copy(ctx, scope, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 1, TargetPeriodID: 100, Mode: CopyGoalModeMove,
	}, 7); !errors.Is(err, domain.ErrTransferTargetSameAsSource) {
		t.Fatalf("expected domain.ErrTransferTargetSameAsSource, got %v", err)
	}

	// real move: copy+delete are one store call (DeleteSource), not a separate DeleteGoal.
	if _, err := svc.Copy(ctx, scope, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeMove,
	}, 7); err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(gf.CopyGoalCalls) != 1 || !gf.CopyGoalCalls[0].DeleteSource {
		t.Fatalf("move must delete source atomically via CopyGoal.DeleteSource, got %+v", gf.CopyGoalCalls)
	}
	if len(gf.DeleteGoalCalls) != 0 {
		t.Fatalf("move must not issue a separate DeleteGoal, got %v", gf.DeleteGoalCalls)
	}
}

func TestCopyGoalRejectsTargetOutsideTenant(t *testing.T) {
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	// Target team not in tenant.
	gf := servicetest.NewGoalStore()
	gf.Goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	gf.MissingTeams = map[int64]bool{2: true}
	if _, err := newGoalTestService(gf).Copy(ctx, scope, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeCopy,
	}, 7); !errors.Is(err, domain.ErrTransferTargetNotFound) {
		t.Fatalf("expected domain.ErrTransferTargetNotFound for cross-tenant team, got %v", err)
	}

	// Target period not in tenant.
	gf2 := servicetest.NewGoalStore()
	gf2.Goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 100, Title: "G"}
	gf2.MissingPeriods = map[int64]bool{200: true}
	if _, err := newGoalTestService(gf2).Copy(ctx, scope, CopyGoalParams{
		SourceGoalID: 10, TargetTeamID: 2, TargetPeriodID: 200, Mode: CopyGoalModeCopy,
	}, 7); !errors.Is(err, domain.ErrTransferTargetNotFound) {
		t.Fatalf("expected domain.ErrTransferTargetNotFound for cross-tenant period, got %v", err)
	}
	if len(gf2.CopyGoalCalls) != 0 {
		t.Fatal("must not copy when the target period is invalid")
	}
}
