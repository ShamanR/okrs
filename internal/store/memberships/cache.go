package memberships

import (
	"context"
	"sync"
	"time"

	"okrs/internal/domain"
)

// membershipBackend is the read contract MembershipCache needs. *MembershipRepository satisfies it.
type membershipBackend interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}

const defaultMembershipCacheTTL = 5 * time.Minute

type cachedMemberships struct {
	data     []domain.Membership
	cachedAt time.Time
}

// MembershipCache is a per-user, TTL-based read-through cache over a MembershipRepository.
// It serves the per-request tenant-resolve hot path. Writes must call InvalidateUser.
type MembershipCache struct {
	backend membershipBackend
	ttl     time.Duration
	mu      sync.RWMutex
	byUser  map[int64]cachedMemberships
}

// NewMembershipCache wraps a MembershipRepository with a 5-minute per-user cache.
func NewMembershipCache(r *MembershipRepository) *MembershipCache {
	return newMembershipCacheWithBackend(r, defaultMembershipCacheTTL)
}

func newMembershipCacheWithBackend(b membershipBackend, ttl time.Duration) *MembershipCache {
	return &MembershipCache{backend: b, ttl: ttl, byUser: make(map[int64]cachedMemberships)}
}

// ListByUser returns the user's active memberships, served from cache within the TTL.
func (c *MembershipCache) ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error) {
	c.mu.RLock()
	e, ok := c.byUser[userID]
	c.mu.RUnlock()
	if ok && time.Since(e.cachedAt) < c.ttl {
		return e.data, nil
	}
	data, err := c.backend.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.byUser[userID] = cachedMemberships{data: data, cachedAt: time.Now()}
	c.mu.Unlock()
	return data, nil
}

// InvalidateUser drops the cached memberships for one user (call after membership writes).
func (c *MembershipCache) InvalidateUser(userID int64) {
	c.mu.Lock()
	delete(c.byUser, userID)
	c.mu.Unlock()
}

// InvalidateAll clears the whole cache.
func (c *MembershipCache) InvalidateAll() {
	c.mu.Lock()
	c.byUser = make(map[int64]cachedMemberships)
	c.mu.Unlock()
}
