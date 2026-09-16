package notificationchannel

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"okrs/notifychannel"
)

// secretMask is what a redacted secret looks like, in an error and in a log
// record alike.
const secretMask = "••••"

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
	return secretMaskingSender{inner: sender, secret: secret}
}

// secretMaskingSender forwards each method of Sender explicitly instead of
// embedding the interface.
//
// Embedding is shorter, and is what this type used to do, but it makes the
// wrapper silently incomplete: a method added to Sender keeps compiling here and
// starts passing through unredacted. That is not hypothetical — SendNow returns
// its error straight into a tenant admin's HTTP response, which is the single
// place where the secret most must not appear. Spelling the methods out means the
// next addition to the contract breaks this file at compile time and has to be
// decided about rather than inherited.
//
// The same mechanic is why SendNow and Flush live on Sender rather than in
// optional interfaces beside it: embedding an interface promotes only the methods
// declared on that interface, so a type assertion from a wrapped value to any
// other interface of the contract fails — checked empirically. An optional
// SendNow would therefore be invisible here, the test-send button would fall back
// to the buffering Send, and it would answer "delivered" for a channel that never
// delivered anything. notifychannel.Linker still carries that limitation; nothing
// asserts to it today, and the channel that needs account linking will have to
// solve it differently.
type secretMaskingSender struct {
	inner  notifychannel.Sender
	secret string
}

func (s secretMaskingSender) Send(ctx context.Context, target notifychannel.Target, msg notifychannel.Message) error {
	return s.mask(s.inner.Send(ctx, target, msg))
}

func (s secretMaskingSender) SendNow(ctx context.Context, target notifychannel.Target, msg notifychannel.Message) error {
	return s.mask(s.inner.SendNow(ctx, target, msg))
}

func (s secretMaskingSender) Close(ctx context.Context) error {
	return s.mask(s.inner.Close(ctx))
}

func (s secretMaskingSender) mask(err error) error {
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
	return strings.ReplaceAll(e.err.Error(), e.secret, secretMask)
}

func (e *maskedError) Unwrap() error { return e.err }

// scrubbingLogger returns the logger to hand a channel: one that cannot write the
// tenant's secret, whatever the channel passes it.
//
// A channel that holds messages has no caller left when delivery finally fails,
// so it reports the failure to this logger instead of returning it. That moves
// the failure text out of reach of maskSecretErrors, which only ever sees what
// Send returns. Redacting at the handler restores the second barrier at the one
// layer that knows the plaintext, and does it in a way the channel cannot forget
// or opt out of.
func scrubbingLogger(base *slog.Logger, secret string) *slog.Logger {
	if base == nil || secret == "" {
		return base
	}
	return slog.New(&scrubbingHandler{inner: base.Handler(), secret: secret})
}

type scrubbingHandler struct {
	inner  slog.Handler
	secret string
}

func (h *scrubbingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *scrubbingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, h.scrub(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.scrubAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func (h *scrubbingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = h.scrubAttr(a)
	}
	return &scrubbingHandler{inner: h.inner.WithAttrs(scrubbed), secret: h.secret}
}

func (h *scrubbingHandler) WithGroup(name string) slog.Handler {
	return &scrubbingHandler{inner: h.inner.WithGroup(name), secret: h.secret}
}

func (h *scrubbingHandler) scrub(s string) string {
	return strings.ReplaceAll(s, h.secret, secretMask)
}

func (h *scrubbingHandler) scrubAttr(a slog.Attr) slog.Attr {
	a.Value = h.scrubValue(a.Value)
	return a
}

// dirtyRendering returns a rendering of v that contains the secret, or "" when
// none of the forms v can reach a handler through does.
//
// One rendering is not enough, because handlers do not agree on how to print a
// KindAny value and a value can hide the secret from one form while showing it
// in another. The concrete trap: %v and %+v hand a fmt.Stringer straight to its
// String method, so a type with a tidy String and a token in an exported field
// looks clean — while slog.JSONHandler never calls String and json.Marshals the
// fields instead. Checking one form and forwarding the value on that basis is
// how the secret gets out.
//
// So every form is checked, cheapest first, and the value is forwarded only when
// all of them are clean:
//
//   - %+v — what slog.TextHandler falls back to, and what an error or Stringer
//     renders as. Also shows unexported fields of a plain struct.
//   - %#v — the field-level view, which is what defeats the Stringer trap above.
//   - json.Marshal — what slog.JSONHandler actually emits, and the only form
//     that honours a MarshalJSON able to synthesize text no reflection-based
//     rendering would show.
//   - MarshalText — the first thing a text handler tries, same reasoning.
func dirtyRendering(v any, secret string) string {
	if text := fmt.Sprintf("%+v", v); strings.Contains(text, secret) {
		return text
	}
	if text := fmt.Sprintf("%#v", v); strings.Contains(text, secret) {
		return text
	}
	if b, err := json.Marshal(v); err == nil && strings.Contains(string(b), secret) {
		return string(b)
	}
	if tm, ok := v.(encoding.TextMarshaler); ok {
		if b, err := tm.MarshalText(); err == nil && strings.Contains(string(b), secret) {
			return string(b)
		}
	}
	return ""
}

// scrubValue rewrites the value kinds that can carry text. An error is the case
// that matters most: a channel logging "err", err is exactly how a token folded
// into a request URL would reach the log.
func (h *scrubbingHandler) scrubValue(v slog.Value) slog.Value {
	switch v.Kind() {
	case slog.KindString:
		return slog.StringValue(h.scrub(v.String()))
	case slog.KindGroup:
		group := v.Group()
		scrubbed := make([]slog.Attr, len(group))
		for i, a := range group {
			scrubbed[i] = h.scrubAttr(a)
		}
		return slog.GroupValue(scrubbed...)
	case slog.KindLogValuer:
		return h.scrubValue(v.Resolve())
	case slog.KindAny:
		// Anything can arrive here, and the downstream handler will render it its
		// own way. A struct or a map handed to slog.Any is serialized field by
		// field, so a token sitting in an exported field — slog.Any("settings",
		// d.Settings) is the obvious way to reach this — would go out verbatim
		// while this wrapper reported success.
		//
		// So the value is inspected HERE, in every form a handler could print it
		// as (see dirtyRendering). A value that carries the secret is replaced by
		// the rendering that exposed it, redacted; everything else is forwarded
		// untouched, keeping the handler's own formatting for the overwhelming
		// majority of records.
		if dirty := dirtyRendering(v.Any(), h.secret); dirty != "" {
			return slog.StringValue(h.scrub(dirty))
		}
		return v
	default:
		// Numbers, booleans, times and durations cannot carry the secret.
		return v
	}
}
