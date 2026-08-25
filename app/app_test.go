package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"okrs/app"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/store/memberships"
	"okrs/internal/store/sessions"
	"okrs/internal/store/testutil"

	"github.com/go-chi/chi/v5"
)

func TestNoAccessPageInjectsCustomMessage(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO system_settings (key, value_json) VALUES ('no_access_message', '"ping the **ops** team"')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a, err := app.New(app.Config{Pool: pool, Zone: time.UTC, Auth: auth.DefaultConfig()})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	rw := httptest.NewRecorder()
	a.Handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/no-access", nil))
	if rw.Code != http.StatusOK || !strings.Contains(rw.Body.String(), "ping the **ops** team") {
		t.Fatalf("no-access must embed the custom message; code=%d hasIt=%v", rw.Code, strings.Contains(rw.Body.String(), "ops"))
	}
}

func TestAppAssemblesAndServes(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()

	mounted := false
	a, err := app.New(app.Config{
		Pool:                 pool,
		Zone:                 time.UTC,
		Auth:                 auth.DefaultConfig(), // AUTH_MODE=disabled
		ResolveStrategyNames: []string{"session"},
		// Mount into the public tier so the test needs no auth/session plumbing.
		PublicRoutes: func(r chi.Router) {
			r.Get("/ext/ping", func(w http.ResponseWriter, _ *http.Request) { mounted = true; w.WriteHeader(http.StatusOK) })
		},
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}

	rw := httptest.NewRecorder()
	a.Handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/ext/ping", nil))
	if rw.Code != http.StatusOK || !mounted {
		t.Fatalf("mounted route not reachable: code=%d mounted=%v", rw.Code, mounted)
	}
}

// The no-membership page must set the CSRF cookie so its join-request form can double-submit;
// otherwise the POST fails with "csrf token is missing or invalid".
func TestNoAccessPageSetsCSRFCookie(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	a, err := app.New(app.Config{Pool: pool, Zone: time.UTC, Auth: auth.DefaultConfig()})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	rw := httptest.NewRecorder()
	a.Handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/no-access", nil))
	if rw.Code != http.StatusOK {
		t.Fatalf("/no-access = %d, want 200", rw.Code)
	}
	var found bool
	for _, c := range rw.Result().Cookies() {
		if c.Name == "okr_csrf_token" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("/no-access must set okr_csrf_token cookie for the join-request form")
	}
}

func TestSystemPlaneGatedInDisabledMode(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	a, err := app.New(app.Config{Pool: pool, Zone: time.UTC, Auth: auth.DefaultConfig()})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	rw := httptest.NewRecorder()
	a.Handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants", nil))
	if rw.Code != http.StatusForbidden {
		t.Fatalf("system plane must be gated even in AUTH_MODE=disabled, got %d", rw.Code)
	}
}

// A near-empty Config (no Auth settings) must yield the OSS box: auth defaults to disabled, so
// the tracker serves anonymously instead of demanding a login against zero configured providers.
func TestAppEmptyAuthConfigDefaultsToDisabled(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	a, err := app.New(app.Config{Pool: pool, Zone: time.UTC}) // no Auth supplied
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	rw := httptest.NewRecorder()
	a.Handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/", nil))
	if rw.Code != http.StatusOK {
		t.Fatalf("GET / with empty Auth = %d (Location %q), want 200 (disabled-mode OSS default)",
			rw.Code, rw.Header().Get("Location"))
	}
}

func TestAppUnknownEntitlementsName(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	if _, err := app.New(app.Config{Pool: pool, Zone: time.UTC, Auth: auth.DefaultConfig(), EntitlementsName: "nope"}); err == nil {
		t.Fatal("unknown entitlements name must error")
	}
}

func TestAppUnknownResolveStrategy(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	if _, err := app.New(app.Config{Pool: pool, Zone: time.UTC, Auth: auth.DefaultConfig(), ResolveStrategyNames: []string{"nope"}}); err == nil {
		t.Fatal("unknown resolve strategy must error")
	}
}

// Removing a member must take effect on the very next request: the tenant resolver and the
// mutating services must share one membership cache, or the resolver keeps serving the deleted
// membership until its TTL and the user still reaches the tracker (empty goals, but periods
// still listed because periods are tenant-scoped, not grant-scoped).
func TestRemovedMemberLosesTenantResolutionImmediately(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('github:victim','github','victim','Victim','v@example.com') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := memberships.NewMembershipRepository(pool).Upsert(ctx, domain.Membership{
		UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipActive,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if _, err := sessions.NewSessionRepository(pool).CreateSession(ctx, "sess-victim", uid, "github", time.Hour, "", ""); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	a, err := app.New(app.Config{
		Pool: pool, Zone: time.UTC,
		Auth: auth.Config{Mode: auth.ModeEnabled, SessionCookie: "okrs_session", SessionTTL: time.Hour, ProvisioningToken: "tkn"},
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	cookie := &http.Cookie{Name: "okrs_session", Value: "sess-victim"}

	// 1. Active member → tracker resolves and serves (this populates the resolver's cache).
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	a.Handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("pre-removal GET / = %d, want 200", rw.Code)
	}

	// 2. Remove the member via the system plane (invalidates the services' membership cache).
	rw = httptest.NewRecorder()
	del := httptest.NewRequest(http.MethodDelete, "/api/v1/system/tenants/1/members/"+strconv.FormatInt(uid, 10), nil)
	del.AddCookie(cookie)
	del.Header.Set("Authorization", "Bearer tkn")
	a.Handler.ServeHTTP(rw, del)
	if rw.Code != http.StatusNoContent {
		t.Fatalf("remove member = %d (%s), want 204", rw.Code, rw.Body.String())
	}

	// 3. The removed user must no longer resolve the tenant → membership gate redirects to /no-access.
	rw = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	a.Handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusFound || rw.Header().Get("Location") != "/no-access" {
		t.Fatalf("post-removal GET / = %d → %q, want 302 → /no-access (stale resolver membership cache)", rw.Code, rw.Header().Get("Location"))
	}
}

// A cookieless machine caller (Authorization: Bearer <PROVISIONING_TOKEN>, no session) must
// reach the system plane in AUTH_MODE=enabled — the control-plane provisioning contract
// (spec 040). Regression: RequireAuth used to 401 such callers before the system-admin gate
// could honor the token, so provisioning only worked when a session cookie was also present.
func TestSystemPlaneReachableByTokenOnlyMachineCaller(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()

	a, err := app.New(app.Config{
		Pool: pool, Zone: time.UTC,
		Auth: auth.Config{Mode: auth.ModeEnabled, SessionCookie: "okrs_session", SessionTTL: time.Hour, ProvisioningToken: "tkn"},
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}

	// Bearer token, no cookie → admitted by the sole system-admin gate.
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants", nil)
	req.Header.Set("Authorization", "Bearer tkn")
	a.Handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("token-only machine caller = %d (%s), want 200", rw.Code, rw.Body.String())
	}

	// No cookie, no token → denied (403), never leaking the plane to anonymous callers.
	rw = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/system/tenants", nil)
	a.Handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusForbidden {
		t.Fatalf("anonymous machine caller = %d, want 403", rw.Code)
	}
}

// A user whose active tenant is suspended must still reach the tenant switcher to recover:
// the list/switch routes are authenticated but NOT membership-gated, so RequireMembership can't
// lock them out of a tenant they're still active in.
func TestTenantSwitcherReachableWhenActiveTenantSuspended(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	// A second, active tenant the user can switch to.
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('github:sw','github','sw','Switcher','sw@example.com') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	mem := memberships.NewMembershipRepository(pool)
	for _, tid := range []int64{1, 2} {
		if _, err := mem.Upsert(ctx, domain.Membership{UserID: uid, TenantID: tid, Role: domain.RoleUser, Status: domain.MembershipActive}); err != nil {
			t.Fatalf("seed membership %d: %v", tid, err)
		}
	}
	sessRepo := sessions.NewSessionRepository(pool)
	if _, err := sessRepo.CreateSession(ctx, "sess-sw", uid, "github", time.Hour, "", ""); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	// Focus the session on tenant 1, then suspend it — the resolver now yields a suspended tenant.
	if err := sessRepo.SetActiveTenant(ctx, "sess-sw", 1); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tenants SET status = 'suspended' WHERE id = 1`); err != nil {
		t.Fatalf("suspend tenant 1: %v", err)
	}

	a, err := app.New(app.Config{
		Pool: pool, Zone: time.UTC,
		Auth: auth.Config{Mode: auth.ModeEnabled, SessionCookie: "okrs_session", SessionTTL: time.Hour},
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	cookie := &http.Cookie{Name: "okrs_session", Value: "sess-sw"}

	// Sanity: the tracker itself is blocked (active tenant suspended).
	rw := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "/", nil)
	greq.AddCookie(cookie)
	a.Handler.ServeHTTP(rw, greq)
	if rw.Code != http.StatusFound || rw.Header().Get("Location") != "/no-access" {
		t.Fatalf("GET / with suspended active tenant = %d → %q, want redirect to /no-access", rw.Code, rw.Header().Get("Location"))
	}

	// The switcher list must still be reachable (was 403 behind the membership gate). The GET
	// also seeds the CSRF double-submit cookie the switch POST needs.
	rw = httptest.NewRecorder()
	lreq := httptest.NewRequest(http.MethodGet, "/api/v1/session/tenants", nil)
	lreq.AddCookie(cookie)
	a.Handler.ServeHTTP(rw, lreq)
	if rw.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/session/tenants = %d, want 200 (switcher must stay reachable)", rw.Code)
	}
	var csrf string
	for _, c := range rw.Result().Cookies() {
		if c.Name == "okr_csrf_token" {
			csrf = c.Value
		}
	}
	if csrf == "" {
		t.Fatal("expected okr_csrf_token cookie from switcher GET")
	}

	// And the user can switch to the still-active tenant 2 (double-submit CSRF).
	rw = httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodPost, "/api/v1/session/tenant", strings.NewReader(`{"tenant_id":2}`))
	sreq.AddCookie(cookie)
	sreq.AddCookie(&http.Cookie{Name: "okr_csrf_token", Value: csrf})
	sreq.Header.Set("X-CSRF-Token", csrf)
	a.Handler.ServeHTTP(rw, sreq)
	if rw.Code != http.StatusNoContent {
		t.Fatalf("POST /api/v1/session/tenant = %d (%s), want 204", rw.Code, rw.Body.String())
	}
}
