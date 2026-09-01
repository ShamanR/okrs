package goal

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	goalsvc "okrs/internal/service/goal"
	goallinksvc "okrs/internal/service/goallink"
	"okrs/internal/service/servicetest"
	"okrs/internal/store/goallinks"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	storeteams "okrs/internal/store/teams"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

type linkFixture struct {
	ctx    context.Context
	svc    *UseCase
	links  *goallinksvc.Service
	pool   *pgxpool.Pool
	tr     *storeteams.TeamRepository
	pr     *periods.PeriodRepository
	gr     *goals.GoalRepository
	scope  domain.TenantScope
	period int64
	bus    *servicetest.FakeBus
}

func newLinkFixture(t *testing.T) (*linkFixture, func()) {
	t.Helper()
	pool, cleanup := testutil.SetupDB(t)
	ctx := context.Background()
	gr := goals.NewGoalRepository(pool, krs.NewKRRepository(pool))
	bus := &servicetest.FakeBus{}
	f := &linkFixture{
		ctx:   ctx,
		pool:  pool,
		tr:    storeteams.NewTeamRepository(pool),
		pr:    periods.NewPeriodRepository(pool),
		gr:    gr,
		scope: domain.TenantScope{TenantID: 1},
		bus:   bus,
		svc: newFromRepos(rawDeps{
			Goals:  gr,
			Links:  goallinks.NewGoalLinkRepository(pool),
			Events: bus,
		}),
		// Чтение связей — операция сервиса сущности, а не сценария: тест проверяет
		// результат SetParents, поэтому читает напрямую через goallink-сервис.
		links: goallinksvc.New(goallinks.NewGoalLinkRepository(pool), goalsvc.New(gr)),
	}
	pid, err := f.pr.CreatePeriod(ctx, f.scope, periods.PeriodInput{
		Name:      "Q1 2026",
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		cleanup()
		t.Fatalf("create period: %v", err)
	}
	f.period = pid
	return f, cleanup
}

func (f *linkFixture) team(t *testing.T, name string) int64 {
	t.Helper()
	id, err := f.tr.CreateTeam(f.ctx, f.scope, storeteams.TeamInput{Name: name, Type: domain.TeamTypeUnit})
	if err != nil {
		t.Fatalf("create team %s: %v", name, err)
	}
	return id
}

func (f *linkFixture) goal(t *testing.T, teamID int64, title string) int64 {
	t.Helper()
	id, err := f.gr.CreateGoal(f.ctx, f.scope, goals.GoalInput{
		TeamID: teamID, PeriodID: f.period, Title: title,
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal %s: %v", title, err)
	}
	return id
}

func TestSetGoalParents_ValidatesAccessAndCycle(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()
	teamA := f.team(t, "A")
	teamB := f.team(t, "B")
	child := f.goal(t, teamA, "child")
	parent := f.goal(t, teamB, "parent")

	// Parent in teamB but caller scoped only to teamA → not accessible.
	if err := f.svc.SetParents(f.ctx, f.scope, []int64{teamA}, false, child, []int64{parent}, 1); err != domain.ErrGoalLinkNotAccessible {
		t.Fatalf("scoped err = %v, want domain.ErrGoalLinkNotAccessible", err)
	}

	// Scope on both → ok.
	if err := f.svc.SetParents(f.ctx, f.scope, []int64{teamA, teamB}, false, child, []int64{parent}, 1); err != nil {
		t.Fatalf("link both: %v", err)
	}

	// Self-link.
	if err := f.svc.SetParents(f.ctx, f.scope, []int64{teamA}, false, child, []int64{child}, 1); err != domain.ErrGoalLinkSelf {
		t.Fatalf("self err = %v, want domain.ErrGoalLinkSelf", err)
	}

	// Cycle: parent -> child (reverse) with scope on both.
	if err := f.svc.SetParents(f.ctx, f.scope, []int64{teamA, teamB}, false, parent, []int64{child}, 1); err != domain.ErrGoalLinkCycle {
		t.Fatalf("cycle err = %v, want domain.ErrGoalLinkCycle", err)
	}
}

func TestSetGoalParents_AdminAllBypassesScope(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()
	teamA := f.team(t, "A")
	teamB := f.team(t, "B")
	child := f.goal(t, teamA, "child")
	parent := f.goal(t, teamB, "parent")

	// adminAll=true → no scope restriction.
	if err := f.svc.SetParents(f.ctx, f.scope, nil, true, child, []int64{parent}, 1); err != nil {
		t.Fatalf("admin link: %v", err)
	}
	parents, _, err := f.links.ListForGoals(f.ctx, f.scope, []int64{child}, nil, true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(parents[child]) != 1 || parents[child][0].ID != parent {
		t.Fatalf("parents[child] = %v, want [%d]", parents[child], parent)
	}
}

func TestSetGoalParents_RecordsActivity(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()
	team := f.team(t, "A")
	child := f.goal(t, team, "child")
	p1 := f.goal(t, team, "p1")

	if err := f.svc.SetParents(f.ctx, f.scope, []int64{team}, false, child, []int64{p1}, 1); err != nil {
		t.Fatalf("link: %v", err)
	}
	// Replace with empty → goal_unlinked.
	if err := f.svc.SetParents(f.ctx, f.scope, []int64{team}, false, child, nil, 1); err != nil {
		t.Fatalf("unlink: %v", err)
	}

	// The usecase publishes; whether the journal turns that into a row is
	// service/activity's job, covered separately (journal_test.go, activity_test.go).
	if n := countKind(f.bus, event.KindGoalLinked); n != 1 {
		t.Fatalf("goal_linked count = %d, want 1: %+v", n, f.bus.Events)
	}
	if n := countKind(f.bus, event.KindGoalUnlinked); n != 1 {
		t.Fatalf("goal_unlinked count = %d, want 1: %+v", n, f.bus.Events)
	}
}

func countKind(bus *servicetest.FakeBus, kind event.Kind) int {
	var n int
	for _, k := range bus.KindsPublished() {
		if k == kind {
			n++
		}
	}
	return n
}
