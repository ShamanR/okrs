// Package notificationdelivery hands created notifications to a tenant's external
// channels.
//
// It is the implementation of notification.Deliverer, kept out of the
// notification usecase on purpose: that one decides WHO is notified and writes
// the journal row, this one decides how the same thing is said and addressed
// outside the product. Only this side knows that channels exist as types, which
// is what keeps notifychannel out of the usecase that publishes to it.
package notificationdelivery

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/platform/logging"
	"okrs/internal/render/notify"
	"okrs/internal/store/users"
	"okrs/internal/usecase/notification"
	"okrs/notifychannel"
)

// removedActorName is what a notification says when the person who caused it no
// longer exists. Same wording as the bell: one event must not read differently
// depending on where it is read.
const removedActorName = "Бывший участник"

// Channels is the port to the tenant's configured channels, declared
// consumer-side per specs/010.
//
// DeliverySender is asked once per channel per batch, never per message: it reads
// the channel's configuration row, and the instance it returns is the live one
// that holds pending messages.
//
// The delivery variant, not the plain one: it refuses a channel the administrator
// has switched off. The channel list this usecase was handed can be a moment old,
// and buffering new work into a channel that is already off is exactly what that
// staleness would otherwise cost.
type Channels interface {
	DeliverySender(ctx context.Context, scope domain.TenantScope, name string) (notifychannel.Sender, error)
}

// Contacts resolves who to name and where to address, for a whole batch at once.
//
// Tenant-scoped: naming someone depends on whether they are still a member, and
// that is a per-tenant fact.
type Contacts interface {
	ContactsByIDs(ctx context.Context, scope domain.TenantScope, ids []int64) (map[int64]users.Contact, error)
}

type Deps struct {
	Channels Channels
	Contacts Contacts
	// BaseURL is the product's own address, used to turn a notification's
	// site-relative link into one that works from outside the site. Empty when the
	// deployment did not configure AUTH_BASE_URL — see the note on absoluteURL.
	BaseURL string
	// Logger is optional; a nil logger silently skips logging.
	Logger *slog.Logger
}

type UseCase struct {
	channels Channels
	contacts Contacts
	baseURL  string
	logger   *slog.Logger
	// warnNoBase fires the "no base address" warning once per process rather than
	// once per batch: an unconfigured deployment would otherwise repeat it forever.
	warnNoBase sync.Once
}

func New(deps Deps) *UseCase {
	return &UseCase{
		channels: deps.Channels,
		contacts: deps.Contacts,
		baseURL:  strings.TrimRight(deps.BaseURL, "/"),
		logger:   deps.Logger,
	}
}

// absoluteURL turns a notification's site-relative link into one a messenger can
// open, and returns empty when it cannot.
//
// The bell gets away with "/?team=13&goal=72": the browser resolves it against
// the page it is already on. A message in Mattermost has no such page — the same
// string arrives as plain text the reader cannot click, or worse, resolves
// against the messenger's own host.
//
// The base can only come from configuration here. Invite links reconstruct it
// from the request and its X-Forwarded-* headers, but delivery runs on a
// background goroutine long after the request is gone.
//
// With no base configured the link is dropped rather than sent relative: a link
// that cannot be opened is not a link, and one pointing at the messenger's own
// host is actively misleading. The message itself still says what happened.
func (u *UseCase) absoluteURL(path string) string {
	if path == "" {
		return ""
	}
	if u.baseURL == "" {
		u.warnNoBase.Do(func() {
			if u.logger != nil {
				u.logger.Warn("notificationdelivery: AUTH_BASE_URL не задан — ссылки в сообщения внешних каналов не попадут",
					slog.String(logging.KeyEvent, logging.EventAppStart))
			}
		})
		return ""
	}
	return u.baseURL + path
}

