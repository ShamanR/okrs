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
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"okrs/internal/core/domain"
	"okrs/internal/platform/logging"
	hcsvc "okrs/internal/service/healthcheckin"
	progresssnapsvc "okrs/internal/service/progresssnap"
)

// progressSnapshotLockKey is a fixed advisory-lock key so that, across K8s replicas,
// only one instance runs the daily snapshot pass (the upsert is idempotent regardless).
const progressSnapshotLockKey = 918273645

// notificationRetentionLockKey is a fixed advisory-lock key so only one replica runs
// the daily notification purge (deleting twice is harmless, but the scan is not
// free). Deliberately different from progressSnapshotLockKey — sharing a key would
// make the two independent passes wait on each other for no reason.
const notificationRetentionLockKey = 918273646

// snapshotCheckInterval is how often the loop wakes to check whether any period is due a
// new snapshot. The actual spacing between recorded points is the per-tenant
// progress_snapshot_interval_days setting; this is just the polling granularity.
const snapshotCheckInterval = time.Hour

// hcRefreshInterval is how often the health check-in cache is proactively refreshed for
// every tenant's currently-active period, so the first request after a TTL expiry does
// not pay the cold-load cost.
const hcRefreshInterval = 5 * time.Minute

// notificationRetentionInterval is how often the notification purge pass runs. Daily
// is plenty for a retention window measured in months.
const notificationRetentionInterval = 24 * time.Hour

// Read notifications survive 90 days, anything at all survives 180. Constants until
// there is evidence someone needs different — see notifications design spec §7.6.
const (
	notificationReadRetentionDays = 90
	notificationAnyRetentionDays  = 180
)

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

// NotificationPurger removes notifications past retention. Narrow port so the
// scheduler does not depend on the whole notification service. *notification.Service
// satisfies it.
type NotificationPurger interface {
	Purge(ctx context.Context, readDays, anyDays int) (int64, error)
}

type Deps struct {
	DB            *pgxpool.Pool
	HCCache       *hcsvc.Cache
	Snapshot      SnapshotRunner
	Periods       PeriodFinder
	Active        ActivePeriodLister
	Snaps         *progresssnapsvc.Service
	Tenants       TenantLister
	Settings      hcsvc.SettingsReader
	Notifications NotificationPurger
	Zone          *time.Location
	Logger        *slog.Logger
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
	s.startNotificationRetentionLoop(ctx, notificationRetentionInterval)
}

// logger возвращает логгер планировщика. Deps.Logger может быть nil в тестах,
// поэтому проверка живёт в одном месте, а не перед каждой записью.
func (s *Scheduler) logger() *slog.Logger {
	if s.deps.Logger == nil {
		return logging.Discard()
	}
	return s.deps.Logger
}

// runLockedPass выполняет один проход фоновой задачи под advisory-lock и логирует
// его запуск, завершение и ошибки.
//
// Логирование живёт здесь, а не в каждом цикле, чтобы у всех фоновых задач был
// одинаковый состав полей: иначе выборка «что сейчас падает в фоне» в Kibana не
// строится одним запросом.
func (s *Scheduler) runLockedPass(ctx context.Context, name string, lockKey int, pass func(context.Context) ([]any, error)) {
	logger := s.logger()
	// Перехват на этом уровне закрывает работу с соединением и advisory-локом;
	// сам проход прикрыт таким же перехватом внутри runPass. Оба нужны:
	// внутренний проверяется тестом без базы данных, внешний — единственное,
	// что стоит между паникой в драйвере и падением всего процесса.
	defer logging.RecoverBackground(ctx, logger, name)

	conn, err := s.deps.DB.Acquire(ctx)
	if err != nil {
		logger.WarnContext(ctx, "background task could not acquire a connection",
			taskAttrs(name, slog.String("outcome", "skipped"), slog.String("err", err.Error()))...)
		return
	}
	defer conn.Release()

	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&got); err != nil {
		logger.WarnContext(ctx, "background task could not take its lock",
			taskAttrs(name, slog.String("outcome", "skipped"), slog.String("err", err.Error()))...)
		return
	}
	if !got {
		// Лок держит другая реплика. В Kubernetes это ожидаемый исход на всех
		// репликах, кроме одной, и на info он завалил бы лог шумом.
		logger.DebugContext(ctx, "background task skipped, lock held by another replica",
			taskAttrs(name, slog.String("outcome", "skipped"))...)
		return
	}
	defer func() { _, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey) }()

	s.runPass(ctx, name, pass)
}

