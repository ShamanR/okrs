package store

import (
	"context"
	"errors"
	"time"

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

func (s *Store) GetTeamPeriodStatusWithTime(ctx context.Context, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error) {
	var status domain.TeamPeriodStatus
	var updatedAt *time.Time
	row := s.DB.QueryRow(ctx, `SELECT status, updated_at FROM team_period_statuses WHERE team_id=$1 AND period_id=$2`, teamID, periodID)
	if err := row.Scan(&status, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TeamPeriodStatusNoGoals, nil, nil
		}
		return "", nil, err
	}
	return status, updatedAt, nil
}

func (s *Store) SetTeamPeriodStatus(ctx context.Context, teamID, periodID int64, status domain.TeamPeriodStatus) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO team_period_statuses (team_id, period_id, status, updated_at)
		VALUES ($1,$2,$3,NOW())
		ON CONFLICT (team_id, period_id)
		DO UPDATE SET status=EXCLUDED.status, updated_at=NOW()`,
		teamID, periodID, status,
	)
	return err
}

func (s *Store) ListTeamPeriodStatuses(ctx context.Context, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error) {
	statuses := make(map[int64]domain.TeamPeriodStatus, len(teamIDs))
	if len(teamIDs) == 0 {
		return statuses, nil
	}
	rows, err := s.DB.Query(ctx, `
		SELECT team_id, status
		FROM team_period_statuses
		WHERE period_id=$1 AND team_id = ANY($2)`, periodID, teamIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var teamID int64
		var status domain.TeamPeriodStatus
		if err := rows.Scan(&teamID, &status); err != nil {
			return nil, err
		}
		statuses[teamID] = status
	}
	return statuses, rows.Err()
}
