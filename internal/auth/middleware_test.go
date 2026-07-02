package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/domain"
)

func TestRequireSystemAdminAllowsSystemAdminUser(t *testing.T) {
	called := false
	h := RequireSystemAdminMiddleware("")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants", nil)
	req = req.WithContext(WithUser(req.Context(), &domain.User{ID: 1, IsSystemAdmin: true}))
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if !called || rw.Code != http.StatusOK {
		t.Fatalf("system admin must pass; called=%v code=%d", called, rw.Code)
	}
}

func TestRequireSystemAdminBlocksNonAdmin(t *testing.T) {
	called := false
	h := RequireSystemAdminMiddleware("")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants", nil)
	req = req.WithContext(WithUser(req.Context(), &domain.User{ID: 2, IsSystemAdmin: false}))
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if called || rw.Code != http.StatusForbidden {
		t.Fatalf("non system admin must be blocked; called=%v code=%d", called, rw.Code)
	}
}

func TestRequireSystemAdminAllowsProvisioningToken(t *testing.T) {
	called := false
	h := RequireSystemAdminMiddleware("secret")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if !called || rw.Code != http.StatusOK {
		t.Fatalf("valid provisioning token must pass; called=%v code=%d", called, rw.Code)
	}
}

func TestRequireTenantAdminAllowsAdminRole(t *testing.T) {
	called := false
	h := RequireTenantAdminMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req = req.WithContext(WithActiveRole(req.Context(), domain.RoleAdmin))
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if !called || rw.Code != http.StatusOK {
		t.Fatalf("admin role must pass; called=%v code=%d", called, rw.Code)
	}
}

func TestRequireTenantAdminBlocksPlainMember(t *testing.T) {
	called := false
	h := RequireTenantAdminMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req = req.WithContext(WithActiveRole(req.Context(), domain.RoleUser))
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if called || rw.Code != http.StatusForbidden {
		t.Fatalf("plain member must be blocked; called=%v code=%d", called, rw.Code)
	}
}

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
