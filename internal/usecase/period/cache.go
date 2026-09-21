package period

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/platform/logging"
)

// PeriodLoader loads raw data for a period (within a tenant) from the DB.
// Built by NewPeriodLoader over the entity services.
type PeriodLoader func(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error)

// cacheKey identifies a cached period within a tenant. periodID alone is globally unique,
// but keying by tenant keeps the cache and refresh loop explicitly tenant-scoped.
type cacheKey struct {
	tenantID int64
	periodID int64
}

// ActivePeriod is one tenant's active period, used by the refresh loop.
type ActivePeriod struct {
	Scope    domain.TenantScope
	PeriodID int64
}

// PeriodData is the pre-loaded data for one period, held in PeriodCache.
type PeriodData struct {
	PeriodID    int64
	Period      domain.Period
	Teams       []domain.Team
	GoalsByTeam map[int64][]domain.Goal
	Statuses    map[int64]domain.TeamPeriodStatus
	CachedAt    time.Time
}

// PeriodCache holds PeriodData per (tenant, period) with TTL-based expiry.
type PeriodCache struct {
	mu      sync.RWMutex
	periods map[cacheKey]*PeriodData
	ttl     time.Duration
	loader  PeriodLoader
	logger  *slog.Logger
}

// NewPeriodCache creates a new cache with the given loader and TTL.
func NewPeriodCache(loader PeriodLoader, ttl time.Duration, logger *slog.Logger) *PeriodCache {
	return &PeriodCache{
		periods: make(map[cacheKey]*PeriodData),
		ttl:     ttl,
		loader:  loader,
		logger:  logger,
	}
}

// Get returns cached PeriodData for the given tenant+period, loading from DB if stale or absent.
func (c *PeriodCache) Get(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error) {
	key := cacheKey{tenantID: scope.TenantID, periodID: periodID}
	c.mu.RLock()
	entry := c.periods[key]
	c.mu.RUnlock()

	if entry != nil && time.Since(entry.CachedAt) < c.ttl {
		return entry, nil
	}
	return c.reload(ctx, scope, periodID)
}

func (c *PeriodCache) reload(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error) {
	data, err := c.loader(ctx, scope, periodID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.periods[cacheKey{tenantID: scope.TenantID, periodID: periodID}] = data
	c.mu.Unlock()
	return data, nil
}

// InvalidateAll clears all cached entries; next Get will reload from DB.
func (c *PeriodCache) InvalidateAll() {
	c.mu.Lock()
	c.periods = make(map[cacheKey]*PeriodData)
	c.mu.Unlock()
}

// StartRefreshLoop runs a background goroutine that proactively refreshes the active period
// of every tenant. activePeriodsFn returns one entry per tenant that has an active period.
func (c *PeriodCache) StartRefreshLoop(ctx context.Context, interval time.Duration, activePeriodsFn func(ctx context.Context) []ActivePeriod) {
	// Один тик — одна защищённая единица работы. Перехват стоит вокруг тела
	// тика, а не вокруг всей горутины: снаружи паника остановила бы обновление
	// кеша навсегда, и кеш молча отдавал бы устаревшие данные до перезапуска.
	// Без перехвата вовсе паника здесь уносит весь процесс, не оставляя
	// структурированной записи о причине.
	tick := func() {
		defer logging.RecoverBackground(ctx, c.logger, "period_cache_refresh")

		for _, a := range activePeriodsFn(ctx) {
			if a.PeriodID == 0 {
				continue
			}
			if _, err := c.reload(ctx, a.Scope, a.PeriodID); err != nil {
				if c.logger != nil {
					c.logger.WarnContext(ctx, "period cache refresh failed",
						slog.String(logging.KeyEvent, logging.EventBackgroundTask),
						slog.String("task", "period_cache_refresh"),
						slog.String("outcome", "failed"),
						slog.Int64(logging.KeyTenantID, a.Scope.TenantID),
						slog.Int64("period_id", a.PeriodID),
						slog.String("err", err.Error()))
				}
			}
		}
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tick()
			}
		}
	}()
}
