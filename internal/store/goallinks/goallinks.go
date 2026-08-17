// Package goallinks persists goal↔goal parent/child links (the goal_links table).
// Links form a directed acyclic graph: an edge means "child_goal_id → parent_goal_id".
// Cycle prevention lives here (see ReplaceParents); the graph stays acyclic so any
// future traversal is bounded.
package goallinks

import (
	"context"
	"errors"
	"strings"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrCycle reports that the requested parent set would close a cycle in the link graph
// (or that a goal links to itself).
var ErrCycle = errors.New("goallinks: cycle")

// linkMutationLockClass namespaces the per-tenant advisory lock that serializes goal-link
// mutations (see ReplaceParents). The two-int advisory-lock space is disjoint from the
// single-bigint space used elsewhere (progress snapshots), so keys never collide.
const linkMutationLockClass = 0x474c4e4b // 'GLNK'

// GoalLinkRepository persists goal_links rows.
type GoalLinkRepository struct {
	db *pgxpool.Pool
}

func NewGoalLinkRepository(db *pgxpool.Pool) *GoalLinkRepository {
	return &GoalLinkRepository{db: db}
}

// LinkableGoal is a goal candidate for the parent picker: a GoalRef plus the owner
// team's lead (for search/display).
type LinkableGoal struct {
	domain.GoalRef
	Lead string
}

// ReplaceParents atomically replaces the **caller-visible** set of parents of childID.
//
// Reads are scope-filtered, so the caller only ever sees (and sends) parents whose owner team
// is in their scope. To avoid silently deleting parent links the caller cannot see, the replace
// touches only the visible subset: it deletes only child→parent edges whose parent team is in
// allowedTeamIDs (adminAll → all), then inserts the new set (all validated in-scope by the
// service). Parents in teams outside the caller's scope are left untouched.
//
// A cycle can only be introduced by a NEW edge C→Pi; removing edges never creates one. So we
// check, in a single recursive CTE, whether childID is reachable upward (via parent edges) from
// the proposed set {Pi} in the graph that EXCLUDES C's own outgoing edges. If childID is an
// ancestor of any Pi — or a Pi equals childID — the set would close a cycle and ErrCycle is
// returned. No per-edge query loop: one CTE per operation, one INSERT..SELECT unnest.
func (r *GoalLinkRepository) ReplaceParents(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64) (added, removed []int64, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize goal-link mutations within a tenant. Under READ COMMITTED two concurrent
	// requests adding opposite edges (A→B and B→A) would each run the cycle check against the
	// pre-write graph, both pass, then insert distinct rows and both commit — closing a cycle.
	// A transaction-scoped advisory lock keyed by tenant makes check-then-insert atomic per
	// tenant; it releases automatically on commit/rollback. Link mutations are rare (a user
	// editing a goal's parents), so per-tenant serialization is cheap.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1::int, $2::int)`, linkMutationLockClass, scope.TenantID); err != nil {
		return nil, nil, err
	}

	// Dedup and reject self-links up front (parent membership/scope is validated in service).
	seen := make(map[int64]bool, len(parentIDs))
	uniq := make([]int64, 0, len(parentIDs))
	for _, p := range parentIDs {
		if p == childID {
			return nil, nil, ErrCycle // self-link
		}
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}

	// Current VISIBLE parent set (parent team in scope) — for the diff and the scoped delete.
	before := make(map[int64]bool)
	rows, err := tx.Query(ctx, `
		SELECT gl.parent_goal_id
		FROM goal_links gl
		JOIN goals pg ON pg.id = gl.parent_goal_id AND pg.tenant_id = gl.tenant_id
		WHERE gl.tenant_id = $1 AND gl.child_goal_id = $2 AND ($3 OR pg.team_id = ANY($4))`,
		scope.TenantID, childID, adminAll, allowedTeamIDs)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var p int64
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, nil, err
		}
		before[p] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Cycle check: is childID reachable as an ancestor of the proposed parent set, in the
	// graph without childID's outgoing edges? A path array carried through the recursion
	// (`NOT parent = ANY(path)`) bounds it even if the data ever contains a cycle — the
	// write path keeps the graph acyclic, so this is a defensive guard (works on PG 11,
	// which lacks the SQL-standard CYCLE clause).
	if len(uniq) > 0 {
		var childHit int
		err = tx.QueryRow(ctx, `
			WITH RECURSIVE anc(goal_id, path) AS (
				SELECT gl.parent_goal_id, ARRAY[gl.child_goal_id, gl.parent_goal_id]
				FROM goal_links gl
				WHERE gl.tenant_id = $1 AND gl.child_goal_id = ANY($2) AND gl.child_goal_id <> $3
				UNION ALL
				SELECT gl.parent_goal_id, anc.path || gl.parent_goal_id
				FROM goal_links gl
				JOIN anc ON gl.child_goal_id = anc.goal_id
				WHERE gl.tenant_id = $1 AND gl.child_goal_id <> $3
				  AND NOT gl.parent_goal_id = ANY(anc.path)
			)
			SELECT count(*) FROM anc WHERE goal_id = $3`,
			scope.TenantID, uniq, childID).Scan(&childHit)
		if err != nil {
			return nil, nil, err
		}
		if childHit > 0 {
			return nil, nil, ErrCycle
		}
	}

	// Delete only the VISIBLE subset of childID's outgoing edges (parent team in scope),
	// preserving links to parents the caller cannot see.
	if _, err := tx.Exec(ctx, `
		DELETE FROM goal_links gl
		USING goals pg
		WHERE gl.tenant_id = $1 AND gl.child_goal_id = $2
		  AND pg.id = gl.parent_goal_id AND pg.tenant_id = gl.tenant_id
		  AND ($3 OR pg.team_id = ANY($4))`,
		scope.TenantID, childID, adminAll, allowedTeamIDs); err != nil {
		return nil, nil, err
	}
	// Insert the new set. All entries are in-scope (validated by the service) and disjoint from
	// preserved out-of-scope links; ON CONFLICT DO NOTHING guards against races/duplicates.
	if len(uniq) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO goal_links (tenant_id, child_goal_id, parent_goal_id)
			SELECT $1, $2, unnest($3::bigint[])
			ON CONFLICT DO NOTHING`, scope.TenantID, childID, uniq); err != nil {
			return nil, nil, err
		}
	}

	for _, p := range uniq {
		if !before[p] {
			added = append(added, p)
		}
	}
	for p := range before {
		if !seen[p] {
			removed = append(removed, p)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return added, removed, nil
}

// ListLinksForGoals returns, for each goalID, its parent and child goal summaries, filtered
// to teams the viewer can access (adminAll=true → all teams of the tenant). A linked goal is
// visible only if its owner team (goals.team_id) is in the allowed set. Progress is left at 0
// here; the service layer fills it (goals has no stored progress column).
func (r *GoalLinkRepository) ListLinksForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (parents, children map[int64][]domain.GoalRef, err error) {
	parents = make(map[int64][]domain.GoalRef)
	children = make(map[int64][]domain.GoalRef)
	if len(goalIDs) == 0 {
		return parents, children, nil
	}

	// Parents: for each goal in goalIDs, its parent goals.
	pr, err := r.db.Query(ctx, `
		SELECT gl.child_goal_id, pg.id, pg.title, pg.period_id, pp.name, pg.team_id, pt.name, pt.team_type
		FROM goal_links gl
		JOIN goals   pg ON pg.id = gl.parent_goal_id AND pg.tenant_id = gl.tenant_id
		JOIN teams   pt ON pt.id = pg.team_id
		JOIN periods pp ON pp.id = pg.period_id
		WHERE gl.tenant_id = $1 AND gl.child_goal_id = ANY($2)
		  AND ($3 OR pg.team_id = ANY($4))
		ORDER BY pt.team_type, pt.name, pg.sort_order, pg.id`,
		scope.TenantID, goalIDs, adminAll, allowedTeamIDs)
	if err != nil {
		return nil, nil, err
	}
	if err := scanRefs(pr, parents); err != nil {
		return nil, nil, err
	}

	// Children: for each goal in goalIDs, its child goals.
	cr, err := r.db.Query(ctx, `
		SELECT gl.parent_goal_id, cg.id, cg.title, cg.period_id, cp.name, cg.team_id, ct.name, ct.team_type
		FROM goal_links gl
		JOIN goals   cg ON cg.id = gl.child_goal_id AND cg.tenant_id = gl.tenant_id
		JOIN teams   ct ON ct.id = cg.team_id
		JOIN periods cp ON cp.id = cg.period_id
		WHERE gl.tenant_id = $1 AND gl.parent_goal_id = ANY($2)
		  AND ($3 OR cg.team_id = ANY($4))
		ORDER BY ct.team_type, ct.name, cg.sort_order, cg.id`,
		scope.TenantID, goalIDs, adminAll, allowedTeamIDs)
	if err != nil {
		return nil, nil, err
	}
	if err := scanRefs(cr, children); err != nil {
		return nil, nil, err
	}
	return parents, children, nil
}

// ListLinkable returns candidate goals for the parent picker: goals of accessible teams in
// the tenant, optionally filtered by period, excluding excludeGoalID, matching q against goal
// title / team name / team lead (case-insensitive). Ordered by team hierarchy (type, name)
// then goal sort order.
func (r *GoalLinkRepository) ListLinkable(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]LinkableGoal, error) {
	trimmed := strings.TrimSpace(q)
	like := "%" + strings.ToLower(trimmed) + "%"
	rows, err := r.db.Query(ctx, `
		SELECT g.id, g.title, g.period_id, p.name, g.team_id, t.name, t.team_type, COALESCE(t.lead,'')
		FROM goals g
		JOIN teams   t ON t.id = g.team_id
		JOIN periods p ON p.id = g.period_id
		WHERE g.tenant_id = $1
		  AND g.id <> $2
		  AND ($3 OR g.team_id = ANY($4))
		  AND ($5::bigint IS NULL OR g.period_id = $5)
		  AND ($6 = '' OR lower(g.title) LIKE $7 OR lower(t.name) LIKE $7 OR lower(COALESCE(t.lead,'')) LIKE $7)
		ORDER BY t.team_type, t.name, g.sort_order, g.id`,
		scope.TenantID, excludeGoalID, adminAll, allowedTeamIDs, periodID, trimmed, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LinkableGoal, 0)
	for rows.Next() {
		var lg LinkableGoal
		if err := rows.Scan(&lg.ID, &lg.Title, &lg.PeriodID, &lg.PeriodName, &lg.TeamID, &lg.TeamName, &lg.TeamType, &lg.Lead); err != nil {
			return nil, err
		}
		out = append(out, lg)
	}
	return out, rows.Err()
}

// scanRefs reads (key, GoalRef) rows and appends each ref under its key. Progress stays 0.
func scanRefs(rows pgx.Rows, dst map[int64][]domain.GoalRef) error {
	defer rows.Close()
	for rows.Next() {
		var key int64
		var ref domain.GoalRef
		if err := rows.Scan(&key, &ref.ID, &ref.Title, &ref.PeriodID, &ref.PeriodName, &ref.TeamID, &ref.TeamName, &ref.TeamType); err != nil {
			return err
		}
		dst[key] = append(dst[key], ref)
	}
	return rows.Err()
}
