package tenantsettings

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"okrs/internal/core/domain"
)

const defaultTenantSettingsCacheTTL = 5 * time.Minute

type snapshotBackend interface {
	GetAll(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error)
}

type cacheEntry struct {
	snapshot map[string]json.RawMessage
	cachedAt time.Time
}

// TenantSettingsCache wraps a snapshot backend with a TTL + invalidate-on-write cache,
// keyed by tenant id. Single-process; cross-instance invalidation is a SaaS-scale concern.
type TenantSettingsCache struct {
	backend snapshotBackend
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[int64]cacheEntry
}

func NewTenantSettingsCache(repo *TenantSettingsRepository) *TenantSettingsCache {
	return newTenantSettingsCacheWithBackend(repo, defaultTenantSettingsCacheTTL)
}

func newTenantSettingsCacheWithBackend(b snapshotBackend, ttl time.Duration) *TenantSettingsCache {
	return &TenantSettingsCache{backend: b, ttl: ttl, entries: make(map[int64]cacheEntry)}
}

func (c *TenantSettingsCache) GetAll(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	c.mu.RLock()
	e, ok := c.entries[scope.TenantID]
	c.mu.RUnlock()
	if ok && time.Since(e.cachedAt) < c.ttl {
		return e.snapshot, nil
	}
	snap, err := c.backend.GetAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries[scope.TenantID] = cacheEntry{snapshot: snap, cachedAt: time.Now()}
	c.mu.Unlock()
	return snap, nil
}

func (c *TenantSettingsCache) Invalidate(tenantID int64) {
	c.mu.Lock()
	delete(c.entries, tenantID)
	c.mu.Unlock()
}

func (c *TenantSettingsCache) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[int64]cacheEntry)
	c.mu.Unlock()
}
