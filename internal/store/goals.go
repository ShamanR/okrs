package store

import (
	"context"
	"errors"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateGoal(ctx context.Context, input GoalInput) (int64, error) {
	var id int64
	err := s.DB.QueryRow(ctx, `
		INSERT INTO goals (team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM goals WHERE team_id=$1 AND period_id=$2))
		RETURNING id`,
		input.TeamID, input.PeriodID, input.Title, input.Description, input.Priority, input.Weight, input.WorkType, input.FocusType, input.OwnerText,
	).Scan(&id)
	return id, err
}

func (s *Store) ListGoalsByTeamPeriod(ctx context.Context, teamID, periodID int64) ([]domain.Goal, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT g.id, g.team_id, g.period_id, g.title, g.description, g.priority,
		       COALESCE(gs.weight, g.weight) AS weight,
		       g.work_type, g.focus_type, g.owner_text, g.created_at, g.updated_at,
		       COALESCE(gs.sort_order, g.sort_order) AS team_sort_order
		FROM goals g
		LEFT JOIN goal_shares gs ON gs.goal_id = g.id AND gs.team_id = $1
		WHERE g.period_id=$2 AND (g.team_id=$1 OR gs.team_id IS NOT NULL)
		ORDER BY team_sort_order, g.id`, teamID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make([]domain.Goal, 0)
	for rows.Next() {
		var goal domain.Goal
		var sortOrder int
		if err := rows.Scan(&goal.ID, &goal.TeamID, &goal.PeriodID, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.CreatedAt, &goal.UpdatedAt, &sortOrder); err != nil {
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

func (s *Store) MoveGoal(ctx context.Context, goalID int64, direction int) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var teamID int64
	var periodID int64
	var currentOrder int
	row := tx.QueryRow(ctx, `SELECT team_id, period_id, sort_order FROM goals WHERE id=$1 FOR UPDATE`, goalID)
	if err := row.Scan(&teamID, &periodID, &currentOrder); err != nil {
		return err
	}

	var neighborID int64
	var neighborOrder int
	if direction < 0 {
		row = tx.QueryRow(ctx, `
			SELECT id, sort_order FROM goals
			WHERE team_id=$1 AND period_id=$2 AND sort_order < $3
			ORDER BY sort_order DESC LIMIT 1
			FOR UPDATE`, teamID, periodID, currentOrder)
	} else {
		row = tx.QueryRow(ctx, `
			SELECT id, sort_order FROM goals
			WHERE team_id=$1 AND period_id=$2 AND sort_order > $3
			ORDER BY sort_order ASC LIMIT 1
			FOR UPDATE`, teamID, periodID, currentOrder)
	}
	if err := row.Scan(&neighborID, &neighborOrder); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE goals SET sort_order=$1 WHERE id=$2`, neighborOrder, goalID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE goals SET sort_order=$1 WHERE id=$2`, currentOrder, neighborID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) GetGoal(ctx context.Context, id int64) (domain.Goal, error) {
	var goal domain.Goal
	row := s.DB.QueryRow(ctx, `
		SELECT id, team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, created_at, updated_at
		FROM goals WHERE id=$1`, id)
	if err := row.Scan(&goal.ID, &goal.TeamID, &goal.PeriodID, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
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
	Goal       domain.Goal
	TeamName   string
	TeamType   domain.TeamType
	PeriodName string
}

type GoalUpdateInput struct {
	ID          int64
	Title       string
	Description string
	Priority    domain.Priority
	Weight      int
	WorkType    domain.WorkType
	FocusType   domain.FocusType
	OwnerText   string
}

type GoalFieldsUpdateInput struct {
	ID          int64
	Title       string
	Description string
	Priority    domain.Priority
	WorkType    domain.WorkType
	FocusType   domain.FocusType
	OwnerText   string
}

func (s *Store) ListGoalsByPeriod(ctx context.Context, periodID int64) ([]GoalWithTeam, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT g.id, g.team_id, g.period_id, g.title, g.description, g.priority, g.weight, g.work_type, g.focus_type, g.owner_text, g.created_at, g.updated_at,
		       t.name, t.team_type, p.name
		FROM goals g
		JOIN teams t ON t.id = g.team_id
		JOIN periods p ON p.id = g.period_id
		WHERE g.period_id=$1
		ORDER BY g.priority, g.weight DESC`, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]GoalWithTeam, 0)
	for rows.Next() {
		var goal domain.Goal
		var teamName string
		var teamType domain.TeamType
		var periodName string
		if err := rows.Scan(&goal.ID, &goal.TeamID, &goal.PeriodID, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.CreatedAt, &goal.UpdatedAt, &teamName, &teamType, &periodName); err != nil {
			return nil, err
		}
		results = append(results, GoalWithTeam{Goal: goal, TeamName: teamName, TeamType: teamType, PeriodName: periodName})
	}
	return results, rows.Err()
}

func (s *Store) UpdateGoal(ctx context.Context, input GoalUpdateInput) error {
	_, err := s.DB.Exec(ctx, `
		UPDATE goals
		SET title=$1, description=$2, priority=$3, weight=$4, work_type=$5, focus_type=$6, owner_text=$7, updated_at=NOW()
		WHERE id=$8`,
		input.Title, input.Description, input.Priority, input.Weight, input.WorkType, input.FocusType, input.OwnerText, input.ID,
	)
	return err
}

func (s *Store) UpdateGoalFields(ctx context.Context, input GoalFieldsUpdateInput) error {
	_, err := s.DB.Exec(ctx, `
		UPDATE goals
		SET title=$1, description=$2, priority=$3, work_type=$4, focus_type=$5, owner_text=$6, updated_at=NOW()
		WHERE id=$7`,
		input.Title, input.Description, input.Priority, input.WorkType, input.FocusType, input.OwnerText, input.ID,
	)
	return err
}

func (s *Store) UpdateGoalOwner(ctx context.Context, goalID, teamID int64, weight int) error {
	_, err := s.DB.Exec(ctx, `
		UPDATE goals
		SET team_id=$1, weight=$2, updated_at=NOW()
		WHERE id=$3`,
		teamID, weight, goalID,
	)
	return err
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
