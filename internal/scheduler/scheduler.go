// Package scheduler runs the periodic background passes: refreshing the health
// check-in cache and materialising per-team progress snapshots.
//
// It lives outside internal/http on purpose. Previously these loops were started as a
// side effect of building the router, which meant Routes() could not be called in a
// test without spawning goroutines and touching the database. Starting them belongs to
// application assembly (app.New), not to route registration.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"okrs/internal/core/domain"
	hcsvc "okrs/internal/service/healthcheckin"
	progresssnapsvc "okrs/internal/service/progresssnap"
)

// progressSnapshotLockKey is a fixed advisory-lock key so that, across K8s replicas,
// only one instance runs the daily snapshot pass (the upsert is idempotent regardless).
const progressSnapshotLockKey = 918273645

// snapshotCheckInterval is how often the loop wakes to check whether any period is due a
// new snapshot. The actual spacing between recorded points is the per-tenant
// progress_snapshot_interval_days setting; this is just the polling granularity.
const snapshotCheckInterval = time.Hour

// hcRefreshInterval is how often the health check-in cache is proactively refreshed for
// every tenant's currently-active period, so the first request after a TTL expiry does
// not pay the cold-load cost.
const hcRefreshInterval = 5 * time.Minute

// TenantLister enumerates the tenants a pass must cover.
type TenantLister interface {
	List(ctx context.Context) ([]domain.Tenant, error)
}

// ActivePeriodLister returns the periods whose date range contains the given day.
type ActivePeriodLister interface {
	ListActivePeriodsForDate(ctx context.Context, scope domain.TenantScope, date time.Time) ([]domain.Period, error)
}

// PeriodFinder resolves a tenant's currently-active period. Narrow port so the
// scheduler does not depend on the whole period service. *period.Service satisfies it.
type PeriodFinder interface {
	FindForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error)
}

// SnapshotRunner records one pass of per-team progress points. Narrow port for the
// same reason. *period.UseCase satisfies it.
type SnapshotRunner interface {
	SnapshotActivePeriods(ctx context.Context, day time.Time, actives []hcsvc.Active) error
}

type Deps struct {
	DB       *pgxpool.Pool
	HCCache  *hcsvc.Cache
	Snapshot SnapshotRunner
	Periods  PeriodFinder
	Active   ActivePeriodLister
	Snaps    *progresssnapsvc.Service
	Tenants  TenantLister
	Settings hcsvc.SettingsReader
	Zone     *time.Location
	Logger   *slog.Logger
}

type Scheduler struct {
	deps Deps
	// lastAttempt throttles periods that legitimately produce no snapshots (only no-goal
	// or forming teams): those never advance LatestDate, so without an attempt record
	// they'd be reprocessed every poll. Only the single advisory-lock holder runs the
	// pass per tick, so this map is mutated by one goroutine.
	lastAttempt map[[2]int64]time.Time
}

func New(deps Deps) *Scheduler {
	return &Scheduler{deps: deps, lastAttempt: make(map[[2]int64]time.Time)}
}

// Start launches the background loops. They stop when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.deps.HCCache.StartRefreshLoop(ctx, hcRefreshInterval, s.activePeriods)
	s.startProgressSnapshotLoop(ctx, snapshotCheckInterval)
}

// activePeriods enumerates each tenant's currently-active (date-based) period,
// so closed/archived periods are naturally excluded.
func (s *Scheduler) activePeriods(ctx context.Context) []hcsvc.Active {
	now := time.Now().In(s.deps.Zone)
	tenants, err := s.deps.Tenants.List(ctx)
	if err != nil {
		return nil
	}
	var active []hcsvc.Active
	for _, tn := range tenants {
		scope := domain.TenantScope{TenantID: tn.ID}
		p, err := s.deps.Periods.FindForDate(ctx, scope, now)
		if err != nil {
			continue
		}
		active = append(active, hcsvc.Active{Scope: scope, PeriodID: p.ID})
	}
	return active
}

// snapshotDuePeriods returns every active period whose configured interval
// (progress_snapshot_interval_days, ≥1) has elapsed since its last recorded point.
// Nested periods (year + quarter) each get their own points.
func (s *Scheduler) snapshotDuePeriods(ctx context.Context) []hcsvc.Active {
	now := time.Now().In(s.deps.Zone)
	tenants, err := s.deps.Tenants.List(ctx)
	if err != nil {
		return nil
	}
	var due []hcsvc.Active
	for _, tn := range tenants {
		scope := domain.TenantScope{TenantID: tn.ID}
		intervalDays := hcsvc.LoadProgressSnapshotIntervalDays(ctx, scope, s.deps.Settings)
		periods, err := s.deps.Active.ListActivePeriodsForDate(ctx, scope, now)
		if err != nil {
			continue
		}
		for _, p := range periods {
			latest, has, err := s.deps.Snaps.LatestDate(ctx, scope, p.ID)
			if err != nil {
				continue
			}
			key := [2]int64{tn.ID, p.ID}
			// Effective "last handled" = the newer of the recorded point and our last
			// attempt, so empty passes still count toward the interval.
			if a, ok := s.lastAttempt[key]; ok && (!has || a.After(latest)) {
				latest, has = a, true
			}
			if !has || daysBetween(latest, now) >= intervalDays {
				due = append(due, hcsvc.Active{Scope: scope, PeriodID: p.ID})
				s.lastAttempt[key] = now
			}
		}
	}
	return due
}

// startProgressSnapshotLoop runs a background goroutine that materialises each active
// period's per-team progress once per interval. An initial pass runs at startup.
func (s *Scheduler) startProgressSnapshotLoop(ctx context.Context, interval time.Duration) {
	run := func() {
		conn, err := s.deps.DB.Acquire(ctx)
		if err != nil {
			return
		}
		defer conn.Release()
		var got bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, progressSnapshotLockKey).Scan(&got); err != nil || !got {
			return // another replica holds the lock this cycle
		}
		defer func() { _, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, progressSnapshotLockKey) }()

		day := time.Now().In(s.deps.Zone)
		if err := s.deps.Snapshot.SnapshotActivePeriods(ctx, day, s.snapshotDuePeriods(ctx)); err != nil && s.deps.Logger != nil {
			s.deps.Logger.Warn("progress snapshot failed", "err", err)
		}
	}
	go func() {
		run() // capture today at startup
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// daysBetween returns the whole-day difference between two dates (b - a), tz-agnostic.
func daysBetween(a, b time.Time) int {
	ad := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	bd := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(bd.Sub(ad).Hours()) / 24
}
