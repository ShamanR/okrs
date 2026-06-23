package goals_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

// seedGoal creates a team, period and goal under the given tenant scope, returning the goal id.
func seedGoal(t *testing.T, ctx context.Context, gr *goals.GoalRepository, tr *teams.TeamRepository, pr *periods.PeriodRepository, scope domain.TenantScope, name string) (int64, int64, int64) {
	t.Helper()
	teamID, err := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: name, Type: domain.TeamTypeTeam})
	if err != nil {
		t.Fatalf("create team (%s): %v", name, err)
	}
	periodID, err := pr.CreatePeriod(ctx, scope, periods.PeriodInput{
		Name:      name + " period",
		StartDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create period (%s): %v", name, err)
	}
	goalID, err := gr.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: name + " goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal (%s): %v", name, err)
	}
	return teamID, periodID, goalID
}

func TestGoalsScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	scope1 := domain.TenantScope{TenantID: 1}
	scope2 := domain.TenantScope{TenantID: 2}

	team1, period1, goal1 := seedGoal(t, ctx, gr, tr, pr, scope1, "T1")
	_, _, goal2 := seedGoal(t, ctx, gr, tr, pr, scope2, "T2")

	// Cross-tenant read of a goal is blocked.
	if _, err := gr.GetGoal(ctx, scope2, goal1); err == nil {
		t.Fatalf("scope2 must not read goal of tenant 1")
	}
	if _, err := gr.GetGoal(ctx, scope1, goal2); err == nil {
		t.Fatalf("scope1 must not read goal of tenant 2")
	}

	// Listing tenant-1's team/period under scope2 returns nothing.
	cross, err := gr.ListGoalsByTeamPeriod(ctx, scope2, team1, period1)
	if err != nil {
		t.Fatalf("list cross: %v", err)
	}
	if len(cross) != 0 {
		t.Fatalf("scope2 saw %d goals for tenant-1 team/period, want 0", len(cross))
	}

	// Under its own scope the goal is visible.
	own, err := gr.ListGoalsByTeamPeriod(ctx, scope1, team1, period1)
	if err != nil {
		t.Fatalf("list own: %v", err)
	}
	if len(own) != 1 || own[0].ID != goal1 {
		t.Fatalf("scope1 saw %+v, want goal %d", own, goal1)
	}
}
