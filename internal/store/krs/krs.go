package krs

import (
	"context"
	"encoding/json"
	"errors"

	"okrs/internal/core/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	GoalID          int64
	Title           string
	Description     string
	ZeroingCriteria string
	Weight          int
	Kind            domain.KRKind
}

// KeyResultUpdateInput is used by UpdateKeyResult.
type KeyResultUpdateInput struct {
	ID              int64
	Title           string
	Description     string
	ZeroingCriteria string
	Weight          int
	Kind            domain.KRKind
}

// ProjectStageInput is used by ReplaceProjectStages.
type ProjectStageInput struct {
	KeyResultID int64
	Title       string
	Weight      int
	SortOrder   int
	IsDone      bool
}

// NumericalMetaInput is used by UpsertNumericalMeta.
type NumericalMetaInput struct {
	KeyResultID  int64
	StartValue   float64
	TargetValue  float64
	CurrentValue float64
	Unit         string
	Checkpoints  []domain.KRNumericalCheckpoint
}

// ParseCheckpoints decodes the key_results.checkpoints JSONB payload.
func ParseCheckpoints(raw []byte) ([]domain.KRNumericalCheckpoint, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var cps []domain.KRNumericalCheckpoint
	if err := json.Unmarshal(raw, &cps); err != nil {
		return nil, err
	}
	return cps, nil
}

// scanNumerical builds a *domain.KRNumerical from nullable column holders.
func scanNumerical(start, target, current *float64, unit *string, checkpointsRaw []byte) (*domain.KRNumerical, error) {
	num := &domain.KRNumerical{}
	if start != nil {
		num.StartValue = *start
	}
	if target != nil {
		num.TargetValue = *target
	}
	if current != nil {
		num.CurrentValue = *current
	}
	if unit != nil {
		num.Unit = *unit
	}
	cps, err := ParseCheckpoints(checkpointsRaw)
	if err != nil {
		return nil, err
	}
	num.Checkpoints = cps
	return num, nil
}

func (r *KRRepository) CreateKeyResult(ctx context.Context, scope domain.TenantScope, input KeyResultInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO key_results (goal_id, title, description, zeroing_criteria, weight, kind, sort_order, tenant_id)
		VALUES ($1,$2,$3,$4,$5,$6, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM key_results WHERE goal_id=$1 AND tenant_id=$7), $7)
		RETURNING id`,
		input.GoalID, input.Title, input.Description, input.ZeroingCriteria, input.Weight, input.Kind, scope.TenantID,
	).Scan(&id)
	return id, err
}

func (r *KRRepository) ListKeyResultsByGoal(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.KeyResult, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at,
		       start_value, target_value, current_value, unit, checkpoints, zeroing_criteria, health_status
		FROM key_results WHERE goal_id=$1 AND tenant_id=$2 ORDER BY sort_order, id`, goalID, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var krs []domain.KeyResult
	for rows.Next() {
		var kr domain.KeyResult
		var startValue, targetValue, currentValue *float64
		var unit, zeroing *string
		var checkpointsRaw []byte
		if err := rows.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt,
			&startValue, &targetValue, &currentValue, &unit, &checkpointsRaw, &zeroing, &kr.HealthStatus); err != nil {
			return nil, err
		}
		if zeroing != nil {
			kr.ZeroingCriteria = *zeroing
		}
		if kr.Kind == domain.KRKindNumerical {
			num, err := scanNumerical(startValue, targetValue, currentValue, unit, checkpointsRaw)
			if err != nil {
				return nil, err
			}
			kr.Numerical = num
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
			stages, err := r.ListProjectStages(ctx, scope, kr.ID)
			if err != nil {
				return nil, err
			}
			kr.Project = &domain.KRProject{Stages: stages}
		case domain.KRKindBoolean:
			meta, err := r.GetBooleanMeta(ctx, scope, kr.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if meta != nil {
				kr.Boolean = meta
			}
		}

	}

	krIDs := make([]int64, len(krs))
	for i, kr := range krs {
		krIDs[i] = kr.ID
	}
	notes, err := r.BatchLoadNotes(ctx, scope, krIDs)
	if err != nil {
		return nil, err
	}
	for i := range krs {
		krs[i].Note = notes[krs[i].ID]
	}

	return krs, nil
}

