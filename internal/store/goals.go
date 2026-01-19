package store

import (
	"context"

	"okrs/internal/domain"
)

func (s *Store) CreateGoal(ctx context.Context, input GoalInput) (int64, error) {
	var id int64
	err := s.DB.QueryRow(ctx, `
		INSERT INTO goals (team_id, year, quarter, title, description, priority, weight, work_type, focus_type, owner_text)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id`,
		input.TeamID, input.Year, input.Quarter, input.Title, input.Description, input.Priority, input.Weight, input.WorkType, input.FocusType, input.OwnerText,
	).Scan(&id)
	return id, err
}

func (s *Store) ListGoalsByTeamQuarter(ctx context.Context, teamID int64, year, quarter int) ([]domain.Goal, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, team_id, year, quarter, title, description, priority, weight, work_type, focus_type, owner_text, created_at, updated_at
		FROM goals WHERE team_id=$1 AND year=$2 AND quarter=$3
		ORDER BY priority, weight DESC`, teamID, year, quarter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make([]domain.Goal, 0)
	for rows.Next() {
		var goal domain.Goal
		if err := rows.Scan(&goal.ID, &goal.TeamID, &goal.Year, &goal.Quarter, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
			return nil, err
		}
		goals = append(goals, goal)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range goals {
		krs, err := s.ListKeyResultsByGoal(ctx, goals[i].ID)
		if err != nil {
			return nil, err
		}
		goals[i].KeyResults = krs
	}
	return goals, nil
}

func (s *Store) GetGoal(ctx context.Context, id int64) (domain.Goal, error) {
	var goal domain.Goal
	row := s.DB.QueryRow(ctx, `
		SELECT id, team_id, year, quarter, title, description, priority, weight, work_type, focus_type, owner_text, created_at, updated_at
		FROM goals WHERE id=$1`, id)
	if err := row.Scan(&goal.ID, &goal.TeamID, &goal.Year, &goal.Quarter, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
		return domain.Goal{}, err
	}
	krs, err := s.ListKeyResultsByGoal(ctx, goal.ID)
	if err != nil {
		return domain.Goal{}, err
	}
	goal.KeyResults = krs
	goal.Comments, _ = s.ListGoalComments(ctx, goal.ID)
	return goal, nil
}

func (s *Store) DeleteGoal(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM goals WHERE id=$1`, id)
	return err
}

type GoalWithTeam struct {
	Goal     domain.Goal
	TeamName string
}

func (s *Store) ListGoalsByYear(ctx context.Context, year int) ([]GoalWithTeam, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT g.id, g.team_id, g.year, g.quarter, g.title, g.description, g.priority, g.weight, g.work_type, g.focus_type, g.owner_text, g.created_at, g.updated_at,
		       t.name
		FROM goals g
		JOIN teams t ON t.id = g.team_id
		WHERE g.year=$1
		ORDER BY g.quarter, g.priority, g.weight DESC`, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]GoalWithTeam, 0)
	for rows.Next() {
		var goal domain.Goal
		var teamName string
		if err := rows.Scan(&goal.ID, &goal.TeamID, &goal.Year, &goal.Quarter, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.CreatedAt, &goal.UpdatedAt, &teamName); err != nil {
			return nil, err
		}
		results = append(results, GoalWithTeam{Goal: goal, TeamName: teamName})
	}
	return results, rows.Err()
}

func (s *Store) AddGoalComment(ctx context.Context, goalID int64, text string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO goal_comments (goal_id, text) VALUES ($1,$2)`, goalID, text)
	return err
}

func (s *Store) ListGoalComments(ctx context.Context, goalID int64) ([]domain.GoalComment, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, goal_id, text, created_at FROM goal_comments WHERE goal_id=$1 ORDER BY created_at DESC`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var comments []domain.GoalComment
	for rows.Next() {
		var c domain.GoalComment
		if err := rows.Scan(&c.ID, &c.GoalID, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}
