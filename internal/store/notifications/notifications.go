// Package notifications persists per-recipient notifications. One row is both the
// in-app bell entry and the anchor external deliveries will hang off in phase 2.
package notifications

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"okrs/internal/core/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Notification is one delivered notification.
type Notification struct {
	ID            int64
	Type          string
	Kind          string
	ActorUserID   int64
	TeamID        *int64
	PeriodID      *int64
	GoalID        *int64
	KRID          *int64
	CommentID     *int64
	EntityTitle   string
	Payload       map[string]any
	CoalesceCount int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ReadAt        *time.Time

	// Actor is resolved on read, in the same query — a second lookup per row would
	// be N+1, and the renderer needs the name for every notification.
	ActorDisplayName string
	ActorAvatarURL   string
	// ActorRemoved marks an actor who is no longer an active member of the tenant.
	// Same PII rule as the activity journal: name and avatar of a former member are
	// not exposed, only a neutral placeholder.
	ActorRemoved bool

	// TeamPath is the notification's team with its ancestors, root first
	// ("Компания / Платформа / Биллинг"), and GoalTitle is the goal it happened on.
	// Both are resolved on read in the same query as the actor — a lookup per row
	// would be N+1 on a page of up to 100 — so a renamed team or goal shows its
	// current name, which is what makes the list scannable. Empty when the
	// notification carries no team, or the goal has since been deleted.
	TeamPath  string
	GoalTitle string
}

// InsertInput is one notification to store. CoalesceKey is built by the caller
// (usecase/notification) and encodes type:entity:actor:time-bucket.
type InsertInput struct {
	UserID      int64
	Type        string
	Kind        string
	ActorUserID int64
	TeamID      *int64
	PeriodID    *int64
	GoalID      *int64
	KRID        *int64
	CommentID   *int64
	EntityTitle string
	Payload     map[string]any
	CoalesceKey string
}

// Cursor is the keyset pagination position, mirroring store/activity.
type Cursor struct {
	CreatedAt time.Time
	ID        int64
}

type ListFilter struct {
	UnreadOnly bool
	Limit      int
	Cursor     *Cursor
}

const insertSQL = `
INSERT INTO notifications
  (tenant_id, user_id, type, kind, actor_user_id, team_id, period_id, goal_id, kr_id,
   comment_id, entity_title, payload_json, coalesce_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (tenant_id, user_id, coalesce_key) DO UPDATE
   SET coalesce_count = notifications.coalesce_count + 1,
       updated_at     = now(),
       payload_json   = EXCLUDED.payload_json,
       -- kind and entity_title are deliberately NOT refreshed here: a coalesced row
       -- keeps rendering with the first event's wording (kind, entity_title) even
       -- though payload_json now holds the latest event's data. Cheap trade, not an
       -- oversight — the alternative (refresh everything) would need kind and
       -- entity_title to be compatible across every event a coalesce key can group,
       -- which they are not guaranteed to be, so the first occurrence wins.
       -- A repeat inside the window is new information: light the badge again.
       read_at        = NULL
RETURNING (xmax = 0) AS inserted`

// Insert stores one notification, coalescing into an existing row when the key
// matches. Returns whether a new row was created (false = coalesced).
//
// One atomic statement, so concurrent replicas cannot duplicate a row: the unique
// index arbitrates instead of a read-then-write race.
func (r *Repository) Insert(ctx context.Context, scope domain.TenantScope, in InsertInput) (bool, error) {
	raw, err := marshalPayload(in.Payload)
	if err != nil {
		return false, err
	}
	var inserted bool
	err = r.db.QueryRow(ctx, insertSQL,
		scope.TenantID, in.UserID, in.Type, in.Kind, in.ActorUserID,
		in.TeamID, in.PeriodID, in.GoalID, in.KRID, in.CommentID,
		in.EntityTitle, raw, in.CoalesceKey,
	).Scan(&inserted)
	return inserted, err
}

