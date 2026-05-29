package goals

import (
	"context"
	"errors"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/krs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GoalRepository handles all goal persistence (goals, comments).
// It depends on KRRepository to load key results as part of the goal aggregate.
type GoalRepository struct {
	db  *pgxpool.Pool
	krs *krs.KRRepository
}

func NewGoalRepository(db *pgxpool.Pool, krsRepo *krs.KRRepository) *GoalRepository {
	return &GoalRepository{db: db, krs: krsRepo}
}

// DB returns the underlying connection pool (used by seed helpers).
func (r *GoalRepository) DB() *pgxpool.Pool {
	return r.db
}

type GoalInput struct {
	TeamID      int64
	PeriodID    int64
	Title       string
	Description string
	Priority    domain.Priority
	Weight      int
	WorkType    domain.WorkType
	FocusType   domain.FocusType
	OwnerText   string
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

type TeamOverviewStats struct {
	TeamID     int64
	Goals      int
	PriorityP0 int
	PriorityP1 int
	PriorityP2 int
	PriorityP3 int
	Discovery  int
	Delivery   int
}

func (r *GoalRepository) ListGoalsByTeamsPeriod(ctx context.Context, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error) {
	result := make(map[int64][]domain.Goal, len(teamIDs))
	if len(teamIDs) == 0 {
		return result, nil
	}
	teamGoalOrder := make(map[int64][]int64, len(teamIDs))
	teamGoals := make(map[int64]map[int64]*domain.Goal, len(teamIDs))

	goalRows, err := r.db.Query(ctx, `
		SELECT g.id, t.team_id, g.period_id, g.title, g.description, g.priority,
		       COALESCE(gs.weight, g.weight) AS weight,
		       g.work_type, g.focus_type, g.owner_text, g.created_at, g.updated_at
		FROM (
			SELECT g.id, g.team_id
			FROM goals g
			WHERE g.period_id = $1 AND g.team_id = ANY($2)
			UNION
			SELECT g.id, gs.team_id
			FROM goals g
			JOIN goal_shares gs ON gs.goal_id = g.id
			WHERE g.period_id = $1 AND gs.team_id = ANY($2)
		) t
		JOIN goals g ON g.id = t.id
		LEFT JOIN goal_shares gs ON gs.goal_id = g.id AND gs.team_id = t.team_id
		ORDER BY t.team_id, g.id`, periodID, teamIDs)
	if err != nil {
		return nil, err
	}
	defer goalRows.Close()

	goalsByID := map[int64][]*domain.Goal{}
	goalIDs := make([]int64, 0)
	goalSeen := map[int64]struct{}{}
	for goalRows.Next() {
		var goal domain.Goal
		if err := goalRows.Scan(
			&goal.ID,
			&goal.TeamID,
			&goal.PeriodID,
			&goal.Title,
			&goal.Description,
			&goal.Priority,
			&goal.Weight,
			&goal.WorkType,
			&goal.FocusType,
			&goal.OwnerText,
			&goal.CreatedAt,
			&goal.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if teamGoals[goal.TeamID] == nil {
			teamGoals[goal.TeamID] = make(map[int64]*domain.Goal)
		}
		goalValue := goal
		goalRef := &goalValue
		teamGoals[goal.TeamID][goal.ID] = goalRef
		teamGoalOrder[goal.TeamID] = append(teamGoalOrder[goal.TeamID], goal.ID)
		goalsByID[goalRef.ID] = append(goalsByID[goalRef.ID], goalRef)
		if _, exists := goalSeen[goalRef.ID]; !exists {
			goalIDs = append(goalIDs, goalRef.ID)
			goalSeen[goalRef.ID] = struct{}{}
		}
	}
	if err := goalRows.Err(); err != nil {
		return nil, err
	}
	goalLastKRActivity, err := r.listGoalLastKRActivity(ctx, goalIDs)
	if err != nil {
		return nil, err
	}
	for _, goals := range teamGoals {
		for _, goal := range goals {
			if updatedAt, ok := goalLastKRActivity[goal.ID]; ok {
				goal.UpdatedAt = updatedAt
			}
		}
	}
	if len(goalIDs) == 0 {
		return resultFromGoalPointers(teamGoals, teamGoalOrder), nil
	}

	krRows, err := r.db.Query(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at
		FROM key_results
		WHERE goal_id = ANY($1)
		ORDER BY goal_id, sort_order, id`, goalIDs)
	if err != nil {
		return nil, err
	}
	defer krRows.Close()

	krByID := map[int64][]*domain.KeyResult{}
	krIDs := make([]int64, 0)
	krIDSeen := map[int64]struct{}{}
	for krRows.Next() {
		var kr domain.KeyResult
		if err := krRows.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt); err != nil {
			return nil, err
		}
		goals, ok := goalsByID[kr.GoalID]
		if !ok {
			continue
		}
		for _, goal := range goals {
			goal.KeyResults = append(goal.KeyResults, kr)
		}
		if _, exists := krIDSeen[kr.ID]; !exists {
			krIDs = append(krIDs, kr.ID)
			krIDSeen[kr.ID] = struct{}{}
		}
	}
	if err := krRows.Err(); err != nil {
		return nil, err
	}

	// Build pointers in a separate pass after all appends are complete.
	// This avoids stale pointers when goal.KeyResults backing arrays reallocate.
	for _, goals := range goalsByID {
		for _, goal := range goals {
			for i := range goal.KeyResults {
				krRef := &goal.KeyResults[i]
				krByID[krRef.ID] = append(krByID[krRef.ID], krRef)
			}
		}
	}
	if len(krIDs) == 0 {
		return resultFromGoalPointers(teamGoals, teamGoalOrder), nil
	}

	projectRows, err := r.db.Query(ctx, `
		SELECT id, key_result_id, title, weight, is_done, sort_order
		FROM kr_project_stages
		WHERE key_result_id = ANY($1)
		ORDER BY key_result_id, sort_order`, krIDs)
	if err != nil {
		return nil, err
	}
	defer projectRows.Close()
	for projectRows.Next() {
		var stage domain.KRProjectStage
		if err := projectRows.Scan(&stage.ID, &stage.KeyResultID, &stage.Title, &stage.Weight, &stage.IsDone, &stage.SortOrder); err != nil {
			return nil, err
		}
		if krsSlice, ok := krByID[stage.KeyResultID]; ok {
			for _, kr := range krsSlice {
				if kr.Project == nil {
					kr.Project = &domain.KRProject{}
				}
				kr.Project.Stages = append(kr.Project.Stages, stage)
			}
		}
	}
	if err := projectRows.Err(); err != nil {
		return nil, err
	}

	percentRows, err := r.db.Query(ctx, `
		SELECT key_result_id, start_value, target_value, current_value
		FROM kr_percent_meta
		WHERE key_result_id = ANY($1)`, krIDs)
	if err != nil {
		return nil, err
	}
	defer percentRows.Close()
	for percentRows.Next() {
		var keyResultID int64
		var meta domain.KRPercent
		if err := percentRows.Scan(&keyResultID, &meta.StartValue, &meta.TargetValue, &meta.CurrentValue); err != nil {
			return nil, err
		}
		if krsSlice, ok := krByID[keyResultID]; ok {
			for _, kr := range krsSlice {
				value := meta
				kr.Percent = &value
			}
		}
	}
	if err := percentRows.Err(); err != nil {
		return nil, err
	}

	checkpointRows, err := r.db.Query(ctx, `
		SELECT id, key_result_id, metric_value, kr_percent
		FROM kr_percent_checkpoints
		WHERE key_result_id = ANY($1)
		ORDER BY key_result_id, metric_value`, krIDs)
	if err != nil {
		return nil, err
	}
	defer checkpointRows.Close()
	for checkpointRows.Next() {
		var cp domain.KRPercentCheckpoint
		if err := checkpointRows.Scan(&cp.ID, &cp.KeyResultID, &cp.MetricValue, &cp.KRPercent); err != nil {
			return nil, err
		}
		if krsSlice, ok := krByID[cp.KeyResultID]; ok {
			for _, kr := range krsSlice {
				if kr.Percent == nil {
					kr.Percent = &domain.KRPercent{}
				}
				kr.Percent.Checkpoints = append(kr.Percent.Checkpoints, cp)
			}
		}
	}
	if err := checkpointRows.Err(); err != nil {
		return nil, err
	}

	linearRows, err := r.db.Query(ctx, `
		SELECT key_result_id, start_value, target_value, current_value
		FROM kr_linear_meta
		WHERE key_result_id = ANY($1)`, krIDs)
	if err != nil {
		return nil, err
	}
	defer linearRows.Close()
	for linearRows.Next() {
		var keyResultID int64
		var meta domain.KRLinear
		if err := linearRows.Scan(&keyResultID, &meta.StartValue, &meta.TargetValue, &meta.CurrentValue); err != nil {
			return nil, err
		}
		if krsSlice, ok := krByID[keyResultID]; ok {
			for _, kr := range krsSlice {
				value := meta
				kr.Linear = &value
			}
		}
	}
	if err := linearRows.Err(); err != nil {
		return nil, err
	}

	booleanRows, err := r.db.Query(ctx, `
		SELECT key_result_id, is_done
		FROM kr_boolean_meta
		WHERE key_result_id = ANY($1)`, krIDs)
	if err != nil {
		return nil, err
	}
	defer booleanRows.Close()
	for booleanRows.Next() {
		var keyResultID int64
		var meta domain.KRBoolean
		if err := booleanRows.Scan(&keyResultID, &meta.IsDone); err != nil {
			return nil, err
		}
		if krsSlice, ok := krByID[keyResultID]; ok {
			for _, kr := range krsSlice {
				value := meta
				kr.Boolean = &value
			}
		}
	}
	if err := booleanRows.Err(); err != nil {
		return nil, err
	}

	return resultFromGoalPointers(teamGoals, teamGoalOrder), nil
}

func resultFromGoalPointers(teamGoals map[int64]map[int64]*domain.Goal, teamGoalOrder map[int64][]int64) map[int64][]domain.Goal {
	result := make(map[int64][]domain.Goal, len(teamGoals))
	for teamID, ids := range teamGoalOrder {
		for _, goalID := range ids {
			goalRef, ok := teamGoals[teamID][goalID]
			if !ok {
				continue
			}
			result[teamID] = append(result[teamID], *goalRef)
		}
	}
	return result
}

func (r *GoalRepository) CreateGoal(ctx context.Context, input GoalInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO goals (team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM goals WHERE team_id=$1 AND period_id=$2))
		RETURNING id`,
		input.TeamID, input.PeriodID, input.Title, input.Description, input.Priority, input.Weight, input.WorkType, input.FocusType, input.OwnerText,
	).Scan(&id)
	return id, err
}

func (r *GoalRepository) ListTeamOverviewStats(ctx context.Context, periodID int64, teamIDs []int64) (map[int64]TeamOverviewStats, error) {
	stats := make(map[int64]TeamOverviewStats, len(teamIDs))
	if len(teamIDs) == 0 {
		return stats, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT
			t.team_id,
			COUNT(*)::bigint AS goals_count,
			COUNT(*) FILTER (WHERE g.priority = 'P0')::bigint AS p0_count,
			COUNT(*) FILTER (WHERE g.priority = 'P1')::bigint AS p1_count,
			COUNT(*) FILTER (WHERE g.priority = 'P2')::bigint AS p2_count,
			COUNT(*) FILTER (WHERE g.priority = 'P3')::bigint AS p3_count,
			COUNT(*) FILTER (WHERE g.work_type = 'Discovery')::bigint AS discovery_count,
			COUNT(*) FILTER (WHERE g.work_type = 'Delivery')::bigint AS delivery_count
		FROM (
			SELECT g.id, g.team_id
			FROM goals g
			WHERE g.period_id = $1 AND g.team_id = ANY($2)
			UNION
			SELECT g.id, gs.team_id
			FROM goals g
			JOIN goal_shares gs ON gs.goal_id = g.id
			WHERE g.period_id = $1 AND gs.team_id = ANY($2)
		) t
		JOIN goals g ON g.id = t.id
		GROUP BY t.team_id`, periodID, teamIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item TeamOverviewStats
		if err := rows.Scan(
			&item.TeamID,
			&item.Goals,
			&item.PriorityP0,
			&item.PriorityP1,
			&item.PriorityP2,
			&item.PriorityP3,
			&item.Discovery,
			&item.Delivery,
		); err != nil {
			return nil, err
		}
		stats[item.TeamID] = item
	}
	return stats, rows.Err()
}

func (r *GoalRepository) ListGoalsByTeamPeriod(ctx context.Context, teamID, periodID int64) ([]domain.Goal, error) {
	rows, err := r.db.Query(ctx, `
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
	goalIDs := make([]int64, 0, len(goals))
	for _, goal := range goals {
		goalIDs = append(goalIDs, goal.ID)
	}
	goalLastKRActivity, err := r.listGoalLastKRActivity(ctx, goalIDs)
	if err != nil {
		return nil, err
	}
	for i := range goals {
		if updatedAt, ok := goalLastKRActivity[goals[i].ID]; ok {
			goals[i].UpdatedAt = updatedAt
		}
	}

	if err := r.loadKRsForGoals(ctx, goals, goalIDs); err != nil {
		return nil, err
	}
	commentsByGoal, err := r.listGoalCommentsBatch(ctx, goalIDs)
	if err != nil {
		return nil, err
	}
	for i := range goals {
		goals[i].Comments = commentsByGoal[goals[i].ID]
	}
	return goals, nil
}

// loadKRsForGoals batch-loads key results (all meta + last 3 comments) for the given goals.
// Updates goals[i].KeyResults in place. goalIDs must match the goals slice indices.
func (r *GoalRepository) loadKRsForGoals(ctx context.Context, goals []domain.Goal, goalIDs []int64) error {
	if len(goalIDs) == 0 {
		return nil
	}

	// Index goals by ID to attach KRs without re-scanning.
	goalByID := make(map[int64]*domain.Goal, len(goals))
	for i := range goals {
		goalByID[goals[i].ID] = &goals[i]
	}

	krRows, err := r.db.Query(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at
		FROM key_results
		WHERE goal_id = ANY($1)
		ORDER BY goal_id, sort_order, id`, goalIDs)
	if err != nil {
		return err
	}
	defer krRows.Close()

	krsByID := make(map[int64]*domain.KeyResult)
	for krRows.Next() {
		var kr domain.KeyResult
		if err := krRows.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt); err != nil {
			return err
		}
		g, ok := goalByID[kr.GoalID]
		if !ok {
			continue
		}
		g.KeyResults = append(g.KeyResults, kr)
	}
	if err := krRows.Err(); err != nil {
		return err
	}

	// Build KR pointer map after all appends — avoids stale pointers on slice reallocation.
	krIDs := make([]int64, 0)
	for i := range goals {
		for j := range goals[i].KeyResults {
			kr := &goals[i].KeyResults[j]
			krsByID[kr.ID] = kr
			krIDs = append(krIDs, kr.ID)
		}
	}
	if len(krIDs) == 0 {
		return nil
	}

	if err := r.loadProjectStages(ctx, krIDs, krsByID); err != nil {
		return err
	}
	if err := r.loadPercentMeta(ctx, krIDs, krsByID); err != nil {
		return err
	}
	if err := r.loadPercentCheckpoints(ctx, krIDs, krsByID); err != nil {
		return err
	}
	if err := r.loadLinearMeta(ctx, krIDs, krsByID); err != nil {
		return err
	}
	if err := r.loadBooleanMeta(ctx, krIDs, krsByID); err != nil {
		return err
	}
	return r.loadKRNotes(ctx, krIDs, krsByID)
}

func (r *GoalRepository) loadProjectStages(ctx context.Context, krIDs []int64, krsByID map[int64]*domain.KeyResult) error {
	rows, err := r.db.Query(ctx, `
		SELECT id, key_result_id, title, weight, is_done, sort_order
		FROM kr_project_stages
		WHERE key_result_id = ANY($1)
		ORDER BY key_result_id, sort_order`, krIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var stage domain.KRProjectStage
		if err := rows.Scan(&stage.ID, &stage.KeyResultID, &stage.Title, &stage.Weight, &stage.IsDone, &stage.SortOrder); err != nil {
			return err
		}
		if kr, ok := krsByID[stage.KeyResultID]; ok {
			if kr.Project == nil {
				kr.Project = &domain.KRProject{}
			}
			kr.Project.Stages = append(kr.Project.Stages, stage)
		}
	}
	return rows.Err()
}

func (r *GoalRepository) loadPercentMeta(ctx context.Context, krIDs []int64, krsByID map[int64]*domain.KeyResult) error {
	rows, err := r.db.Query(ctx, `
		SELECT key_result_id, start_value, target_value, current_value
		FROM kr_percent_meta
		WHERE key_result_id = ANY($1)`, krIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var krID int64
		var meta domain.KRPercent
		if err := rows.Scan(&krID, &meta.StartValue, &meta.TargetValue, &meta.CurrentValue); err != nil {
			return err
		}
		if kr, ok := krsByID[krID]; ok {
			value := meta
			kr.Percent = &value
		}
	}
	return rows.Err()
}

func (r *GoalRepository) loadPercentCheckpoints(ctx context.Context, krIDs []int64, krsByID map[int64]*domain.KeyResult) error {
	rows, err := r.db.Query(ctx, `
		SELECT id, key_result_id, metric_value, kr_percent
		FROM kr_percent_checkpoints
		WHERE key_result_id = ANY($1)
		ORDER BY key_result_id, metric_value`, krIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cp domain.KRPercentCheckpoint
		if err := rows.Scan(&cp.ID, &cp.KeyResultID, &cp.MetricValue, &cp.KRPercent); err != nil {
			return err
		}
		if kr, ok := krsByID[cp.KeyResultID]; ok {
			if kr.Percent == nil {
				kr.Percent = &domain.KRPercent{}
			}
			kr.Percent.Checkpoints = append(kr.Percent.Checkpoints, cp)
		}
	}
	return rows.Err()
}

func (r *GoalRepository) loadLinearMeta(ctx context.Context, krIDs []int64, krsByID map[int64]*domain.KeyResult) error {
	rows, err := r.db.Query(ctx, `
		SELECT key_result_id, start_value, target_value, current_value
		FROM kr_linear_meta
		WHERE key_result_id = ANY($1)`, krIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var krID int64
		var meta domain.KRLinear
		if err := rows.Scan(&krID, &meta.StartValue, &meta.TargetValue, &meta.CurrentValue); err != nil {
			return err
		}
		if kr, ok := krsByID[krID]; ok {
			value := meta
			kr.Linear = &value
		}
	}
	return rows.Err()
}

func (r *GoalRepository) loadBooleanMeta(ctx context.Context, krIDs []int64, krsByID map[int64]*domain.KeyResult) error {
	rows, err := r.db.Query(ctx, `
		SELECT key_result_id, is_done
		FROM kr_boolean_meta
		WHERE key_result_id = ANY($1)`, krIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var krID int64
		var meta domain.KRBoolean
		if err := rows.Scan(&krID, &meta.IsDone); err != nil {
			return err
		}
		if kr, ok := krsByID[krID]; ok {
			value := meta
			kr.Boolean = &value
		}
	}
	return rows.Err()
}

// loadKRNotes batch-loads the single note per KR using key_result_notes.
func (r *GoalRepository) loadKRNotes(ctx context.Context, krIDs []int64, krsByID map[int64]*domain.KeyResult) error {
	rows, err := r.db.Query(ctx, `
		SELECT krn.key_result_id, krn.text, u.display_name, u.udid, krn.updated_at
		FROM key_result_notes krn
		JOIN users u ON u.id = krn.author_user_id
		WHERE krn.key_result_id = ANY($1)`, krIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var n domain.KeyResultNote
		if err := rows.Scan(&n.KeyResultID, &n.Text, &n.AuthorName, &n.AuthorUDID, &n.UpdatedAt); err != nil {
			return err
		}
		if kr, ok := krsByID[n.KeyResultID]; ok {
			note := n
			kr.Note = &note
		}
	}
	return rows.Err()
}

func (r *GoalRepository) listGoalCommentsBatch(ctx context.Context, goalIDs []int64) (map[int64][]domain.GoalComment, error) {
	if len(goalIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT gc.id, gc.goal_id, gc.text, u.display_name, u.udid, gc.created_at
		FROM goal_comments gc
		JOIN users u ON u.id = gc.author_user_id
		WHERE gc.goal_id = ANY($1)
		ORDER BY gc.created_at DESC`, goalIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64][]domain.GoalComment)
	for rows.Next() {
		var c domain.GoalComment
		if err := rows.Scan(&c.ID, &c.GoalID, &c.Text, &c.AuthorName, &c.AuthorUDID, &c.CreatedAt); err != nil {
			return nil, err
		}
		result[c.GoalID] = append(result[c.GoalID], c)
	}
	return result, rows.Err()
}

func (r *GoalRepository) listGoalLastKRActivity(ctx context.Context, goalIDs []int64) (map[int64]time.Time, error) {
	result := make(map[int64]time.Time, len(goalIDs))
	if len(goalIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT
			kr.goal_id,
			CASE
				WHEN MAX(kr.progress_updated_at) IS NULL THEN MAX(krn.updated_at)
				WHEN MAX(krn.updated_at) IS NULL THEN MAX(kr.progress_updated_at)
				ELSE GREATEST(MAX(kr.progress_updated_at), MAX(krn.updated_at))
			END AS last_updated
		FROM key_results kr
		LEFT JOIN key_result_notes krn ON krn.key_result_id = kr.id
		WHERE kr.goal_id = ANY($1)
		GROUP BY kr.goal_id
		HAVING MAX(kr.progress_updated_at) IS NOT NULL OR MAX(krn.updated_at) IS NOT NULL`, goalIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var goalID int64
		var updatedAt time.Time
		if err := rows.Scan(&goalID, &updatedAt); err != nil {
			return nil, err
		}
		result[goalID] = updatedAt
	}
	return result, rows.Err()
}

func (r *GoalRepository) MoveGoal(ctx context.Context, goalID int64, direction int) error {
	tx, err := r.db.Begin(ctx)
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

func (r *GoalRepository) GetGoal(ctx context.Context, id int64) (domain.Goal, error) {
	var goal domain.Goal
	row := r.db.QueryRow(ctx, `
		SELECT id, team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, created_at, updated_at
		FROM goals WHERE id=$1`, id)
	if err := row.Scan(&goal.ID, &goal.TeamID, &goal.PeriodID, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
		return domain.Goal{}, err
	}
	krsSlice, err := r.krs.ListKeyResultsByGoal(ctx, goal.ID)
	if err != nil {
		return domain.Goal{}, err
	}
	goal.KeyResults = krsSlice
	goal.Comments, _ = r.ListGoalComments(ctx, goal.ID)
	return goal, nil
}

func (r *GoalRepository) DeleteGoal(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM goals WHERE id=$1`, id)
	return err
}

func (r *GoalRepository) ListTeamLastGoalUpdateInPeriod(ctx context.Context, periodID int64, teamIDs []int64) (map[int64]time.Time, error) {
	updates := make(map[int64]time.Time, len(teamIDs))
	if len(teamIDs) == 0 {
		return updates, nil
	}
	rows, err := r.db.Query(ctx, `
		WITH team_goals AS (
			SELECT g.id AS goal_id, g.team_id AS team_id
			FROM goals g
			WHERE g.period_id = $1 AND g.team_id = ANY($2)
			UNION
			SELECT g.id AS goal_id, gs.team_id AS team_id
			FROM goals g
			JOIN goal_shares gs ON gs.goal_id = g.id
			WHERE g.period_id = $1 AND gs.team_id = ANY($2)
		),
			goal_updates AS (
				SELECT
					kr.goal_id,
					CASE
						WHEN MAX(kr.progress_updated_at) IS NULL THEN MAX(krn.updated_at)
						WHEN MAX(krn.updated_at) IS NULL THEN MAX(kr.progress_updated_at)
						ELSE GREATEST(MAX(kr.progress_updated_at), MAX(krn.updated_at))
					END AS last_update_at
					FROM key_results kr
				LEFT JOIN key_result_notes krn ON krn.key_result_id = kr.id
				WHERE kr.goal_id IN (SELECT DISTINCT goal_id FROM team_goals)
				GROUP BY kr.goal_id
				HAVING MAX(kr.progress_updated_at) IS NOT NULL OR MAX(krn.updated_at) IS NOT NULL
			)
		SELECT tg.team_id, MAX(gu.last_update_at) AS last_update_at
		FROM team_goals tg
		JOIN goal_updates gu ON gu.goal_id = tg.goal_id
		GROUP BY tg.team_id`, periodID, teamIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var teamID int64
		var updatedAt time.Time
		if err := rows.Scan(&teamID, &updatedAt); err != nil {
			return nil, err
		}
		updates[teamID] = updatedAt
	}
	return updates, rows.Err()
}

func (r *GoalRepository) UpdateGoal(ctx context.Context, input GoalUpdateInput) error {
	_, err := r.db.Exec(ctx, `
		UPDATE goals
		SET title=$1, description=$2, priority=$3, weight=$4, work_type=$5, focus_type=$6, owner_text=$7, updated_at=NOW()
		WHERE id=$8`,
		input.Title, input.Description, input.Priority, input.Weight, input.WorkType, input.FocusType, input.OwnerText, input.ID,
	)
	return err
}

func (r *GoalRepository) UpdateGoalFields(ctx context.Context, input GoalFieldsUpdateInput) error {
	_, err := r.db.Exec(ctx, `
		UPDATE goals
		SET title=$1, description=$2, priority=$3, work_type=$4, focus_type=$5, owner_text=$6, updated_at=NOW()
		WHERE id=$7`,
		input.Title, input.Description, input.Priority, input.WorkType, input.FocusType, input.OwnerText, input.ID,
	)
	return err
}

func (r *GoalRepository) UpdateGoalOwner(ctx context.Context, goalID, teamID int64, weight int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE goals
		SET team_id=$1, weight=$2, updated_at=NOW()
		WHERE id=$3`,
		teamID, weight, goalID,
	)
	return err
}

func (r *GoalRepository) AddGoalComment(ctx context.Context, goalID int64, text string, authorUserID int64) error {
	_, err := r.db.Exec(ctx, `INSERT INTO goal_comments (goal_id, text, author_user_id) VALUES ($1,$2,$3)`, goalID, text, authorUserID)
	return err
}

func (r *GoalRepository) ListGoalComments(ctx context.Context, goalID int64) ([]domain.GoalComment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT gc.id, gc.goal_id, gc.text, u.display_name, u.udid, gc.created_at
		FROM goal_comments gc
		JOIN users u ON u.id = gc.author_user_id
		WHERE gc.goal_id = $1
		ORDER BY gc.created_at DESC`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var comments []domain.GoalComment
	for rows.Next() {
		var c domain.GoalComment
		if err := rows.Scan(&c.ID, &c.GoalID, &c.Text, &c.AuthorName, &c.AuthorUDID, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}
