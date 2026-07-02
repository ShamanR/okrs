package tenants

import (
	"context"
	"sync"
	"time"

	"okrs/internal/domain"
)

// tenantBackend is the read contract TenantCache needs. *TenantRepository satisfies it.
type tenantBackend interface {
	GetByID(ctx context.Context, id int64) (*domain.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
}

const defaultTenantCacheTTL = 5 * time.Minute

type cachedTenant struct {
	t        *domain.Tenant
	cachedAt time.Time
}

// TenantCache is a TTL-based read-through cache over a TenantRepository, serving the
// per-request tenant-resolve hot path. Writes (suspend/rename/delete) must call Invalidate.
type TenantCache struct {
	backend tenantBackend
	ttl     time.Duration
	mu      sync.RWMutex
	byID    map[int64]cachedTenant
	bySlug  map[string]cachedTenant
}

// NewTenantCache wraps a TenantRepository with a 5-minute cache keyed by id and slug.
func NewTenantCache(r *TenantRepository) *TenantCache {
	return newTenantCacheWithBackend(r, defaultTenantCacheTTL)
}

func newTenantCacheWithBackend(b tenantBackend, ttl time.Duration) *TenantCache {
	return &TenantCache{
		backend: b,
		ttl:     ttl,
		byID:    make(map[int64]cachedTenant),
		bySlug:  make(map[string]cachedTenant),
	}
}

func (c *TenantCache) GetByID(ctx context.Context, id int64) (*domain.Tenant, error) {
	c.mu.RLock()
	e, ok := c.byID[id]
	c.mu.RUnlock()
	if ok && time.Since(e.cachedAt) < c.ttl {
		return e.t, nil
	}
	t, err := c.backend.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	c.store(t)
	return t, nil
}

func (c *TenantCache) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	c.mu.RLock()
	e, ok := c.bySlug[slug]
	c.mu.RUnlock()
	if ok && time.Since(e.cachedAt) < c.ttl {
		return e.t, nil
	}
	t, err := c.backend.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	c.store(t)
	return t, nil
}

func (c *TenantCache) store(t *domain.Tenant) {
	entry := cachedTenant{t: t, cachedAt: time.Now()}
	c.mu.Lock()
	c.byID[t.ID] = entry
	c.bySlug[t.Slug] = entry
	c.mu.Unlock()
}

// Invalidate drops cached entries for one tenant (call after a tenant write).
func (c *TenantCache) Invalidate(id int64) {
	c.mu.Lock()
	delete(c.byID, id)
	for slug, e := range c.bySlug {
		if e.t != nil && e.t.ID == id {
			delete(c.bySlug, slug)
		}
	}
	c.mu.Unlock()
}

// InvalidateAll clears the whole cache.
func (c *TenantCache) InvalidateAll() {
	c.mu.Lock()
	c.byID = make(map[int64]cachedTenant)
	c.bySlug = make(map[string]cachedTenant)
	c.mu.Unlock()
}
