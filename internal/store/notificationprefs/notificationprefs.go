// Package notificationprefs persists per-user notification preferences and answers
// the question the fan-out actually asks: who is subscribed to an event that
// happened in team T.
//
// Resolution lives here rather than in a separate read model because it is a read of
// preferences enriched with the team tree — not a second entity.
package notificationprefs

import (
	"context"
	"sort"

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

// ChannelInApp is the reserved name of the bell feed among the delivery channels.
// It is not a notifychannel.Channel and never will be: it has no Sender, makes no
// network call, and its row has to be written whatever the user chose — the
// digest an external channel sends is assembled from exactly those rows.
const ChannelInApp = "in_app"

type Preference struct {
	Type    string
	Enabled bool
	Scope   string
	// ChannelOverrides is the user's explicit choice per channel: true for "send
	// it here", false for "do not". A channel absent from the map is one the user
	// never expressed an opinion about, and follows the administrator's default
	// for that channel.
	//
	// Sparse on purpose. A list of enabled channels cannot tell "the user turned
	// this off" from "this channel did not exist when the row was written", and a
	// channel connected later has to reach everyone — including people who saved
	// their settings long before it existed.
	ChannelOverrides map[string]bool
}

// Target is one event's addressing input: the team it happened in and who did it.
type Target struct {
	TeamID  int64
	ActorID int64
}

// Recipient is one resolved addressee. Ord is the index of the originating Target,
// so the caller maps results back onto its batch.
type Recipient struct {
	Ord    int
	UserID int64
	// ChannelOverrides is this recipient's explicit per-channel choice; see
	// Preference.ChannelOverrides. Turning it into the actual set of channels
	// needs the tenant's channel defaults, which live a layer up — this type
	// stays out of the availability gate.
	ChannelOverrides map[string]bool
}

// defaultPreference is what applies when the user has never touched settings:
// enabled, own team only, in-app. Missing rows are the norm, not an exception —
// that is why nothing is backfilled on user creation.
func defaultPreference(t string) Preference {
	p := Preference{Type: t, Enabled: true, ChannelOverrides: map[string]bool{}}
	if !IsAddressed(t) {
		p.Scope = ScopeOwn
	}
	return p
}

// GetAll returns all four types, substituting defaults for rows that do not exist.
func (r *Repository) GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]Preference, error) {
	rows, err := r.db.Query(ctx,
		`SELECT type, enabled, COALESCE(scope, ''), channel_overrides
		   FROM notification_preferences WHERE tenant_id = $1 AND user_id = $2`,
		scope.TenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stored := make(map[string]Preference)
	for rows.Next() {
		var p Preference
		if err := rows.Scan(&p.Type, &p.Enabled, &p.Scope, &p.ChannelOverrides); err != nil {
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

// Set upserts one preference row, MERGING the submitted channel choices into
// whatever is already stored rather than replacing them wholesale.
//
// The merge is what protects a choice the screen could not show. A channel the
// administrator temporarily switched off has no column in the matrix, so the
// payload carries nothing about it — and a plain replace would erase the user's
// stored answer for it. Switching the channel back on would then hand them the
// administrator's default and resume delivery they had explicitly refused.
//
// Every channel the screen DID show is present in the payload with an explicit
// value, so merging never keeps a stale answer for a visible channel: the
// submitted side always wins.
func (r *Repository) Set(ctx context.Context, scope domain.TenantScope, userID int64, p Preference) error {
	var scopeVal any
	if !IsAddressed(p.Type) && p.Scope != "" {
		scopeVal = p.Scope
	}
	overrides := p.ChannelOverrides
	if overrides == nil {
		// A nil map would encode as SQL NULL and violate the NOT NULL column; an
		// empty map is the honest value anyway — "no explicit choice about any
		// channel".
		overrides = map[string]bool{}
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channel_overrides)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, user_id, type) DO UPDATE
		   SET enabled = EXCLUDED.enabled, scope = EXCLUDED.scope,
		       channel_overrides =
		           notification_preferences.channel_overrides || EXCLUDED.channel_overrides`,
		scope.TenantID, userID, p.Type, p.Enabled, scopeVal, overrides)
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
SELECT DISTINCT c.ord - 1, u.id, COALESCE(p.channel_overrides, '{}'::jsonb)
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
		if err := rows.Scan(&rc.Ord, &rc.UserID, &rc.ChannelOverrides); err != nil {
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
		SELECT src.ord - 1, u.id, COALESCE(p.channel_overrides, '{}'::jsonb)
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
		if err := rows.Scan(&rc.Ord, &rc.UserID, &rc.ChannelOverrides); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// EffectiveChannels resolves where one recipient's notification actually goes:
// the tenant's defaults, overridden per channel by whatever the user chose.
//
// Lives here rather than in a service because both the settings service and the
// fan-out answer this same question, and two copies would be two chances for the
// three states — on, off, never chose — to be read differently.
//
// The result is sorted so callers, logs and tests see a stable order.
func EffectiveChannels(defaults map[string]bool, overrides map[string]bool) []string {
	out := make([]string, 0, len(defaults))
	for name, on := range defaults {
		if choice, chose := overrides[name]; chose {
			on = choice
		}
		if on {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// ExternalChannels is EffectiveChannels without the bell: the channels a
// notification has to be *sent* to, as opposed to the row that is written anyway.
func ExternalChannels(defaults map[string]bool, overrides map[string]bool) []string {
	all := EffectiveChannels(defaults, overrides)
	out := all[:0]
	for _, name := range all {
		if name != ChannelInApp {
			out = append(out, name)
		}
	}
	return out
}
