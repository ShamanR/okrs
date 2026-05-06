package store

import (
	"context"
	"sync"
	"time"
)

// grantsBackend is the persistence contract used by GrantsCache.
// *Store satisfies it; tests can inject a fake.
type grantsBackend interface {
	loadAllGrants(ctx context.Context) (map[int64][]HierarchyGrant, error)
	addUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error
	removeUserGrant(ctx context.Context, userID, teamID int64) error
	ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error)
}

// storeGrantsBackend adapts *Store to grantsBackend.
type storeGrantsBackend struct{ st *Store }

func (b *storeGrantsBackend) loadAllGrants(ctx context.Context) (map[int64][]HierarchyGrant, error) {
	return b.st.listAllGrants(ctx)
}
func (b *storeGrantsBackend) addUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error {
	return b.st.AddUserGrant(ctx, userID, teamID, grantedByUserID)
}
func (b *storeGrantsBackend) removeUserGrant(ctx context.Context, userID, teamID int64) error {
	return b.st.RemoveUserGrant(ctx, userID, teamID)
}
func (b *storeGrantsBackend) ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error) {
	return b.st.ListDescendantTeamIDs(ctx, rootIDs)
}

// GrantsCache is an in-memory read-through cache for the user_hierarchy_grants table.
// The cache is invalidated immediately on any write and refreshed lazily on the next read
// after the TTL (5 minutes) has elapsed.
//
// The returned map from AllGrants is replaced atomically on refresh and never mutated
// in-place, so callers may safely read the reference without copying.
type GrantsCache struct {
	backend     grantsBackend
	mu          sync.RWMutex
	data        map[int64][]HierarchyGrant // keyed by userID; replaced atomically on refresh
	refreshedAt time.Time
	ttl         time.Duration
}

const defaultGrantsCacheTTL = 5 * time.Minute

// NewGrantsCache wraps st with a 5-minute in-memory cache for user_hierarchy_grants.
func NewGrantsCache(st *Store) *GrantsCache {
	return &GrantsCache{
		backend: &storeGrantsBackend{st},
		ttl:     defaultGrantsCacheTTL,
	}
}

// newGrantsCacheWithBackend creates a GrantsCache with a custom backend (used in tests).
func newGrantsCacheWithBackend(b grantsBackend, ttl time.Duration) *GrantsCache {
	return &GrantsCache{backend: b, ttl: ttl}
}

// ListUserGrants returns the cached grants for userID.
func (c *GrantsCache) ListUserGrants(ctx context.Context, userID int64) ([]HierarchyGrant, error) {
	data, err := c.ensureFresh(ctx)
	if err != nil {
		return nil, err
	}
	return data[userID], nil
}

// AddUserGrant writes to the backing store and invalidates the cache.
func (c *GrantsCache) AddUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error {
	if err := c.backend.addUserGrant(ctx, userID, teamID, grantedByUserID); err != nil {
		return err
	}
	c.invalidate()
	return nil
}

// RemoveUserGrant writes to the backing store and invalidates the cache.
func (c *GrantsCache) RemoveUserGrant(ctx context.Context, userID, teamID int64) error {
	if err := c.backend.removeUserGrant(ctx, userID, teamID); err != nil {
		return err
	}
	c.invalidate()
	return nil
}

// ListDescendantTeamIDs delegates to the backing store (queries the teams table, not grants).
func (c *GrantsCache) ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error) {
	return c.backend.ListDescendantTeamIDs(ctx, rootIDs)
}

// AllGrants returns the full cached snapshot of user_hierarchy_grants as a map[userID][]HierarchyGrant.
// The returned map is safe to read concurrently; do not write to it.
func (c *GrantsCache) AllGrants(ctx context.Context) (map[int64][]HierarchyGrant, error) {
	return c.ensureFresh(ctx)
}

func (c *GrantsCache) ensureFresh(ctx context.Context) (map[int64][]HierarchyGrant, error) {
	c.mu.RLock()
	if c.data != nil && time.Since(c.refreshedAt) < c.ttl {
		data := c.data
		c.mu.RUnlock()
		return data, nil
	}
	c.mu.RUnlock()
	return c.refresh(ctx)
}

func (c *GrantsCache) refresh(ctx context.Context) (map[int64][]HierarchyGrant, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check under write lock to avoid redundant DB calls.
	if c.data != nil && time.Since(c.refreshedAt) < c.ttl {
		return c.data, nil
	}
	newData, err := c.backend.loadAllGrants(ctx)
	if err != nil {
		return nil, err
	}
	c.data = newData
	c.refreshedAt = time.Now()
	return c.data, nil
}

func (c *GrantsCache) invalidate() {
	c.mu.Lock()
	c.refreshedAt = time.Time{} // zero forces refresh on next read
	c.mu.Unlock()
}
