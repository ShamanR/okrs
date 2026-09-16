// Package notification turns domain events into per-recipient notifications.
//
// It is the bus subscriber registered for the 13 event types that notify anyone;
// the other 8 never reach it. Registered asynchronously — resolving recipients and
// inserting rows has no business holding up an HTTP response.
package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/platform/logging"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/notifications"
)

// CoalesceWindow is how long repeats collapse into one notification. Fixed buckets,
// not a sliding window: a sliding one needs read-then-write and races between
// replicas, and the boundary artefact is the cheaper trade (spec §7.2).
const CoalesceWindow = 10 * time.Minute

// NotificationWriter and PrefResolver are consumer-side ports, per specs/010.
type NotificationWriter interface {
	CreateBatch(ctx context.Context, scope domain.TenantScope, ins []notifications.InsertInput) error
}

type PrefResolver interface {
	Resolve(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error)
	ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error)
	// DeliveryDefaults is what a recipient who never chose gets, per channel.
	// Read once per group, not per recipient: a recipient's own answer travels in
	// Recipient.ChannelOverrides, and only the tenant-wide default is missing.
	DeliveryDefaults(ctx context.Context, scope domain.TenantScope) (map[string]bool, error)
}

// Delivery is one created notification on its way to a recipient's external
// channels: what happened, to whom, and where it has to be said.
//
// It carries ids and payload rather than rendered text, so this usecase stays out
// of both wording and addressing. Turning it into a message is the delivery
// side's job — see usecase/notificationdelivery.
type Delivery struct {
	// UserID is the recipient; ActorUserID is the person whose action caused it.
	UserID      int64
	ActorUserID int64
	// Channels are the external channels this recipient gets it in — never the
	// bell, which is the stored row itself.
	Channels []string

	Kind        string
	EntityTitle string
	Count       int
	Payload     map[string]any

	GoalID    *int64
	TeamID    *int64
	PeriodID  *int64
	KRID      *int64
	CommentID *int64
	// GoalGone marks a notification whose goal no longer exists — the deletion
	// itself. The id outlives the row: the goal is deleted before the event is
	// published, so a link built from it would send the reader looking for
	// something that is not there. The feed works this out from its own LEFT JOIN;
	// delivery has no such join and has to be told.
	GoalGone bool
}

// Deliverer takes created notifications to external channels.
//
// Declared consumer-side per specs/010, and deliberately free of any channel
// type: a channel is a notifychannel.Sender, and this usecase must not know that
// — it decides who is notified, not how a messenger is spoken to.
type Deliverer interface {
	Deliver(ctx context.Context, scope domain.TenantScope, items []Delivery) error
}

type Deps struct {
	Notifications NotificationWriter
	Prefs         PrefResolver
	// Delivery sends notifications to external channels. nil means the build has
	// none: the bell is written exactly as before.
	Delivery Deliverer
	// Logger is optional; a nil logger silently skips logging. Used only for the
	// out-of-range Ord guard below, which should never fire on correct data.
	Logger *slog.Logger
}

type UseCase struct {
	notifications NotificationWriter
	prefs         PrefResolver
	delivery      Deliverer
	logger        *slog.Logger
}

func New(deps Deps) *UseCase {
	return &UseCase{
		notifications: deps.Notifications,
		prefs:         deps.Prefs,
		delivery:      deps.Delivery,
		logger:        deps.Logger,
	}
}

// pending is one event already classified, awaiting its recipients.
type pending struct {
	ev     event.Event
	anchor anchor
	typ    string
}

