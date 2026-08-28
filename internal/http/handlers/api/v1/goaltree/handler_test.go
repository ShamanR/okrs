package goaltree_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/store"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	storetestutil "okrs/internal/store/testutil"
	"okrs/internal/store/users"

	"github.com/jackc/pgx/v5/pgxpool"
)

type treeResp struct {
	Periods []struct {
		ID    int64 `json:"id"`
		Depth int   `json:"depth"`
	} `json:"periods"`
	Teams []struct {
		ID      int64 `json:"id"`
		LedByMe bool  `json:"led_by_me"`
	} `json:"teams"`
	Goals []struct {
		ID            int64   `json:"id"`
		PeriodID      int64   `json:"period_id"`
		TeamID        int64   `json:"team_id"`
		ParentGoalIDs []int64 `json:"parent_goal_ids"`
		ChildGoalIDs  []int64 `json:"child_goal_ids"`
	} `json:"goals"`
}

func setup(t *testing.T) (*pgxpool.Pool, *store.Store, func()) {
	t.Helper()
	pool, cleanup := storetestutil.SetupDB(t)
	repo := store.New(pool)
	return pool, repo, cleanup
}

func TestGoalTree_Contract(t *testing.T) {
	pool, repo, teardown := setup(t)
	defer teardown()
	ctx := t.Context()
	scope := domain.TenantScope{TenantID: 1}

	leadUser, err := repo.Users.UpsertUser(ctx, users.UpsertUserInput{
		ProviderSubjectKey: "test:goaltree-lead", Provider: "test", Subject: "goaltree-lead",
		DisplayName: "Lead User",
	})
	if err != nil {
		t.Fatalf("upsert lead user: %v", err)
	}
	leadUDID := leadUser.UDID
	var teamA, teamB int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, lead_udid) VALUES ('A','unit',$1) RETURNING id`, leadUDID).Scan(&teamA); err != nil {
		t.Fatalf("teamA: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('B','unit') RETURNING id`).Scan(&teamB); err != nil {
		t.Fatalf("teamB: %v", err)
	}
	var pAnnual, pQuarter int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('2026','2026-01-01','2026-12-31') RETURNING id`).Scan(&pAnnual); err != nil {
		t.Fatalf("pAnnual: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date) VALUES ('Q1','2026-01-01','2026-03-31') RETURNING id`).Scan(&pQuarter); err != nil {
		t.Fatalf("pQuarter: %v", err)
	}
	mkGoal := func(team, period int64, title string) int64 {
		id, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
			TeamID: team, PeriodID: period, Title: title,
			Priority: domain.PriorityP1, Weight: 100,
			WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
		})
		if err != nil {
			t.Fatalf("goal %s: %v", title, err)
		}
		return id
	}
	annualGoal := mkGoal(teamA, pAnnual, "annual")    // родитель, teamA (я лид)
	quarterGoal := mkGoal(teamB, pQuarter, "quarter") // ребёнок, teamB

	gc := grants.NewGrantsCache(repo.Grants)

	// Связь: quarter → annual (ребёнок ссылается на родителя).
	adminSrv := httptest.NewServer(testutil.NewAPIV1RouterWithScope(t, repo, gc, nil))
	defer adminSrv.Close()
	body := fmt.Sprintf(`{"parent_goal_ids":[%d]}`, annualGoal)
	if resp, _ := http.Post(fmt.Sprintf("%s/api/v1/goals/%d/links", adminSrv.URL, quarterGoal), "application/json", jsonBody(body)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("link status = %d, want 204", resp.StatusCode)
	}

	// cross_period=1 (все периоды): 2 цели, ребро между ними, оба периода с depth.
	var tr treeResp
	getJSON(t, adminSrv.URL+"/api/v1/goal-tree?cross_period=1", &tr)
	if len(tr.Goals) != 2 {
		t.Fatalf("cross_period=1 goals = %d, want 2", len(tr.Goals))
	}
	child := findGoal(t, tr, quarterGoal)
	if len(child.ParentGoalIDs) != 1 || child.ParentGoalIDs[0] != annualGoal {
		t.Fatalf("child.parent_goal_ids = %v, want [%d]", child.ParentGoalIDs, annualGoal)
	}
	parent := findGoal(t, tr, annualGoal)
	if len(parent.ChildGoalIDs) != 1 || parent.ChildGoalIDs[0] != quarterGoal {
		t.Fatalf("parent.child_goal_ids = %v, want [%d]", parent.ChildGoalIDs, quarterGoal)
	}

	// cross_period=0 c period_id=quarter: только quarter-цель; ребро вверх обрезано (родитель вне набора).
	var trOne treeResp
	getJSON(t, fmt.Sprintf("%s/api/v1/goal-tree?period_id=%d&cross_period=0", adminSrv.URL, pQuarter), &trOne)
	if len(trOne.Goals) != 1 || trOne.Goals[0].ID != quarterGoal {
		t.Fatalf("cross_period=0 goals = %+v, want [%d]", trOne.Goals, quarterGoal)
	}
	if len(trOne.Goals[0].ParentGoalIDs) != 0 {
		t.Fatalf("cross_period=0 parent edges = %v, want [] (parent out of set)", trOne.Goals[0].ParentGoalIDs)
	}

	// Scope только teamB: annual-цель (teamA) недоступна → её нет, ребро вверх обрезано.
	scopedSrv := httptest.NewServer(testutil.NewAPIV1RouterWithScope(t, repo, gc, []int64{teamB}))
	defer scopedSrv.Close()
	var trScoped treeResp
	getJSON(t, scopedSrv.URL+"/api/v1/goal-tree?cross_period=1", &trScoped)
	if len(trScoped.Goals) != 1 || trScoped.Goals[0].ID != quarterGoal {
		t.Fatalf("scoped goals = %+v, want [%d]", trScoped.Goals, quarterGoal)
	}
	if len(trScoped.Goals[0].ParentGoalIDs) != 0 {
		t.Fatalf("scoped parent edges = %v, want [] (parent team out of scope)", trScoped.Goals[0].ParentGoalIDs)
	}

	// led_by_me: как лид teamA (leadUDID) — teamA.led_by_me=true, teamB=false.
	leadSrv := httptest.NewServer(testutil.NewAPIV1RouterWithUser(t, repo, gc, nil, &domain.User{ID: 3, UDID: leadUDID}))
	defer leadSrv.Close()
	var trLead treeResp
	getJSON(t, leadSrv.URL+"/api/v1/goal-tree?cross_period=1", &trLead)
	for _, tm := range trLead.Teams {
		if tm.ID == teamA && !tm.LedByMe {
			t.Fatalf("teamA led_by_me = false, want true")
		}
		if tm.ID == teamB && tm.LedByMe {
			t.Fatalf("teamB led_by_me = true, want false")
		}
	}
}