func upsertKeyResultNote(ctx context.Context, q execer, scope domain.TenantScope, krID int64, text string, authorUserID int64) error {
	_, err := q.Exec(ctx, `
		INSERT INTO key_result_notes (key_result_id, text, author_user_id, updated_at, tenant_id)
		VALUES ($1, $2, $3, NOW(), $4)
		ON CONFLICT (key_result_id) DO UPDATE
		SET text = EXCLUDED.text,
		    author_user_id = EXCLUDED.author_user_id,
		    updated_at = NOW()`,
		krID, text, authorUserID, scope.TenantID,
	)
	return err
}

func (r *KRRepository) UpsertKeyResultNote(ctx context.Context, scope domain.TenantScope, krID int64, text string, authorUserID int64) error {
	return upsertKeyResultNote(ctx, r.db, scope, krID, text, authorUserID)
}

// GetKeyResultNote returns the note for a single KR, or nil if it has none.
func (r *KRRepository) GetKeyResultNote(ctx context.Context, scope domain.TenantScope, krID int64) (*domain.KeyResultNote, error) {
	notes, err := r.BatchLoadNotes(ctx, scope, []int64{krID})
	if err != nil {
		return nil, err
	}
	return notes[krID], nil
}

// BatchLoadNotes returns a map from krID to *domain.KeyResultNote.
// KRs without a note are absent from the map (not nil-keyed).
func (r *KRRepository) BatchLoadNotes(ctx context.Context, scope domain.TenantScope, krIDs []int64) (map[int64]*domain.KeyResultNote, error) {
	if len(krIDs) == 0 {
		return map[int64]*domain.KeyResultNote{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT krn.key_result_id, krn.text, u.display_name, u.udid, krn.updated_at
		FROM key_result_notes krn
		JOIN users u ON u.id = krn.author_user_id
		WHERE krn.key_result_id = ANY($1) AND krn.tenant_id = $2`, krIDs, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]*domain.KeyResultNote, len(krIDs))
	for rows.Next() {
		var n domain.KeyResultNote
		if err := rows.Scan(&n.KeyResultID, &n.Text, &n.AuthorName, &n.AuthorUDID, &n.UpdatedAt); err != nil {
			return nil, err
		}
		note := n
		result[n.KeyResultID] = &note
	}
	return result, rows.Err()
}

func (r *KRRepository) AddProjectStage(ctx context.Context, scope domain.TenantScope, input ProjectStageInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (SELECT 1 FROM key_results k WHERE k.id = $1 AND k.tenant_id = $6)`,
		input.KeyResultID, input.Title, input.Weight, input.IsDone, input.SortOrder, scope.TenantID,
	)
	if err != nil {
		return err
	}
	return r.touchKeyResultUpdatedAt(ctx, scope, input.KeyResultID)
}

func (r *KRRepository) UpdateProjectStageDone(ctx context.Context, scope domain.TenantScope, stageID int64, done bool) error {
	_, err := r.db.Exec(ctx, `
		UPDATE kr_project_stages SET is_done=$1
		WHERE id=$2
		  AND EXISTS (SELECT 1 FROM key_results k WHERE k.id = kr_project_stages.key_result_id AND k.tenant_id = $3)`,
		done, stageID, scope.TenantID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		UPDATE key_results
		SET updated_at=NOW(), progress_updated_at=NOW()
		WHERE id=(SELECT key_result_id FROM kr_project_stages WHERE id=$1)
		  AND tenant_id=$2`, stageID, scope.TenantID)
	return err
}

// BatchUpdateProjectStagesDone updates is_done for multiple stages in two queries:
// one batch UPDATE for stages, one touch on the parent key_result.
func batchUpdateProjectStagesDone(ctx context.Context, q execer, scope domain.TenantScope, krID int64, updates map[int64]bool) error {
	if len(updates) == 0 {
		return nil
	}
	stageIDs := make([]int64, 0, len(updates))
	doneValues := make([]bool, 0, len(updates))
	for id, done := range updates {
		stageIDs = append(stageIDs, id)
		doneValues = append(doneValues, done)
	}
	_, err := q.Exec(ctx, `
		UPDATE kr_project_stages SET is_done = u.done
		FROM (SELECT unnest($1::bigint[]) AS id, unnest($2::boolean[]) AS done) u
		WHERE kr_project_stages.id = u.id
		  AND EXISTS (SELECT 1 FROM key_results k WHERE k.id = kr_project_stages.key_result_id AND k.tenant_id = $3)`,
		stageIDs, doneValues, scope.TenantID)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		UPDATE key_results SET updated_at=NOW(), progress_updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, krID, scope.TenantID)
	return err
}

func (r *KRRepository) BatchUpdateProjectStagesDone(ctx context.Context, scope domain.TenantScope, krID int64, updates map[int64]bool) error {
	return batchUpdateProjectStagesDone(ctx, r.db, scope, krID, updates)
}

func (r *KRRepository) ListProjectStages(ctx context.Context, scope domain.TenantScope, krID int64) ([]domain.KRProjectStage, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, key_result_id, title, weight, is_done, sort_order
		FROM kr_project_stages
		WHERE key_result_id=$1
		  AND EXISTS (SELECT 1 FROM key_results k WHERE k.id = $1 AND k.tenant_id = $2)
		ORDER BY sort_order`, krID, scope.TenantID)
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

func (r *KRRepository) ReplaceProjectStages(ctx context.Context, scope domain.TenantScope, krID int64, stages []ProjectStageInput) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM kr_project_stages
		WHERE key_result_id=$1
		  AND EXISTS (SELECT 1 FROM key_results k WHERE k.id = $1 AND k.tenant_id = $2)`,
		krID, scope.TenantID)
	if err != nil {
		return err
	}
	for _, stage := range stages {
		if _, err := r.db.Exec(ctx, `
			INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
			SELECT $1, $2, $3, $4, $5
			WHERE EXISTS (SELECT 1 FROM key_results k WHERE k.id = $1 AND k.tenant_id = $6)`,
			krID, stage.Title, stage.Weight, stage.IsDone, stage.SortOrder, scope.TenantID,
		); err != nil {
			return err
		}
	}
	return r.touchKeyResultUpdatedAt(ctx, scope, krID)
}

func (r *KRRepository) UpdateKeyResult(ctx context.Context, scope domain.TenantScope, input KeyResultUpdateInput) error {
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET title=$1, description=$2, zeroing_criteria=$3, weight=$4, kind=$5, updated_at=NOW()
		WHERE id=$6 AND tenant_id=$7`,
		input.Title, input.Description, input.ZeroingCriteria, input.Weight, input.Kind, input.ID, scope.TenantID,
	)
	return err
}

