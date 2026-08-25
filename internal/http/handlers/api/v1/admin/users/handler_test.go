package users

// Тесты переехали из пакета admin вместе с обработчиком GET /api/v1/admin/users.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/store/grants"
	"okrs/internal/store/users"
	"testing"
)

// fakeUsers is an in-memory userAdminStore for handler tests.
type fakeUsers struct {
	users       []*domain.User
	tenantUsers []users.TenantUser
}

func (f *fakeUsers) ListUsers(context.Context) ([]*domain.User, error) { return f.users, nil }

func (f *fakeUsers) GetUser(context.Context, int64) (*domain.User, error) { return nil, nil }

func (f *fakeUsers) ListByTenant(context.Context, domain.TenantScope) ([]users.TenantUser, error) {
	return f.tenantUsers, nil
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

// withTenant attaches the default tenant #1 so TenantScopeFromContext returns {1}.
func withTenant(r *http.Request) *http.Request {
	return r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Name: "Acme", Status: domain.TenantActive}))
}

// The users list is tenant-scoped (members + requesters), each item carries Status, and
// GrantedNodeCount counts only grants to still-active teams (requesters have none).
func TestHandleListUsersIsTenantScopedWithStatus(t *testing.T) {
	fu := &fakeUsers{tenantUsers: []users.TenantUser{
		{User: &domain.User{ID: 10, DisplayName: "Active"}, Status: domain.MembershipActive, Role: domain.RoleUser},
		{User: &domain.User{ID: 20, DisplayName: "Requester"}, Status: domain.MembershipRequested, Role: domain.RoleUser},
	}}
	g := &fakeGrants{
		all: map[int64][]grants.HierarchyGrant{
			10: {{UserID: 10, TeamID: 1}, {UserID: 10, TeamID: 2}}, // team 1 active, team 2 deleted
		},
		activeTeamIDs: map[int64]bool{1: true},
	}
	h := New(g, fu)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	r = r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Status: domain.TenantActive}))
	w := httptest.NewRecorder()
	h.Get(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got []struct {
		ID               int64
		Status           string
		GrantedNodeCount int
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 tenant users, got %d", len(got))
	}
	by := map[int64]struct {
		ID               int64
		Status           string
		GrantedNodeCount int
	}{}
	for _, u := range got {
		by[u.ID] = u
	}
	if by[10].Status != "active" || by[10].GrantedNodeCount != 1 {
		t.Errorf("active member = %+v (want status=active, count=1 active team only)", by[10])
	}
	if by[20].Status != "requested" || by[20].GrantedNodeCount != 0 {
		t.Errorf("requester = %+v (want status=requested, count=0)", by[20])
	}
}
