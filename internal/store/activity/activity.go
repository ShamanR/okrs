package activity

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ActivityRepository persists the append-only activity_events journal.
type ActivityRepository struct {
	db *pgxpool.Pool
}

func NewActivityRepository(db *pgxpool.Pool) *ActivityRepository {
	return &ActivityRepository{db: db}
}

var ErrNotFound = errors.New("activity event not found")

// Cursor is the keyset pagination position (created_at, id) for List.
type Cursor struct {
	CreatedAt time.Time
	ID        int64
}

// ListFilter holds the optional query filters for List.
type ListFilter struct {
	PeriodID  *int64
	TeamIDs   []int64
	Category  string
	ActorUDID string
	Since     *time.Time
	Query     string
	Limit     int
	Cursor    *Cursor
}

// Record inserts one event and returns its id.
func (r *ActivityRepository) Record(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) (int64, error) {
	payload := ev.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.db.QueryRow(ctx, `
		INSERT INTO activity_events
			(tenant_id, actor_user_id, category, action, team_id, period_id, goal_id, kr_id, comment_id, entity_title, payload_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		scope.TenantID, ev.ActorUserID, ev.Category, ev.Action,
		ev.TeamID, ev.PeriodID, ev.GoalID, ev.KRID, ev.CommentID, ev.EntityTitle, raw,
	).Scan(&id)
	return id, err
}

// RecordBatch inserts many events in a single pipelined round-trip. Empty is a no-op.
func (r *ActivityRepository) RecordBatch(ctx context.Context, scope domain.TenantScope, evs []domain.ActivityEvent) error {
	if len(evs) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for _, ev := range evs {
		payload := ev.Payload
		if payload == nil {
			payload = map[string]any{}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		b.Queue(`
			INSERT INTO activity_events
				(tenant_id, actor_user_id, category, action, team_id, period_id, goal_id, kr_id, comment_id, entity_title, payload_json)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			scope.TenantID, ev.ActorUserID, ev.Category, ev.Action,
			ev.TeamID, ev.PeriodID, ev.GoalID, ev.KRID, ev.CommentID, ev.EntityTitle, raw,
		)
	}
	br := r.db.SendBatch(ctx, b)
	defer br.Close()
	for range evs {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// GetByID returns a single event (no actor join; used by tests and internal lookups).
func (r *ActivityRepository) GetByID(ctx context.Context, scope domain.TenantScope, id int64) (domain.ActivityEvent, error) {
	var ev domain.ActivityEvent
	var raw []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, actor_user_id, category, action, team_id, period_id, goal_id, kr_id, comment_id, entity_title, payload_json, created_at
		FROM activity_events WHERE id=$1 AND tenant_id=$2`, id, scope.TenantID).
		Scan(&ev.ID, &ev.ActorUserID, &ev.Category, &ev.Action, &ev.TeamID, &ev.PeriodID,
			&ev.GoalID, &ev.KRID, &ev.CommentID, &ev.EntityTitle, &raw, &ev.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ActivityEvent{}, ErrNotFound
		}
		return domain.ActivityEvent{}, err
	}
	_ = json.Unmarshal(raw, &ev.Payload)
	return ev, nil
}

// audiencePredicate returns SQL matching events whose audience (owner team OR any shared team)
// intersects the team ids bound to placeholder p.
func audiencePredicate(p string) string {
	return "(e.team_id = ANY(" + p + ") OR EXISTS (SELECT 1 FROM goal_shares gs WHERE gs.goal_id = e.goal_id AND gs.team_id = ANY(" + p + ")))"
}

// filterWhere builds the shared WHERE predicates for List/CategoryCounts against the aliases
// e (activity_events) and u (users). includeCategory=false omits the category filter (used for
// per-category totals that must not change with the selected tab); includeCursor=false omits
// keyset pagination.
func filterWhere(scope domain.TenantScope, allowedTeamIDs []int64, f ListFilter, arg func(any) string, includeCategory, includeCursor bool) []string {
	var where []string
	where = append(where, "e.tenant_id = "+arg(scope.TenantID))
	if allowedTeamIDs != nil { // nil = admin/unrestricted
		where = append(where, audiencePredicate(arg(allowedTeamIDs)))
	}
	if f.PeriodID != nil {
		where = append(where, "e.period_id = "+arg(*f.PeriodID))
	}
	if len(f.TeamIDs) > 0 {
		where = append(where, audiencePredicate(arg(f.TeamIDs)))
	}
	if includeCategory && f.Category != "" {
		where = append(where, "e.category = "+arg(f.Category))
	}
	if f.ActorUDID != "" {
		where = append(where, "e.actor_user_id = (SELECT id FROM users WHERE udid::text = "+arg(f.ActorUDID)+")")
	}
	if f.Since != nil {
		where = append(where, "e.created_at >= "+arg(*f.Since))
	}
	if f.Query != "" {
		q := arg("%" + strings.ToLower(f.Query) + "%")
		where = append(where, "(LOWER(e.entity_title) LIKE "+q+" OR LOWER(e.payload_json::text) LIKE "+q+" OR LOWER(COALESCE(u.display_name,'')) LIKE "+q+")")
	}
	if includeCursor && f.Cursor != nil {
		where = append(where, "(e.created_at, e.id) < ("+arg(f.Cursor.CreatedAt)+", "+arg(f.Cursor.ID)+")")
	}
	return where
}

// CategoryCounts returns event counts per category for the given filters, EXCLUDING the category
// filter itself, so the feed's tab counters stay stable regardless of which tab is active.
func (r *ActivityRepository) CategoryCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f ListFilter) (map[string]int, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	where := filterWhere(scope, allowedTeamIDs, f, arg, false, false)
	sql := `
		SELECT e.category, count(*)
		FROM activity_events e
		LEFT JOIN users u ON u.id = e.actor_user_id
		WHERE ` + strings.Join(where, " AND ") + `
		GROUP BY e.category`
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var cat string
		var n int
		if err := rows.Scan(&cat, &n); err != nil {
			return nil, err
		}
		counts[cat] = n
	}
	return counts, rows.Err()
}

// List returns a page of events (newest first) for the tenant, filtered and scoped.
// allowedTeamIDs == nil means admin/unrestricted; a non-nil slice (incl. empty) restricts
// visibility to events whose audience intersects it. Removed actors have blanked PII.
func (r *ActivityRepository) List(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f ListFilter) ([]domain.ActivityEvent, *Cursor, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	where := filterWhere(scope, allowedTeamIDs, f, arg, true, true)

	// Navigation target team: the owner team if the viewer can access it, else a shared team the
	// viewer can access (a viewer may see an event via goal_shares without owner-team access).
	// For admin (allowedTeamIDs == nil) the owner team is always fine.
	targetTeamExpr := "e.team_id"
	if allowedTeamIDs != nil {
		p := arg(allowedTeamIDs)
		targetTeamExpr = "CASE WHEN e.team_id = ANY(" + p + ") THEN e.team_id " +
			"ELSE (SELECT gs.team_id FROM goal_shares gs WHERE gs.goal_id = e.goal_id AND gs.team_id = ANY(" + p + ") ORDER BY gs.team_id LIMIT 1) END"
	}

	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	limPlaceholder := arg(limit + 1) // one extra row => there is a next page

	sql := `
		SELECT e.id, e.actor_user_id, e.category, e.action, e.team_id, e.period_id, e.goal_id, e.kr_id, e.comment_id,
		       e.entity_title, e.payload_json, e.created_at,
		       COALESCE(u.udid::text,''), COALESCE(u.display_name,''), COALESCE(u.avatar_url,''), COALESCE(u.provider,''),
		       EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = e.actor_user_id AND m.tenant_id = e.tenant_id AND m.status = 'active'),
		       ` + targetTeamExpr + ` AS target_team_id
		FROM activity_events e
		LEFT JOIN users u ON u.id = e.actor_user_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT ` + limPlaceholder

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []domain.ActivityEvent
	for rows.Next() {
		var ev domain.ActivityEvent
		var raw []byte
		var provider string
		var active bool
		if err := rows.Scan(&ev.ID, &ev.ActorUserID, &ev.Category, &ev.Action, &ev.TeamID, &ev.PeriodID,
			&ev.GoalID, &ev.KRID, &ev.CommentID, &ev.EntityTitle, &raw, &ev.CreatedAt,
			&ev.ActorUDID, &ev.ActorDisplayName, &ev.ActorAvatarURL, &provider, &active, &ev.TargetTeamID); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal(raw, &ev.Payload)
		// A former member (non-system, not currently active) must not leak PII.
		if provider != "system" && !active {
			ev.ActorRemoved = true
			ev.ActorDisplayName = ""
			ev.ActorAvatarURL = ""
			ev.ActorUDID = ""
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *Cursor
	if len(out) > limit {
		last := out[limit-1]
		next = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		out = out[:limit]
	}
	return out, next, nil
}

// TreeCounts returns direct per-team event counts (audience-expanded: owner team + each shared
// team). allowedTeamIDs == nil = admin (all teams). The caller rolls these up over the subtree.
func (r *ActivityRepository) TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error) {
	var filter []string
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }

	filter = append(filter, "e.tenant_id = "+arg(scope.TenantID))
	if periodID != nil {
		filter = append(filter, "e.period_id = "+arg(*periodID))
	}
	if since != nil {
		filter = append(filter, "e.created_at >= "+arg(*since))
	}
	base := strings.Join(filter, " AND ")

	sql := `
		SELECT team_id, count(*) FROM (
			SELECT e.id, e.team_id AS team_id FROM activity_events e WHERE ` + base + ` AND e.team_id IS NOT NULL
			UNION ALL
			SELECT e.id, gs.team_id FROM activity_events e JOIN goal_shares gs ON gs.goal_id = e.goal_id WHERE ` + base + `
		) x`
	if allowedTeamIDs != nil {
		sql += " WHERE team_id = ANY(" + arg(allowedTeamIDs) + ")"
	}
	sql += " GROUP BY team_id"

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int64]int)
	for rows.Next() {
		var teamID int64
		var n int
		if err := rows.Scan(&teamID, &n); err != nil {
			return nil, err
		}
		counts[teamID] = n
	}
	return counts, rows.Err()
}

// Purge deletes journal rows for the tenant. olderThan == nil deletes all; returns rows deleted.
func (r *ActivityRepository) Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error) {
	var tag pgconn.CommandTag
	var err error
	if olderThan == nil {
		tag, err = r.db.Exec(ctx, `DELETE FROM activity_events WHERE tenant_id=$1`, scope.TenantID)
	} else {
		tag, err = r.db.Exec(ctx, `DELETE FROM activity_events WHERE tenant_id=$1 AND created_at < $2`, scope.TenantID, *olderThan)
	}
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
