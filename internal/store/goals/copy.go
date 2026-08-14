package goals

import (
	"context"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
)

// CopyGoalInput describes a deep-copy of a source goal into a target (team, period).
type CopyGoalInput struct {
	SourceGoalID   int64
	TargetTeamID   int64
	TargetPeriodID int64
	WithProgress   bool // carry KR progress (current_value / is_done / health_status) and KR notes
	WithComments   bool // carry goal comments (tasks + replies), authors and resolve state preserved
}

// CopyGoal deep-copies a goal (goal fields, all KRs with their meta, optionally KR
// notes/progress and goal comments) into the target team/period within one transaction.
// Shares are never copied. Returns the new goal id.
func (r *GoalRepository) CopyGoal(ctx context.Context, scope domain.TenantScope, in CopyGoalInput) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// 1) New goal row, sort_order appended to the target board.
	var newGoalID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO goals (team_id, period_id, title, description, priority, weight, work_type, focus_type, owner_text, owner_udids, sort_order, tenant_id)
		SELECT $1, $2, g.title, g.description, g.priority, g.weight, g.work_type, g.focus_type, g.owner_text, g.owner_udids,
		       (SELECT COALESCE(MAX(sort_order),0)+1 FROM goals WHERE team_id=$1 AND period_id=$2 AND tenant_id=$4),
		       $4
		FROM goals g WHERE g.id=$3 AND g.tenant_id=$4
		RETURNING id`,
		in.TargetTeamID, in.TargetPeriodID, in.SourceGoalID, scope.TenantID,
	).Scan(&newGoalID); err != nil {
		return 0, err
	}

	// 2) Copy KRs (ordered), each with its meta.
	rows, err := tx.Query(ctx, `
		SELECT id, title, description, weight, kind, sort_order, zeroing_criteria, health_status,
		       start_value, target_value, current_value, unit, checkpoints
		FROM key_results WHERE goal_id=$1 AND tenant_id=$2 ORDER BY sort_order, id`,
		in.SourceGoalID, scope.TenantID)
	if err != nil {
		return 0, err
	}
	type srcKR struct {
		id                     int64
		title, description     string
		weight, sortOrder      int
		kind                   string
		zeroing                *string
		health                 string
		start, target, current *float64
		unit                   *string
		checkpoints            []byte
	}
	var srcKRs []srcKR
	for rows.Next() {
		var k srcKR
		if err := rows.Scan(&k.id, &k.title, &k.description, &k.weight, &k.kind, &k.sortOrder, &k.zeroing, &k.health,
			&k.start, &k.target, &k.current, &k.unit, &k.checkpoints); err != nil {
			rows.Close()
			return 0, err
		}
		srcKRs = append(srcKRs, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, k := range srcKRs {
		health := k.health
		if !in.WithProgress {
			health = string(domain.KRHealthNotStarted)
		}
		var newKRID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO key_results (goal_id, title, description, zeroing_criteria, weight, kind, sort_order, health_status, tenant_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			newGoalID, k.title, k.description, k.zeroing, k.weight, k.kind, k.sortOrder, health, scope.TenantID,
		).Scan(&newKRID); err != nil {
			return 0, err
		}

		switch domain.KRKind(k.kind) {
		case domain.KRKindNumerical:
			current := 0.0
			if k.start != nil {
				current = *k.start // reset → start
			}
			if in.WithProgress && k.current != nil {
				current = *k.current
			}
			if _, err := tx.Exec(ctx, `
				UPDATE key_results SET start_value=$1, target_value=$2, current_value=$3, unit=$4, checkpoints=$5
				WHERE id=$6 AND tenant_id=$7`,
				k.start, k.target, current, k.unit, k.checkpoints, newKRID, scope.TenantID); err != nil {
				return 0, err
			}
		case domain.KRKindBoolean:
			var done bool
			if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT is_done FROM kr_boolean_meta WHERE key_result_id=$1), false)`, k.id).Scan(&done); err != nil {
				return 0, err
			}
			if !in.WithProgress {
				done = false
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO kr_boolean_meta (key_result_id, is_done) VALUES ($1,$2)
				ON CONFLICT (key_result_id) DO UPDATE SET is_done=EXCLUDED.is_done`, newKRID, done); err != nil {
				return 0, err
			}
		case domain.KRKindProject:
			stageRows, err := tx.Query(ctx, `
				SELECT title, weight, is_done, sort_order FROM kr_project_stages WHERE key_result_id=$1 ORDER BY sort_order`, k.id)
			if err != nil {
				return 0, err
			}
			type stg struct {
				title  string
				weight int
				done   bool
				order  int
			}
			var stages []stg
			for stageRows.Next() {
				var s stg
				if err := stageRows.Scan(&s.title, &s.weight, &s.done, &s.order); err != nil {
					stageRows.Close()
					return 0, err
				}
				stages = append(stages, s)
			}
			stageRows.Close()
			if err := stageRows.Err(); err != nil {
				return 0, err
			}
			for _, s := range stages {
				done := s.done
				if !in.WithProgress {
					done = false
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO kr_project_stages (key_result_id, title, weight, is_done, sort_order)
					VALUES ($1,$2,$3,$4,$5)`, newKRID, s.title, s.weight, done, s.order); err != nil {
					return 0, err
				}
			}
		}

		// KR note travels with progress.
		if in.WithProgress {
			if _, err := tx.Exec(ctx, `
				INSERT INTO key_result_notes (key_result_id, text, author_user_id, updated_at, tenant_id)
				SELECT $1, n.text, n.author_user_id, n.updated_at, $3
				FROM key_result_notes n WHERE n.key_result_id=$2 AND n.tenant_id=$3`,
				newKRID, k.id, scope.TenantID); err != nil {
				return 0, err
			}
		}
	}

	// 3) Optionally copy comments (tasks first, then replies with remapped parent_id).
	if in.WithComments {
		if err := copyGoalComments(ctx, tx, scope, in.SourceGoalID, newGoalID); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return newGoalID, nil
}

