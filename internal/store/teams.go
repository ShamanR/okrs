package store

import (
	"context"

	"okrs/internal/domain"
)

func (s *Store) ListTeams(ctx context.Context) ([]domain.Team, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, name, created_at, updated_at FROM teams ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []domain.Team
	for rows.Next() {
		var team domain.Team
		if err := rows.Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (s *Store) GetTeam(ctx context.Context, id int64) (domain.Team, error) {
	var team domain.Team
	row := s.DB.QueryRow(ctx, `SELECT id, name, created_at, updated_at FROM teams WHERE id=$1`, id)
	if err := row.Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt); err != nil {
		return domain.Team{}, err
	}
	return team, nil
}
