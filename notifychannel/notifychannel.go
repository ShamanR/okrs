// Package notifychannel is the public contract of a notification delivery channel.
//
// It holds types only: no I/O, no imports of okrs/internal/**. That is deliberate.
// A channel author in a separate module cannot import okrs/internal/... — Go's
// visibility rule forbids it — so a channel seam built on an internal registry
// would be unusable from outside. Channels are supplied instead through
// app.Config.NotificationChannels, assembled next to main.
package notifychannel

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// ErrMissingSecret is what a channel constructor returns when its Descriptor
// declares a SecretField but Settings.Secret is empty — typically because the
// deployment has no encryption key configured.
var ErrMissingSecret = errors.New("notifychannel: secret is required but empty")

// Target is the addressee. A channel uses whichever field it can address by.
type Target struct {
	// ExternalID is a stored account link (a Telegram chat id, a Mattermost user id).
	ExternalID string
	// Email lets a channel resolve the addressee itself, with no linking step.
	Email string
}

// Message is what the core hands a channel. Title and Body are already rendered
// by internal/render/notify, so every channel says the same thing.
type Message struct {
	Title string
	Body  string
	// URL is an absolute or site-relative link back to the goal, may be empty.
	URL string
}

// Sender delivers messages. Implementations must be safe for concurrent use: the
// core sends from several goroutines at once, and a channel that batches is
// additionally read by its own timer.
//
// The three methods differ in WHEN delivery happens and WHO learns the outcome:
//
//   - Send accepts a message for delivery and may hold it. It returns nil once
//     the message is accepted, so its result says nothing about whether the
//     external service took it. A channel that holds messages therefore must log
//     failures through Deps.Log — no caller is left to return them to.
//   - SendNow delivers immediately and returns the real outcome. The admin's
//     "test this channel" button is built on it: its answer reaches a human, so
//     a channel MUST NOT swallow the error or defer the work.
//   - Flush delivers whatever is still held. The core calls it on shutdown and
//     before replacing a channel whose configuration changed.
//
// A channel that does not batch implements all three the same way: Send and
// SendNow both deliver, and Flush does nothing and returns nil.
//
// None of these methods should repeat the tenant's configured secret in an
// error or a log record (a channel that folds a token into a request URL is the
// known trap). The core wraps both the returned errors and the supplied logger
// to scrub any known secret value, but that is a second barrier, not a license
// to be careless with the first.
type Sender interface {
	// Send accepts one message for delivery. Returning nil means accepted, not
	// delivered.
	Send(ctx context.Context, target Target, msg Message) error
	// SendNow delivers one message immediately and reports the real outcome.
	SendNow(ctx context.Context, target Target, msg Message) error
	// Flush delivers everything currently held. A channel holding nothing
	// returns nil.
	Flush(ctx context.Context) error
}

// Settings is a channel's configuration inside one tenant. Secret arrives already
// decrypted; storage and encryption are the core's business, not the channel's.
type Settings struct {
	Values map[string]any
	Secret string
}

// discard swallows every record, so a channel constructed without a logger stays
// silent rather than writing to the process default.
var discard = slog.New(slog.DiscardHandler)

// Deps is everything the core hands a channel at construction time.
//
// It is a struct rather than a parameter list so that the next dependency added
// here does not break the constructor of every channel ever written against this
// contract. Settings is not the place for the others: it means "what this tenant
// configured", and neither a logger nor a clock is tenant configuration.
type Deps struct {
	// Settings is this channel's configuration inside one tenant.
	Settings Settings

	// Logger receives delivery failures. Send cannot report an outcome — by the
	// time delivery is attempted its caller is gone — so a channel that holds
	// messages logs the failure itself.
	//
	// The core supplies a logger whose handler already redacts this tenant's
	// secret, because the core is the only layer that knows the plaintext. A
	// channel neither needs nor should attempt its own redaction.
	//
	// May be nil. Read it through Log, never directly.
	Logger *slog.Logger

	// Now is the channel's clock. A channel that batches owns a deadline, and a
	// test for "collected three updates, sent one message when the window
	// closed" has to move time rather than wait for it.
	//
	// May be nil. Read it through Clock, never directly.
	Now func() time.Time
}

// Log returns the logger to use: the supplied one, or a silent logger.
func (d Deps) Log() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return discard
}

// Clock returns the clock to use: the supplied one, or time.Now.
func (d Deps) Clock() func() time.Time {
	if d.Now != nil {
		return d.Now
	}
	return time.Now
}

// FieldKind tells the admin UI how to render a configuration field.
type FieldKind string

const (
	FieldText   FieldKind = "text"
	FieldURL    FieldKind = "url"
	FieldSecret FieldKind = "secret"
)

// Field is one input in the channel's configuration form.
type Field struct {
	Key      string
	Label    string
	Hint     string
	Required bool
	Kind     FieldKind
}

// Descriptor describes the channel to the core and to the admin UI. Because the
// UI renders from this, the admin screen knows nothing about any specific channel
// and a channel from another repository appears in it unchanged.
type Descriptor struct {
	// Name is the channel key: stored in the database, echoed in user preferences,
	// and used to build the entitlement key entitlement.notifications.<Name>.
	Name  string
	Title string
	// SecretField names the field the core encrypts at rest. Empty means the
	// channel has no secret.
	SecretField string
	Fields      []Field
}

// Channel is one unit of wiring: how to describe it, and how to build a Sender
// from a tenant's settings and the runtime dependencies the core supplies.
type Channel struct {
	Descriptor Descriptor
	New        func(Deps) (Sender, error)
}

// Linker is implemented by a channel that needs an explicit account link — a
// one-time token and a deep link — rather than resolving the addressee by email.
// Optional: a channel without it is addressed through Target.Email.
//
// Note that an optional interface is only reachable on an unwrapped Sender: see
// the known limitation documented on secretMaskingSender in
// internal/service/notificationchannel/secretmask.go. That is why batching,
// immediate delivery and flushing live on Sender itself rather than here.
type Linker interface {
	LinkURL(s Settings, token string) string
}
