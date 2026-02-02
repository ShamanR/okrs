package store

import (
	"context"
	"database/sql"

	"okrs/internal/domain"
)

func (s *Store) ListTeams(ctx context.Context) ([]domain.Team, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, name, team_type, parent_id, lead, description, created_at, updated_at FROM teams ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []domain.Team
	for rows.Next() {
		var team domain.Team
		var parentID sql.NullInt64
		if err := rows.Scan(&team.ID, &team.Name, &team.Type, &parentID, &team.Lead, &team.Description, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			value := parentID.Int64
			team.ParentID = &value
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (s *Store) GetTeam(ctx context.Context, id int64) (domain.Team, error) {
	var team domain.Team
	var parentID sql.NullInt64
	row := s.DB.QueryRow(ctx, `SELECT id, name, team_type, parent_id, lead, description, created_at, updated_at FROM teams WHERE id=$1`, id)
	if err := row.Scan(&team.ID, &team.Name, &team.Type, &parentID, &team.Lead, &team.Description, &team.CreatedAt, &team.UpdatedAt); err != nil {
		return domain.Team{}, err
	}
	if parentID.Valid {
		value := parentID.Int64
		team.ParentID = &value
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

func (s *Store) DeleteTeam(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM teams WHERE id=$1`, id)
	return err
}
