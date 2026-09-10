package overview

// Тесты переехали из пакета admin вместе с обработчиком GET /api/v1/periods/{periodID}/overview.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	hcsvc "okrs/internal/service/healthcheckin"
	"okrs/internal/store/grants"
	perioduc "okrs/internal/usecase/period"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// withTenant attaches the default tenant #1 so TenantScopeFromContext returns {1}.
func withTenant(r *http.Request) *http.Request {
	return r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Name: "Acme", Status: domain.TenantActive}))
}

// fakeGrants is an in-memory grantsStore. activeTeamIDs models which granted
// teams are still active; ListDescendantTeamIDs returns only the active roots
// (descendant expansion is irrelevant for the membership test the handler does).
type fakeGrants struct {
	all             map[int64][]grants.HierarchyGrant
	activeTeamIDs   map[int64]bool
	leadScope       map[string][]int64
	leadScopeCalled bool
}

func (f *fakeGrants) ListLeadTeamScope(_ context.Context, _ domain.TenantScope, udid string) ([]int64, error) {
	f.leadScopeCalled = true
	return f.leadScope[udid], nil
}

func (f *fakeGrants) ListUserGrants(context.Context, domain.TenantScope, int64) ([]grants.HierarchyGrant, error) {
	return nil, nil
}

func (f *fakeGrants) AllGrants(context.Context) (map[int64][]grants.HierarchyGrant, error) {
	return f.all, nil
}

func (f *fakeGrants) ListDescendantTeamIDs(_ context.Context, _ domain.TenantScope, roots []int64) ([]int64, error) {
	var out []int64
	for _, id := range roots {
		if f.activeTeamIDs[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeGrants) AddUserGrant(context.Context, domain.TenantScope, int64, int64, int64) error {
	return nil
}

func (f *fakeGrants) RemoveUserGrant(context.Context, domain.TenantScope, int64, int64) error {
	return nil
}

func withUserRole(r *http.Request, udid string, isAdmin bool) *http.Request {
	ctx := auth.WithUser(r.Context(), &domain.User{UDID: udid})
	role := domain.RoleUser
	if isAdmin {
		role = domain.RoleAdmin
	}
	return r.WithContext(auth.WithActiveRole(ctx, role))
}

// withURLParam injects a chi URL param into the request context, mimicking chi's router.
func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestHandlePeriodOverviewScoped_OrgForbiddenForNonAdmin(t *testing.T) {
	h := New(perioduc.New(perioduc.Deps{}), nil, nil, &fakeGrants{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=org", nil)
	req = withURLParam(withTenant(withUserRole(req, "u-1", false)), "periodID", "1")
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin org scope, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestHandlePeriodOverviewScoped_MyTeamsResolvesLeadScope(t *testing.T) {
	fg := &fakeGrants{leadScope: map[string][]int64{"u-1": {10, 11}}}
	h := New(perioduc.New(perioduc.Deps{}), nil, nil, fg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=my_teams", nil)
	req = withURLParam(withTenant(withUserRole(req, "u-1", false)), "periodID", "1")
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !fg.leadScopeCalled {
		t.Fatalf("expected ListLeadTeamScope to be consulted for my_teams")
	}
}

// The overview response is serialized straight from the usecase struct, so this pins
// the wire contract the drill-down links rely on: a KR row must carry goal_id and
// team_id, not only the titles it displays, and its team must be the same team its
// goal is bound to — otherwise the two rows would link to different boards.
func TestHandlePeriodOverview_KRRowsCarryLinkTargets(t *testing.T) {
	goal := domain.Goal{ID: 10, TeamID: 1, Title: "G1", Weight: 100, KeyResults: []domain.KeyResult{{
		ID: 100, Title: "KR1", Kind: domain.KRKindNumerical, Weight: 100,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 40},
	}}}
	data := &hcsvc.PeriodData{
		PeriodID:    1,
		Teams:       []domain.Team{{ID: 1, Name: "T1"}},
		GoalsByTeam: map[int64][]domain.Goal{1: {goal}},
		Statuses:    map[int64]domain.TeamPeriodStatus{1: domain.TeamPeriodStatusInProgress},
		CachedAt:    time.Now(),
	}
	loader := func(context.Context, domain.TenantScope, int64) (*hcsvc.PeriodData, error) { return data, nil }
	h := New(perioduc.New(perioduc.Deps{HCCache: hcsvc.NewCache(loader, time.Minute, nil)}), nil, nil, &fakeGrants{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=org", nil)
	req = withURLParam(withTenant(withUserRole(req, "admin-1", true)), "periodID", "1")
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	var body struct {
		Goals []map[string]json.RawMessage `json:"goals"`
		KRs   []map[string]json.RawMessage `json:"krs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (%s)", err, w.Body.String())
	}
	if len(body.Goals) != 1 || len(body.KRs) != 1 {
		t.Fatalf("expected one goal row and one KR row, got %d and %d (%s)", len(body.Goals), len(body.KRs), w.Body.String())
	}
	for _, key := range []string{"goal_id", "team_id", "goal_title", "team_name"} {
		if _, ok := body.KRs[0][key]; !ok {
			t.Fatalf("KR row is missing %q: %v", key, body.KRs[0])
		}
	}
	if string(body.KRs[0]["goal_id"]) != "10" || string(body.KRs[0]["team_id"]) != "1" {
		t.Fatalf("KR link target wrong: goal_id=%s team_id=%s, want 10 and 1", body.KRs[0]["goal_id"], body.KRs[0]["team_id"])
	}
	if string(body.KRs[0]["team_id"]) != string(body.Goals[0]["team_id"]) {
		t.Fatalf("KR team_id %s must match its goal team_id %s", body.KRs[0]["team_id"], body.Goals[0]["team_id"])
	}
}

func TestHandlePeriodOverviewScoped_OrgAllowedForAdmin(t *testing.T) {
	h := New(perioduc.New(perioduc.Deps{}), nil, nil, &fakeGrants{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=org", nil)
	req = withURLParam(withTenant(withUserRole(req, "admin-1", true)), "periodID", "1")
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin org scope, got %d (%s)", w.Code, w.Body.String())
	}
}
