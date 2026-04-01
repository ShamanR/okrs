package store

import (
	"context"
	"database/sql"

	"okrs/internal/domain"
)

func (s *Store) ListTeams(ctx context.Context) ([]domain.Team, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, name, team_type, parent_id, lead, description, deleted_at, created_at, updated_at
		FROM teams
		WHERE deleted_at IS NULL
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTeams(rows)
}

func (s *Store) ListDeletedTeams(ctx context.Context) ([]domain.Team, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, name, team_type, parent_id, lead, description, deleted_at, created_at, updated_at
		FROM teams
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTeams(rows)
}

func (s *Store) ListAllTeams(ctx context.Context) ([]domain.Team, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, name, team_type, parent_id, lead, description, deleted_at, created_at, updated_at
		FROM teams
		ORDER BY name`)
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
		var deletedAt sql.NullTime
		if err := rows.Scan(&team.ID, &team.Name, &team.Type, &parentID, &team.Lead, &team.Description, &deletedAt, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			value := parentID.Int64
			team.ParentID = &value
		}
		if deletedAt.Valid {
			value := deletedAt.Time
			team.DeletedAt = &value
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (s *Store) GetTeam(ctx context.Context, id int64) (domain.Team, error) {
	var team domain.Team
	var parentID sql.NullInt64
	var deletedAt sql.NullTime
	row := s.DB.QueryRow(ctx, `
		SELECT id, name, team_type, parent_id, lead, description, deleted_at, created_at, updated_at
		FROM teams
		WHERE id=$1`, id)
	if err := row.Scan(&team.ID, &team.Name, &team.Type, &parentID, &team.Lead, &team.Description, &deletedAt, &team.CreatedAt, &team.UpdatedAt); err != nil {
		return domain.Team{}, err
	}
	if parentID.Valid {
		value := parentID.Int64
		team.ParentID = &value
	}
	if deletedAt.Valid {
		value := deletedAt.Time
		team.DeletedAt = &value
	}
	return team, nil
}

type TeamInput struct {
	Name        string
	Type        domain.TeamType
	ParentID    *int64
	Lead        string
	Description string
}

func (s *Store) CreateTeam(ctx context.Context, input TeamInput) (int64, error) {
	var id int64
	err := s.DB.QueryRow(ctx, `INSERT INTO teams (name, team_type, parent_id, lead, description) VALUES ($1,$2,$3,$4,$5) RETURNING id`, input.Name, input.Type, input.ParentID, input.Lead, input.Description).Scan(&id)
	return id, err
}

func (s *Store) UpdateTeam(ctx context.Context, input TeamInput, id int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE teams SET name=$1, team_type=$2, parent_id=$3, lead=$4, description=$5, updated_at=NOW() WHERE id=$6`, input.Name, input.Type, input.ParentID, input.Lead, input.Description, id)
	return err
}

func (s *Store) TeamHasGoals(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := s.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM goals WHERE team_id = $1
			UNION ALL
			SELECT 1
			FROM goal_shares gs
			WHERE gs.team_id = $1
			LIMIT 1
		)`, id).Scan(&exists)
	return exists, err
}

func (s *Store) TeamHasGoalsInPeriod(ctx context.Context, id, periodID int64) (bool, error) {
	var exists bool
	err := s.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM goals g
			WHERE g.team_id = $1 AND g.period_id = $2
			UNION ALL
			SELECT 1
			FROM goal_shares gs
			JOIN goals g ON g.id = gs.goal_id
			WHERE gs.team_id = $1 AND g.period_id = $2
			LIMIT 1
		)`, id, periodID).Scan(&exists)
	return exists, err
}

func (s *Store) ListTeamIDsWithGoalsInPeriod(ctx context.Context, periodID int64) (map[int64]struct{}, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT DISTINCT team_id
		FROM (
			SELECT g.team_id AS team_id
			FROM goals g
			WHERE g.period_id = $1
			UNION ALL
			SELECT gs.team_id AS team_id
			FROM goal_shares gs
			JOIN goals g ON g.id = gs.goal_id
			WHERE g.period_id = $1
		) teams_with_goals`, periodID)
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

func (s *Store) SoftDeleteTeam(ctx context.Context, id int64) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var parentID sql.NullInt64
	if err := tx.QueryRow(ctx, `SELECT parent_id FROM teams WHERE id=$1 FOR UPDATE`, id).Scan(&parentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE teams SET parent_id=$1, updated_at=NOW() WHERE parent_id=$2`, nullableParent(parentID), id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE teams SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RestoreTeam(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE teams SET deleted_at=NULL, updated_at=NOW() WHERE id=$1`, id)
	return err
}

func (s *Store) HardDeleteTeam(ctx context.Context, id int64) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var parentID sql.NullInt64
	if err := tx.QueryRow(ctx, `SELECT parent_id FROM teams WHERE id=$1 FOR UPDATE`, id).Scan(&parentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE teams SET parent_id=$1, updated_at=NOW() WHERE parent_id=$2`, nullableParent(parentID), id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM teams WHERE id=$1`, id); err != nil {
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