func (r *KRRepository) UpdateKeyResultWeight(ctx context.Context, scope domain.TenantScope, krID int64, weight int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET weight=$1, updated_at=NOW()
		WHERE id=$2 AND tenant_id=$3`,
		weight, krID, scope.TenantID,
	)
	return err
}

func (r *KRRepository) UpdateKeyResultDescription(ctx context.Context, scope domain.TenantScope, krID int64, description string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET description=$1, updated_at=NOW()
		WHERE id=$2 AND tenant_id=$3`,
		description, krID, scope.TenantID,
	)
	return err
}

func (r *KRRepository) UpsertNumericalMeta(ctx context.Context, scope domain.TenantScope, input NumericalMetaInput) error {
	var checkpointsJSON []byte
	if len(input.Checkpoints) > 0 {
		b, err := json.Marshal(input.Checkpoints)
		if err != nil {
			return err
		}
		checkpointsJSON = b
	}
	_, err := r.db.Exec(ctx, `
		UPDATE key_results
		SET start_value=$1, target_value=$2, current_value=$3, unit=$4,
		    checkpoints=$5, updated_at=NOW()
		WHERE id=$6 AND tenant_id=$7`,
		input.StartValue, input.TargetValue, input.CurrentValue, input.Unit,
		checkpointsJSON, input.KeyResultID, scope.TenantID,
	)
	return err
}

// execer is the slice of pgx that both *pgxpool.Pool and pgx.Tx satisfy. Every
// check-in write below is expressed against it exactly once, so the transactional
// path (ApplyCheckIn) and the standalone callers run the SAME statement — a second
// copy of this SQL is how the two would quietly drift apart.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func updateNumericalCurrent(ctx context.Context, q execer, scope domain.TenantScope, krID int64, current float64) error {
	_, err := q.Exec(ctx, `
		UPDATE key_results
		SET current_value=$1, updated_at=NOW(), progress_updated_at=NOW()
		WHERE id=$2 AND tenant_id=$3`, current, krID, scope.TenantID)
	return err
}

