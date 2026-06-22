package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/domain"
)

func TestRequireMembershipBlocksWhenNoTenant(t *testing.T) {
	called := false
	h := RequireMembershipMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if called {
		t.Fatalf("handler should not be called without tenant in context")
	}
	if rw.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rw.Code)
	}
}

func TestRequireMembershipBlocksSuspendedTenant(t *testing.T) {
	called := false
	h := RequireMembershipMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	ctx := WithTenant(req.Context(), &domain.Tenant{ID: 2, Status: domain.TenantSuspended})
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req.WithContext(ctx))

	if called || rw.Code != http.StatusForbidden {
		t.Fatalf("suspended tenant must be blocked; called=%v code=%d", called, rw.Code)
	}
}

func TestRequireMembershipAllowsActiveTenant(t *testing.T) {
	called := false
	h := RequireMembershipMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	ctx := WithTenant(req.Context(), &domain.Tenant{ID: 1, Status: domain.TenantActive})
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req.WithContext(ctx))

	if !called || rw.Code != http.StatusOK {
		t.Fatalf("active tenant must pass; called=%v code=%d", called, rw.Code)
	}
}
