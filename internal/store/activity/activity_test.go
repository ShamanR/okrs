package activity_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/activity"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/shares"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

const seedUserID = int64(1)

func q1(name string) periods.PeriodInput {
	return periods.PeriodInput{
		Name:      name,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	}
}

func makeGoal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, scope domain.TenantScope, teamID, periodID int64, title string) int64 {
	t.Helper()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	id, err := gr.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: title, Priority: domain.PriorityP1,
		Weight: 100, WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal (%s): %v", title, err)
	}
	return id
}

func TestRecordAndGetByID(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	ar := activity.NewActivityRepository(pool)

	teamID, err := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Платформа", Type: domain.TeamTypeTeam})
	if err != nil {
		t.Fatalf("team: %v", err)
	}
	periodID, err := pr.CreatePeriod(ctx, scope, q1("Q1"))
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	goalID := makeGoal(t, ctx, pool, scope, teamID, periodID, "P95 latency")

	id, err := ar.Record(ctx, scope, domain.ActivityEvent{
		ActorUserID: seedUserID, Category: domain.ActivityProgress, Action: domain.ActionKRProgress,
		TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, EntityTitle: "P95 latency",
		Payload: map[string]any{"before": map[string]any{"progress": 40}, "after": map[string]any{"progress": 61}},
	})
	if err != nil || id == 0 {
		t.Fatalf("record: id=%d err=%v", id, err)
	}
	got, err := ar.GetByID(ctx, scope, id)
	if err != nil {
		t.Fatalf("getByID: %v", err)
	}
	if got.Action != domain.ActionKRProgress || got.EntityTitle != "P95 latency" {
		t.Fatalf("unexpected event: %+v", got)
	}
	if got.Payload["after"].(map[string]any)["progress"].(float64) != 61 {
		t.Fatalf("payload roundtrip: %+v", got.Payload)
	}
	if _, err := ar.GetByID(ctx, domain.TenantScope{TenantID: 999}, id); err != activity.ErrNotFound {
		t.Fatalf("cross-tenant read: want ErrNotFound, got %v", err)
	}
}

func TestListShareAwareAndScope(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	sr := shares.NewGoalShareRepository(pool)
	ar := activity.NewActivityRepository(pool)

	ownerTeam, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Owner", Type: domain.TeamTypeTeam})
	shareTeam, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Sharee", Type: domain.TeamTypeTeam})
	otherTeam, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Other", Type: domain.TeamTypeTeam})
	periodID, _ := pr.CreatePeriod(ctx, scope, q1("Q1"))
	goalID := makeGoal(t, ctx, pool, scope, ownerTeam, periodID, "Shared goal")
	if err := sr.ReplaceGoalShares(ctx, scope, goalID, []shares.GoalShareInput{{TeamID: shareTeam, Weight: 50}}); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityDiscussion, Action: domain.ActionCommentAdded, TeamID: &ownerTeam, PeriodID: &periodID, GoalID: &goalID, EntityTitle: "Shared goal"}); err != nil {
		t.Fatalf("record: %v", err)
	}

	if evs, _, _ := ar.List(ctx, scope, nil, activity.ListFilter{}); len(evs) != 1 {
		t.Fatalf("admin list: want 1 got %d", len(evs))
	}
	if evs, _, _ := ar.List(ctx, scope, []int64{shareTeam}, activity.ListFilter{}); len(evs) != 1 {
		t.Fatalf("sharee list: want 1 got %d", len(evs))
	}
	if evs, _, _ := ar.List(ctx, scope, []int64{otherTeam}, activity.ListFilter{}); len(evs) != 0 {
		t.Fatalf("other list: want 0 got %d", len(evs))
	}
	if evs, _, _ := ar.List(ctx, scope, []int64{}, activity.ListFilter{}); len(evs) != 0 {
		t.Fatalf("empty list: want 0 got %d", len(evs))
	}
	wrong := periodID + 12345
	if evs, _, _ := ar.List(ctx, scope, nil, activity.ListFilter{PeriodID: &wrong}); len(evs) != 0 {
		t.Fatalf("wrong period: want 0 got %d", len(evs))
	}
}

func TestListTargetTeamResolution(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	sr := shares.NewGoalShareRepository(pool)
	ar := activity.NewActivityRepository(pool)

	owner, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Owner", Type: domain.TeamTypeTeam})
	sharee, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Sharee", Type: domain.TeamTypeTeam})
	periodID, _ := pr.CreatePeriod(ctx, scope, q1("Q1"))
	goalID := makeGoal(t, ctx, pool, scope, owner, periodID, "Shared goal")
	_ = sr.ReplaceGoalShares(ctx, scope, goalID, []shares.GoalShareInput{{TeamID: sharee, Weight: 50}})
	_, _ = ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityDiscussion, Action: domain.ActionCommentAdded, TeamID: &owner, PeriodID: &periodID, GoalID: &goalID})

	// Viewer accesses only the sharee team → target must be the sharee team, not the owner.
	evs, _, _ := ar.List(ctx, scope, []int64{sharee}, activity.ListFilter{})
	if len(evs) != 1 || evs[0].TargetTeamID == nil || *evs[0].TargetTeamID != sharee {
		t.Fatalf("sharee-only viewer target: want %d, got %+v", sharee, evs)
	}
	// Viewer accesses the owner team → target is the owner.
	evs, _, _ = ar.List(ctx, scope, []int64{owner}, activity.ListFilter{})
	if len(evs) != 1 || evs[0].TargetTeamID == nil || *evs[0].TargetTeamID != owner {
		t.Fatalf("owner viewer target: want %d, got %+v", owner, evs)
	}
	// Admin (nil) → owner team.
	evs, _, _ = ar.List(ctx, scope, nil, activity.ListFilter{})
	if len(evs) != 1 || evs[0].TargetTeamID == nil || *evs[0].TargetTeamID != owner {
		t.Fatalf("admin target: want %d, got %+v", owner, evs)
	}
}