// TestGoalTree_TenantIsolation убеждается, что цели/команды другого тенанта не попадают
// в ответ /api/v1/goal-tree, даже несмотря на cross_period=1 и admin-scope (allowed=nil).
func TestGoalTree_TenantIsolation(t *testing.T) {
	pool, repo, teardown := setup(t)
	defer teardown()
	ctx := t.Context()
	scope2 := domain.TenantScope{TenantID: 2}

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	var team2 int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, tenant_id) VALUES ('Other Tenant Team','unit',2) RETURNING id`).Scan(&team2); err != nil {
		t.Fatalf("team2: %v", err)
	}
	var period2 int64
	if err := pool.QueryRow(ctx, `INSERT INTO periods (name, start_date, end_date, tenant_id) VALUES ('2026-t2','2026-01-01','2026-12-31',2) RETURNING id`).Scan(&period2); err != nil {
		t.Fatalf("period2: %v", err)
	}
	goal2, err := repo.Goals.CreateGoal(ctx, scope2, goals.GoalInput{
		TeamID: team2, PeriodID: period2, Title: "tenant2 goal",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("goal2: %v", err)
	}

	gc := grants.NewGrantsCache(repo.Grants)
	// Роутер жёстко фиксирует tenant=1 (см. testutil.NewAPIV1RouterWithScope); allowed=nil → admin-scope.
	adminSrv := httptest.NewServer(testutil.NewAPIV1RouterWithScope(t, repo, gc, nil))
	defer adminSrv.Close()

	var tr treeResp
	getJSON(t, adminSrv.URL+"/api/v1/goal-tree?cross_period=1", &tr)
	for _, g := range tr.Goals {
		if g.ID == goal2 {
			t.Fatalf("goal-tree (tenant 1) leaked tenant-2 goal %d", goal2)
		}
	}
	for _, tm := range tr.Teams {
		if tm.ID == team2 {
			t.Fatalf("goal-tree (tenant 1) leaked tenant-2 team %d", team2)
		}
	}
}

func jsonBody(s string) *bytes.Reader { return bytes.NewReader([]byte(s)) }
func getJSON(t *testing.T, url string, dst any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}
func findGoal(t *testing.T, tr treeResp, id int64) struct {
	ID            int64   `json:"id"`
	PeriodID      int64   `json:"period_id"`
	TeamID        int64   `json:"team_id"`
	ParentGoalIDs []int64 `json:"parent_goal_ids"`
	ChildGoalIDs  []int64 `json:"child_goal_ids"`
} {
	t.Helper()
	for _, g := range tr.Goals {
		if g.ID == id {
			return g
		}
	}
	t.Fatalf("goal %d not found in %+v", id, tr.Goals)
	return tr.Goals[0]
}
