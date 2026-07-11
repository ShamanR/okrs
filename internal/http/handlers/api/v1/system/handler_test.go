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
	"okrs/internal/store/grants"
	"okrs/internal/store/invitations"
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
	grantsCache := grants.NewGrantsCache(grants.NewGrantRepository(pool))
	onboardingSvc := service.NewOnboardingService(
		invitations.NewInvitationRepository(pool), memRepo, memberships.NewMembershipCache(memRepo),
		tnRepo, settingsSvc, grantsCache,
	)
	prov := service.NewProvisioningService(
		tnRepo, tenants.NewTenantCache(tnRepo),
		memRepo, memberships.NewMembershipCache(memRepo),
		settingsSvc, grantsCache, onboardingSvc, userRepo,
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
	r.Patch("/api/v1/system/tenants/{id}", h.HandlePatchTenant)
	r.Post("/api/v1/system/tenants/{id}/members", h.HandleAttachMember)
	r.Get("/api/v1/system/tenants/{id}/members", h.HandleListMembers)
	r.Post("/api/v1/system/tenants/{id}/members/{userID}/deny", h.HandleDenyMember)
	r.Put("/api/v1/system/tenants/{id}/members/{userID}/role", h.HandleSetMemberRole)
	r.Delete("/api/v1/system/tenants/{id}/members/{userID}", h.HandleRemoveMember)
	r.Put("/api/v1/system/tenants/{id}/entitlements", h.HandleSetEntitlements)
	r.Get("/api/v1/system/tenants/{id}/entitlements", h.HandleGetEntitlements)
	r.Post("/api/v1/system/tenants/{id}/suspend", h.HandleSuspend)
	r.Get("/api/v1/system/settings", h.HandleGetSettings)
	r.Get("/api/v1/system/users", h.HandleListUsers)
	r.Put("/api/v1/system/users/{userID}/system-admin", h.HandleSetSystemAdmin)
	r.Put("/api/v1/system/settings/default-registration-tenant", h.HandleSetDefaultRegistrationTenant)
	r.Put("/api/v1/system/settings/no-access-message", h.HandleSetNoAccessMessage)
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

func TestSystemPatchTenant(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, tnRepo, _ := buildRouter(t, admin)
	ctx := context.Background()

	tn, err := tnRepo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := tnRepo.Create(ctx, "globex", "Globex"); err != nil {
		t.Fatalf("seed globex: %v", err)
	}

	patch := func(id string, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/v1/system/tenants/"+id, strings.NewReader(body)))
		return w
	}

	// success: rename + slug change
	w := patch(itoa(tn.ID), `{"name":"Acme LLC","slug":"acme-llc"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: code %d (%s)", w.Code, w.Body.String())
	}
	var got struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.Slug != "acme-llc" || got.Name != "Acme LLC" {
		t.Fatalf("patch result = %+v", got)
	}

	// 409 slug taken
	if w := patch(itoa(tn.ID), `{"name":"X","slug":"globex"}`); w.Code != http.StatusConflict {
		t.Fatalf("taken slug: code %d, want 409", w.Code)
	}
	// 422 invalid slug
	if w := patch(itoa(tn.ID), `{"name":"X","slug":"AB"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid slug: code %d, want 422", w.Code)
	}
	// 422 empty name
	if w := patch(itoa(tn.ID), `{"name":"  ","slug":"acme-llc"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: code %d, want 422", w.Code)
	}
	// 404 missing tenant
	if w := patch("999999", `{"name":"X","slug":"free-slug"}`); w.Code != http.StatusNotFound {
		t.Fatalf("missing tenant: code %d, want 404", w.Code)
	}
}

func TestSystemPatchTenantRequiresSystemAdmin(t *testing.T) {
	r, _, _ := buildRouter(t, nil) // no user in context → gate rejects
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/v1/system/tenants/1", strings.NewReader(`{"name":"X","slug":"x-slug"}`)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin: code %d, want 403", w.Code)
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

func TestSystemDenyMemberRouteWired(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)
	// No requested row → DeleteRequested is a no-op; the route should still resolve to 204.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/1/members/999/deny", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("deny route: %d (%s)", w.Code, w.Body.String())
	}
}

func TestSystemRemoveMember(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/1/members",
		strings.NewReader(`{"user_id":1,"role":"admin"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("attach: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/system/tenants/1/members/1", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("remove: %d (%s)", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants/1/members", nil))
	var members []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&members)
	for _, m := range members {
		if m["user_id"].(float64) == 1 {
			t.Fatalf("user 1 should be removed, still present: %v", members)
		}
	}
}

func TestSystemSetMemberRole(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)

	// Attach user 1 as admin (sole admin of tenant 1).
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/1/members",
		strings.NewReader(`{"user_id":1,"role":"admin"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("attach: %d (%s)", w.Code, w.Body.String())
	}

	// Invalid role → 422.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/tenants/1/members/1/role",
		strings.NewReader(`{"role":"boss"}`)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid role: %d", w.Code)
	}

	// Demoting the sole admin → 409.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/tenants/1/members/1/role",
		strings.NewReader(`{"role":"user"}`)))
	if w.Code != http.StatusConflict {
		t.Fatalf("last-admin demote: %d (%s)", w.Code, w.Body.String())
	}

	// Unknown membership → 404.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/tenants/1/members/999/role",
		strings.NewReader(`{"role":"user"}`)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown member: %d", w.Code)
	}

	// Add a second admin, then promoting/demoting works → 204.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/tenants/1/members",
		strings.NewReader(`{"user_id":2,"role":"admin"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("attach 2: %d (%s)", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/tenants/1/members/1/role",
		strings.NewReader(`{"role":"user"}`)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("demote with 2 admins: %d (%s)", w.Code, w.Body.String())
	}
}

func TestSystemSetSystemAdmin(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)

	put := func(userID, v string) int {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/users/"+userID+"/system-admin",
			strings.NewReader(`{"is_system_admin":`+v+`}`)))
		return w.Code
	}

	// Grant user 2 → 204.
	if code := put("2", "true"); code != http.StatusNoContent {
		t.Fatalf("grant user 2: %d", code)
	}
	// Revoke sole system-admin (user 2) → 409.
	if code := put("2", "false"); code != http.StatusConflict {
		t.Fatalf("revoke last admin: %d", code)
	}
	// Grant caller (user 1) too → 204.
	if code := put("1", "true"); code != http.StatusNoContent {
		t.Fatalf("grant user 1: %d", code)
	}
	// Caller revoking self → 409 (self-lockout).
	if code := put("1", "false"); code != http.StatusConflict {
		t.Fatalf("self revoke: %d", code)
	}
	// Unknown user → 404.
	if code := put("999", "true"); code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", code)
	}
}

func TestSystemNoAccessMessageRoundTrip(t *testing.T) {
	admin := &domain.User{ID: 1, IsSystemAdmin: true}
	r, _, _ := buildRouter(t, admin)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/settings/no-access-message",
		strings.NewReader(`{"message":"# Hello\nask **ops**"}`)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("put: %d (%s)", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/settings", nil))
	var got struct {
		NoAccessMessage string `json:"no_access_message"`
	}
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.NoAccessMessage != "# Hello\nask **ops**" {
		t.Fatalf("no_access_message = %q", got.NoAccessMessage)
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
