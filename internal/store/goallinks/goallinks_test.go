package goallinks_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/goallinks"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

type fixture struct {
	ctx    context.Context
	gr     *goals.GoalRepository
	tr     *teams.TeamRepository
	pr     *periods.PeriodRepository
	repo   *goallinks.GoalLinkRepository
	scope  domain.TenantScope
	period int64
}

func newFixture(t *testing.T) (*fixture, func()) {
	t.Helper()
	pool, cleanup := testutil.SetupDB(t)
	ctx := context.Background()
	f := &fixture{
		ctx:   ctx,
		gr:    goals.NewGoalRepository(pool, krs.NewKRRepository(pool)),
		tr:    teams.NewTeamRepository(pool),
		pr:    periods.NewPeriodRepository(pool),
		repo:  goallinks.NewGoalLinkRepository(pool),
		scope: domain.TenantScope{TenantID: 1},
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

func (f *fixture) team(t *testing.T, name string, typ domain.TeamType) int64 {
	t.Helper()
	id, err := f.tr.CreateTeam(f.ctx, f.scope, teams.TeamInput{Name: name, Type: typ})
	if err != nil {
		t.Fatalf("create team %s: %v", name, err)
	}
	return id
}

func (f *fixture) goal(t *testing.T, teamID, periodID int64, title string) int64 {
	t.Helper()
	id, err := f.gr.CreateGoal(f.ctx, f.scope, goals.GoalInput{
		TeamID: teamID, PeriodID: periodID, Title: title,
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create goal %s: %v", title, err)
	}
	return id
}

func hasID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestReplaceParents_SetAndClear(t *testing.T) {
	f, cleanup := newFixture(t)
	defer cleanup()
	team := f.team(t, "Платформа", domain.TeamTypeTeam)
	child := f.goal(t, team, f.period, "child")
	p1 := f.goal(t, team, f.period, "p1")
	p2 := f.goal(t, team, f.period, "p2")

	added, removed, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, child, []int64{p1, p2})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if len(added) != 2 || !hasID(added, p1) || !hasID(added, p2) {
		t.Fatalf("added = %v, want [p1 p2]", added)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want empty", removed)
	}

	// Replace with {p1}: p2 is removed.
	added, removed, err = f.repo.ReplaceParents(f.ctx, f.scope, nil, true, child, []int64{p1})
	if err != nil {
		t.Fatalf("replace2: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("added = %v, want empty", added)
	}
	if len(removed) != 1 || removed[0] != p2 {
		t.Fatalf("removed = %v, want [p2]", removed)
	}

	// Empty set clears all.
	_, removed, err = f.repo.ReplaceParents(f.ctx, f.scope, nil, true, child, nil)
	if err != nil {
		t.Fatalf("replace3: %v", err)
	}
	if len(removed) != 1 || removed[0] != p1 {
		t.Fatalf("removed = %v, want [p1]", removed)
	}
	parents, _, err := f.repo.ListLinksForGoals(f.ctx, f.scope, []int64{child}, nil, true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(parents[child]) != 0 {
		t.Fatalf("parents after clear = %v, want empty", parents[child])
	}
}

func TestReplaceParents_RejectsCycles(t *testing.T) {
	f, cleanup := newFixture(t)
	defer cleanup()
	team := f.team(t, "Платформа", domain.TeamTypeTeam)
	a := f.goal(t, team, f.period, "A")
	b := f.goal(t, team, f.period, "B")
	c := f.goal(t, team, f.period, "C")

	// A -> B
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, a, []int64{b}); err != nil {
		t.Fatalf("A->B: %v", err)
	}
	// B -> A closes A->B->A
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, b, []int64{a}); err != goallinks.ErrCycle {
		t.Fatalf("B->A err = %v, want ErrCycle", err)
	}
	// Transitive: B -> C ok, then C -> A closes A->B->C->A
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, b, []int64{c}); err != nil {
		t.Fatalf("B->C: %v", err)
	}
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, c, []int64{a}); err != goallinks.ErrCycle {
		t.Fatalf("C->A err = %v, want ErrCycle", err)
	}
	// Self-link.
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, a, []int64{a}); err != goallinks.ErrCycle {
		t.Fatalf("A->A err = %v, want ErrCycle", err)
	}
}

func TestListLinksForGoals_ScopeFiltered(t *testing.T) {
	f, cleanup := newFixture(t)
	defer cleanup()
	teamA := f.team(t, "A", domain.TeamTypeCluster)
	teamB := f.team(t, "B", domain.TeamTypeUnit)
	child := f.goal(t, teamA, f.period, "child")
	parent := f.goal(t, teamB, f.period, "parent")
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, child, []int64{parent}); err != nil {
		t.Fatalf("link: %v", err)
	}

	// Viewer scoped only to teamA: parent (in teamB) is hidden.
	parents, _, err := f.repo.ListLinksForGoals(f.ctx, f.scope, []int64{child}, []int64{teamA}, false)
	if err != nil {
		t.Fatalf("list scoped: %v", err)
	}
	if len(parents[child]) != 0 {
		t.Fatalf("parents[child] = %v, want empty (out of scope)", parents[child])
	}

	// Viewer scoped to both teams: link visible from both sides.
	parents, children, err := f.repo.ListLinksForGoals(f.ctx, f.scope, []int64{child, parent}, []int64{teamA, teamB}, false)
	if err != nil {
		t.Fatalf("list both: %v", err)
	}
	if len(parents[child]) != 1 || parents[child][0].ID != parent {
		t.Fatalf("parents[child] = %v, want [%d]", parents[child], parent)
	}
	if parents[child][0].TeamName != "B" || parents[child][0].TeamType != string(domain.TeamTypeUnit) {
		t.Fatalf("parent ref team = %q/%q, want B/unit", parents[child][0].TeamName, parents[child][0].TeamType)
	}
	if len(children[parent]) != 1 || children[parent][0].ID != child {
		t.Fatalf("children[parent] = %v, want [%d]", children[parent], child)
	}
}