func TestListCursorPagination(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	tr := teams.NewTeamRepository(pool)
	ar := activity.NewActivityRepository(pool)
	team, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "T", Type: domain.TeamTypeTeam})
	for i := 0; i < 5; i++ {
		if _, err := ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &team}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	page1, cur, _ := ar.List(ctx, scope, nil, activity.ListFilter{Limit: 2})
	if len(page1) != 2 || cur == nil {
		t.Fatalf("page1: n=%d cur=%v", len(page1), cur)
	}
	page2, _, _ := ar.List(ctx, scope, nil, activity.ListFilter{Limit: 2, Cursor: cur})
	if len(page2) != 2 || page2[0].ID >= page1[1].ID {
		t.Fatalf("page2 not older than page1: %+v %+v", page1, page2)
	}
}

func TestCategoryCounts(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	ar := activity.NewActivityRepository(pool)
	team, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "T", Type: domain.TeamTypeTeam})
	periodID, _ := pr.CreatePeriod(ctx, scope, q1("Q1"))
	rec := func(cat domain.ActivityCategory, act domain.ActivityAction) {
		if _, err := ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: cat, Action: act, TeamID: &team, PeriodID: &periodID}); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	rec(domain.ActivityProgress, domain.ActionKRProgress)
	rec(domain.ActivityProgress, domain.ActionKRProgress)
	rec(domain.ActivityStatus, domain.ActionStatusChanged)

	// A category filter in the input is ignored — counts cover all categories for the other filters.
	counts, err := ar.CategoryCounts(ctx, scope, nil, activity.ListFilter{PeriodID: &periodID, Category: "status"})
	if err != nil {
		t.Fatalf("category counts: %v", err)
	}
	if counts["progress"] != 2 || counts["status"] != 1 {
		t.Fatalf("counts wrong (want progress=2 status=1): %+v", counts)
	}
}

func TestTreeCountsAudience(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	sr := shares.NewGoalShareRepository(pool)
	ar := activity.NewActivityRepository(pool)

	owner, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Owner", Type: domain.TeamTypeTeam})
	sharee, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "Sharee", Type: domain.TeamTypeTeam})
	periodID, _ := pr.CreatePeriod(ctx, scope, q1("Q1"))
	goalID := makeGoal(t, ctx, pool, scope, owner, periodID, "G")
	_ = sr.ReplaceGoalShares(ctx, scope, goalID, []shares.GoalShareInput{{TeamID: sharee, Weight: 50}})
	_, _ = ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityProgress, Action: domain.ActionKRProgress, TeamID: &owner, PeriodID: &periodID, GoalID: &goalID})
	_, _ = ar.Record(ctx, scope, domain.ActivityEvent{ActorUserID: seedUserID, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &owner, PeriodID: &periodID})

	counts, err := ar.TreeCounts(ctx, scope, nil, &periodID, nil)
	if err != nil {
		t.Fatalf("treecounts: %v", err)
	}
	if counts[owner] != 2 {
		t.Fatalf("owner count: want 2 got %d", counts[owner])
	}
	if counts[sharee] != 1 {
		t.Fatalf("sharee count: want 1 got %d", counts[sharee])
	}
	restricted, _ := ar.TreeCounts(ctx, scope, []int64{sharee}, &periodID, nil)
	if restricted[owner] != 0 || restricted[sharee] != 1 {
		t.Fatalf("restricted counts: %+v", restricted)
	}
}

func TestRecordBatch(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	tr := teams.NewTeamRepository(pool)
	pr := periods.NewPeriodRepository(pool)
	ar := activity.NewActivityRepository(pool)

	t1, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "A", Type: domain.TeamTypeTeam})
	t2, _ := tr.CreateTeam(ctx, scope, teams.TeamInput{Name: "B", Type: domain.TeamTypeTeam})
	periodID, _ := pr.CreatePeriod(ctx, scope, q1("BatchP"))

	evs := []domain.ActivityEvent{
		{ActorUserID: seedUserID, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &t1, PeriodID: &periodID, EntityTitle: "A", Payload: map[string]any{"after": map[string]any{"status": "in_progress"}}},
		{ActorUserID: seedUserID, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &t2, PeriodID: &periodID, EntityTitle: "B", Payload: map[string]any{"after": map[string]any{"status": "in_progress"}}},
	}
	if err := ar.RecordBatch(ctx, scope, evs); err != nil {
		t.Fatalf("RecordBatch: %v", err)
	}

	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM activity_events WHERE tenant_id=$1 AND period_id=$2 AND action='status_changed'`, scope.TenantID, periodID).Scan(&n)
	if n != 2 {
		t.Fatalf("expected 2 rows, got %d", n)
	}

	// Empty slice is a no-op.
	if err := ar.RecordBatch(ctx, scope, nil); err != nil {
		t.Fatalf("empty must be no-op: %v", err)
	}
}
