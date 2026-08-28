package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	apiactivity "okrs/internal/http/handlers/api/v1/activity"
	activitycat "okrs/internal/http/handlers/api/v1/activity/categorycounts"
	activitytree "okrs/internal/http/handlers/api/v1/activity/treecounts"
	apigoals "okrs/internal/http/handlers/api/v1/goals"
	goalscomments "okrs/internal/http/handlers/api/v1/goals/comments"
	goalsreplies "okrs/internal/http/handlers/api/v1/goals/comments/replies"
	goalsresolve "okrs/internal/http/handlers/api/v1/goals/comments/resolve"
	goalsunresolve "okrs/internal/http/handlers/api/v1/goals/comments/unresolve"
	"okrs/internal/http/handlers/api/v1/goals/goalcommon"
	goalskeyresults "okrs/internal/http/handlers/api/v1/goals/keyresults"
	goalslinkable "okrs/internal/http/handlers/api/v1/goals/linkable"
	goalslinks "okrs/internal/http/handlers/api/v1/goals/links"
	goalsmovedown "okrs/internal/http/handlers/api/v1/goals/movedown"
	goalsmoveup "okrs/internal/http/handlers/api/v1/goals/moveup"
	goalsshare "okrs/internal/http/handlers/api/v1/goals/share"
	goalstransfer "okrs/internal/http/handlers/api/v1/goals/transfer"
	goalsweight "okrs/internal/http/handlers/api/v1/goals/weight"
	apigoaltree "okrs/internal/http/handlers/api/v1/goaltree"
	apihierarchy "okrs/internal/http/handlers/api/v1/hierarchy"
	apikrs "okrs/internal/http/handlers/api/v1/krs"
	krsdescription "okrs/internal/http/handlers/api/v1/krs/description"
	"okrs/internal/http/handlers/api/v1/krs/krscommon"
	krsmovedown "okrs/internal/http/handlers/api/v1/krs/movedown"
	krsmoveup "okrs/internal/http/handlers/api/v1/krs/moveup"
	krsnote "okrs/internal/http/handlers/api/v1/krs/note"
	krsboolean "okrs/internal/http/handlers/api/v1/krs/progress/boolean"
	krsnumerical "okrs/internal/http/handlers/api/v1/krs/progress/numerical"
	krsproject "okrs/internal/http/handlers/api/v1/krs/progress/project"
	apiperiods "okrs/internal/http/handlers/api/v1/periods"
	apiteams "okrs/internal/http/handlers/api/v1/teams"
	teamsexport "okrs/internal/http/handlers/api/v1/teams/export"
	teamsgoals "okrs/internal/http/handlers/api/v1/teams/goals"
	teamsokrs "okrs/internal/http/handlers/api/v1/teams/okrs"
	teamsoverview "okrs/internal/http/handlers/api/v1/teams/overview"
	teamsstatus "okrs/internal/http/handlers/api/v1/teams/status"
	"okrs/internal/http/httpdeps"
	"okrs/internal/platform/eventbus"
	"okrs/internal/store"
	"okrs/internal/store/grants"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func NewAPIV1Router(t testing.TB, st *store.Store, grantsCache *grants.GrantsCache) *chi.Mux {
	return NewAPIV1RouterWithScope(t, st, grantsCache, nil)
}

