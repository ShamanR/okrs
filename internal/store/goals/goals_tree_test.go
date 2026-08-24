package goals_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/testutil"
)

func TestListGoalsForPeriods(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))

	mustTeam := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ($1,'unit') RETURNING id`, name).Scan(&id); err != nil {
			t.Fatalf("team: %v", err)
		}
		return id
	}
	mustPeriod := func(name, start, end string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ($1,$2,$3) RETURNING id`, name, start, end).Scan(&id); err != nil {
			t.Fatalf("period: %v", err)
		}
		return id
	}
	teamA := mustTeam("A")
	teamB := mustTeam("B")
	pA := mustPeriod("2026", "2026-01-01", "2026-12-31")
	pB := mustPeriod("Q1", "2026-01-01", "2026-03-31")

	mkGoal := func(team, period int64, title string) int64 {
		id, err := gr.CreateGoal(ctx, scope, goals.GoalInput{
			TeamID: team, PeriodID: period, Title: title,
			Priority: domain.PriorityP1, Weight: 100,
			WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
		})
		if err != nil {
			t.Fatalf("goal: %v", err)
		}
		return id
	}
	gA := mkGoal(teamA, pA, "annual")
	gB := mkGoal(teamB, pB, "quarter")
	_ = gB

	// Admin (adminAll=true): оба периода → 2 цели.
	all, err := gr.ListGoalsForPeriods(ctx, scope, []int64{pA, pB}, nil, true)
	if err != nil {
		t.Fatalf("ListGoalsForPeriods admin: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("admin all periods = %d goals, want 2", len(all))
	}

	// Scope только teamA → только gA.
	scoped, err := gr.ListGoalsForPeriods(ctx, scope, []int64{pA, pB}, []int64{teamA}, false)
	if err != nil {
		t.Fatalf("ListGoalsForPeriods scoped: %v", err)
	}
	if len(scoped) != 1 || scoped[0].ID != gA {
		t.Fatalf("scoped = %+v, want [%d]", scoped, gA)
	}

	// Один период pB → только gB.
	onlyB, err := gr.ListGoalsForPeriods(ctx, scope, []int64{pB}, nil, true)
	if err != nil {
		t.Fatalf("ListGoalsForPeriods onlyB: %v", err)
	}
	if len(onlyB) != 1 || onlyB[0].ID != gB {
		t.Fatalf("onlyB = %+v, want [%d]", onlyB, gB)
	}

	// Пустой набор периодов → пусто.
	empty, err := gr.ListGoalsForPeriods(ctx, scope, nil, nil, true)
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty periods = %d goals, want 0", len(empty))
	}
}
