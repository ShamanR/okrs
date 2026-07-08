package goals

import (
	"context"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/krs"

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
	OwnerUDIDs  []string
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
	OwnerUDIDs  []string
}

type GoalFieldsUpdateInput struct {
	ID          int64
	Title       string
	Description string
	Priority    domain.Priority
	WorkType    domain.WorkType
	FocusType   domain.FocusType
	OwnerText   string
	OwnerUDIDs  []string
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

func (r *GoalRepository) ListGoalsByTeamsPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error) {
	result := make(map[int64][]domain.Goal, len(teamIDs))
	if len(teamIDs) == 0 {
		return result, nil
	}
	teamGoalOrder := make(map[int64][]int64, len(teamIDs))
	teamGoals := make(map[int64]map[int64]*domain.Goal, len(teamIDs))

	goalRows, err := r.db.Query(ctx, `
		SELECT g.id, t.team_id, g.period_id, g.title, g.description, g.priority,
		       COALESCE(gs.weight, g.weight) AS weight,
		       g.work_type, g.focus_type, g.owner_text, g.owner_udids, g.created_at, g.updated_at
		FROM (
			SELECT g.id, g.team_id
			FROM goals g
			WHERE g.period_id = $1 AND g.team_id = ANY($2) AND g.tenant_id = $3
			UNION
			SELECT g.id, gs.team_id
			FROM goals g
			JOIN goal_shares gs ON gs.goal_id = g.id AND gs.tenant_id = $3
			WHERE g.period_id = $1 AND gs.team_id = ANY($2) AND g.tenant_id = $3
		) t
		JOIN goals g ON g.id = t.id AND g.tenant_id = $3
		LEFT JOIN goal_shares gs ON gs.goal_id = g.id AND gs.team_id = t.team_id AND gs.tenant_id = $3
		ORDER BY t.team_id, g.id`, periodID, teamIDs, scope.TenantID)
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
			&goal.OwnerUDIDs,
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
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at, progress_updated_at,
		       start_value, target_value, current_value, unit, checkpoints, zeroing_criteria
		FROM key_results
		WHERE goal_id = ANY($1) AND tenant_id = $2
		ORDER BY goal_id, sort_order, id`, goalIDs, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer krRows.Close()

	krByID := map[int64][]*domain.KeyResult{}
	krIDs := make([]int64, 0)
	krIDSeen := map[int64]struct{}{}
	for krRows.Next() {
		var kr domain.KeyResult
		var startValue, targetValue, currentValue *float64
		var unit, zeroing *string
		var checkpointsRaw []byte
		if err := krRows.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt, &kr.ProgressUpdatedAt,
			&startValue, &targetValue, &currentValue, &unit, &checkpointsRaw, &zeroing); err != nil {
			return nil, err
		}
		if kr.Kind == domain.KRKindNumerical {
			num, err := krs.ParseCheckpoints(checkpointsRaw)
			if err != nil {
				return nil, err
			}
			kr.Numerical = &domain.KRNumerical{Unit: derefString(unit), ZeroingCriteria: derefString(zeroing), Checkpoints: num}
			if startValue != nil {
				kr.Numerical.StartValue = *startValue
			}
			if targetValue != nil {
				kr.Numerical.TargetValue = *targetValue
			}
			if currentValue != nil {
				kr.Numerical.CurrentValue = *currentValue
			}
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

func (r *GoalRepository) CreateGoal(ctx context.Context, scope domain.TenantScope, input GoalInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO goals (team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, owner_udids, sort_order, tenant_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM goals WHERE team_id=$1 AND period_id=$2 AND tenant_id=$11), $11)
		RETURNING id`,
		input.TeamID, input.PeriodID, input.Title, input.Description, input.Priority, input.Weight, input.WorkType, input.FocusType, input.OwnerText, input.OwnerUDIDs, scope.TenantID,
	).Scan(&id)
	return id, err
}

