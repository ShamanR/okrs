// Package app is the public façade that assembles the OKR application from a Config plus
// injectable seams. internal/* packages stay internal; this is the only exported surface, so a
// private okrs-saas module can import the box, register SaaS implementations, and mount its
// control-plane routes in one process.
package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"okrs/internal/auth"
	"okrs/internal/entitlements"
	httpserver "okrs/internal/http"
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
	// Embedded control-plane route mounts (SaaS), one per middleware tier; each nil in OSS.
	PublicRoutes func(chi.Router)
	AuthedRoutes func(chi.Router)
	TenantRoutes func(chi.Router)
}

type App struct {
	Handler http.Handler
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

	authMgr, err := auth.NewManager(cfg.Auth, st)
	if err != nil {
		return nil, fmt.Errorf("app: auth: %w", err)
	}

	// Resolve strategies (default: session).
	names := cfg.ResolveStrategyNames
	if len(names) == 0 {
		names = []string{"session"}
	}
	deps := auth.ResolveDeps{
		Tenants:     tenants.NewTenantCache(st.Tenants),
		Memberships: memberships.NewMembershipCache(st.Memberships),
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

	srv, err := httpserver.NewServer(st, grantsCache, logger, zone, authMgr, httpserver.Options{
		Resolver:         auth.NewTenantResolver(strategies...),
		Entitlements:     entFactory(),
		NoMembershipName: cfg.NoMembershipName,
		PublicRoutes:     cfg.PublicRoutes,
		AuthedRoutes:     cfg.AuthedRoutes,
		TenantRoutes:     cfg.TenantRoutes,
	})
	if err != nil {
		return nil, err
	}
	return &App{Handler: srv.Routes()}, nil
}
