package notificationchannel

import (
	"context"
	"strings"

	"okrs/notifychannel"
)

// maskSecretErrors wraps sender so a delivery failure never repeats the tenant's
// secret in its error text. A channel receives the secret already decrypted (in
// Settings.Secret) — nothing stops its Send from folding that value into an error
// message. Telegram's Bot API is the concrete case this guards against: the token
// sits in the request URL (/bot<TOKEN>/sendMessage), so a channel that just passes
// through a bare url.Error would repeat it verbatim in a 502 body. The core is the
// only layer that knows the plaintext, so the core is where the redaction has to
// live — a channel author, however careful, cannot be relied on for this alone.
//
// A channel with no secret (empty Descriptor.SecretField, or a secret-bearing
// field left empty) is returned unwrapped: there is nothing to redact, and no
// reason to pay for a wrapper — or for the extra error allocation on every
// failure — where it protects nothing.
func maskSecretErrors(sender notifychannel.Sender, secret string) notifychannel.Sender {
	if secret == "" {
		return sender
	}
	return secretMaskingSender{Sender: sender, secret: secret}
}

// secretMaskingSender embeds Sender so every method besides Send — there is only
// the one today — keeps passing straight through.
//
// Known limitation, checked empirically: embedding Sender does NOT promote any
// other interface of the notifychannel contract. A type assertion from a wrapped
// value to notifychannel.Linker fails, because Go only forwards methods declared
// on the embedded interface itself, not other interfaces the concrete value
// underneath happens to also implement. Nothing in this module asserts to Linker
// today, but Linker is part of the public contract, and the next channel that
// needs account linking (Telegram) will hit this.
type secretMaskingSender struct {
	notifychannel.Sender
	secret string
}

func (s secretMaskingSender) Send(ctx context.Context, target notifychannel.Target, msg notifychannel.Message) error {
	err := s.Sender.Send(ctx, target, msg)
	if err == nil {
		return nil
	}
	return &maskedError{err: err, secret: s.secret}
}

// maskedError redacts the secret out of Error() while keeping the original error
// reachable through Unwrap, so errors.Is/errors.As — e.g. mattermost.IsPermanent —
// still see through the wrapper to whatever the channel actually returned.
type maskedError struct {
	err    error
	secret string
}

func (e *maskedError) Error() string {
	return strings.ReplaceAll(e.err.Error(), e.secret, "••••")
}

func (e *maskedError) Unwrap() error { return e.err }