func (r *GoalRepository) ListTeamOverviewStats(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]TeamOverviewStats, error) {
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
			WHERE g.period_id = $1 AND g.team_id = ANY($2) AND g.tenant_id = $3
			UNION
			SELECT g.id, gs.team_id
			FROM goals g
			JOIN goal_shares gs ON gs.goal_id = g.id AND gs.tenant_id = $3
			WHERE g.period_id = $1 AND gs.team_id = ANY($2) AND g.tenant_id = $3
		) t
		JOIN goals g ON g.id = t.id AND g.tenant_id = $3
		GROUP BY t.team_id`, periodID, teamIDs, scope.TenantID)
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

func (r *GoalRepository) ListGoalsByTeamPeriod(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	rows, err := r.db.Query(ctx, `
		SELECT g.id, g.team_id, g.period_id, g.title, g.description, g.priority,
		       COALESCE(gs.weight, g.weight) AS weight,
		       g.work_type, g.focus_type, g.owner_text, g.owner_udids, g.created_at, g.updated_at,
		       COALESCE(gs.sort_order, g.sort_order) AS team_sort_order
		FROM goals g
		LEFT JOIN goal_shares gs ON gs.goal_id = g.id AND gs.team_id = $1 AND gs.tenant_id = $3
		WHERE g.period_id=$2 AND g.tenant_id=$3 AND (g.team_id=$1 OR gs.team_id IS NOT NULL)
		ORDER BY team_sort_order, g.id`, teamID, periodID, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make([]domain.Goal, 0)
	for rows.Next() {
		var goal domain.Goal
		var sortOrder int
		if err := rows.Scan(&goal.ID, &goal.TeamID, &goal.PeriodID, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.OwnerUDIDs, &goal.CreatedAt, &goal.UpdatedAt, &sortOrder); err != nil {
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

	if err := r.loadKRsForGoals(ctx, scope, goals, goalIDs); err != nil {
		return nil, err
	}
	commentsByGoal, err := r.listGoalCommentsBatch(ctx, scope, goalIDs)
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
func (r *GoalRepository) loadKRsForGoals(ctx context.Context, scope domain.TenantScope, goals []domain.Goal, goalIDs []int64) error {
	if len(goalIDs) == 0 {
		return nil
	}

	// Index goals by ID to attach KRs without re-scanning.
	goalByID := make(map[int64]*domain.Goal, len(goals))
	for i := range goals {
		goalByID[goals[i].ID] = &goals[i]
	}

	krRows, err := r.db.Query(ctx, `
		SELECT id, goal_id, title, description, weight, kind, sort_order, created_at, updated_at,
		       start_value, target_value, current_value, unit, checkpoints, zeroing_criteria
		FROM key_results
		WHERE goal_id = ANY($1) AND tenant_id = $2
		ORDER BY goal_id, sort_order, id`, goalIDs, scope.TenantID)
	if err != nil {
		return err
	}
	defer krRows.Close()

	krsByID := make(map[int64]*domain.KeyResult)
	for krRows.Next() {
		var kr domain.KeyResult
		var startValue, targetValue, currentValue *float64
		var unit, zeroing *string
		var checkpointsRaw []byte
		if err := krRows.Scan(&kr.ID, &kr.GoalID, &kr.Title, &kr.Description, &kr.Weight, &kr.Kind, &kr.SortOrder, &kr.CreatedAt, &kr.UpdatedAt,
			&startValue, &targetValue, &currentValue, &unit, &checkpointsRaw, &zeroing); err != nil {
			return err
		}
		if kr.Kind == domain.KRKindNumerical {
			cps, err := krs.ParseCheckpoints(checkpointsRaw)
			if err != nil {
				return err
			}
			kr.Numerical = &domain.KRNumerical{Unit: derefString(unit), ZeroingCriteria: derefString(zeroing), Checkpoints: cps}
			if startValue != nil {
				kr.Numerical.StartValue = *startValue
			}
			if targetValue != nil {
				kr.Numerical.TargetValue = *targetValue
			}
			if currentValue != nil {
				kr.Numerical.CurrentValue = *currentValue
			}
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
	if err := r.loadBooleanMeta(ctx, krIDs, krsByID); err != nil {
		return err
	}
	return r.loadKRNotes(ctx, scope, krIDs, krsByID)
}

// derefString returns the pointed-to string or "" when nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
func (r *GoalRepository) loadKRNotes(ctx context.Context, scope domain.TenantScope, krIDs []int64, krsByID map[int64]*domain.KeyResult) error {
	rows, err := r.db.Query(ctx, `
		SELECT krn.key_result_id, krn.text, u.display_name, u.udid, krn.updated_at
		FROM key_result_notes krn
		JOIN users u ON u.id = krn.author_user_id
		WHERE krn.key_result_id = ANY($1) AND krn.tenant_id = $2`, krIDs, scope.TenantID)
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

func (r *GoalRepository) listGoalCommentsBatch(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]domain.GoalComment, error) {
	if len(goalIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT gc.id, gc.goal_id, gc.text, u.display_name, u.udid, gc.created_at
		FROM goal_comments gc
		JOIN users u ON u.id = gc.author_user_id
		WHERE gc.goal_id = ANY($1) AND gc.tenant_id = $2
		ORDER BY gc.created_at DESC`, goalIDs, scope.TenantID)
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

