package system_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
	apisystem "okrs/internal/http/handlers/api/v1/system"
	"okrs/internal/service"
	"okrs/internal/store/memberships"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"
	"okrs/internal/store/users"

	"github.com/go-chi/chi/v5"
)

// buildRouter wires the system routes behind the system-admin gate, injecting the
// given user into context (nil = anonymous, exercises the 403 path).
func buildRouter(t *testing.T, user *domain.User) (*chi.Mux, *tenants.TenantRepository, *users.UserRepository) {
	t.Helper()
	pool, cleanup := testutil.SetupDB(t)
	t.Cleanup(cleanup)

	tnRepo := tenants.NewTenantRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	userRepo := users.NewUserRepository(pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	prov := service.NewProvisioningService(
		tnRepo, tenants.NewTenantCache(tnRepo),
		memRepo, memberships.NewMembershipCache(memRepo),
		settingsSvc,
	)
	h := apisystem.New(prov, settingsSvc, userRepo, tnRepo, memRepo)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if user != nil {
				req = req.WithContext(auth.WithUser(req.Context(), user))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Use(auth.RequireSystemAdminMiddleware(""))
	r.Post("/api/v1/system/tenants", h.HandleCreateTenant)
	r.Get("/api/v1/system/tenants", h.HandleListTenants)
	r.Post("/api/v1/system/tenants/{id}/members", h.HandleAttachMember)
	r.Get("/api/v1/system/tenants/{id}/members", h.HandleListMembers)
	r.Put("/api/v1/system/tenants/{id}/entitlements", h.HandleSetEntitlements)
	r.Get("/api/v1/system/tenants/{id}/entitlements", h.HandleGetEntitlements)
	r.Post("/api/v1/system/tenants/{id}/suspend", h.HandleSuspend)
	r.Get("/api/v1/system/settings", h.HandleGetSettings)
	r.Put("/api/v1/system/settings/default-registration-tenant", h.HandleSetDefaultRegistrationTenant)
	return r, tnRepo, userRepo
}

func TestSystemCreateTenantAndAttachMember(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, tnRepo, _ := buildRouter(t, admin)
	ctx := context.Background()

	// Create tenant.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants",
		strings.NewReader(`{"name":"Acme","slug":"acme","entitlements":{"sso":true}}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create tenant: code %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.Slug != "acme" {
		t.Fatalf("slug = %q", created.Slug)
	}

	// Attach anonymous-local user (id 1) as admin.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		"/api/v1/system/tenants/"+itoa(created.ID)+"/members",
		strings.NewReader(`{"user_id":1,"role":"admin"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("attach member: code %d (%s)", w.Code, w.Body.String())
	}

	// Suspend.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		"/api/v1/system/tenants/"+itoa(created.ID)+"/suspend", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("suspend: code %d (%s)", w.Code, w.Body.String())
	}
	got, _ := tnRepo.GetByID(ctx, created.ID)
	if got.Status != domain.TenantSuspended {
		t.Fatalf("status = %q, want suspended", got.Status)
	}
}

func TestSystemListMembersAndEntitlements(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants",
		strings.NewReader(`{"name":"Acme","slug":"acme"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	id := itoa(created.ID)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/"+id+"/members",
		strings.NewReader(`{"user_id":1,"role":"admin"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("attach: %d (%s)", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/tenants/"+id+"/entitlements",
		strings.NewReader(`{"sso":true}`)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("set entitlements: %d (%s)", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants/"+id+"/members", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("members: %d", w.Code)
	}
	var members []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&members)
	if len(members) != 1 || members[0]["role"] != "admin" || members[0]["status"] != "active" {
		t.Fatalf("members = %v", members)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants/"+id+"/entitlements", nil))
	var ent map[string]any
	_ = json.NewDecoder(w.Body).Decode(&ent)
	if ent["sso"] != true {
		t.Fatalf("entitlements = %v", ent)
	}
}

func TestSystemGetSettings(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/settings/default-registration-tenant",
		strings.NewReader(`{"tenant_id":1}`)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("set default: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/settings", nil))
	var got struct {
		DefaultRegistrationTenantID *int64 `json:"default_registration_tenant_id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.DefaultRegistrationTenantID == nil || *got.DefaultRegistrationTenantID != 1 {
		t.Fatalf("default tenant = %v", got.DefaultRegistrationTenantID)
	}
}

func TestSystemAPIForbiddenForNonAdmin(t *testing.T) {
	r, _, _ := buildRouter(t, &domain.User{ID: 2, IsSystemAdmin: false})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("non system-admin must get 403, got %d", w.Code)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
