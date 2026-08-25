package goal

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
	"okrs/internal/store/goals"
	"okrs/internal/store/shares"
)

// ── servicetest.GoalStore ─────────────────────────────────────────────────────────────

func TestCreateGoalBlockedByClosedPeriod(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Statuses[[2]int64{1, 10}] = domain.TeamPeriodStatusClosed
	svc := newGoalTestService(st)

	_, err := svc.Create(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 1, PeriodID: 10}, 1)
	if err != domain.ErrPeriodClosed {
		t.Fatalf("expected domain.ErrPeriodClosed, got %v", err)
	}
}

func TestCreateGoalBlockedByInProgressPeriod(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Statuses[[2]int64{1, 10}] = domain.TeamPeriodStatusInProgress
	svc := newGoalTestService(st)

	_, err := svc.Create(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 1, PeriodID: 10}, 1)
	if err != domain.ErrPeriodClosed {
		t.Fatalf("expected domain.ErrPeriodClosed for in_progress, got %v", err)
	}
}

func TestCreateGoalAdvancesStatusFromNoGoals(t *testing.T) {
	st := servicetest.NewGoalStore()
	// no entry in statuses → defaults to NoGoals
	svc := newGoalTestService(st)

	goalID, err := svc.Create(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 2, PeriodID: 5}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if goalID == 0 {
		t.Fatal("expected non-zero goal ID")
	}
	if len(st.SetStatusCalls) != 1 {
		t.Fatalf("expected SetTeamPeriodStatus called once, got %d", len(st.SetStatusCalls))
	}
	call := st.SetStatusCalls[0]
	if call.TeamID != 2 || call.PeriodID != 5 || call.Status != domain.TeamPeriodStatusForming {
		t.Fatalf("unexpected SetTeamPeriodStatus call: %+v", call)
	}
}

func TestCreateGoalKeepsStatusWhenAlreadyForming(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Statuses[[2]int64{2, 5}] = domain.TeamPeriodStatusForming
	svc := newGoalTestService(st)

	_, err := svc.Create(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 2, PeriodID: 5}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.SetStatusCalls) != 0 {
		t.Fatalf("expected no SetTeamPeriodStatus call when already Forming, got %d", len(st.SetStatusCalls))
	}
}

// ── DeleteGoal tests ──────────────────────────────────────────────────────────

func TestDeleteGoalBySharedTeamRemovesShareOnly(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Goals[7] = domain.Goal{ID: 7, TeamID: 1, PeriodID: 5}
	svc := newGoalTestService(st)

	effectiveTeam, periodID, err := svc.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 7, 2, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if effectiveTeam != 2 || periodID != 5 {
		t.Fatalf("expected effectiveTeam=2 periodID=5, got %d %d", effectiveTeam, periodID)
	}
	if len(st.DeleteShareCalls) != 1 || st.DeleteShareCalls[0].GoalID != 7 || st.DeleteShareCalls[0].TeamID != 2 {
		t.Fatalf("expected share deletion for goal 7 / team 2, got %+v", st.DeleteShareCalls)
	}
	if len(st.DeleteGoalCalls) != 0 {
		t.Fatal("goal itself should not be deleted when shared team removes share")
	}
}

func TestDeleteGoalByOwnerTransfersOwnershipWhenShared(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Goals[8] = domain.Goal{ID: 8, TeamID: 1, PeriodID: 5, Weight: 50}
	st.GoalShares[8] = []shares.GoalShare{{GoalID: 8, TeamID: 3, Weight: 30}}
	svc := newGoalTestService(st)

	_, _, err := svc.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 8, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Ownership transferred to team 3
	if len(st.UpdateOwnerCalls) != 1 || st.UpdateOwnerCalls[0].TeamID != 3 {
		t.Fatalf("expected ownership transfer to team 3, got %+v", st.UpdateOwnerCalls)
	}
	// Old share for team 3 removed
	if len(st.DeleteShareCalls) != 1 || st.DeleteShareCalls[0].TeamID != 3 {
		t.Fatalf("expected share deletion for team 3, got %+v", st.DeleteShareCalls)
	}
	// Goal itself not deleted
	if len(st.DeleteGoalCalls) != 0 {
		t.Fatal("goal itself should not be deleted when ownership transfers")
	}
}