// MoveGoal shifts goalID one position up (direction < 0) or down (direction >= 0)
// within teamID's ordered view of its period.
//
// A team's view mixes its own goals (ordered by goals.sort_order) with goals
// shared into it (ordered by goal_shares.sort_order for that team), matching
// ListGoalsByTeamPeriod's `COALESCE(gs.sort_order, g.sort_order)` ordering.
// Because shared goals inherit the owner's sort_order on share, effective order
// values can collide across the two sources, so a plain value swap is unstable.
// Instead we renumber the whole visible list after swapping the two neighbours,
// writing goals.sort_order for owned goals and goal_shares.sort_order for shared
// goals — never touching another team's ordering.
func (r *GoalRepository) MoveGoal(ctx context.Context, scope domain.TenantScope, teamID, goalID int64, direction int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Resolve the period from the goal itself; it is the same period the team views.
	var periodID int64
	if err := tx.QueryRow(ctx, `SELECT period_id FROM goals WHERE id=$1 AND tenant_id=$2`, goalID, scope.TenantID).Scan(&periodID); err != nil {
		return err
	}

	// Load the team's visible goals in display order, locking the underlying rows.
	rows, err := tx.Query(ctx, `
		SELECT g.id, gs.team_id IS NOT NULL AS is_shared
		FROM goals g
		LEFT JOIN goal_shares gs ON gs.goal_id = g.id AND gs.team_id = $1 AND gs.tenant_id = $3
		WHERE g.period_id = $2 AND g.tenant_id = $3 AND (g.team_id = $1 OR gs.team_id IS NOT NULL)
		ORDER BY COALESCE(gs.sort_order, g.sort_order), g.id
		FOR UPDATE OF g`, teamID, periodID, scope.TenantID)
	if err != nil {
		return err
	}

	type visibleGoal struct {
		id       int64
		isShared bool
	}
	ordered := make([]visibleGoal, 0)
	index := -1
	for rows.Next() {
		var vg visibleGoal
		if err := rows.Scan(&vg.id, &vg.isShared); err != nil {
			rows.Close()
			return err
		}
		if vg.id == goalID {
			index = len(ordered)
		}
		ordered = append(ordered, vg)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Goal is not part of this team's view: nothing to move.
	if index < 0 {
		return tx.Commit(ctx)
	}

	target := index - 1
	if direction >= 0 {
		target = index + 1
	}
	// Already at an edge: no-op.
	if target < 0 || target >= len(ordered) {
		return tx.Commit(ctx)
	}

	ordered[index], ordered[target] = ordered[target], ordered[index]

	// Renumber the whole list to contiguous sort_order values, writing to the
	// column that backs each goal's position for this team.
	for pos, vg := range ordered {
		if vg.isShared {
			if _, err := tx.Exec(ctx, `UPDATE goal_shares SET sort_order=$1 WHERE goal_id=$2 AND team_id=$3 AND tenant_id=$4`, pos, vg.id, teamID, scope.TenantID); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE goals SET sort_order=$1 WHERE id=$2 AND tenant_id=$3`, pos, vg.id, scope.TenantID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *GoalRepository) GetGoal(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error) {
	var goal domain.Goal
	row := r.db.QueryRow(ctx, `
		SELECT id, team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, owner_udids, created_at, updated_at
		FROM goals WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID)
	if err := row.Scan(&goal.ID, &goal.TeamID, &goal.PeriodID, &goal.Title, &goal.Description, &goal.Priority, &goal.Weight, &goal.WorkType, &goal.FocusType, &goal.OwnerText, &goal.OwnerUDIDs, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
		return domain.Goal{}, err
	}
	krsSlice, err := r.krs.ListKeyResultsByGoal(ctx, scope, goal.ID)
	if err != nil {
		return domain.Goal{}, err
	}
	goal.KeyResults = krsSlice
	goal.Comments, _ = r.ListGoalComments(ctx, scope, goal.ID)
	return goal, nil
}

func (r *GoalRepository) DeleteGoal(ctx context.Context, scope domain.TenantScope, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM goals WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID)
	return err
}

func (r *GoalRepository) ListTeamLastGoalUpdateInPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]time.Time, error) {
	updates := make(map[int64]time.Time, len(teamIDs))
	if len(teamIDs) == 0 {
		return updates, nil
	}
	rows, err := r.db.Query(ctx, `
		WITH team_goals AS (
			SELECT g.id AS goal_id, g.team_id AS team_id
			FROM goals g
			WHERE g.period_id = $1 AND g.team_id = ANY($2) AND g.tenant_id = $3
			UNION
			SELECT g.id AS goal_id, gs.team_id AS team_id
			FROM goals g
			JOIN goal_shares gs ON gs.goal_id = g.id AND gs.tenant_id = $3
			WHERE g.period_id = $1 AND gs.team_id = ANY($2) AND g.tenant_id = $3
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
		GROUP BY tg.team_id`, periodID, teamIDs, scope.TenantID)
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

func (r *GoalRepository) UpdateGoal(ctx context.Context, scope domain.TenantScope, input GoalUpdateInput) error {
	_, err := r.db.Exec(ctx, `
		UPDATE goals
		SET title=$1, description=$2, priority=$3, weight=$4, work_type=$5, focus_type=$6, owner_text=$7, owner_udids=$8, updated_at=NOW()
		WHERE id=$9 AND tenant_id=$10`,
		input.Title, input.Description, input.Priority, input.Weight, input.WorkType, input.FocusType, input.OwnerText, input.OwnerUDIDs, input.ID, scope.TenantID,
	)
	return err
}

func (r *GoalRepository) UpdateGoalFields(ctx context.Context, scope domain.TenantScope, input GoalFieldsUpdateInput) error {
	_, err := r.db.Exec(ctx, `
		UPDATE goals
		SET title=$1, description=$2, priority=$3, work_type=$4, focus_type=$5, owner_text=$6, owner_udids=$7, updated_at=NOW()
		WHERE id=$8 AND tenant_id=$9`,
		input.Title, input.Description, input.Priority, input.WorkType, input.FocusType, input.OwnerText, input.OwnerUDIDs, input.ID, scope.TenantID,
	)
	return err
}

func (r *GoalRepository) UpdateGoalOwner(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE goals
		SET team_id=$1, weight=$2, updated_at=NOW()
		WHERE id=$3 AND tenant_id=$4`,
		teamID, weight, goalID, scope.TenantID,
	)
	return err
}

func (r *GoalRepository) AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error {
	_, err := r.db.Exec(ctx, `INSERT INTO goal_comments (goal_id, text, author_user_id, tenant_id) VALUES ($1,$2,$3,$4)`, goalID, text, authorUserID, scope.TenantID)
	return err
}

func (r *GoalRepository) ListGoalComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT gc.id, gc.goal_id, gc.text, u.display_name, u.udid, gc.created_at
		FROM goal_comments gc
		JOIN users u ON u.id = gc.author_user_id
		WHERE gc.goal_id = $1 AND gc.tenant_id = $2
		ORDER BY gc.created_at DESC`, goalID, scope.TenantID)
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
