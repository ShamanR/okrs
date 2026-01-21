package store

import (
	"context"
	"errors"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) GetTeamQuarterStatus(ctx context.Context, teamID int64, year, quarter int) (domain.TeamQuarterStatus, error) {
	var status domain.TeamQuarterStatus
	row := s.DB.QueryRow(ctx, `SELECT status FROM team_quarter_statuses WHERE team_id=$1 AND year=$2 AND quarter=$3`, teamID, year, quarter)
	if err := row.Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TeamQuarterStatusNoGoals, nil
		}
		return "", err
	}
	return status, nil
}

func (s *Store) SetTeamQuarterStatus(ctx context.Context, teamID int64, year, quarter int, status domain.TeamQuarterStatus) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO team_quarter_statuses (team_id, year, quarter, status)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (team_id, year, quarter)
		DO UPDATE SET status=EXCLUDED.status, updated_at=NOW()`,
		teamID, year, quarter, status,
	)
	return err
}
