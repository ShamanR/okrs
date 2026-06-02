package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// periodLoader loads raw data for a period from the DB.
// Implemented as a closure in server.go that captures store repos.
type periodLoader func(ctx context.Context, periodID int64) (*PeriodData, error)

// HealthCheckInCache holds PeriodData per period_id with TTL-based expiry.
type HealthCheckInCache struct {
	mu      sync.RWMutex
	periods map[int64]*PeriodData
	ttl     time.Duration
	loader  periodLoader
	logger  *slog.Logger
}

// NewHealthCheckInCache creates a new cache with the given loader and TTL.
func NewHealthCheckInCache(loader periodLoader, ttl time.Duration, logger *slog.Logger) *HealthCheckInCache {
	return &HealthCheckInCache{
		periods: make(map[int64]*PeriodData),
		ttl:     ttl,
		loader:  loader,
		logger:  logger,
	}
}

// Get returns cached PeriodData for the given period, loading from DB if stale or absent.
func (c *HealthCheckInCache) Get(ctx context.Context, periodID int64) (*PeriodData, error) {
	c.mu.RLock()
	entry := c.periods[periodID]
	c.mu.RUnlock()

	if entry != nil && time.Since(entry.CachedAt) < c.ttl {
		return entry, nil
	}
	return c.reload(ctx, periodID)
}

func (c *HealthCheckInCache) reload(ctx context.Context, periodID int64) (*PeriodData, error) {
	data, err := c.loader(ctx, periodID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.periods[periodID] = data
	c.mu.Unlock()
	return data, nil
}

// InvalidateAll clears all cached entries; next Get will reload from DB.
func (c *HealthCheckInCache) InvalidateAll() {
	c.mu.Lock()
	c.periods = make(map[int64]*PeriodData)
	c.mu.Unlock()
}

// StartRefreshLoop runs a background goroutine that proactively refreshes the active period.
// activePeriodFn returns 0 if no active period exists.
func (c *HealthCheckInCache) StartRefreshLoop(ctx context.Context, interval time.Duration, activePeriodFn func(ctx context.Context) int64) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				periodID := activePeriodFn(ctx)
				if periodID == 0 {
					continue
				}
				if _, err := c.reload(ctx, periodID); err != nil {
					if c.logger != nil {
						c.logger.Warn("health-checkin cache refresh failed", "err", err)
					}
				}
			}
		}
	}()
}