func TestListLinkable_SearchPeriodExclude(t *testing.T) {
	f, cleanup := newFixture(t)
	defer cleanup()
	teamA := f.team(t, "Платформа", domain.TeamTypeUnit)
	self := f.goal(t, teamA, f.period, "self")
	other := f.goal(t, teamA, f.period, "Снизить Time-to-Deploy")

	// exclude self; search by goal title.
	got, err := f.repo.ListLinkable(f.ctx, f.scope, []int64{teamA}, false, nil, self, "time-to-deploy")
	if err != nil {
		t.Fatalf("linkable: %v", err)
	}
	if len(got) != 1 || got[0].ID != other {
		t.Fatalf("linkable(title) = %v, want [%d]", got, other)
	}

	// search by team name.
	got, err = f.repo.ListLinkable(f.ctx, f.scope, []int64{teamA}, false, nil, self, "платформа")
	if err != nil {
		t.Fatalf("linkable team: %v", err)
	}
	if len(got) < 1 {
		t.Fatalf("linkable(team) returned nothing")
	}
	for _, g := range got {
		if g.ID == self {
			t.Fatalf("linkable must exclude self goal %d", self)
		}
	}

	// period filter that matches nothing.
	missing := f.period + 9999
	got, err = f.repo.ListLinkable(f.ctx, f.scope, []int64{teamA}, false, &missing, self, "")
	if err != nil {
		t.Fatalf("linkable period: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("linkable(period=missing) = %v, want empty", got)
	}
}

// A scoped caller's full-replace must NOT delete parent links whose team is outside their
// scope (they can't see those links). Regression for silent cross-scope data loss.
func TestReplaceParents_PreservesOutOfScopeLinks(t *testing.T) {
	f, cleanup := newFixture(t)
	defer cleanup()
	teamA := f.team(t, "A", domain.TeamTypeUnit)
	teamB := f.team(t, "B", domain.TeamTypeUnit)
	child := f.goal(t, teamA, f.period, "child")
	parentA := f.goal(t, teamA, f.period, "pA")
	parentB := f.goal(t, teamB, f.period, "pB")

	// Admin links child to parents in both teams.
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, child, []int64{parentA, parentB}); err != nil {
		t.Fatalf("admin link: %v", err)
	}

	// Caller scoped to teamA sees only parentA and re-saves just {parentA}. parentB (out of
	// scope) must survive.
	added, removed, err := f.repo.ReplaceParents(f.ctx, f.scope, []int64{teamA}, false, child, []int64{parentA})
	if err != nil {
		t.Fatalf("scoped replace: %v", err)
	}
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("scoped no-op replace: added=%v removed=%v, want empty", added, removed)
	}
	parents, _, err := f.repo.ListLinksForGoals(f.ctx, f.scope, []int64{child}, nil, true)
	if err != nil {
		t.Fatalf("admin list: %v", err)
	}
	got := map[int64]bool{}
	for _, p := range parents[child] {
		got[p.ID] = true
	}
	if !got[parentA] || !got[parentB] {
		t.Fatalf("after scoped replace parents = %v, want both %d and %d", parents[child], parentA, parentB)
	}

	// Scoped caller clears their visible parents (empty set): parentA removed, parentB kept.
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, []int64{teamA}, false, child, nil); err != nil {
		t.Fatalf("scoped clear: %v", err)
	}
	parents, _, err = f.repo.ListLinksForGoals(f.ctx, f.scope, []int64{child}, nil, true)
	if err != nil {
		t.Fatalf("admin list 2: %v", err)
	}
	if len(parents[child]) != 1 || parents[child][0].ID != parentB {
		t.Fatalf("after scoped clear parents = %v, want [%d] (parentB kept)", parents[child], parentB)
	}
}

func TestGoalLinks_TenantIsolationAndCascade(t *testing.T) {
	f, cleanup := newFixture(t)
	defer cleanup()
	team := f.team(t, "Платформа", domain.TeamTypeTeam)
	child := f.goal(t, team, f.period, "child")
	parent := f.goal(t, team, f.period, "parent")
	if _, _, err := f.repo.ReplaceParents(f.ctx, f.scope, nil, true, child, []int64{parent}); err != nil {
		t.Fatalf("link: %v", err)
	}

	// Another tenant sees no links.
	scope2 := domain.TenantScope{TenantID: 2}
	parents, _, err := f.repo.ListLinksForGoals(f.ctx, scope2, []int64{child}, nil, true)
	if err != nil {
		t.Fatalf("list scope2: %v", err)
	}
	if len(parents[child]) != 0 {
		t.Fatalf("scope2 parents = %v, want empty", parents[child])
	}

	// Deleting the parent cascades the link away.
	if err := f.gr.DeleteGoal(f.ctx, f.scope, parent); err != nil {
		t.Fatalf("delete parent: %v", err)
	}
	parents, _, err = f.repo.ListLinksForGoals(f.ctx, f.scope, []int64{child}, nil, true)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(parents[child]) != 0 {
		t.Fatalf("parents after cascade = %v, want empty", parents[child])
	}
}
