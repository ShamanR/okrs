package store

import (
	"context"
	"errors"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateKeyResult(ctx context.Context, input KeyResultInput) (int64, error) {
	var id int64
	err := s.DB.QueryRow(ctx, `
		INSERT INTO key_results (goal_id, title, description, weight, kind, sort_order)
		VALUES ($1,$2,$3,$4,$5, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM key_results WHERE goal_id=$1))
		RETURNING id`,
		input.GoalID, input.Title, input.Description, input.Weight, input.Kind,
	).Scan(&id)
	return id, err
}

type KeyResultUpdateInput struct {
	ID          int64
	Title       string
	Description string
	Weight      int
	Kind        domain.KRKind
}

func (s *Store) ListKeyResultsByGoal(ctx context.Context, goalID int64) ([]domain.KeyResult, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at
		FROM key_results WHERE goal_id=$1 ORDER BY sort_order, id`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var krs []domain.KeyResult
	for rows.Next() {
		var kr domain.KeyResult
		if err := rows.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt); err != nil {
			return nil, err
		}
		krs = append(krs, kr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range krs {
		kr := &krs[i]
		switch kr.Kind {
		case domain.KRKindProject:
			stages, err := s.ListProjectStages(ctx, kr.ID)
			if err != nil {
				return nil, err
			}
			kr.Project = &domain.KRProject{Stages: stages}
		case domain.KRKindPercent:
			meta, checkpoints, err := s.GetPercentMeta(ctx, kr.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if meta != nil {
				meta.Checkpoints = checkpoints
				kr.Percent = meta
			}
		case domain.KRKindBoolean:
			meta, err := s.GetBooleanMeta(ctx, kr.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if meta != nil {
				kr.Boolean = meta
			}
		}

		comments, _ := s.ListKeyResultComments(ctx, kr.ID)
		kr.Comments = comments
	}

	return krs, nil
}

func (s *Store) AddKeyResultComment(ctx context.Context, krID int64, text string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO key_result_comments (key_result_id, text) VALUES ($1,$2)`, krID, text)
	return err
}

func (s *Store) ListKeyResultComments(ctx context.Context, krID int64) ([]domain.KeyResultComment, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, key_result_id, text, created_at FROM key_result_comments WHERE key_result_id=$1 ORDER BY created_at DESC`, krID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var comments []domain.KeyResultComment
	for rows.Next() {
		var c domain.KeyResultComment
		if err := rows.Scan(&c.ID, &c.KeyResultID, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (s *Store) AddProjectStage(ctx context.Context, input ProjectStageInput) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
		VALUES ($1,$2,$3,$4,$5)`,
		input.KeyResultID, input.Title, input.Weight, input.IsDone, input.SortOrder,
	)
	return err
}

func (s *Store) UpdateProjectStageDone(ctx context.Context, stageID int64, done bool) error {
	_, err := s.DB.Exec(ctx, `UPDATE kr_project_stages SET is_done=$1 WHERE id=$2`, done, stageID)
	return err
}

func (s *Store) ListProjectStages(ctx context.Context, krID int64) ([]domain.KRProjectStage, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, key_result_id, title, weight, is_done, sort_order
		FROM kr_project_stages WHERE key_result_id=$1 ORDER BY sort_order`, krID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stages []domain.KRProjectStage
	for rows.Next() {
		var stage domain.KRProjectStage
		if err := rows.Scan(&stage.ID, &stage.KeyResultID, &stage.Title, &stage.Weight, &stage.IsDone, &stage.SortOrder); err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	return stages, rows.Err()
}

func (s *Store) ReplaceProjectStages(ctx context.Context, krID int64, stages []ProjectStageInput) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM kr_project_stages WHERE key_result_id=$1`, krID)
	if err != nil {
		return err
	}
	for _, stage := range stages {
		if _, err := s.DB.Exec(ctx, `
			INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
			VALUES ($1,$2,$3,$4,$5)`,
			krID, stage.Title, stage.Weight, stage.IsDone, stage.SortOrder,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateKeyResult(ctx context.Context, input KeyResultUpdateInput) error {
	_, err := s.DB.Exec(ctx, `
		UPDATE key_results
		SET title=$1, description=$2, weight=$3, kind=$4, updated_at=NOW()
		WHERE id=$5`,
		input.Title, input.Description, input.Weight, input.Kind, input.ID,
	)
	return err
}

func (s *Store) UpsertPercentMeta(ctx context.Context, input PercentMetaInput) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO kr_percent_meta (key_result_id, start_value, target_value, current_value)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (key_result_id) DO UPDATE SET
			start_value=EXCLUDED.start_value,
			target_value=EXCLUDED.target_value,
			current_value=EXCLUDED.current_value`,
		input.KeyResultID, input.StartValue, input.TargetValue, input.CurrentValue,
	)
	return err
}

func (s *Store) UpdatePercentCurrent(ctx context.Context, krID int64, current float64) error {
	_, err := s.DB.Exec(ctx, `UPDATE kr_percent_meta SET current_value=$1 WHERE key_result_id=$2`, current, krID)
	return err
}

func (s *Store) AddPercentCheckpoint(ctx context.Context, input PercentCheckpointInput) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO kr_percent_checkpoints (key_result_id, metric_value, kr_percent)
		VALUES ($1,$2,$3)`,
		input.KeyResultID, input.MetricValue, input.KRPercent,
	)
	return err
}

