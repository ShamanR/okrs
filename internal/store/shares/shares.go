package shares

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GoalShareRepository handles goal_shares persistence.
type GoalShareRepository struct {
	db *pgxpool.Pool
}

func NewGoalShareRepository(db *pgxpool.Pool) *GoalShareRepository {
	return &GoalShareRepository{db: db}
}

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

func (r *GoalShareRepository) ListGoalShares(ctx context.Context, goalID int64) ([]GoalShare, error) {
	rows, err := r.db.Query(ctx, `SELECT goal_id, team_id, weight, sort_order FROM goal_shares WHERE goal_id=$1 ORDER BY sort_order, team_id`, goalID)
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

// ListGoalSharesByGoalIDs loads shares for all given goals in a single query.
// Returns a map[goalID][]GoalShare; goals without shares are absent from the map.
func (r *GoalShareRepository) ListGoalSharesByGoalIDs(ctx context.Context, goalIDs []int64) (map[int64][]GoalShare, error) {
	result := make(map[int64][]GoalShare, len(goalIDs))
	if len(goalIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT goal_id, team_id, weight, sort_order
		FROM goal_shares
		WHERE goal_id = ANY($1)
		ORDER BY goal_id, sort_order`, goalIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var share GoalShare
		if err := rows.Scan(&share.GoalID, &share.TeamID, &share.Weight, &share.SortOrder); err != nil {
			return nil, err
		}
		result[share.GoalID] = append(result[share.GoalID], share)
	}
	return result, rows.Err()
}

func (r *GoalShareRepository) GetGoalShare(ctx context.Context, goalID, teamID int64) (GoalShare, error) {
	var share GoalShare
	row := r.db.QueryRow(ctx, `SELECT goal_id, team_id, weight, sort_order FROM goal_shares WHERE goal_id=$1 AND team_id=$2`, goalID, teamID)
	if err := row.Scan(&share.GoalID, &share.TeamID, &share.Weight, &share.SortOrder); err != nil {
		return GoalShare{}, err
	}
	return share, nil
}

func (r *GoalShareRepository) ReplaceGoalShares(ctx context.Context, goalID int64, shares []GoalShareInput) error {
	tx, err := r.db.Begin(ctx)
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

func (r *GoalShareRepository) DeleteGoalShare(ctx context.Context, goalID, teamID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM goal_shares WHERE goal_id=$1 AND team_id=$2`, goalID, teamID)
	return err
}

func (r *GoalShareRepository) UpdateGoalTeamWeight(ctx context.Context, goalID, teamID int64, weight int) error {
	res, err := r.db.Exec(ctx, `UPDATE goals SET weight=$1, updated_at=NOW() WHERE id=$2 AND team_id=$3`, weight, goalID, teamID)
	if err != nil {
		return err
	}
	if res.RowsAffected() > 0 {
		return nil
	}

	_, err = r.db.Exec(ctx, `UPDATE goal_shares SET weight=$1, updated_at=NOW() WHERE goal_id=$2 AND team_id=$3`, weight, goalID, teamID)
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