// NewAPIV1RouterWithScope returns a test router with a fixed scope injected into every request context.
// Pass nil for unrestricted access (admin), an empty slice for no access, or a specific list of team IDs.
//
// t is used only to register the bus teardown (t.Cleanup) that mirrors what app.New
// does in production: Start the bus after Build, Close it when the router is done with.
// Today the only registered subscriber is Sync (the activity journal), which needs no
// goroutine, so skipping Start would still pass every test — but the next phase adds
// an async subscriber, and a router that never starts the bus would silently never
// exercise it end to end.
func NewAPIV1RouterWithScope(t testing.TB, st *store.Store, grantsCache *grants.GrantsCache, allowedTeamIDs []int64) *chi.Mux {
	t.Helper()
	// Собираем тот же граф сервисов и usecase, что и боевой сервер: handlers теперь
	// принимают отдельные зависимости, а не фасад. The bus needs a real logger (not
	// nil, unlike the activity service below) because it logs straight through on a
	// dropped/failed delivery, with no nil-safe guard.
	bus := eventbus.New(slog.Default())
	d := httpdeps.Build(st, grantsCache, nil, bus, nil)
	bus.Start(context.Background())
	t.Cleanup(func() {
		if err := bus.Close(5 * time.Second); err != nil {
			t.Logf("eventbus close: %v", err)
		}
	})

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithAllowedTeamIDs(r.Context(), allowedTeamIDs)
			ctx = auth.WithTenant(ctx, &domain.Tenant{ID: 1})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	apihierarchy.RegisterRoutes(router, apihierarchy.New(d.Teams, d.Board, d.Periods, d.Users))
	apiperiods.RegisterRoutes(router, apiperiods.New(d.Periods))
	apiteams.RegisterRoutes(router, apiteams.New(d.Teams))
	teamsokrs.RegisterRoutes(router, teamsokrs.New(d.Board, d.Periods, d.Users))
	teamsoverview.RegisterRoutes(router, teamsoverview.New(d.Board, d.Periods, d.Users))
	teamsexport.RegisterRoutes(router, teamsexport.New(d.ExportUC))
	teamsstatus.RegisterRoutes(router, teamsstatus.New(d.PeriodUC))
	teamsgoals.RegisterRoutes(router, teamsgoals.New(d.GoalUC, d.Users))
	apigoals.RegisterRoutes(router, apigoals.New(d.Goals, d.Shares, d.Links, d.Users, d.GoalUC))
	goalslinkable.RegisterRoutes(router, goalslinkable.New(d.Links))
	goalslinks.RegisterRoutes(router, goalslinks.New(d.Goals, d.GoalUC))
	goalsshare.RegisterRoutes(router, goalsshare.New(d.Goals, d.Shares, d.GoalUC))
	goalstransfer.RegisterRoutes(router, goalstransfer.New(d.Goals, d.GoalUC))
	goalsweight.RegisterRoutes(router, goalsweight.New(d.Goals, d.Shares))
	goalscomments.RegisterRoutes(router, goalscomments.New(d.Goals, d.GoalUC, d.Shares))
	goalsreplies.RegisterRoutes(router, goalsreplies.New(d.Goals, d.GoalUC, d.Shares))
	goalsresolve.RegisterRoutes(router, goalsresolve.New(goalcommon.ResolveDeps{Goals: d.Goals, Shares: d.Shares, UC: d.GoalUC}))
	goalsunresolve.RegisterRoutes(router, goalsunresolve.New(goalcommon.ResolveDeps{Goals: d.Goals, Shares: d.Shares, UC: d.GoalUC}))
	goalsmoveup.RegisterRoutes(router, goalsmoveup.New(goalcommon.MoveDeps{Goals: d.Goals, Shares: d.Shares, Mover: d.Goals}))
	goalsmovedown.RegisterRoutes(router, goalsmovedown.New(goalcommon.MoveDeps{Goals: d.Goals, Shares: d.Shares, Mover: d.Goals}))
	goalskeyresults.RegisterRoutes(router, goalskeyresults.New(d.Goals, d.KrUC))
	apigoaltree.RegisterRoutes(router, apigoaltree.New(d.Periods, d.TreeUC))
	apikrs.RegisterRoutes(router, apikrs.New(d.Goals, d.Krs, d.KrUC))
	krsnumerical.RegisterRoutes(router, krsnumerical.New(d.Goals, d.Krs, d.KrUC))
	krsboolean.RegisterRoutes(router, krsboolean.New(d.Goals, d.Krs, d.KrUC))
	krsproject.RegisterRoutes(router, krsproject.New(d.Goals, d.Krs, d.KrUC))
	krsnote.RegisterRoutes(router, krsnote.New(d.Goals, d.Krs, d.KrUC))
	krsdescription.RegisterRoutes(router, krsdescription.New(d.Goals, d.Krs))
	krsmoveup.RegisterRoutes(router, krsmoveup.New(krscommon.MoveDeps{KRs: d.Krs, Goals: d.Goals}))
	krsmovedown.RegisterRoutes(router, krsmovedown.New(krscommon.MoveDeps{KRs: d.Krs, Goals: d.Goals}))
	apiactivity.RegisterRoutes(router, apiactivity.New(d.Activity))
	activitytree.RegisterRoutes(router, activitytree.New(d.Activity))
	activitycat.RegisterRoutes(router, activitycat.New(d.Activity))
	v1.RegisterMethodNotAllowed(router)
	return router
}

// NewAPIV1RouterWithUser — как NewAPIV1RouterWithScope, но дополнительно кладёт пользователя в
// контекст (для эндпоинтов, зависящих от UDID вызывающего, напр. led_by_me в goal-tree).
func NewAPIV1RouterWithUser(t testing.TB, st *store.Store, grantsCache *grants.GrantsCache, allowedTeamIDs []int64, user *domain.User) *chi.Mux {
	router := NewAPIV1RouterWithScope(t, st, grantsCache, allowedTeamIDs)
	wrapped := chi.NewRouter()
	wrapped.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
		})
	})
	wrapped.Mount("/", router)
	return wrapped
}

func RunMigrations(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		return err
	}
	migrationsPath, err := resolveMigrationsPath()
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	// Migration 032 drops the transitional tenant_id DEFAULT 1 so a forgotten tenant_id fails
	// in production. Integration fixtures are single-tenant and insert rows with raw SQL that
	// omits tenant_id; restore the default so those rows land in tenant 1.
	for _, tbl := range []string{
		"teams", "periods", "goals", "goal_shares", "team_period_statuses",
		"user_hierarchy_grants", "key_results", "goal_comments", "key_result_notes",
	} {
		if _, err := db.ExecContext(ctx, "ALTER TABLE "+tbl+" ALTER COLUMN tenant_id SET DEFAULT 1"); err != nil {
			return err
		}
	}
	return nil
}

func RequireDockerOrSkip(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
}

func resolveMigrationsPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "migrations"), nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found (start dir: %s)", dir)
		}
		dir = parent
	}
}