// InsertBatch stores many notifications in a single pipelined round-trip.
// Батчевая операция: не превращать в цикл Insert — это N+1 на горячем пути fan-out.
func (r *Repository) InsertBatch(ctx context.Context, scope domain.TenantScope, ins []InsertInput) error {
	if len(ins) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for _, in := range ins {
		raw, err := marshalPayload(in.Payload)
		if err != nil {
			return err
		}
		b.Queue(insertSQL,
			scope.TenantID, in.UserID, in.Type, in.Kind, in.ActorUserID,
			in.TeamID, in.PeriodID, in.GoalID, in.KRID, in.CommentID,
			in.EntityTitle, raw, in.CoalesceKey)
	}
	br := r.db.SendBatch(ctx, b)
	defer br.Close()
	for range ins {
		var inserted bool
		if err := br.QueryRow().Scan(&inserted); err != nil {
			return err
		}
	}
	return nil
}

func marshalPayload(p map[string]any) ([]byte, error) {
	if p == nil {
		p = map[string]any{}
	}
	return json.Marshal(p)
}

// List returns a page of the recipient's notifications, newest first.
func (r *Repository) List(ctx context.Context, scope domain.TenantScope, userID int64, f ListFilter) ([]Notification, *Cursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	// arg() mirrors store/activity's placeholder builder: numbering follows insertion
	// order automatically, so an added filter can never desync the $N literals below.
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }

	// tenantArg is captured once and reused: the same placeholder scopes the page,
	// every step of the ancestor walk and the goal join. Re-adding the value would
	// only grow args without changing the query.
	tenantArg := arg(scope.TenantID)
	userArg := arg(userID)

	// The actor is joined here rather than looked up per row: one query, no N+1.
	// A former member (no active membership, and not a system user) is returned as a
	// neutral placeholder — the journal applies the same PII rule.
	q := `WITH RECURSIVE page AS (
	        SELECT n.id, n.type, n.kind, n.actor_user_id, n.team_id, n.period_id, n.goal_id,
	               n.kr_id, n.comment_id, n.entity_title, n.payload_json, n.coalesce_count,
	               n.created_at, n.updated_at, n.read_at,
	               CASE WHEN m.user_id IS NULL AND u.provider <> 'system'
	                    THEN '' ELSE u.display_name END AS actor_display_name,
	               CASE WHEN m.user_id IS NULL AND u.provider <> 'system'
	                    THEN '' ELSE COALESCE(u.avatar_url, '') END AS actor_avatar_url,
	               (m.user_id IS NULL AND u.provider <> 'system') AS actor_removed
	          FROM notifications n
	          JOIN users u ON u.id = n.actor_user_id
	          LEFT JOIN memberships m
	                 ON m.user_id = u.id AND m.tenant_id = n.tenant_id AND m.status = 'active'
	         WHERE n.tenant_id = ` + tenantArg + ` AND n.user_id = ` + userArg
	if f.UnreadOnly {
		q += ` AND n.read_at IS NULL`
	}
	if f.Cursor != nil {
		q += ` AND (n.created_at, n.id) < (` + arg(f.Cursor.CreatedAt) + `, ` + arg(f.Cursor.ID) + `)`
	}
	// +1 запрашивается, чтобы узнать, есть ли следующая страница
	q += ` ORDER BY n.created_at DESC, n.id DESC LIMIT ` + arg(limit+1) + `
	      ),
	      -- Ancestors are walked only for the teams this page actually references,
	      -- not for the whole tenant tree. Every step re-checks tenant_id: parent_id
	      -- alone would walk into another tenant's team if the column were ever
	      -- corrupted, and the path is shown to the user. depth caps the walk so a
	      -- parent cycle degrades into a truncated path instead of hanging the query.
	      ancestors AS (
	        SELECT t.id AS leaf, t.id, t.name, t.parent_id, 0 AS depth
	          FROM teams t
	         WHERE t.tenant_id = ` + tenantArg + `
	           AND t.id IN (SELECT team_id FROM page WHERE team_id IS NOT NULL)
	        UNION ALL
	        SELECT a.leaf, p.id, p.name, p.parent_id, a.depth + 1
	          FROM teams p
	          JOIN ancestors a ON p.id = a.parent_id
	         WHERE p.tenant_id = ` + tenantArg + ` AND a.depth < 32
	      ),
	      team_path AS (
	        SELECT leaf, string_agg(name, ' / ' ORDER BY depth DESC) AS path
	          FROM ancestors GROUP BY leaf
	      )
	      SELECT page.id, page.type, page.kind, page.actor_user_id, page.team_id,
	             page.period_id, page.goal_id, page.kr_id, page.comment_id,
	             page.entity_title, page.payload_json, page.coalesce_count,
	             page.created_at, page.updated_at, page.read_at,
	             page.actor_display_name, page.actor_avatar_url, page.actor_removed,
	             COALESCE(tp.path, ''), COALESCE(g.title, '')
	        FROM page
	        LEFT JOIN team_path tp ON tp.leaf = page.team_id
	        LEFT JOIN goals g ON g.id = page.goal_id AND g.tenant_id = ` + tenantArg + `
	       ORDER BY page.created_at DESC, page.id DESC`

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		var raw []byte
		if err := rows.Scan(&n.ID, &n.Type, &n.Kind, &n.ActorUserID, &n.TeamID, &n.PeriodID,
			&n.GoalID, &n.KRID, &n.CommentID, &n.EntityTitle, &raw, &n.CoalesceCount,
			&n.CreatedAt, &n.UpdatedAt, &n.ReadAt,
			&n.ActorDisplayName, &n.ActorAvatarURL, &n.ActorRemoved,
			&n.TeamPath, &n.GoalTitle); err != nil {
			return nil, nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &n.Payload)
		}
		out = append(out, n)
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