func (s *Store) GetPercentMeta(ctx context.Context, krID int64) (*domain.KRPercent, []domain.KRPercentCheckpoint, error) {
	var meta domain.KRPercent
	row := s.DB.QueryRow(ctx, `SELECT start_value, target_value, current_value FROM kr_percent_meta WHERE key_result_id=$1`, krID)
	if err := row.Scan(&meta.StartValue, &meta.TargetValue, &meta.CurrentValue); err != nil {
		return nil, nil, err
	}
	checkpoints, err := s.ListPercentCheckpoints(ctx, krID)
	if err != nil {
		return nil, nil, err
	}
	return &meta, checkpoints, nil
}

func (s *Store) ListPercentCheckpoints(ctx context.Context, krID int64) ([]domain.KRPercentCheckpoint, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, key_result_id, metric_value, kr_percent
		FROM kr_percent_checkpoints WHERE key_result_id=$1 ORDER BY metric_value`, krID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var checkpoints []domain.KRPercentCheckpoint
	for rows.Next() {
		var cp domain.KRPercentCheckpoint
		if err := rows.Scan(&cp.ID, &cp.KeyResultID, &cp.MetricValue, &cp.KRPercent); err != nil {
			return nil, err
		}
		checkpoints = append(checkpoints, cp)
	}
	return checkpoints, rows.Err()
}

func (s *Store) UpsertBooleanMeta(ctx context.Context, krID int64, done bool) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO kr_boolean_meta (key_result_id, is_done)
		VALUES ($1,$2)
		ON CONFLICT (key_result_id) DO UPDATE SET is_done=EXCLUDED.is_done`,
		krID, done,
	)
	return err
}

func (s *Store) GetBooleanMeta(ctx context.Context, krID int64) (*domain.KRBoolean, error) {
	var meta domain.KRBoolean
	row := s.DB.QueryRow(ctx, `SELECT is_done FROM kr_boolean_meta WHERE key_result_id=$1`, krID)
	if err := row.Scan(&meta.IsDone); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *Store) DeleteKeyResult(ctx context.Context, id int64) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM key_results WHERE id=$1`, id)
	return err
}

func (s *Store) MoveKeyResult(ctx context.Context, krID int64, direction int) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var goalID int64
	var currentOrder int
	row := tx.QueryRow(ctx, `SELECT goal_id, sort_order FROM key_results WHERE id=$1 FOR UPDATE`, krID)
	if err := row.Scan(&goalID, &currentOrder); err != nil {
		return err
	}

	var neighborID int64
	var neighborOrder int
	if direction < 0 {
		row = tx.QueryRow(ctx, `
			SELECT id, sort_order FROM key_results
			WHERE goal_id=$1 AND sort_order < $2
			ORDER BY sort_order DESC LIMIT 1
			FOR UPDATE`, goalID, currentOrder)
	} else {
		row = tx.QueryRow(ctx, `
			SELECT id, sort_order FROM key_results
			WHERE goal_id=$1 AND sort_order > $2
			ORDER BY sort_order ASC LIMIT 1
			FOR UPDATE`, goalID, currentOrder)
	}
	if err := row.Scan(&neighborID, &neighborOrder); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE key_results SET sort_order=$1 WHERE id=$2`, neighborOrder, krID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE key_results SET sort_order=$1 WHERE id=$2`, currentOrder, neighborID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
