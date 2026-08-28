// Package app is the public façade that assembles the OKR application from a Config plus
// injectable seams. internal/* packages stay internal; this is the only exported surface, so a
// private okrs-saas module can import the box, register SaaS implementations, and mount its
// control-plane routes in one process.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"okrs/internal/auth"
	httpserver "okrs/internal/http"
	"okrs/internal/platform/entitlements"
	"okrs/internal/platform/eventbus"
	"okrs/internal/store"
	"okrs/internal/store/grants"
	"okrs/internal/store/memberships"
	"okrs/internal/store/tenants"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config assembles the application. The caller owns the DB connection and migrations; everything
// else has an OSS default (so a near-empty Config produces the OSS box).
type Config struct {
	Pool                 *pgxpool.Pool
	Logger               *slog.Logger
	Zone                 *time.Location
	Auth                 auth.Config
	EntitlementsName     string   // registry key; "" → "unlimited"
	NoMembershipName     string   // "" → "stub"
	ResolveStrategyNames []string // nil → ["session"]
	AssetsDev            bool     // serve development vendored React; false → production build
	// Embedded control-plane route mounts (SaaS), one per middleware tier; each nil in OSS.
	PublicRoutes func(chi.Router)
	AuthedRoutes func(chi.Router)
	TenantRoutes func(chi.Router)
}

type App struct {
	Handler http.Handler

	bus *eventbus.Bus
	// stopBackground cancels the context srv.StartBackground was launched with (the
	// health check-in refresh, progress-snapshot and notification-retention loops in
	// internal/scheduler). Called from Close, before draining the bus, so those loops
	// stop touching the pool during the same exit window the bus drain happens in —
	// see the doc comment on Close below for why the order matters.
	stopBackground context.CancelFunc
}

// Close releases every background resource New started, in the order the caller in
// cmd/server relies on: stop the scheduler loops first (health check-in refresh,
// progress snapshots, notification retention — internal/scheduler), so none of them
// can start a new query against the pool during shutdown, THEN drain the event bus's
// async subscriber goroutines (none in the OSS box today, but a SaaS build's notifier
// registers one) so anything already in flight finishes against a pool that is still
// open. The caller closes the pool only after Close returns. Waits up to timeout for
// the bus to drain before giving up; the scheduler loops stop immediately (ctx
// cancellation, no drain to wait for).
func (a *App) Close(timeout time.Duration) error {
	a.stopBackground()
	return a.bus.Close(timeout)
}

// withAuthDefaults fills unset auth fields with OSS defaults so a near-empty Config yields the
// OSS box (AUTH_MODE=disabled) rather than an unusable empty Mode — which would demand a login
// while no providers are configured. Explicitly-set fields are left untouched.
func withAuthDefaults(c auth.Config) auth.Config {
	d := auth.DefaultConfig()
	if c.Mode == "" {
		c.Mode = d.Mode
	}
	if c.SessionCookie == "" {
		c.SessionCookie = d.SessionCookie
	}
	if c.SessionTTL == 0 {
		c.SessionTTL = d.SessionTTL
	}
	return c
}

func New(cfg Config) (*App, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("app: Config.Pool is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	zone := cfg.Zone
	if zone == nil {
		zone = time.UTC
	}

	st := store.New(cfg.Pool)
	grantsCache := grants.NewGrantsCache(st.Grants)

	authMgr, err := auth.NewManager(withAuthDefaults(cfg.Auth), st)
	if err != nil {
		return nil, fmt.Errorf("app: auth: %w", err)
	}

	// Resolve strategies (default: session).
	names := cfg.ResolveStrategyNames
	if len(names) == 0 {
		names = []string{"session"}
	}
	// One tenant + membership cache instance, shared between the resolver (read) and the
	// server's provisioning/onboarding services (invalidate). Two instances would let a write
	// invalidate a cache the resolver never reads, so removals/adds lag by the cache TTL.
	tenantCache := tenants.NewTenantCache(st.Tenants)
	membershipCache := memberships.NewMembershipCache(st.Memberships)
	deps := auth.ResolveDeps{
		Tenants:     tenantCache,
		Memberships: membershipCache,
	}
	var strategies []auth.ResolveStrategy
	for _, name := range names {
		f, ok := auth.ResolveStrategyFactoryByName(name)
		if !ok {
			return nil, fmt.Errorf("app: unknown resolve strategy %q", name)
		}
		strategies = append(strategies, f(deps))
	}

	// Entitlements implementation.
	entName := cfg.EntitlementsName
	if entName == "" {
		entName = "unlimited"
	}
	entFactory, ok := entitlements.Get(entName)
	if !ok {
		return nil, fmt.Errorf("app: unknown entitlements %q", entName)
	}

	// The bus is created before the server so httpdeps.Build (called from within
	// NewServer) can register subscribers on it; Start only happens below, once
	// assembly is done — Subscribe after Start panics by design, so this order is
	// what catches an assembly-ordering mistake immediately instead of in prod.
	bus := eventbus.New(logger)

	srv, err := httpserver.NewServer(st, grantsCache, logger, zone, authMgr, bus, httpserver.Options{
		Resolver:         auth.NewTenantResolver(strategies...),
		TenantCache:      tenantCache,
		MembershipCache:  membershipCache,
		Entitlements:     entFactory(),
		NoMembershipName: cfg.NoMembershipName,
		AssetsDev:        cfg.AssetsDev,
		PublicRoutes:     cfg.PublicRoutes,
		AuthedRoutes:     cfg.AuthedRoutes,
		TenantRoutes:     cfg.TenantRoutes,
	})
	if err != nil {
		return nil, err
	}
	bus.Start(context.Background())
	// Фоновые петли запускаются здесь, а не внутри Routes(): сборка роутера должна
	// оставаться чистой, иначе её нельзя вызвать в тесте без goroutine и БД.
	handler := srv.Routes()
	// A dedicated cancellable context, not context.Background(): Close needs a way to
	// stop these loops on shutdown (see Close's doc comment), and Background() can
	// never be cancelled.
	bgCtx, stopBackground := context.WithCancel(context.Background())
	srv.StartBackground(bgCtx)
	return &App{Handler: handler, bus: bus, stopBackground: stopBackground}, nil
}
