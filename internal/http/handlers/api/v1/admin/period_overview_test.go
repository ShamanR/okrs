package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/service"
)

// withUserRole injects an authenticated user (by UDID) and their active-tenant role.
func withUserRole(r *http.Request, udid string, isAdmin bool) *http.Request {
	ctx := auth.WithUser(r.Context(), &domain.User{UDID: udid})
	role := domain.RoleUser
	if isAdmin {
		role = domain.RoleAdmin
	}
	return r.WithContext(auth.WithActiveRole(ctx, role))
}

func TestHandlePeriodOverviewScoped_OrgForbiddenForNonAdmin(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil, &fakeGrants{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=org", nil)
	req = withURLParam(withTenant(withUserRole(req, "u-1", false)), "periodID", "1")
	w := httptest.NewRecorder()
	h.HandlePeriodOverviewScoped(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin org scope, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestHandlePeriodOverviewScoped_MyTeamsResolvesLeadScope(t *testing.T) {
	fg := &fakeGrants{leadScope: map[string][]int64{"u-1": {10, 11}}}
	h := NewServiceHandler(service.New(service.Deps{}), nil, fg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=my_teams", nil)
	req = withURLParam(withTenant(withUserRole(req, "u-1", false)), "periodID", "1")
	w := httptest.NewRecorder()
	h.HandlePeriodOverviewScoped(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !fg.leadScopeCalled {
		t.Fatalf("expected ListLeadTeamScope to be consulted for my_teams")
	}
}

func TestHandlePeriodOverviewScoped_OrgAllowedForAdmin(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil, &fakeGrants{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/periods/1/overview?scope=org", nil)
	req = withURLParam(withTenant(withUserRole(req, "admin-1", true)), "periodID", "1")
	w := httptest.NewRecorder()
	h.HandlePeriodOverviewScoped(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin org scope, got %d (%s)", w.Code, w.Body.String())
	}
}

// Without a tenant scope in context, admin period endpoints must 403.
func TestHandlePeriodOverview_ForbiddenWithoutScope(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/periods/1/overview", nil)
	req = withURLParam(req, "periodID", "1")
	w := httptest.NewRecorder()
	h.HandlePeriodOverview(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestHandlePeriodStats_ForbiddenWithoutScope(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/periods/stats", nil)
	w := httptest.NewRecorder()
	h.HandlePeriodStats(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}