// Deliver renders each notification and hands it to every channel its recipient
// gets it in.
//
// Батчевая операция: и адреса, и отправители резолвятся на пачку, не на
// сообщение — иначе доставка одного батча событий даёт запрос на каждого
// получателя и чтение конфигурации канала на каждое сообщение.
func (u *UseCase) Deliver(ctx context.Context, scope domain.TenantScope, items []notification.Delivery) error {
	if len(items) == 0 {
		return nil
	}

	// One lookup for the whole batch, covering both the people being named and the
	// people being addressed.
	ids := make([]int64, 0, len(items)*2)
	seen := make(map[int64]bool, len(items)*2)
	for _, it := range items {
		for _, id := range [2]int64{it.UserID, it.ActorUserID} {
			if id != 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	contacts, err := u.contacts.ContactsByIDs(ctx, scope, ids)
	if err != nil {
		return err
	}

	senders := make(map[string]notifychannel.Sender)
	var errs []error
	unaddressable, departed := 0, 0

	for _, it := range items {
		recipient, known := contacts[it.UserID]
		switch {
		case recipient.Removed:
			// Membership was revoked between resolving the recipients and reading
			// their contacts. The bell row stays — it is inside the product, and
			// this person can no longer open it — but the message must not leave
			// for a personal account that is no longer part of the tenant.
			departed++
			continue
		case !known || recipient.Email == "":
			// Every channel in this build addresses by email. Counted and reported
			// once for the batch rather than per message: a tenant whose staff have
			// no addresses would otherwise fill the log with one line each.
			unaddressable++
			continue
		}
		msg := u.render(it, contacts)
		target := notifychannel.Target{Email: recipient.Email}

		for _, name := range it.Channels {
			sender, ok := senders[name]
			if !ok {
				s, err := u.channels.DeliverySender(ctx, scope, name)
				if err != nil {
					// The channel is gone, unconfigured or refused its own stored
					// settings. Record it once per batch, not once per message.
					errs = append(errs, err)
					senders[name] = nil
					continue
				}
				sender, senders[name] = s, s
			}
			if sender == nil {
				continue
			}
			// Send accepts the message into the channel's window; a failure at
			// actual delivery time is the channel's to log, through the scrubbing
			// logger the core handed it.
			if err := sender.Send(ctx, target, msg); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if unaddressable > 0 && u.logger != nil {
		u.logger.WarnContext(ctx, "notificationdelivery: получателям не на что адресовать сообщение",
			slog.String(logging.KeyEvent, logging.EventDomainEvent),
			slog.Int64(logging.KeyTenantID, scope.TenantID),
			slog.Int("recipients", unaddressable))
	}
	if departed > 0 && u.logger != nil {
		u.logger.InfoContext(ctx, "notificationdelivery: получатель больше не состоит в тенанте, внешняя отправка пропущена",
			slog.String(logging.KeyEvent, logging.EventDomainEvent),
			slog.Int64(logging.KeyTenantID, scope.TenantID),
			slog.Int("recipients", departed))
	}
	return errors.Join(errs...)
}

// render turns one delivery into the text a channel sends. The wording comes from
// render/notify, the same package the bell renders with, so one event does not
// read one way in the product and another way in a messenger.
func (u *UseCase) render(it notification.Delivery, contacts map[int64]users.Contact) notifychannel.Message {
	// A former member is named by the neutral placeholder, never by their name.
	// The bell applies the same rule through its own membership join; a message
	// leaving the product must not be the one place it lapses.
	actor := removedActorName
	if c, ok := contacts[it.ActorUserID]; ok && !c.Removed && c.DisplayName != "" {
		actor = c.DisplayName
	}
	text := notify.Render(notify.Input{
		Kind:        event.Kind(it.Kind),
		ActorName:   actor,
		EntityTitle: it.EntityTitle,
		Count:       it.Count,
		Payload:     it.Payload,
	})
	body := text.Body
	if text.Subject != "" {
		// The bell shows the subject on its own line above the body; a channel gets
		// one Body, so the same two lines are joined here rather than lost.
		body = text.Subject + "\n" + body
	}
	return notifychannel.Message{
		Title: text.Title,
		Body:  body,
		URL: u.absoluteURL(notify.TargetURL(notify.LinkInput{
			GoalID:    it.GoalID,
			TeamID:    it.TeamID,
			PeriodID:  it.PeriodID,
			KRID:      it.KRID,
			CommentID: it.CommentID,
			// Usually the event has just happened and its goal exists — but not for
			// the deletion itself, which is published after the row is gone. The
			// usecase knows which event this was; this side only carries the answer.
			GoalMissing: it.GoalGone,
		})),
	}
}
