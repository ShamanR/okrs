package statuses

import (
	"context"
	"errors"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TeamStatusRepository handles team_period_statuses persistence.
type TeamStatusRepository struct {
	db *pgxpool.Pool
}

func NewTeamStatusRepository(db *pgxpool.Pool) *TeamStatusRepository {
	return &TeamStatusRepository{db: db}
}

func (r *TeamStatusRepository) GetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	var status domain.TeamPeriodStatus
	row := r.db.QueryRow(ctx, `SELECT status FROM team_period_statuses WHERE team_id=$1 AND period_id=$2 AND tenant_id=$3`, teamID, periodID, scope.TenantID)
	if err := row.Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TeamPeriodStatusNoGoals, nil
		}
		return "", err
	}
	return status, nil
}

func (r *TeamStatusRepository) GetTeamPeriodStatusWithTime(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error) {
	var status domain.TeamPeriodStatus
	var updatedAt *time.Time
	row := r.db.QueryRow(ctx, `SELECT status, updated_at FROM team_period_statuses WHERE team_id=$1 AND period_id=$2 AND tenant_id=$3`, teamID, periodID, scope.TenantID)
	if err := row.Scan(&status, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TeamPeriodStatusNoGoals, nil, nil
		}
		return "", nil, err
	}
	return status, updatedAt, nil
}

func (r *TeamStatusRepository) SetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO team_period_statuses (team_id, period_id, status, tenant_id, updated_at)
		VALUES ($1,$2,$3,$4,NOW())
		ON CONFLICT (team_id, period_id)
		DO UPDATE SET status=EXCLUDED.status, updated_at=NOW()`,
		teamID, periodID, status, scope.TenantID,
	)
	return err
}

func (r *TeamStatusRepository) ListTeamPeriodStatuses(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error) {
	statuses := make(map[int64]domain.TeamPeriodStatus, len(teamIDs))
	if len(teamIDs) == 0 {
		return statuses, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT team_id, status
		FROM team_period_statuses
		WHERE period_id=$1 AND team_id = ANY($2) AND tenant_id=$3`, periodID, teamIDs, scope.TenantID)
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