// runPass выполняет проход и логирует его запуск, длительность и исход.
//
// Вынесено из runLockedPass, чтобы состав записей проверялся тестом без базы данных:
// сам advisory-lock к логированию исхода ничего не добавляет, а требовать поднятый
// Postgres ради проверки набора полей — плохой обмен.
func (s *Scheduler) runPass(ctx context.Context, name string, pass func(context.Context) ([]any, error)) {
	logger := s.logger()
	// Паника прохода не должна ни ронять процесс, ни останавливать цикл:
	// перехват здесь оставляет тикер живым, и следующий тик отработает.
	defer logging.RecoverBackground(ctx, logger, name)

	logger.InfoContext(ctx, "background task started", taskAttrs(name)...)
	start := time.Now()
	extra, err := pass(ctx)
	done := func(more ...any) []any {
		return taskAttrs(name, append([]any{slog.Int64("duration_ms", time.Since(start).Milliseconds())}, more...)...)
	}
	if err != nil {
		logger.WarnContext(ctx, "background task failed",
			done(slog.String("outcome", "failed"), slog.String("err", err.Error()))...)
		return
	}
	logger.InfoContext(ctx, "background task finished",
		done(append([]any{slog.String("outcome", "ok")}, extra...)...)...)
}

// taskAttrs — общий префикс полей записи фоновой задачи. Один источник, чтобы
// выборка «что сейчас падает в фоне» в Kibana строилась одним запросом.
func taskAttrs(name string, extra ...any) []any {
	return append([]any{
		slog.String(logging.KeyEvent, logging.EventBackgroundTask),
		slog.String("task", name),
	}, extra...)
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
// Ошибки обнаружения возвращаются вызывающему, а не превращаются в пустой список:
// иначе недоступная база даёт «ничего не надо делать», и проход отчитывается
// об успехе, молча пропустив организации.
//
// Частичный результат возвращается вместе с ошибкой: снять снимки с тех
// организаций, где обнаружение удалось, полезнее, чем отказаться от всего
// прохода целиком — но исход задачи всё равно отразит отказ.
func (s *Scheduler) snapshotDuePeriods(ctx context.Context) ([]hcsvc.Active, error) {
	now := time.Now().In(s.deps.Zone)
	tenants, err := s.deps.Tenants.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("список организаций: %w", err)
	}
	var (
		due  []hcsvc.Active
		errs []error
	)
	for _, tn := range tenants {
		scope := domain.TenantScope{TenantID: tn.ID}
		intervalDays := hcsvc.LoadProgressSnapshotIntervalDays(ctx, scope, s.deps.Settings)
		periods, err := s.deps.Active.ListActivePeriodsForDate(ctx, scope, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("активные периоды организации %d: %w", tn.ID, err))
			continue
		}
		for _, p := range periods {
			latest, has, err := s.deps.Snaps.LatestDate(ctx, scope, p.ID)
			if err != nil {
				errs = append(errs, fmt.Errorf("последний снимок периода %d: %w", p.ID, err))
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
	return due, errors.Join(errs...)
}

// startProgressSnapshotLoop runs a background goroutine that materialises each active
// period's per-team progress once per interval. An initial pass runs at startup.
func (s *Scheduler) startProgressSnapshotLoop(ctx context.Context, interval time.Duration) {
	run := func() {
		s.runLockedPass(ctx, "progress_snapshot", progressSnapshotLockKey, func(ctx context.Context) ([]any, error) {
			day := time.Now().In(s.deps.Zone)
			due, discoverErr := s.snapshotDuePeriods(ctx)
			// Снимки снимаются по тому, что удалось обнаружить, но отказ
			// обнаружения попадает в исход задачи.
			return nil, errors.Join(discoverErr, s.deps.Snapshot.SnapshotActivePeriods(ctx, day, due))
		})
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

// startNotificationRetentionLoop runs a background goroutine that purges
// notifications past retention once per interval. An initial pass runs at startup,
// same shape as startProgressSnapshotLoop above — separate advisory-lock key so the
// two passes never wait on each other.
func (s *Scheduler) startNotificationRetentionLoop(ctx context.Context, interval time.Duration) {
	run := func() {
		s.runLockedPass(ctx, "notification_retention", notificationRetentionLockKey, func(ctx context.Context) ([]any, error) {
			n, err := s.deps.Notifications.Purge(ctx, notificationReadRetentionDays, notificationAnyRetentionDays)
			if err != nil {
				return nil, err
			}
			return []any{slog.Int64("deleted", n)}, nil
		})
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
