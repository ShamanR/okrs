package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/core/domain"
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

// A token-only machine caller (no session user in context) must reach the system plane —
// the gate is the sole authorization for /api/v1/system/* and must not depend on RequireAuth.
func TestRequireSystemAdminAllowsTokenWithoutSession(t *testing.T) {
	called := false
	h := RequireSystemAdminMiddleware("secret")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/tenants/1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	// No user in context (cookieless machine caller).
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if !called || rw.Code != http.StatusOK {
		t.Fatalf("cookieless token caller must pass; called=%v code=%d", called, rw.Code)
	}
}

// An unauthenticated browser (no session user) hitting the SSR shell is redirected to login,
// preserving the UX RequireAuth used to provide now that the gate stands alone.
func TestRequireSystemAdminRedirectsUnauthenticatedBrowser(t *testing.T) {
	called := false
	h := RequireSystemAdminMiddleware("secret")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/system", nil)
	req.Header.Set("Accept", "text/html")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if called {
		t.Fatalf("handler must not run for unauthenticated browser")
	}
	if rw.Code != http.StatusFound || rw.Header().Get("Location") != "/login?next=/system" {
		t.Fatalf("want 302 → /login?next=/system, got %d → %q", rw.Code, rw.Header().Get("Location"))
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
