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
// Sender is asked once per channel per batch, never per message: it reads the
// channel's configuration row, and the instance it returns is the live one that
// holds pending messages.
type Channels interface {
	Sender(ctx context.Context, scope domain.TenantScope, name string) (notifychannel.Sender, error)
}

// Contacts resolves who to name and where to address, for a whole batch at once.
type Contacts interface {
	ContactsByIDs(ctx context.Context, ids []int64) (map[int64]users.Contact, error)
}

type Deps struct {
	Channels Channels
	Contacts Contacts
	// Logger is optional; a nil logger silently skips logging.
	Logger *slog.Logger
}

type UseCase struct {
	channels Channels
	contacts Contacts
	logger   *slog.Logger
}

func New(deps Deps) *UseCase {
	return &UseCase{channels: deps.Channels, contacts: deps.Contacts, logger: deps.Logger}
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
	contacts, err := u.contacts.ContactsByIDs(ctx, ids)
	if err != nil {
		return err
	}

	senders := make(map[string]notifychannel.Sender)
	var errs []error
	unaddressable := 0

	for _, it := range items {
		recipient, known := contacts[it.UserID]
		if !known || recipient.Email == "" {
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
				s, err := u.channels.Sender(ctx, scope, name)
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
	return errors.Join(errs...)
}

// render turns one delivery into the text a channel sends. The wording comes from
// render/notify, the same package the bell renders with, so one event does not
// read one way in the product and another way in a messenger.
func (u *UseCase) render(it notification.Delivery, contacts map[int64]users.Contact) notifychannel.Message {
	actor := removedActorName
	if c, ok := contacts[it.ActorUserID]; ok && c.DisplayName != "" {
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
		URL: notify.TargetURL(notify.LinkInput{
			GoalID:    it.GoalID,
			TeamID:    it.TeamID,
			PeriodID:  it.PeriodID,
			KRID:      it.KRID,
			CommentID: it.CommentID,
			// The event has just happened, so its goal exists. Only the feed, which
			// reads rows written long ago, has to consider that it may not.
			GoalMissing: false,
		}),
	}
}
