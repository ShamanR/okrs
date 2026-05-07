package grants

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GrantRepository handles user_hierarchy_grants persistence.
type GrantRepository struct {
	db *pgxpool.Pool
}

func NewGrantRepository(db *pgxpool.Pool) *GrantRepository {
	return &GrantRepository{db: db}
}

type HierarchyGrant struct {
	ID              int64
	UserID          int64
	TeamID          int64
	CreatedAt       time.Time
	CreatedByUserID int64
}

func (r *GrantRepository) ListUserGrants(ctx context.Context, userID int64) ([]HierarchyGrant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, team_id, created_at, created_by_user_id
		FROM user_hierarchy_grants WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var grants []HierarchyGrant
	for rows.Next() {
		var g HierarchyGrant
		if err := rows.Scan(&g.ID, &g.UserID, &g.TeamID, &g.CreatedAt, &g.CreatedByUserID); err != nil {
			return nil, err
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

func (r *GrantRepository) AddUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_hierarchy_grants (user_id, team_id, created_by_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, team_id) DO NOTHING`,
		userID, teamID, grantedByUserID)
	return err
}

func (r *GrantRepository) RemoveUserGrant(ctx context.Context, userID, teamID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM user_hierarchy_grants WHERE user_id = $1 AND team_id = $2`, userID, teamID)
	return err
}

// listAllGrants loads the full user_hierarchy_grants table as a map[userID][]HierarchyGrant.
func (r *GrantRepository) listAllGrants(ctx context.Context) (map[int64][]HierarchyGrant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, team_id, created_at, created_by_user_id
		FROM user_hierarchy_grants`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64][]HierarchyGrant)
	for rows.Next() {
		var g HierarchyGrant
		if err := rows.Scan(&g.ID, &g.UserID, &g.TeamID, &g.CreatedAt, &g.CreatedByUserID); err != nil {
			return nil, err
		}
		result[g.UserID] = append(result[g.UserID], g)
	}
	return result, rows.Err()
}

// ListDescendantTeamIDs returns the given root team IDs plus all their recursive children IDs.
func (r *GrantRepository) ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error) {
	if len(rootIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		WITH RECURSIVE tree AS (
			SELECT id FROM teams WHERE id = ANY($1) AND deleted_at IS NULL
			UNION ALL
			SELECT t.id FROM teams t JOIN tree p ON t.parent_id = p.id WHERE t.deleted_at IS NULL
		)
		SELECT id FROM tree`, rootIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// grantsBackend is the persistence contract used by GrantsCache.
// *GrantRepository satisfies it; tests can inject a fake.
type grantsBackend interface {
	loadAllGrants(ctx context.Context) (map[int64][]HierarchyGrant, error)
	addUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error
	removeUserGrant(ctx context.Context, userID, teamID int64) error
	ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error)
}

// storeGrantsBackend adapts *GrantRepository to grantsBackend.
type storeGrantsBackend struct{ r *GrantRepository }

func (b *storeGrantsBackend) loadAllGrants(ctx context.Context) (map[int64][]HierarchyGrant, error) {
	return b.r.listAllGrants(ctx)
}
func (b *storeGrantsBackend) addUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error {
	return b.r.AddUserGrant(ctx, userID, teamID, grantedByUserID)
}
func (b *storeGrantsBackend) removeUserGrant(ctx context.Context, userID, teamID int64) error {
	return b.r.RemoveUserGrant(ctx, userID, teamID)
}
func (b *storeGrantsBackend) ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error) {
	return b.r.ListDescendantTeamIDs(ctx, rootIDs)
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

// NewGrantsCache wraps a GrantRepository with a 5-minute in-memory cache for user_hierarchy_grants.
func NewGrantsCache(r *GrantRepository) *GrantsCache {
	return &GrantsCache{
		backend: &storeGrantsBackend{r},
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
