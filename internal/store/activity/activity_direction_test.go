package activity_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/activity"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

// Лента отдаёт ТЕКУЩЕЕ направление цели события тем же запросом страницы: после смены
// направления старые записи показывают новое, у удалённой цели и у события без цели
// направления нет.
func TestListReturnsCurrentGoalDirection(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	ar := activity.NewActivityRepository(pool)

	teamID, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Платформа", Type: domain.TeamTypeTeam})
	periodID, _ := pr.CreatePeriod(ctx, scope, q1("Q1"))
	kept := makeGoal(t, ctx, pool, scope, teamID, periodID, "Живая цель") // направление TECH
	deleted := makeGoal(t, ctx, pool, scope, teamID, periodID, "Удалённая цель")

	record := func(action domain.ActivityAction, goalID *int64, title string) int64 {
		t.Helper()
		id, err := ar.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: seedUserID, Category: domain.ActivityComposition, Action: action,
			TeamID: &teamID, PeriodID: &periodID, GoalID: goalID, EntityTitle: title,
		})
		if err != nil {
			t.Fatalf("record %s: %v", title, err)
		}
		return id
	}
	keptEv := record(domain.ActionGoalCreated, &kept, "Живая цель")
	deletedEv := record(domain.ActionGoalCreated, &deleted, "Удалённая цель")
	noGoalEv := record(domain.ActionStatusChanged, nil, "Платформа")

	directions := func() map[int64]*domain.GoalDirection {
		t.Helper()
		evs, _, err := ar.List(ctx, scope, nil, activity.ListFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		out := map[int64]*domain.GoalDirection{}
		for _, ev := range evs {
			out[ev.ID] = ev.GoalDirection
		}
		return out
	}
	is := func(got *domain.GoalDirection, want domain.GoalDirection) bool { return got != nil && *got == want }

	got := directions()
	if !is(got[keptEv], domain.DirectionTech) || !is(got[deletedEv], domain.DirectionTech) {
		t.Fatalf("before changes: kept=%v deleted=%v, want TECH for both", got[keptEv], got[deletedEv])
	}
	if got[noGoalEv] != nil {
		t.Fatalf("event without goal: direction = %v, want nil", *got[noGoalEv])
	}

	if err := gr.UpdateGoal(ctx, scope, goals.GoalUpdateInput{
		ID: kept, Title: "Живая цель", Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, Direction: domain.DirectionProcess,
	}); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM goals WHERE id = $1`, deleted); err != nil {
		t.Fatalf("delete goal: %v", err)
	}

	got = directions()
	if !is(got[keptEv], domain.DirectionProcess) {
		t.Fatalf("after direction change: %v, want PROCESS", got[keptEv])
	}
	if got[deletedEv] != nil {
		t.Fatalf("deleted goal: direction = %v, want nil", *got[deletedEv])
	}
}
