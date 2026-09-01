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

// Sender delivers one message. Implementations must be safe for concurrent use:
// the delivery worker runs several at once. Send's returned error should not
// repeat the tenant's configured secret verbatim (a channel that folds a token
// into a request URL is the known trap) — the core additionally scrubs any known
// secret value out of the error text before it can reach an admin, but that is a
// second barrier, not a license to be careless with the first.
type Sender interface {
	Send(ctx context.Context, target Target, msg Message) error
}

// Settings is a channel's configuration inside one tenant. Secret arrives already
// decrypted; storage and encryption are the core's business, not the channel's.
type Settings struct {
	Values map[string]any
	Secret string
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
// from a tenant's settings.
type Channel struct {
	Descriptor Descriptor
	New        func(Settings) (Sender, error)
}

// Linker is implemented by a channel that needs an explicit account link — a
// one-time token and a deep link — rather than resolving the addressee by email.
// Optional: a channel without it is addressed through Target.Email.
type Linker interface {
	LinkURL(s Settings, token string) string
}
