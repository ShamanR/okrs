package http

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"okrs/internal/auth"
	"okrs/internal/platform/eventbus"
	"okrs/internal/store"
	"okrs/internal/store/grants"
)

var updateGolden = flag.Bool("update-routes", false, "rewrite testdata/routes.golden")

const routesGolden = "testdata/routes.golden"

// TestRoutesGolden walks the assembled router and compares every (method, pattern)
// pair against a golden file.
//
// This is the contract guard for the handler split. Grepping the source for
// r.Get("/…") stopped being sufficient once shell routes moved into a table, where the
// URI is a variable — the grep silently saw fewer routes. Walking the real router sees
// exactly what chi will serve, so a dropped registration, a renamed URI or a changed
// method fails here instead of in production.
//
// Routes() is a pure assembly step (the background loops moved to internal/scheduler),
// so this test needs no database: a zero store is enough to register handlers.
//
// Refresh after an intentional contract change: go test ./internal/http -run RoutesGolden -update-routes
func TestRoutesGolden(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv, err := NewServer(&store.Store{}, &grants.GrantsCache{}, logger,
		time.UTC, authManagerForRouteTest(t), eventbus.New(logger), Options{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	routes, ok := srv.Routes().(chi.Routes)
	if !ok {
		t.Fatal("Routes() did not return a chi.Routes")
	}

	var got []string
	err = chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(got)
	actual := strings.Join(got, "\n") + "\n"

	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(routesGolden, []byte(actual), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden refreshed: %d routes", len(got))
		return
	}

	want, err := os.ReadFile(routesGolden)
	if err != nil {
		t.Fatalf("read %s: %v (create it with -update-routes)", routesGolden, err)
	}
	if actual != string(want) {
		t.Errorf("route set changed.\nActual has %d routes.\nRun with -update-routes only if the change is intentional.\n\n%s",
			len(got), firstDiff(string(want), actual))
	}
}

// firstDiff reports the first differing line, which is far more useful than dumping
// both hundred-line lists.
func firstDiff(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return "first difference at line " + itoa(i+1) + ":\n  golden: " + wl + "\n  actual: " + gl
		}
	}
	return "(no line difference; trailing whitespace?)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// authManagerForRouteTest builds an auth manager in disabled mode: route registration
// branches on auth.Disabled(), and disabled mode registers the same URIs while skipping
// the auth middlewares, which this test does not exercise.
func authManagerForRouteTest(t *testing.T) *auth.Manager {
	t.Helper()
	mgr, err := auth.NewManager(auth.Config{Mode: auth.ModeDisabled}, nil)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	return mgr
}
