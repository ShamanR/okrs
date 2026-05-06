package store

import (
	"context"
	"time"
)

type HierarchyGrant struct {
	ID              int64
	UserID          int64
	TeamID          int64
	CreatedAt       time.Time
	CreatedByUserID int64
}

func (s *Store) ListUserGrants(ctx context.Context, userID int64) ([]HierarchyGrant, error) {
	rows, err := s.DB.Query(ctx, `
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

func (s *Store) AddUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO user_hierarchy_grants (user_id, team_id, created_by_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, team_id) DO NOTHING`,
		userID, teamID, grantedByUserID)
	return err
}

func (s *Store) RemoveUserGrant(ctx context.Context, userID, teamID int64) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM user_hierarchy_grants WHERE user_id = $1 AND team_id = $2`, userID, teamID)
	return err
}

// listAllGrants loads the full user_hierarchy_grants table as a map[userID][]HierarchyGrant.
func (s *Store) listAllGrants(ctx context.Context) (map[int64][]HierarchyGrant, error) {
	rows, err := s.DB.Query(ctx, `
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
func (s *Store) ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error) {
	if len(rootIDs) == 0 {
		return nil, nil
	}
	rows, err := s.DB.Query(ctx, `
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