// Handle is the bus subscriber. It groups the batch by (tenant, notification type),
// resolves recipients once per group, and writes all rows in one batch.
//
// Батчевая операция: резолв и вставка идут на группу, не на событие — иначе батч из
// 50 событий даёт 50 рекурсивных запросов и 50 вставок (правило 9 CLAUDE.md).
func (u *UseCase) Handle(ctx context.Context, evs []event.Event) error {
	type groupKey struct {
		tenantID int64
		typ      string
	}
	groups := make(map[groupKey][]pending)

	for _, ev := range evs {
		typ := notifyType(ev)
		if typ == "" {
			continue
		}
		a := anchorOf(ev)
		m := ev.Context()
		if notificationprefs.IsAddressed(typ) {
			// Nobody is notified about their own action.
			if a.addressee == 0 || a.addressee == m.ActorID {
				continue
			}
		} else if m.TeamID == nil {
			// Without a team the event cannot be scoped to anyone.
			continue
		}
		k := groupKey{tenantID: m.Scope.TenantID, typ: typ}
		groups[k] = append(groups[k], pending{ev: ev, anchor: a, typ: typ})
	}

	// One group's failure must not cost every other tenant its rows: a batch spans
	// tenants because one instance serves many requests, same reasoning as the
	// activity journal's Handle (service/activity/journal.go).
	var errs []error
	for k, items := range groups {
		scope := domain.TenantScope{TenantID: k.tenantID}
		recipients, err := u.resolve(ctx, scope, k.typ, items)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// Read once per group. A recipient's own answer already travelled in
		// Recipient.ChannelOverrides; what is missing is only the tenant-wide
		// default, and asking for it per recipient would be an N+1 over a value
		// that cannot change inside one batch.
		var defaults map[string]bool
		if u.delivery != nil {
			defaults, err = u.prefs.DeliveryDefaults(ctx, scope)
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}

		rows := make([]notifications.InsertInput, 0, len(recipients))
		var deliveries []Delivery
		for _, rc := range recipients {
			if rc.Ord < 0 || rc.Ord >= len(items) {
				// A resolver returning an out-of-range Ord is a bug in that resolver,
				// not a reason to crash the whole batch (this handler runs async,
				// where the bus recovers panics and would silently drop everything).
				if u.logger != nil {
					u.logger.ErrorContext(ctx, "notification: recipient Ord out of range",
						slog.String(logging.KeyEvent, logging.EventDomainEvent),
						slog.Int("ord", rc.Ord),
						slog.Int("items", len(items)),
						slog.String("type", k.typ),
						slog.Int64(logging.KeyTenantID, k.tenantID))
				}
				continue
			}
			p := items[rc.Ord]
			// The row is written whatever the recipient chose about channels. It is
			// the journal entry every reader works from — the feed hides it when the
			// bell is switched off, and the digest an external channel sends is
			// assembled from exactly these. Not writing it would leave the external
			// channel with nothing to say.
			rows = append(rows, u.row(p, rc.UserID))

			if u.delivery == nil {
				continue
			}
			external := notificationprefs.ExternalChannels(defaults, rc.ChannelOverrides)
			if len(external) == 0 {
				continue
			}
			deliveries = append(deliveries, u.deliveryFor(p, rc.UserID, external))
		}
		if len(rows) == 0 {
			// Nobody left to notify for this group: say so, rather than calling
			// CreateBatch with nothing.
			continue
		}
		if err := u.notifications.CreateBatch(ctx, scope, rows); err != nil {
			errs = append(errs, err)
			// The row is the record of what happened; without it there is nothing
			// to have delivered, so a failed write must not produce messages.
			continue
		}
		if len(deliveries) > 0 {
			if err := u.delivery.Deliver(ctx, scope, deliveries); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// resolve picks the addressing strategy for the group's type: addressed types carry
// their recipient, scoped types walk the team tree.
func (u *UseCase) resolve(ctx context.Context, scope domain.TenantScope, typ string, items []pending) ([]notificationprefs.Recipient, error) {
	if notificationprefs.IsAddressed(typ) {
		userIDs := make([]int64, len(items))
		for i, p := range items {
			userIDs[i] = p.anchor.addressee
		}
		return u.prefs.ResolveAddressed(ctx, scope, typ, userIDs)
	}
	targets := make([]notificationprefs.Target, len(items))
	for i, p := range items {
		m := p.ev.Context()
		targets[i] = notificationprefs.Target{TeamID: *m.TeamID, ActorID: m.ActorID}
	}
	return u.prefs.Resolve(ctx, scope, typ, targets)
}

func (u *UseCase) row(p pending, userID int64) notifications.InsertInput {
	m := p.ev.Context()
	return notifications.InsertInput{
		UserID:      userID,
		Type:        p.typ,
		Kind:        string(p.ev.Kind()),
		ActorUserID: m.ActorID,
		TeamID:      m.TeamID,
		PeriodID:    m.PeriodID,
		GoalID:      p.anchor.goalID,
		KRID:        p.anchor.krID,
		CommentID:   p.anchor.commentID,
		EntityTitle: p.anchor.title,
		Payload:     payloadOf(p.ev),
		CoalesceKey: coalesceKey(p, m),
	}
}

// deliveryFor describes one created notification for the delivery side. Count is 1:
// it reports this event, not the coalesced total of the row it may have merged
// into — the accumulated message names how many updates it carries itself.
func (u *UseCase) deliveryFor(p pending, userID int64, channels []string) Delivery {
	m := p.ev.Context()
	return Delivery{
		UserID:      userID,
		ActorUserID: m.ActorID,
		Channels:    channels,
		Kind:        string(p.ev.Kind()),
		EntityTitle: p.anchor.title,
		Count:       1,
		Payload:     payloadOf(p.ev),
		GoalID:      p.anchor.goalID,
		TeamID:      m.TeamID,
		PeriodID:    m.PeriodID,
		KRID:        p.anchor.krID,
		CommentID:   p.anchor.commentID,
		GoalGone:    p.ev.Kind() == event.KindGoalDeleted,
	}
}

// coalesceKey is type:entity:actor:bucket.
//
// The entity is the KR only for kr_progress; everything else keys on the goal, so a
// goal edited together with two of its KRs collapses into one "×3" notification.
func coalesceKey(p pending, m event.Meta) string {
	entity := "goal:0"
	if p.typ == notificationprefs.TypeKRProgress && p.anchor.krID != nil {
		entity = fmt.Sprintf("kr:%d", *p.anchor.krID)
	} else if p.anchor.goalID != nil {
		entity = fmt.Sprintf("goal:%d", *p.anchor.goalID)
	}
	at := m.OccurredAt
	if at.IsZero() {
		at = time.Now()
	}
	bucket := at.Unix() / int64(CoalesceWindow.Seconds())
	return fmt.Sprintf("%s:%s:%d:%d", p.typ, entity, m.ActorID, bucket)
}