func TestDeleteGoalByOwnerDeletesGoalWhenNoSharesAndPeriodOpen(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Goals[9] = domain.Goal{ID: 9, TeamID: 1, PeriodID: 5}
	// statuses defaults to NoGoals → open period
	svc := newGoalTestService(st)

	_, _, err := svc.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 9, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.DeleteGoalCalls) != 1 || st.DeleteGoalCalls[0] != 9 {
		t.Fatalf("expected goal 9 to be deleted, got %+v", st.DeleteGoalCalls)
	}
}

func TestDeleteGoalByOwnerBlockedByClosedPeriodWithNoShares(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 5}
	st.Statuses[[2]int64{1, 5}] = domain.TeamPeriodStatusClosed
	svc := newGoalTestService(st)

	_, _, err := svc.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 10, 1, 1)
	if err != domain.ErrPeriodClosed {
		t.Fatalf("expected domain.ErrPeriodClosed, got %v", err)
	}
	if len(st.DeleteGoalCalls) != 0 {
		t.Fatal("goal should not be deleted when period is closed")
	}
}

func TestDeleteGoalResetsStatusWhenLastGoalRemoved(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Goals[11] = domain.Goal{ID: 11, TeamID: 1, PeriodID: 5}
	st.Statuses[[2]int64{1, 5}] = domain.TeamPeriodStatusForming
	// goalsAfterDelete is empty → no goals remain after deletion
	svc := newGoalTestService(st)

	_, _, err := svc.Delete(context.Background(), domain.TenantScope{TenantID: 1}, 11, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// resetStatusIfNoGoals should set status to NoGoals
	var resetCall *servicetest.SetStatusArg
	for i := range st.SetStatusCalls {
		if st.SetStatusCalls[i].Status == domain.TeamPeriodStatusNoGoals {
			resetCall = &st.SetStatusCalls[i]
		}
	}
	if resetCall == nil {
		t.Fatal("expected status reset to NoGoals after last goal deleted")
	}
}

// ── UpdateGoalOwnerAndShares tests ────────────────────────────────────────────

func TestUpdateGoalOwnerAndSharesBlockedByInProgressPeriod(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Goals[20] = domain.Goal{ID: 20, TeamID: 1, PeriodID: 10, Weight: 40}
	// team 2 is in_progress
	st.Statuses[[2]int64{2, 10}] = domain.TeamPeriodStatusInProgress
	svc := newGoalTestService(st)

	_, _, err := svc.UpdateOwnerAndShares(context.Background(), domain.TenantScope{TenantID: 1}, 20, []int64{2}, 1)
	if err != domain.ErrCannotShareWithClosedPeriod {
		t.Fatalf("expected domain.ErrCannotShareWithClosedPeriod, got %v", err)
	}
}

func TestUpdateGoalOwnerAndSharesChangesOwnerWhenCurrentOwnerNotSelected(t *testing.T) {
	st := servicetest.NewGoalStore()
	st.Goals[21] = domain.Goal{ID: 21, TeamID: 1, PeriodID: 10, Weight: 40}
	// team 3 has open period (defaults to NoGoals)
	svc := newGoalTestService(st)

	ownerID, periodID, err := svc.UpdateOwnerAndShares(context.Background(), domain.TenantScope{TenantID: 1}, 21, []int64{3}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ownerID != 3 {
		t.Fatalf("expected new owner 3, got %d", ownerID)
	}
	if periodID != 10 {
		t.Fatalf("expected period 10, got %d", periodID)
	}
	if len(st.UpdateOwnerCalls) != 1 || st.UpdateOwnerCalls[0].TeamID != 3 {
		t.Fatalf("expected UpdateGoalOwner call for team 3, got %+v", st.UpdateOwnerCalls)
	}
}

// ── Unsupported KR kind errors ────────────────────────────────────────────────

// ── CreateKeyResultWithMeta tests ─────────────────────────────────────────────
