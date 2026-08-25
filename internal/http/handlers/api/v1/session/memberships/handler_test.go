package memberships

// Тесты переехали из пакета tenants вместе с обработчиками
// GET/DELETE /api/v1/session/memberships.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/service/provisioning"
	"okrs/internal/store/memberships"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type stubDeps struct {
	memberships []domain.Membership
	withTenant  []memberships.MembershipWithTenant
	tenants     map[int64]*domain.Tenant
	setCalled   int64
	leftTenant  int64
	leaveErr    error
}

func (s *stubDeps) ListByUser(_ context.Context, _ int64) ([]domain.Membership, error) {
	return s.memberships, nil
}

func (s *stubDeps) ListByUserWithTenant(_ context.Context, _ int64) ([]memberships.MembershipWithTenant, error) {
	return s.withTenant, nil
}

func (s *stubDeps) LeaveTenant(_ context.Context, tenantID, _ int64) error {
	if s.leaveErr != nil {
		return s.leaveErr
	}
	s.leftTenant = tenantID
	return nil
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

func TestListMyMemberships(t *testing.T) {
	deps := &stubDeps{withTenant: []memberships.MembershipWithTenant{
		{TenantID: 1, Slug: "default", Name: "Default", Role: domain.RoleUser, Status: domain.MembershipActive},
		{TenantID: 2, Slug: "acme", Name: "Acme", Role: domain.RoleAdmin, Status: domain.MembershipRequested},
	}}
	h := New(deps, deps)

	rw := httptest.NewRecorder()
	h.Get(rw, authedReq(http.MethodGet, ""))

	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rw.Code)
	}
	var got []membershipDTO
	if err := json.Unmarshal(rw.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].Slug != "default" || got[1].Status != "requested" {
		t.Fatalf("unexpected list: %+v", got)
	}
}

func TestLeaveTenant(t *testing.T) {
	// Success → 204 and the leaver is called with the path tenant id.
	deps := &stubDeps{}
	h := New(deps, deps)
	r := chi.NewRouter()
	r.Delete("/api/v1/session/memberships/{tenantID}", h.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/session/memberships/2", nil)
	req = req.WithContext(auth.WithUser(req.Context(), &domain.User{ID: 10}))
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, req)
	if rw.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rw.Code)
	}
	if deps.leftTenant != 2 {
		t.Fatalf("LeaveTenant called with %d, want 2", deps.leftTenant)
	}

	// Last-admin → 409.
	deps2 := &stubDeps{leaveErr: provisioning.ErrLastAdmin}
	h2 := New(deps2, deps2)
	r2 := chi.NewRouter()
	r2.Delete("/api/v1/session/memberships/{tenantID}", h2.Delete)
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/session/memberships/1", nil)
	req2 = req2.WithContext(auth.WithUser(req2.Context(), &domain.User{ID: 10}))
	rw2 := httptest.NewRecorder()
	r2.ServeHTTP(rw2, req2)
	if rw2.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rw2.Code)
	}
}
