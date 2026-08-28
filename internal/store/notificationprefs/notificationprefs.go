// Package notificationprefs persists per-user notification preferences and answers
// the question the fan-out actually asks: who is subscribed to an event that
// happened in team T.
//
// Resolution lives here rather than in a separate read model because it is a read of
// preferences enriched with the team tree — not a second entity.
package notificationprefs

import (
	"context"

	"okrs/internal/core/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Notification types. Scoped types resolve through the team tree; addressed types
// carry their recipient in the event itself.
const (
	TypeGoalComment       = "goal_comment"
	TypeMyCommentResolved = "my_comment_resolved"
	TypeGoalChanged       = "goal_changed"
	TypeKRProgress        = "kr_progress"
)

// AllTypes is the order the settings screen renders.
var AllTypes = []string{TypeGoalComment, TypeMyCommentResolved, TypeGoalChanged, TypeKRProgress}

// IsAddressed reports whether a type is addressed rather than scope-based. An
// addressed type has no scope selector: it is delivered to a specific person.
func IsAddressed(t string) bool { return t == TypeMyCommentResolved }

// Scope values.
const (
	ScopeOwn            = "own"
	ScopeOwnAndChildren = "own_and_children"
	ScopeSubtree        = "subtree"
)

type Preference struct {
	Type     string
	Enabled  bool
	Scope    string
	Channels []string
}

// Target is one event's addressing input: the team it happened in and who did it.
type Target struct {
	TeamID  int64
	ActorID int64
}

// Recipient is one resolved addressee. Ord is the index of the originating Target,
// so the caller maps results back onto its batch.
type Recipient struct {
	Ord      int
	UserID   int64
	Channels []string
}

// defaultPreference is what applies when the user has never touched settings:
// enabled, own team only, in-app. Missing rows are the norm, not an exception —
// that is why nothing is backfilled on user creation.
func defaultPreference(t string) Preference {
	p := Preference{Type: t, Enabled: true, Channels: []string{"in_app"}}
	if !IsAddressed(t) {
		p.Scope = ScopeOwn
	}
	return p
}

// GetAll returns all four types, substituting defaults for rows that do not exist.
func (r *Repository) GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]Preference, error) {
	rows, err := r.db.Query(ctx,
		`SELECT type, enabled, COALESCE(scope, ''), channels
		   FROM notification_preferences WHERE tenant_id = $1 AND user_id = $2`,
		scope.TenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stored := make(map[string]Preference)
	for rows.Next() {
		var p Preference
		if err := rows.Scan(&p.Type, &p.Enabled, &p.Scope, &p.Channels); err != nil {
			return nil, err
		}
		stored[p.Type] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Preference, 0, len(AllTypes))
	for _, t := range AllTypes {
		if p, ok := stored[t]; ok {
			out = append(out, p)
			continue
		}
		out = append(out, defaultPreference(t))
	}
	return out, nil
}

// Set upserts one preference row.
func (r *Repository) Set(ctx context.Context, scope domain.TenantScope, userID int64, p Preference) error {
	var scopeVal any
	if !IsAddressed(p.Type) && p.Scope != "" {
		scopeVal = p.Scope
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channels)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, user_id, type) DO UPDATE
		   SET enabled = EXCLUDED.enabled, scope = EXCLUDED.scope, channels = EXCLUDED.channels`,
		scope.TenantID, userID, p.Type, p.Enabled, scopeVal, p.Channels)
	return err
}

// Both the seed and recursive terms filter by tenant_id — teams of other tenants are
// never traversed, matching the convention recorded in
// grants.GrantRepository.ListDescendantTeamIDs.
//
// SELECT DISTINCT collapses a lead reached through two ancestor paths of the same
// event (e.g. one person leading both a team and its parent unit) into a single row.
// Without it the fan-out would see the same (event, user) pair twice and, downstream,
// notifications.Insert's ON CONFLICT would bump coalesce_count instead of discarding
// the duplicate, so the user would see an inflated repeat count for one change.
const resolveSQL = `
WITH RECURSIVE chain AS (
    SELECT src.ord, src.actor_id, t.id, t.parent_id, t.lead_udid, 0 AS distance
      FROM unnest($1::bigint[], $4::bigint[]) WITH ORDINALITY AS src(team_id, actor_id, ord)
      JOIN teams t ON t.id = src.team_id AND t.deleted_at IS NULL AND t.tenant_id = $2
    UNION ALL
    SELECT c.ord, c.actor_id, t.id, t.parent_id, t.lead_udid, c.distance + 1
      FROM teams t JOIN chain c ON t.id = c.parent_id
     WHERE t.deleted_at IS NULL AND t.tenant_id = $2
)
SELECT DISTINCT c.ord - 1, u.id, COALESCE(p.channels, '{in_app}'::text[])
  FROM chain c
  JOIN users u       ON u.udid = c.lead_udid
  JOIN memberships m ON m.user_id = u.id AND m.tenant_id = $2 AND m.status = 'active'
  LEFT JOIN notification_preferences p
         ON p.tenant_id = $2 AND p.user_id = u.id AND p.type = $3
 WHERE u.id <> c.actor_id
   AND COALESCE(p.enabled, TRUE)
   AND CASE COALESCE(p.scope, 'own')
         WHEN 'own'              THEN c.distance = 0
         WHEN 'own_and_children' THEN c.distance <= 1
         ELSE TRUE
       END`

// ResolveRecipients answers "who must be notified" for a whole batch of events at
// once: $1 and $4 are parallel arrays of team and actor, one pair per event, and the
// result carries Ord so the caller maps rows back onto its batch.
//
// Actor exclusion is per event (c.actor_id travels down the recursion), not per
// batch: a lead who authored one event must still be notified about the others.
//
// Батчевая операция: не превращать в цикл — это N+1.
func (r *Repository) ResolveRecipients(ctx context.Context, scope domain.TenantScope, notifType string, targets []Target) ([]Recipient, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	teamIDs := make([]int64, len(targets))
	actorIDs := make([]int64, len(targets))
	for i, t := range targets {
		teamIDs[i], actorIDs[i] = t.TeamID, t.ActorID
	}
	rows, err := r.db.Query(ctx, resolveSQL, teamIDs, scope.TenantID, notifType, actorIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recipient
	for rows.Next() {
		var rc Recipient
		if err := rows.Scan(&rc.Ord, &rc.UserID, &rc.Channels); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// ResolveAddressed filters explicitly addressed recipients (e.g. the author of a
// resolved task) by their preferences. No tree walk: the recipient is already known.
//
// Батчевая операция: не превращать в цикл — это N+1.
func (r *Repository) ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]Recipient, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT src.ord - 1, u.id, COALESCE(p.channels, '{in_app}'::text[])
		  FROM unnest($1::bigint[]) WITH ORDINALITY AS src(user_id, ord)
		  JOIN users u       ON u.id = src.user_id
		  JOIN memberships m ON m.user_id = u.id AND m.tenant_id = $2 AND m.status = 'active'
		  LEFT JOIN notification_preferences p
		         ON p.tenant_id = $2 AND p.user_id = u.id AND p.type = $3
		 WHERE COALESCE(p.enabled, TRUE)`,
		userIDs, scope.TenantID, notifType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recipient
	for rows.Next() {
		var rc Recipient
		if err := rows.Scan(&rc.Ord, &rc.UserID, &rc.Channels); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
