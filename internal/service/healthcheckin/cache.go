package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"okrs/internal/core/domain"
)

// periodLoader loads raw data for a period (within a tenant) from the DB.
// Implemented as a closure in server.go that captures store repos.
type periodLoader func(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error)

// hcKey identifies a cached period within a tenant. periodID alone is globally unique,
// but keying by tenant keeps the cache and refresh loop explicitly tenant-scoped.
type hcKey struct {
	tenantID int64
	periodID int64
}

// HCActive is one tenant's active period, used by the refresh loop.
type HCActive struct {
	Scope    domain.TenantScope
	PeriodID int64
}

// HealthCheckInCache holds PeriodData per (tenant, period) with TTL-based expiry.
type HealthCheckInCache struct {
	mu      sync.RWMutex
	periods map[hcKey]*PeriodData
	ttl     time.Duration
	loader  periodLoader
	logger  *slog.Logger
}

// NewHealthCheckInCache creates a new cache with the given loader and TTL.
func NewHealthCheckInCache(loader periodLoader, ttl time.Duration, logger *slog.Logger) *HealthCheckInCache {
	return &HealthCheckInCache{
		periods: make(map[hcKey]*PeriodData),
		ttl:     ttl,
		loader:  loader,
		logger:  logger,
	}
}

// Get returns cached PeriodData for the given tenant+period, loading from DB if stale or absent.
func (c *HealthCheckInCache) Get(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error) {
	key := hcKey{tenantID: scope.TenantID, periodID: periodID}
	c.mu.RLock()
	entry := c.periods[key]
	c.mu.RUnlock()

	if entry != nil && time.Since(entry.CachedAt) < c.ttl {
		return entry, nil
	}
	return c.reload(ctx, scope, periodID)
}

func (c *HealthCheckInCache) reload(ctx context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error) {
	data, err := c.loader(ctx, scope, periodID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.periods[hcKey{tenantID: scope.TenantID, periodID: periodID}] = data
	c.mu.Unlock()
	return data, nil
}

// InvalidateAll clears all cached entries; next Get will reload from DB.
func (c *HealthCheckInCache) InvalidateAll() {
	c.mu.Lock()
	c.periods = make(map[hcKey]*PeriodData)
	c.mu.Unlock()
}

// StartRefreshLoop runs a background goroutine that proactively refreshes the active period
// of every tenant. activePeriodsFn returns one entry per tenant that has an active period.
func (c *HealthCheckInCache) StartRefreshLoop(ctx context.Context, interval time.Duration, activePeriodsFn func(ctx context.Context) []HCActive) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, a := range activePeriodsFn(ctx) {
					if a.PeriodID == 0 {
						continue
					}
					if _, err := c.reload(ctx, a.Scope, a.PeriodID); err != nil {
						if c.logger != nil {
							c.logger.Warn("health-checkin cache refresh failed", "tenant", a.Scope.TenantID, "period", a.PeriodID, "err", err)
						}
					}
				}
			}
		}
	}()
}
