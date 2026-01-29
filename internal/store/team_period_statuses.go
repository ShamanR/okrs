package store

import (
	"context"
	"errors"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) GetTeamPeriodStatus(ctx context.Context, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	var status domain.TeamPeriodStatus
	row := s.DB.QueryRow(ctx, `SELECT status FROM team_period_statuses WHERE team_id=$1 AND period_id=$2`, teamID, periodID)
	if err := row.Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TeamPeriodStatusNoGoals, nil
		}
		return "", err
	}
	return status, nil
}

func (s *Store) SetTeamPeriodStatus(ctx context.Context, teamID, periodID int64, status domain.TeamPeriodStatus) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO team_period_statuses (team_id, period_id, status)
		VALUES ($1,$2,$3)
		ON CONFLICT (team_id, period_id)
		DO UPDATE SET status=EXCLUDED.status`,
		teamID, periodID, status,
	)
	return err
}
