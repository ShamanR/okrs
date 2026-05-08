package krs

import (
	"context"
	"errors"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KRRepository handles key result persistence including meta and progress.
type KRRepository struct {
	db *pgxpool.Pool
}

func NewKRRepository(db *pgxpool.Pool) *KRRepository {
	return &KRRepository{db: db}
}

// KeyResultInput is used by CreateKeyResult.
type KeyResultInput struct {
	GoalID      int64
	Title       string
	Description string
	Weight      int
	Kind        domain.KRKind
}

// KeyResultUpdateInput is used by UpdateKeyResult.
type KeyResultUpdateInput struct {
	ID          int64
	Title       string
	Description string
	Weight      int
	Kind        domain.KRKind
}

// ProjectStageInput is used by ReplaceProjectStages.
type ProjectStageInput struct {
	KeyResultID int64
	Title       string
	Weight      int
	SortOrder   int
	IsDone      bool
}

// PercentMetaInput is used by UpsertPercentMeta.
type PercentMetaInput struct {
	KeyResultID  int64
	StartValue   float64
	TargetValue  float64
	CurrentValue float64
}

// PercentCheckpointInput is used by AddPercentCheckpoint.
type PercentCheckpointInput struct {
	KeyResultID int64
	MetricValue float64
	KRPercent   int
}

// LinearMetaInput is used by UpsertLinearMeta.
type LinearMetaInput struct {
	KeyResultID  int64
	StartValue   float64
	TargetValue  float64
	CurrentValue float64
}

func (r *KRRepository) CreateKeyResult(ctx context.Context, input KeyResultInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO key_results (goal_id, title, description, weight, kind, sort_order)
		VALUES ($1,$2,$3,$4,$5, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM key_results WHERE goal_id=$1))
		RETURNING id`,
		input.GoalID, input.Title, input.Description, input.Weight, input.Kind,
	).Scan(&id)
	return id, err
}

func (r *KRRepository) ListKeyResultsByGoal(ctx context.Context, goalID int64) ([]domain.KeyResult, error) {
	rows, err := r.db.Query(ctx, `
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
			stages, err := r.ListProjectStages(ctx, kr.ID)
			if err != nil {
				return nil, err
			}
			kr.Project = &domain.KRProject{Stages: stages}
		case domain.KRKindPercent:
			meta, checkpoints, err := r.GetPercentMeta(ctx, kr.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if meta != nil {
				meta.Checkpoints = checkpoints
				kr.Percent = meta
			}
		case domain.KRKindLinear:
			meta, err := r.GetLinearMeta(ctx, kr.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if meta != nil {
				kr.Linear = meta
			}
		case domain.KRKindBoolean:
			meta, err := r.GetBooleanMeta(ctx, kr.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if meta != nil {
				kr.Boolean = meta
			}
		}

		comments, _ := r.LastKeyResultComments(ctx, kr.ID)
		kr.Comments = comments
	}

	return krs, nil
}

func (r *KRRepository) AddKeyResultComment(ctx context.Context, krID int64, text string, authorUserID int64) error {
	_, err := r.db.Exec(ctx, `INSERT INTO key_result_comments (key_result_id, text, author_user_id) VALUES ($1,$2,$3)`, krID, text, authorUserID)
	return err
}

func (r *KRRepository) LastKeyResultComments(ctx context.Context, krID int64) ([]domain.KeyResultComment, error) {
	const Limit = 3
	rows, err := r.db.Query(ctx, `
		SELECT krc.id, krc.key_result_id, krc.text, u.display_name, u.udid, krc.created_at
		FROM key_result_comments krc
		JOIN users u ON u.id = krc.author_user_id
		WHERE krc.key_result_id = $1
		ORDER BY krc.created_at DESC LIMIT $2`, krID, Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var comments []domain.KeyResultComment
	for rows.Next() {
		var c domain.KeyResultComment
		if err := rows.Scan(&c.ID, &c.KeyResultID, &c.Text, &c.AuthorName, &c.AuthorUDID, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (r *KRRepository) AddProjectStage(ctx context.Context, input ProjectStageInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
		VALUES ($1,$2,$3,$4,$5)`,
		input.KeyResultID, input.Title, input.Weight, input.IsDone, input.SortOrder,
	)
	if err != nil {
		return err
	}
	return r.touchKeyResultUpdatedAt(ctx, input.KeyResultID)
}

func (r *KRRepository) UpdateProjectStageDone(ctx context.Context, stageID int64, done bool) error {
	_, err := r.db.Exec(ctx, `UPDATE kr_project_stages SET is_done=$1 WHERE id=$2`, done, stageID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		UPDATE key_results
		SET updated_at=NOW(), progress_updated_at=NOW()
		WHERE id=(SELECT key_result_id FROM kr_project_stages WHERE id=$1)`, stageID)
	return err
}

// BatchUpdateProjectStagesDone updates is_done for multiple stages in two queries:
// one batch UPDATE for stages, one touch on the parent key_result.
func (r *KRRepository) BatchUpdateProjectStagesDone(ctx context.Context, krID int64, updates map[int64]bool) error {
	if len(updates) == 0 {
		return nil
	}
	stageIDs := make([]int64, 0, len(updates))
	doneValues := make([]bool, 0, len(updates))
	for id, done := range updates {
		stageIDs = append(stageIDs, id)
		doneValues = append(doneValues, done)
	}
	_, err := r.db.Exec(ctx, `
		UPDATE kr_project_stages SET is_done = u.done
		FROM (SELECT unnest($1::bigint[]) AS id, unnest($2::boolean[]) AS done) u
		WHERE kr_project_stages.id = u.id`, stageIDs, doneValues)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		UPDATE key_results SET updated_at=NOW(), progress_updated_at=NOW() WHERE id=$1`, krID)
	return err
}

func (r *KRRepository) ListProjectStages(ctx context.Context, krID int64) ([]domain.KRProjectStage, error) {
	rows, err := r.db.Query(ctx, `
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

func (r *KRRepository) ReplaceProjectStages(ctx context.Context, krID int64, stages []ProjectStageInput) error {
	_, err := r.db.Exec(ctx, `DELETE FROM kr_project_stages WHERE key_result_id=$1`, krID)
	if err != nil {
		return err
	}
	for _, stage := range stages {
		if _, err := r.db.Exec(ctx, `
			INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
			VALUES ($1,$2,$3,$4,$5)`,
			krID, stage.Title, stage.Weight, stage.IsDone, stage.SortOrder,
		); err != nil {
			return err
		}
	}
	return r.touchKeyResultUpdatedAt(ctx, krID)
}

func (r *KRRepository) UpdateKeyResult(ctx context.Context, input KeyResultUpdateInput) error {
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET title=$1, description=$2, weight=$3, kind=$4, updated_at=NOW()
		WHERE id=$5`,
		input.Title, input.Description, input.Weight, input.Kind, input.ID,
	)
	return err
}

func (r *KRRepository) UpdateKeyResultWeight(ctx context.Context, krID int64, weight int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET weight=$1, updated_at=NOW()
		WHERE id=$2`,
		weight, krID,
	)
	return err
}

func (r *KRRepository) UpsertPercentMeta(ctx context.Context, input PercentMetaInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO kr_percent_meta (key_result_id, start_value, target_value, current_value)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (key_result_id) DO UPDATE SET
			start_value=EXCLUDED.start_value,
			target_value=EXCLUDED.target_value,
			current_value=EXCLUDED.current_value`,
		input.KeyResultID, input.StartValue, input.TargetValue, input.CurrentValue,
	)
	if err != nil {
		return err
	}
	return r.touchKeyResultUpdatedAt(ctx, input.KeyResultID)
}

func (r *KRRepository) UpdatePercentCurrent(ctx context.Context, krID int64, current float64) error {
	_, err := r.db.Exec(ctx, `UPDATE kr_percent_meta SET current_value=$1 WHERE key_result_id=$2`, current, krID)
	if err != nil {
		return err
	}
	return r.touchKeyResultProgressUpdatedAt(ctx, krID)
}

func (r *KRRepository) UpsertLinearMeta(ctx context.Context, input LinearMetaInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO kr_linear_meta (key_result_id, start_value, target_value, current_value)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (key_result_id) DO UPDATE SET
			start_value=EXCLUDED.start_value,
			target_value=EXCLUDED.target_value,
			current_value=EXCLUDED.current_value`,
		input.KeyResultID, input.StartValue, input.TargetValue, input.CurrentValue,
	)
	if err != nil {
		return err
	}
	return r.touchKeyResultUpdatedAt(ctx, input.KeyResultID)
}

func (r *KRRepository) UpdateLinearCurrent(ctx context.Context, krID int64, current float64) error {
	_, err := r.db.Exec(ctx, `UPDATE kr_linear_meta SET current_value=$1 WHERE key_result_id=$2`, current, krID)
	if err != nil {
		return err
	}
	return r.touchKeyResultProgressUpdatedAt(ctx, krID)
}

func (r *KRRepository) UpdateBoolean(ctx context.Context, krID int64, done bool) error {
	if err := r.UpsertBooleanMeta(ctx, krID, done); err != nil {
		return err
	}
	return r.touchKeyResultProgressUpdatedAt(ctx, krID)
}

func (r *KRRepository) GetKeyResult(ctx context.Context, id int64) (domain.KeyResult, error) {
	var kr domain.KeyResult
	row := r.db.QueryRow(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at
		FROM key_results WHERE id=$1`, id)
	if err := row.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt); err != nil {
		return domain.KeyResult{}, err
	}
	return kr, nil
}

func (r *KRRepository) AddPercentCheckpoint(ctx context.Context, input PercentCheckpointInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO kr_percent_checkpoints (key_result_id, metric_value, kr_percent)
		VALUES ($1,$2,$3)`,
		input.KeyResultID, input.MetricValue, input.KRPercent,
	)
	if err != nil {
		return err
	}
	return r.touchKeyResultUpdatedAt(ctx, input.KeyResultID)
}

func (r *KRRepository) GetPercentMeta(ctx context.Context, krID int64) (*domain.KRPercent, []domain.KRPercentCheckpoint, error) {
	var meta domain.KRPercent
	row := r.db.QueryRow(ctx, `SELECT start_value, target_value, current_value FROM kr_percent_meta WHERE key_result_id=$1`, krID)
	if err := row.Scan(&meta.StartValue, &meta.TargetValue, &meta.CurrentValue); err != nil {
		return nil, nil, err
	}
	checkpoints, err := r.ListPercentCheckpoints(ctx, krID)
	if err != nil {
		return nil, nil, err
	}
	return &meta, checkpoints, nil
}

func (r *KRRepository) GetLinearMeta(ctx context.Context, krID int64) (*domain.KRLinear, error) {
	var meta domain.KRLinear
	row := r.db.QueryRow(ctx, `SELECT start_value, target_value, current_value FROM kr_linear_meta WHERE key_result_id=$1`, krID)
	if err := row.Scan(&meta.StartValue, &meta.TargetValue, &meta.CurrentValue); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (r *KRRepository) ListPercentCheckpoints(ctx context.Context, krID int64) ([]domain.KRPercentCheckpoint, error) {
	rows, err := r.db.Query(ctx, `
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

func (r *KRRepository) UpsertBooleanMeta(ctx context.Context, krID int64, done bool) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO kr_boolean_meta (key_result_id, is_done)
		VALUES ($1,$2)
		ON CONFLICT (key_result_id) DO UPDATE SET is_done=EXCLUDED.is_done`,
		krID, done,
	)
	if err != nil {
		return err
	}
	return r.touchKeyResultUpdatedAt(ctx, krID)
}

func (r *KRRepository) GetBooleanMeta(ctx context.Context, krID int64) (*domain.KRBoolean, error) {
	var meta domain.KRBoolean
	row := r.db.QueryRow(ctx, `SELECT is_done FROM kr_boolean_meta WHERE key_result_id=$1`, krID)
	if err := row.Scan(&meta.IsDone); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (r *KRRepository) DeleteKeyResult(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM key_results WHERE id=$1`, id)
	return err
}

func (r *KRRepository) MoveKeyResult(ctx context.Context, krID int64, direction int) error {
	tx, err := r.db.Begin(ctx)
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

func (r *KRRepository) touchKeyResultUpdatedAt(ctx context.Context, krID int64) error {
	_, err := r.db.Exec(ctx, `UPDATE key_results SET updated_at=NOW() WHERE id=$1`, krID)
	return err
}

func (r *KRRepository) touchKeyResultProgressUpdatedAt(ctx context.Context, krID int64) error {
	_, err := r.db.Exec(ctx, `UPDATE key_results SET updated_at=NOW(), progress_updated_at=NOW() WHERE id=$1`, krID)
	return err
}

func (r *KRRepository) FindGoalIDByKR(ctx context.Context, krID int64) (int64, error) {
	var goalID int64
	err := r.db.QueryRow(ctx, `SELECT goal_id FROM key_results WHERE id=$1`, krID).Scan(&goalID)
	return goalID, err
}

func (r *KRRepository) FindGoalIDByStage(ctx context.Context, stageID int64) (int64, error) {
	var goalID int64
	err := r.db.QueryRow(ctx, `
		SELECT kr.goal_id
		FROM kr_project_stages s
		JOIN key_results kr ON kr.id = s.key_result_id
		WHERE s.id=$1`, stageID).Scan(&goalID)
	return goalID, err
}