// UnreadCount powers the bell badge. Partial-index COUNT for one user — cheap enough
// that caching it across K8s replicas would buy staleness, not speed.
func (r *Repository) UnreadCount(ctx context.Context, scope domain.TenantScope, userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE tenant_id = $1 AND user_id = $2 AND read_at IS NULL`,
		scope.TenantID, userID).Scan(&n)
	return n, err
}

// MarkRead marks the given ids read, or everything unread when all is true.
// Scoped by user_id, so one user can never mark another's notifications.
// Delete removes one notification. Scoped to BOTH tenant and recipient: user_id is
// what stops a member of the same tenant from deleting someone else's entries by
// guessing an id. Reports whether a row was actually removed so the handler can
// answer 404 without a prior existence check — one round trip, and no way to probe
// for someone else's notification, since "not yours" and "not there" are the same
// answer.
func (r *Repository) Delete(ctx context.Context, scope domain.TenantScope, userID, id int64) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM notifications WHERE id = $1 AND tenant_id = $2 AND user_id = $3`,
		id, scope.TenantID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) MarkRead(ctx context.Context, scope domain.TenantScope, userID int64, ids []int64, all bool) error {
	if all {
		_, err := r.db.Exec(ctx,
			`UPDATE notifications SET read_at = now()
			  WHERE tenant_id = $1 AND user_id = $2 AND read_at IS NULL`,
			scope.TenantID, userID)
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = now()
		  WHERE tenant_id = $1 AND user_id = $2 AND id = ANY($3) AND read_at IS NULL`,
		scope.TenantID, userID, ids)
	return err
}

// PurgeOlderThan drops read notifications untouched for readDays and anything
// untouched for anyDays. Measured by updated_at, not created_at: coalescing bumps
// updated_at and clears read_at, so a notification that keeps collapsing repeats
// into itself must not be purged out from under an unread badge just because its
// first occurrence is old — "untouched for N days" is what retention means here.
// Cross-tenant on purpose: it is the retention pass, not a user action.
func (r *Repository) PurgeOlderThan(ctx context.Context, readDays, anyDays int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM notifications
		 WHERE (read_at IS NOT NULL AND updated_at < now() - make_interval(days => $1))
		    OR updated_at < now() - make_interval(days => $2)`,
		readDays, anyDays)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