// copyGoalComments duplicates a goal's tasks (parent_id IS NULL) and their replies into
// dstGoalID, preserving author and resolve state and remapping each reply's parent_id.
func copyGoalComments(ctx context.Context, tx pgx.Tx, scope domain.TenantScope, srcGoalID, dstGoalID int64) error {
	rows, err := tx.Query(ctx, `
		SELECT id, text, author_user_id, created_at, resolved_at, resolved_by_user_id
		FROM goal_comments WHERE goal_id=$1 AND tenant_id=$2 AND parent_id IS NULL
		ORDER BY created_at, id`, srcGoalID, scope.TenantID)
	if err != nil {
		return err
	}
	type task struct {
		id         int64
		text       string
		author     int64
		createdAt  time.Time
		resolvedAt *time.Time
		resolvedBy *int64
	}
	var tasks []task
	for rows.Next() {
		var t task
		if err := rows.Scan(&t.id, &t.text, &t.author, &t.createdAt, &t.resolvedAt, &t.resolvedBy); err != nil {
			rows.Close()
			return err
		}
		tasks = append(tasks, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	idMap := make(map[int64]int64, len(tasks))
	for _, t := range tasks {
		var newID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO goal_comments (goal_id, parent_id, text, author_user_id, created_at, resolved_at, resolved_by_user_id, tenant_id)
			VALUES ($1, NULL, $2, $3, $4, $5, $6, $7) RETURNING id`,
			dstGoalID, t.text, t.author, t.createdAt, t.resolvedAt, t.resolvedBy, scope.TenantID).Scan(&newID); err != nil {
			return err
		}
		idMap[t.id] = newID
	}
	replyRows, err := tx.Query(ctx, `
		SELECT parent_id, text, author_user_id, created_at
		FROM goal_comments WHERE goal_id=$1 AND tenant_id=$2 AND parent_id IS NOT NULL
		ORDER BY created_at, id`, srcGoalID, scope.TenantID)
	if err != nil {
		return err
	}
	type reply struct {
		parent    int64
		text      string
		author    int64
		createdAt time.Time
	}
	var replies []reply
	for replyRows.Next() {
		var rp reply
		if err := replyRows.Scan(&rp.parent, &rp.text, &rp.author, &rp.createdAt); err != nil {
			replyRows.Close()
			return err
		}
		replies = append(replies, rp)
	}
	replyRows.Close()
	if err := replyRows.Err(); err != nil {
		return err
	}
	for _, rp := range replies {
		newParent, ok := idMap[rp.parent]
		if !ok {
			continue // orphan guard; single-level depth guarantees a parent task exists
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO goal_comments (goal_id, parent_id, text, author_user_id, created_at, tenant_id)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			dstGoalID, newParent, rp.text, rp.author, rp.createdAt, scope.TenantID); err != nil {
			return err
		}
	}
	return nil
}
