package goals_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/shares"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

// TestMoveGoalReordersSharedGoals verifies that a shared (common) goal can be
// reordered inside a team's period view, alongside the team's own goals.
//
// A goal shared into a team is ordered by goal_shares.sort_order for that team,
// while the team's own goals use goals.sort_order. Moving must renumber the
// team's visible list regardless of whether each goal is owned or shared.
func TestMoveGoalReordersSharedGoals(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	sr := shares.NewGoalShareRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	receiver, err := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Receiver", Type: domain.TeamTypeTeam})
	if err != nil {
		t.Fatalf("create receiver team: %v", err)
	}
	owner, err := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Owner", Type: domain.TeamTypeTeam})
	if err != nil {
		t.Fatalf("create owner team: %v", err)
	}
	periodID, err := pr.CreatePeriod(ctx, scope, periods.PeriodInput{
		Name:      "2025 Q1",
		StartDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create period: %v", err)
	}

	newGoal := func(teamID int64, title string) int64 {
		id, err := gr.CreateGoal(ctx, scope, goals.GoalInput{
			TeamID: teamID, PeriodID: periodID, Title: title,
			Priority: domain.PriorityP1, Weight: 100,
			WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
		})
		if err != nil {
			t.Fatalf("create goal %s: %v", title, err)
		}
		return id
	}

	goalA := newGoal(receiver, "A") // receiver's own, sort_order 1
	goalB := newGoal(receiver, "B") // receiver's own, sort_order 2
	shared := newGoal(owner, "S")   // owned by another team, sort_order 1

	// Share S into the receiver team. sort_order is copied from the owner goal (1),
	// colliding with the receiver's own goal A.
	if err := sr.ReplaceGoalShares(ctx, scope, shared, []shares.GoalShareInput{{TeamID: receiver, Weight: 100}}); err != nil {
		t.Fatalf("share goal: %v", err)
	}

	order := func() []int64 {
		list, err := gr.ListGoalsByTeamPeriod(ctx, scope, receiver, periodID)
		if err != nil {
			t.Fatalf("list goals: %v", err)
		}
		ids := make([]int64, len(list))
		for i, g := range list {
			ids[i] = g.ID
		}
		return ids
	}

	// Initial order (eff, id): A(1), S(1) tie broken by id, B(2).
	if got := order(); !equalIDs(got, []int64{goalA, shared, goalB}) {
		t.Fatalf("initial order = %v, want [A S B] = %v", got, []int64{goalA, shared, goalB})
	}

	// Move the shared goal up: expected S, A, B.
	if err := gr.MoveGoal(ctx, scope, receiver, shared, -1); err != nil {
		t.Fatalf("move shared up: %v", err)
	}
	if got := order(); !equalIDs(got, []int64{shared, goalA, goalB}) {
		t.Fatalf("after move-up order = %v, want [S A B] = %v", got, []int64{shared, goalA, goalB})
	}

	// Move the shared goal down twice: expected A, B, S.
	if err := gr.MoveGoal(ctx, scope, receiver, shared, 1); err != nil {
		t.Fatalf("move shared down: %v", err)
	}
	if err := gr.MoveGoal(ctx, scope, receiver, shared, 1); err != nil {
		t.Fatalf("move shared down again: %v", err)
	}
	if got := order(); !equalIDs(got, []int64{goalA, goalB, shared}) {
		t.Fatalf("after move-down order = %v, want [A B S] = %v", got, []int64{goalA, goalB, shared})
	}

	// The owner team's ordering must be unaffected by reordering inside the receiver.
	ownerList, err := gr.ListGoalsByTeamPeriod(ctx, scope, owner, periodID)
	if err != nil {
		t.Fatalf("list owner goals: %v", err)
	}
	if len(ownerList) != 1 || ownerList[0].ID != shared {
		t.Fatalf("owner view = %+v, want single shared goal %d", ownerList, shared)
	}
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