func (r *KRRepository) UpdateNumericalCurrent(ctx context.Context, scope domain.TenantScope, krID int64, current float64) error {
	return updateNumericalCurrent(ctx, r.db, scope, krID, current)
}

func updateHealthStatus(ctx context.Context, q execer, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	_, err := q.Exec(ctx, `
		UPDATE key_results
		SET health_status=$1, updated_at=NOW()
		WHERE id=$2 AND tenant_id=$3`, string(status), krID, scope.TenantID)
	return err
}

func (r *KRRepository) UpdateHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	return updateHealthStatus(ctx, r.db, scope, krID, status)
}

func (r *KRRepository) UpdateBoolean(ctx context.Context, scope domain.TenantScope, krID int64, done bool) error {
	if err := r.UpsertBooleanMeta(ctx, scope, krID, done); err != nil {
		return err
	}
	return r.touchKeyResultProgressUpdatedAt(ctx, scope, krID)
}

func (r *KRRepository) GetKeyResult(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error) {
	var kr domain.KeyResult
	var startValue, targetValue, currentValue *float64
	var unit, zeroing *string
	var checkpointsRaw []byte
	row := r.db.QueryRow(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at,
		       start_value, target_value, current_value, unit, checkpoints, zeroing_criteria, health_status
		FROM key_results WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID)
	if err := row.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt,
		&startValue, &targetValue, &currentValue, &unit, &checkpointsRaw, &zeroing, &kr.HealthStatus); err != nil {
		return domain.KeyResult{}, err
	}
	if zeroing != nil {
		kr.ZeroingCriteria = *zeroing
	}
	if kr.Kind == domain.KRKindNumerical {
		num, err := scanNumerical(startValue, targetValue, currentValue, unit, checkpointsRaw)
		if err != nil {
			return domain.KeyResult{}, err
		}
		kr.Numerical = num
	}
	return kr, nil
}

func upsertBooleanMeta(ctx context.Context, q execer, scope domain.TenantScope, krID int64, done bool) error {
	_, err := q.Exec(ctx, `
		INSERT INTO kr_boolean_meta (key_result_id, is_done)
		SELECT $1, $2
		WHERE EXISTS (SELECT 1 FROM key_results k WHERE k.id = $1 AND k.tenant_id = $3)
		ON CONFLICT (key_result_id) DO UPDATE SET is_done=EXCLUDED.is_done`,
		krID, done, scope.TenantID,
	)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE key_results SET updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, krID, scope.TenantID)
	return err
}

func (r *KRRepository) UpsertBooleanMeta(ctx context.Context, scope domain.TenantScope, krID int64, done bool) error {
	return upsertBooleanMeta(ctx, r.db, scope, krID, done)
}

func (r *KRRepository) GetBooleanMeta(ctx context.Context, scope domain.TenantScope, krID int64) (*domain.KRBoolean, error) {
	var meta domain.KRBoolean
	row := r.db.QueryRow(ctx, `
		SELECT b.is_done
		FROM kr_boolean_meta b
		WHERE b.key_result_id=$1
		  AND EXISTS (SELECT 1 FROM key_results k WHERE k.id = $1 AND k.tenant_id = $2)`,
		krID, scope.TenantID)
	if err := row.Scan(&meta.IsDone); err != nil {
		return nil, err
	}
	return &meta, nil
}

// CheckInNote is the note half of a check-in. Nil means the submission did not
// include a note, which is not the same as an empty one — empty clears the text.
type CheckInNote struct {
	Text         string
	AuthorUserID int64
}

// CheckInWrites is everything one check-in changes. A nil field was not part of the
// submission and must be left alone.
type CheckInWrites struct {
	Numerical *float64
	Boolean   *bool
	Stages    map[int64]bool
	Health    *domain.KRHealthStatus
	Note      *CheckInNote
}

