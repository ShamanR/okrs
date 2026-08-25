package overview

// Тесты переехали из пакета admin вместе с обработчиком GET /api/v1/periods/{periodID}/overview.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/store/grants"
	perioduc "okrs/internal/usecase/period"
	"testing"

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
