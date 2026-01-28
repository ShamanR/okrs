package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type GoalShare struct {
	GoalID    int64
	TeamID    int64
	Weight    int
	SortOrder int
}

type GoalShareInput struct {
	TeamID int64
	Weight int
}

func (s *Store) ListGoalShares(ctx context.Context, goalID int64) ([]GoalShare, error) {
	rows, err := s.DB.Query(ctx, `SELECT goal_id, team_id, weight, sort_order FROM goal_shares WHERE goal_id=$1`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shares := make([]GoalShare, 0)
	for rows.Next() {
		var share GoalShare
		if err := rows.Scan(&share.GoalID, &share.TeamID, &share.Weight, &share.SortOrder); err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	return shares, rows.Err()
}

func (s *Store) GetGoalShare(ctx context.Context, goalID, teamID int64) (GoalShare, error) {
	var share GoalShare
	row := s.DB.QueryRow(ctx, `SELECT goal_id, team_id, weight, sort_order FROM goal_shares WHERE goal_id=$1 AND team_id=$2`, goalID, teamID)
	if err := row.Scan(&share.GoalID, &share.TeamID, &share.Weight, &share.SortOrder); err != nil {
		return GoalShare{}, err
	}
	return share, nil
}

func (s *Store) ReplaceGoalShares(ctx context.Context, goalID int64, shares []GoalShareInput) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if len(shares) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM goal_shares WHERE goal_id=$1`, goalID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	teamIDs := make([]int64, 0, len(shares))
	for _, share := range shares {
		teamIDs = append(teamIDs, share.TeamID)
	}

	deleteQuery := `DELETE FROM goal_shares WHERE goal_id=$1 AND team_id <> ALL($2)`
	if _, err := tx.Exec(ctx, deleteQuery, goalID, teamIDs); err != nil {
		return err
	}

	for _, share := range shares {
		if err := upsertGoalShare(ctx, tx, goalID, share); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) DeleteGoalShare(ctx context.Context, goalID, teamID int64) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM goal_shares WHERE goal_id=$1 AND team_id=$2`, goalID, teamID)
	return err
}

func (s *Store) UpdateGoalTeamWeight(ctx context.Context, goalID, teamID int64, weight int) error {
	res, err := s.DB.Exec(ctx, `UPDATE goals SET weight=$1, updated_at=NOW() WHERE id=$2 AND team_id=$3`, weight, goalID, teamID)
	if err != nil {
		return err
	}
	if res.RowsAffected() > 0 {
		return nil
	}

	_, err = s.DB.Exec(ctx, `UPDATE goal_shares SET weight=$1, updated_at=NOW() WHERE goal_id=$2 AND team_id=$3`, weight, goalID, teamID)
	return err
}

func upsertGoalShare(ctx context.Context, tx pgx.Tx, goalID int64, share GoalShareInput) error {
	cmd := `
		INSERT INTO goal_shares (goal_id, team_id, weight, sort_order)
		SELECT $1, $2, $3, sort_order FROM goals WHERE id=$1
		ON CONFLICT (goal_id, team_id)
		DO UPDATE SET weight=EXCLUDED.weight, updated_at=NOW()`
	if _, err := tx.Exec(ctx, cmd, goalID, share.TeamID, share.Weight); err != nil {
		return fmt.Errorf("upsert goal share: %w", err)
	}
	return nil
}