// ApplyCheckIn writes a whole check-in in ONE transaction.
//
// Progress, health status and note used to be three separately committed writes.
// A failure on the second left the first persisted, the caller got an error, and
// no KRCheckedIn event was published — so the database moved while the journal and
// the notifications did not, and nothing downstream could tell. The event is the
// record of what happened, so the writes it describes have to land or not land
// together.
func (r *KRRepository) ApplyCheckIn(ctx context.Context, scope domain.TenantScope, krID int64, w CheckInWrites) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	switch {
	case w.Numerical != nil:
		if err := updateNumericalCurrent(ctx, tx, scope, krID, *w.Numerical); err != nil {
			return err
		}
	case w.Boolean != nil:
		if err := upsertBooleanMeta(ctx, tx, scope, krID, *w.Boolean); err != nil {
			return err
		}
		if err := touchProgressUpdatedAt(ctx, tx, scope, krID); err != nil {
			return err
		}
	case w.Stages != nil:
		if err := batchUpdateProjectStagesDone(ctx, tx, scope, krID, w.Stages); err != nil {
			return err
		}
	}
	if w.Health != nil {
		if err := updateHealthStatus(ctx, tx, scope, krID, *w.Health); err != nil {
			return err
		}
	}
	if w.Note != nil {
		if err := upsertKeyResultNote(ctx, tx, scope, krID, w.Note.Text, w.Note.AuthorUserID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *KRRepository) DeleteKeyResult(ctx context.Context, scope domain.TenantScope, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM key_results WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID)
	return err
}

func (r *KRRepository) MoveKeyResult(ctx context.Context, scope domain.TenantScope, krID int64, direction int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var goalID int64
	var currentOrder int
	row := tx.QueryRow(ctx, `SELECT goal_id, sort_order FROM key_results WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, krID, scope.TenantID)
	if err := row.Scan(&goalID, &currentOrder); err != nil {
		return err
	}

	var neighborID int64
	var neighborOrder int
	if direction < 0 {
		row = tx.QueryRow(ctx, `
			SELECT id, sort_order FROM key_results
			WHERE goal_id=$1 AND sort_order < $2 AND tenant_id=$3
			ORDER BY sort_order DESC LIMIT 1
			FOR UPDATE`, goalID, currentOrder, scope.TenantID)
	} else {
		row = tx.QueryRow(ctx, `
			SELECT id, sort_order FROM key_results
			WHERE goal_id=$1 AND sort_order > $2 AND tenant_id=$3
			ORDER BY sort_order ASC LIMIT 1
			FOR UPDATE`, goalID, currentOrder, scope.TenantID)
	}
	if err := row.Scan(&neighborID, &neighborOrder); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE key_results SET sort_order=$1 WHERE id=$2 AND tenant_id=$3`, neighborOrder, krID, scope.TenantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE key_results SET sort_order=$1 WHERE id=$2 AND tenant_id=$3`, currentOrder, neighborID, scope.TenantID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *KRRepository) touchKeyResultUpdatedAt(ctx context.Context, scope domain.TenantScope, krID int64) error {
	_, err := r.db.Exec(ctx, `UPDATE key_results SET updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, krID, scope.TenantID)
	return err
}

func touchProgressUpdatedAt(ctx context.Context, q execer, scope domain.TenantScope, krID int64) error {
	_, err := q.Exec(ctx, `UPDATE key_results SET updated_at=NOW(), progress_updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, krID, scope.TenantID)
	return err
}

func (r *KRRepository) touchKeyResultProgressUpdatedAt(ctx context.Context, scope domain.TenantScope, krID int64) error {
	return touchProgressUpdatedAt(ctx, r.db, scope, krID)
}

func (r *KRRepository) FindGoalIDByKR(ctx context.Context, scope domain.TenantScope, krID int64) (int64, error) {
	var goalID int64
	err := r.db.QueryRow(ctx, `SELECT goal_id FROM key_results WHERE id=$1 AND tenant_id=$2`, krID, scope.TenantID).Scan(&goalID)
	return goalID, err
}

func (r *KRRepository) FindGoalIDByStage(ctx context.Context, scope domain.TenantScope, stageID int64) (int64, error) {
	var goalID int64
	err := r.db.QueryRow(ctx, `
		SELECT kr.goal_id
		FROM kr_project_stages s
		JOIN key_results kr ON kr.id = s.key_result_id
		WHERE s.id=$1 AND kr.tenant_id=$2`, stageID, scope.TenantID).Scan(&goalID)
	return goalID, err
}
