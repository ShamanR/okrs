package teams

import (
	"context"
	"database/sql"

	"okrs/internal/core/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TeamRepository handles all team persistence.
type TeamRepository struct {
	db *pgxpool.Pool
}

func NewTeamRepository(db *pgxpool.Pool) *TeamRepository {
	return &TeamRepository{db: db}
}

func (r *TeamRepository) ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, team_type, parent_id, lead, lead_udid, description, deleted_at, created_at, updated_at
		FROM teams
		WHERE deleted_at IS NULL AND tenant_id = $1
		ORDER BY name`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTeams(rows)
}

func (r *TeamRepository) ListDeletedTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, team_type, parent_id, lead, lead_udid, description, deleted_at, created_at, updated_at
		FROM teams
		WHERE deleted_at IS NOT NULL AND tenant_id = $1
		ORDER BY deleted_at DESC, name`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTeams(rows)
}

func (r *TeamRepository) ListAllTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, team_type, parent_id, lead, lead_udid, description, deleted_at, created_at, updated_at
		FROM teams
		WHERE tenant_id = $1
		ORDER BY name`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTeams(rows)
}

func scanTeams(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}) ([]domain.Team, error) {
	var teams []domain.Team
	for rows.Next() {
		var team domain.Team
		var parentID sql.NullInt64
		var leadUDID *string
		var deletedAt sql.NullTime
		if err := rows.Scan(&team.ID, &team.Name, &team.Type, &parentID, &team.Lead, &leadUDID, &team.Description, &deletedAt, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			value := parentID.Int64
			team.ParentID = &value
		}
		team.LeadUDID = leadUDID
		if deletedAt.Valid {
			value := deletedAt.Time
			team.DeletedAt = &value
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (r *TeamRepository) GetTeam(ctx context.Context, scope domain.TenantScope, id int64) (domain.Team, error) {
	var team domain.Team
	var parentID sql.NullInt64
	var leadUDID *string
	var deletedAt sql.NullTime
	row := r.db.QueryRow(ctx,
		`SELECT id, name, team_type, parent_id, lead, lead_udid, description, deleted_at, created_at, updated_at
		 FROM teams WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID)
	if err := row.Scan(&team.ID, &team.Name, &team.Type, &parentID, &team.Lead, &leadUDID, &team.Description, &deletedAt, &team.CreatedAt, &team.UpdatedAt); err != nil {
		return domain.Team{}, err
	}
	if parentID.Valid {
		value := parentID.Int64
		team.ParentID = &value
	}
	team.LeadUDID = leadUDID
	if deletedAt.Valid {
		value := deletedAt.Time
		team.DeletedAt = &value
	}
	return team, nil
}

// TeamInput is used by CreateTeam and UpdateTeam.
type TeamInput struct {
	Name        string
	Type        domain.TeamType
	ParentID    *int64
	Lead        string
	LeadUDID    *string
	Description string
}

func (r *TeamRepository) CreateTeam(ctx context.Context, scope domain.TenantScope, input TeamInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO teams (name, team_type, parent_id, lead, lead_udid, description, tenant_id) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		input.Name, input.Type, input.ParentID, input.Lead, input.LeadUDID, input.Description, scope.TenantID).Scan(&id)
	return id, err
}

func (r *TeamRepository) UpdateTeam(ctx context.Context, scope domain.TenantScope, input TeamInput, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE teams SET name=$1, team_type=$2, parent_id=$3, lead=$4, lead_udid=$5, description=$6, updated_at=NOW() WHERE id=$7 AND tenant_id=$8`,
		input.Name, input.Type, input.ParentID, input.Lead, input.LeadUDID, input.Description, id, scope.TenantID)
	return err
}

func (r *TeamRepository) TeamHasGoals(ctx context.Context, scope domain.TenantScope, id int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM goals WHERE team_id = $1 AND tenant_id = $2
			UNION ALL
			SELECT 1
			FROM goal_shares gs
			WHERE gs.team_id = $1 AND gs.tenant_id = $2
			LIMIT 1
		)`, id, scope.TenantID).Scan(&exists)
	return exists, err
}

func (r *TeamRepository) TeamHasGoalsInPeriod(ctx context.Context, scope domain.TenantScope, id, periodID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM goals g
			WHERE g.team_id = $1 AND g.period_id = $2 AND g.tenant_id = $3
			UNION ALL
			SELECT 1
			FROM goal_shares gs
			JOIN goals g ON g.id = gs.goal_id
			WHERE gs.team_id = $1 AND g.period_id = $2 AND gs.tenant_id = $3
			LIMIT 1
		)`, id, periodID, scope.TenantID).Scan(&exists)
	return exists, err
}

func (r *TeamRepository) ListTeamIDsWithGoalsInPeriod(ctx context.Context, scope domain.TenantScope, periodID int64) (map[int64]struct{}, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT team_id
		FROM (
			SELECT g.team_id AS team_id
			FROM goals g
			WHERE g.period_id = $1 AND g.tenant_id = $2
			UNION ALL
			SELECT gs.team_id AS team_id
			FROM goal_shares gs
			JOIN goals g ON g.id = gs.goal_id
			WHERE g.period_id = $1 AND gs.tenant_id = $2
		) teams_with_goals`, periodID, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make(map[int64]struct{})
	for rows.Next() {
		var teamID int64
		if err := rows.Scan(&teamID); err != nil {
			return nil, err
		}
		ids[teamID] = struct{}{}
	}
	return ids, rows.Err()
}

func (r *TeamRepository) SoftDeleteTeam(ctx context.Context, scope domain.TenantScope, id int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var parentID sql.NullInt64
	if err := tx.QueryRow(ctx, `SELECT parent_id FROM teams WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, id, scope.TenantID).Scan(&parentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE teams SET parent_id=$1, updated_at=NOW() WHERE parent_id=$2 AND tenant_id=$3`, nullableParent(parentID), id, scope.TenantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE teams SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *TeamRepository) RestoreTeam(ctx context.Context, scope domain.TenantScope, id int64) error {
	_, err := r.db.Exec(ctx, `UPDATE teams SET deleted_at=NULL, updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID)
	return err
}

func (r *TeamRepository) HardDeleteTeam(ctx context.Context, scope domain.TenantScope, id int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var parentID sql.NullInt64
	if err := tx.QueryRow(ctx, `SELECT parent_id FROM teams WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, id, scope.TenantID).Scan(&parentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE teams SET parent_id=$1, updated_at=NOW() WHERE parent_id=$2 AND tenant_id=$3`, nullableParent(parentID), id, scope.TenantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM teams WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nullableParent(parentID sql.NullInt64) any {
	if !parentID.Valid {
		return nil
	}
	return parentID.Int64
}
