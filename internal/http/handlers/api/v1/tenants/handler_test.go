package tenants

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
)

type stubDeps struct {
	memberships []domain.Membership
	tenants     map[int64]*domain.Tenant
	setCalled   int64
}

func (s *stubDeps) ListByUser(_ context.Context, _ int64) ([]domain.Membership, error) {
	return s.memberships, nil
}
func (s *stubDeps) GetBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	for _, t := range s.tenants {
		if t.Slug == slug {
			return t, nil
		}
	}
	return nil, context.Canceled // any non-nil; handler maps to 404
}
func (s *stubDeps) GetByID(_ context.Context, id int64) (*domain.Tenant, error) {
	if t, ok := s.tenants[id]; ok {
		return t, nil
	}
	return nil, context.Canceled
}
func (s *stubDeps) SetActiveTenant(_ context.Context, _ string, tenantID int64) error {
	s.setCalled = tenantID
	return nil
}

func authedReq(method, body string) *http.Request {
	req := httptest.NewRequest(method, "/api/v1/session/tenant", strings.NewReader(body))
	ctx := auth.WithUser(req.Context(), &domain.User{ID: 10})
	ctx = auth.WithSession(ctx, &domain.AuthSession{ID: "s1"})
	return req.WithContext(ctx)
}

func TestSwitchTenantRejectsNonMember(t *testing.T) {
	deps := &stubDeps{tenants: map[int64]*domain.Tenant{2: {ID: 2, Slug: "acme", Status: domain.TenantActive}}}
	h := New(deps, deps, deps)

	rw := httptest.NewRecorder()
	h.SwitchTenant(rw, authedReq(http.MethodPost, `{"slug":"acme"}`))

	if rw.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rw.Code)
	}
	if deps.setCalled != 0 {
		t.Fatalf("SetActiveTenant must not be called for non-member")
	}
}

func TestSwitchTenantUpdatesSession(t *testing.T) {
	deps := &stubDeps{
		memberships: []domain.Membership{{UserID: 10, TenantID: 2, Role: domain.RoleUser}},
		tenants:     map[int64]*domain.Tenant{2: {ID: 2, Slug: "acme", Status: domain.TenantActive}},
	}
	h := New(deps, deps, deps)

	rw := httptest.NewRecorder()
	h.SwitchTenant(rw, authedReq(http.MethodPost, `{"slug":"acme"}`))

	if rw.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rw.Code)
	}
	if deps.setCalled != 2 {
		t.Fatalf("SetActiveTenant called with %d, want 2", deps.setCalled)
	}
}

func TestListMyTenants(t *testing.T) {
	deps := &stubDeps{
		memberships: []domain.Membership{
			{UserID: 10, TenantID: 1, Role: domain.RoleUser},
			{UserID: 10, TenantID: 2, Role: domain.RoleAdmin},
		},
		tenants: map[int64]*domain.Tenant{
			1: {ID: 1, Slug: "default", Name: "Default", Status: domain.TenantActive},
			2: {ID: 2, Slug: "acme", Name: "Acme", Status: domain.TenantActive},
		},
	}
	h := New(deps, deps, deps)

	rw := httptest.NewRecorder()
	h.ListMyTenants(rw, authedReq(http.MethodGet, ""))

	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rw.Code)
	}
	var got []tenantDTO
	if err := json.Unmarshal(rw.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].Slug != "default" || got[1].Slug != "acme" {
		t.Fatalf("unexpected list: %+v", got)
	}
}
