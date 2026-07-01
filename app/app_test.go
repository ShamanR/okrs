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
	"okrs/internal/domain"
	"okrs/internal/entitlements"
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

func init() {
	// The OSS feature-gating impl; app.New selects it by name ("unlimited").
	entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })
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
