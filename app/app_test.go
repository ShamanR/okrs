package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"okrs/app"
	"okrs/internal/auth"
	"okrs/internal/entitlements"
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
