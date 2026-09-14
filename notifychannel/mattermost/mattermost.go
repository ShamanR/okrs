// Package mattermost delivers notifications as Mattermost direct messages.
//
// It addresses by email, so it needs no account-linking step and does not
// implement notifychannel.Linker. Public on purpose: channels are wired through
// app.Config.NotificationChannels next to main, and this package doubles as the
// worked example for a channel written in another repository.
//
// Delivery is batched. Send accepts a message and holds it; once the channel's
// window has elapsed, everything held for one addressee goes out as a single
// post. That policy lives here rather than in the core because it is a property
// of the destination — a chat client is a place where twelve separate pings for
// one editing session are worse than one summary — and because the format of a
// combined message is Mattermost's Markdown, which the core has no business
// knowing.
package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"okrs/notifychannel"
)

const (
	// defaultWindow is how long updates accumulate when the tenant did not choose.
	defaultWindow = 10 * time.Minute

	// maxBuffered caps what one addressee may accumulate. The cap is per
	// addressee, not per channel: a single busy recipient must not evict
	// everyone else's pending updates. On overflow the oldest go first — the
	// newest updates are the ones still worth reading.
	maxBuffered = 200

	// timerFlushTimeout bounds a flush started by the window timer. Nothing is
	// waiting on it, so it needs its own deadline or a hung server would pin the
	// buffer indefinitely.
	timerFlushTimeout = 30 * time.Second
)

// permanentError marks a failure that retrying cannot fix — an addressee with no
// Mattermost account, a malformed request. Flush uses this to decide whether the
// batch is worth keeping for the next window or should be discarded.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// IsPermanent reports whether the failure is worth retrying.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

func permanent(format string, args ...any) error {
	return permanentError{err: fmt.Errorf(format, args...)}
}

// wave represents an in-flight botID resolution. Concurrent callers share the same wave's result.
type wave struct {
	done chan struct{}
	id   string
	err  error
}

// Channel returns the wiring unit to pass to app.Config.NotificationChannels.
func Channel() notifychannel.Channel {
	return notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{
			Name:        "mattermost",
			Title:       "Mattermost",
			SecretField: "token",
			Fields: []notifychannel.Field{
				{
					Key: "base_url", Label: "Адрес сервера", Required: true,
					Kind: notifychannel.FieldURL,
					Hint: "Например https://mattermost.example.com — без завершающего слэша",
				},
				{
					Key: "token", Label: "Токен бота", Required: true,
					Kind: notifychannel.FieldSecret,
					Hint: "Personal Access Token бота. Боту нужны права на создание личных сообщений",
				},
				{
					Key: "window_minutes", Label: "Окно отправки, минут",
					Kind: notifychannel.FieldText,
					Hint: "Обновления копятся указанное время и уходят одним сообщением. По умолчанию 10",
				},
			},
		},
		New: newSender,
	}
}

type sender struct {
	baseURL string
	token   string
	http    *http.Client
	log     *slog.Logger
	now     func() time.Time
	window  time.Duration

	// botID is resolved once on success and reused: delivery goes out in batches, and
	// re-asking who we are on every message is an N+1 over the network.
	// If resolution fails (temporary error), retry on next delivery.
	// Multiple concurrent deliveries coalesce on the same wave: the first fetches, others wait.
	// On error, all waiters share the error and complete immediately—no sequential queueing.
	// On success, botID is cached; the wave is discarded and the next delivery starts fresh if needed.
	mu    sync.Mutex
	botID string // cached only on success
	wave  *wave  // current in-flight resolution, if any

	// buf holds what Send accepted but has not delivered yet, keyed by addressee.
	// deadline is when the current window closes; zero means nothing is held.
	// timer wakes the flush when no further Send arrives to notice the deadline.
	buf      map[notifychannel.Target][]notifychannel.Message
	dropped  map[notifychannel.Target]int
	deadline time.Time
	timer    *time.Timer
}

func newSender(d notifychannel.Deps) (notifychannel.Sender, error) {
	raw, _ := d.Settings.Values["base_url"].(string)
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return nil, errors.New("mattermost: base_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("mattermost: base_url is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("mattermost: base_url must use http or https, got %q", u.Scheme)
	}
	window, err := windowFrom(d.Settings.Values["window_minutes"])
	if err != nil {
		return nil, err
	}
	if d.Settings.Secret == "" {
		return nil, notifychannel.ErrMissingSecret
	}
	return &sender{
		baseURL: raw,
		token:   d.Settings.Secret,
		http:    &http.Client{Timeout: 15 * time.Second},
		log:     d.Log(),
		now:     d.Clock(),
		window:  window,
	}, nil
}

// windowFrom reads the configured window. Absent or blank means the default —
// the field is optional, and a tenant that never touched it gets ten minutes.
// Anything present but not a positive whole number of minutes is refused here,
// at construction, so the core surfaces it as a configuration error the admin
// can fix rather than as a surprise at delivery time.
//
// Values arrive as decoded JSON (float64) from the database and the API, and as
// int or string from tests and hand-built configuration, so all are accepted.
func windowFrom(v any) (time.Duration, error) {
	minutes, ok, err := wholeMinutes(v)
	if err != nil {
		return 0, err
	}
	if !ok {
		return defaultWindow, nil
	}
	if minutes <= 0 {
		return 0, fmt.Errorf("mattermost: window_minutes must be a positive number of minutes, got %d", minutes)
	}
	return time.Duration(minutes) * time.Minute, nil
}

// wholeMinutes reports the value as whole minutes. ok is false when the value is
// absent or blank, which means "not configured" rather than "invalid".
func wholeMinutes(v any) (n int64, ok bool, err error) {
	switch t := v.(type) {
	case nil:
		return 0, false, nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false, nil
		}
		n, convErr := strconv.ParseInt(s, 10, 64)
		if convErr != nil {
			return 0, false, fmt.Errorf("mattermost: window_minutes must be a whole number of minutes, got %q", t)
		}
		return n, true, nil
	case float64:
		if t != float64(int64(t)) {
			return 0, false, fmt.Errorf("mattermost: window_minutes must be a whole number of minutes, got %v", t)
		}
		return int64(t), true, nil
	case int:
		return int64(t), true, nil
	case int64:
		return t, true, nil
	case json.Number:
		n, convErr := t.Int64()
		if convErr != nil {
			return 0, false, fmt.Errorf("mattermost: window_minutes must be a whole number of minutes, got %q", t.String())
		}
		return n, true, nil
	default:
		return 0, false, fmt.Errorf("mattermost: window_minutes must be a whole number of minutes, got %T", v)
	}
}

// Send accepts a message for the next window. It returns nil once the message is
// held: no caller is waiting by the time delivery is attempted, so a failure at
// that point goes to the log instead of up the stack.
func (s *sender) Send(ctx context.Context, target notifychannel.Target, msg notifychannel.Message) error {
	s.mu.Lock()
	if s.buf == nil {
		s.buf = map[notifychannel.Target][]notifychannel.Message{}
	}
	q := append(s.buf[target], msg)
	if over := len(q) - maxBuffered; over > 0 {
		if s.dropped == nil {
			s.dropped = map[notifychannel.Target]int{}
		}
		s.dropped[target] += over
		q = q[over:]
	}
	s.buf[target] = q
	s.armLocked()
	due := !s.now().Before(s.deadline)
	s.mu.Unlock()

	if due {
		// The window closed while messages kept arriving, so this Send is the
		// wakeup. Cheaper than relying on the timer, and it keeps the fake clock
		// in tests sufficient to drive the whole cycle.
		s.flushAndLog(ctx)
	}
	return nil
}

// SendNow delivers one message immediately and reports the real outcome. The
// admin's "test this channel" button is built on it, so it must not touch the
// buffer in either direction: the answer has to describe this message and this
// message only.
func (s *sender) SendNow(ctx context.Context, target notifychannel.Target, msg notifychannel.Message) error {
	return s.deliver(ctx, target, []notifychannel.Message{msg})
}

// Flush delivers everything currently held.
//
// A batch that failed transiently goes back into the buffer: the external
// service being briefly unreachable should not cost the recipient their
// updates, and the cap keeps the retained volume bounded. A batch that failed
// permanently is discarded — repeating it cannot change the outcome.
func (s *sender) Flush(ctx context.Context) error {
	batches, dropped := s.take()
	for target, n := range dropped {
		// The addressee is never logged: the email is exactly what the rest of
		// this file goes out of its way to keep out of error text.
		s.log.Warn("mattermost: часть накопленных обновлений отброшена по достижении предела",
			"dropped", n, "limit", maxBuffered, "held", len(batches[target]))
	}
	var errs []error
	for target, msgs := range batches {
		err := s.deliver(ctx, target, msgs)
		if err == nil {
			continue
		}
		errs = append(errs, err)
		if IsPermanent(err) {
			s.log.Error("mattermost: доставка накопленного отклонена, обновления отброшены",
				"count", len(msgs), "err", err)
			continue
		}
		s.requeue(target, msgs)
		s.log.Warn("mattermost: доставка накопленного не удалась, обновления сохранены до следующего окна",
			"count", len(msgs), "err", err)
	}
	return errors.Join(errs...)
}

// flushAndLog runs a flush whose error has nowhere to go but the log.
func (s *sender) flushAndLog(ctx context.Context) {
	if err := s.Flush(ctx); err != nil {
		// Flush already logged each batch with its cause; this only records that
		// the cycle as a whole did not complete cleanly.
		s.log.Debug("mattermost: окно отправки закрыто с ошибками", "err", err)
	}
}

// onTimer is the wakeup for a window that closed with no further traffic.
func (s *sender) onTimer() {
	ctx, cancel := context.WithTimeout(context.Background(), timerFlushTimeout)
	defer cancel()
	s.flushAndLog(ctx)
}

// armLocked opens a window if none is open. Called with s.mu held.
func (s *sender) armLocked() {
	if !s.deadline.IsZero() {
		return
	}
	s.deadline = s.now().Add(s.window)
	// AfterFunc rather than a goroutine loop: nothing has to be stopped when the
	// sender is discarded with an empty buffer, so the channel needs no Close in
	// the contract.
	s.timer = time.AfterFunc(s.window, s.onTimer)
}

// take swaps the buffer out and closes the window.
func (s *sender) take() (map[notifychannel.Target][]notifychannel.Message, map[notifychannel.Target]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf, dropped := s.buf, s.dropped
	s.buf, s.dropped = nil, nil
	s.deadline = time.Time{}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	return buf, dropped
}

// requeue puts a transiently failed batch back in front of whatever arrived
// while it was being delivered, so the recipient still reads their updates in
// order, and reopens the window.
func (s *sender) requeue(target notifychannel.Target, msgs []notifychannel.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buf == nil {
		s.buf = map[notifychannel.Target][]notifychannel.Message{}
	}
	q := append(append([]notifychannel.Message{}, msgs...), s.buf[target]...)
	if over := len(q) - maxBuffered; over > 0 {
		if s.dropped == nil {
			s.dropped = map[notifychannel.Target]int{}
		}
		s.dropped[target] += over
		q = q[over:]
	}
	s.buf[target] = q
	s.armLocked()
}

// deliver sends one addressee's messages as a single post.
func (s *sender) deliver(ctx context.Context, target notifychannel.Target, msgs []notifychannel.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	if target.Email == "" {
		return permanent("mattermost: no email to address")
	}
	botID, err := s.resolveBotID(ctx)
	if err != nil {
		return err
	}
	var user struct {
		ID string `json:"id"`
	}
	// Метка эндпоинта вместо пути: путь здесь содержит адрес получателя.
	if err := s.call(ctx, http.MethodGet, "/api/v4/users/email/"+url.PathEscape(target.Email), "users/email", nil, &user); err != nil {
		return err
	}
	var dm struct {
		ID string `json:"id"`
	}
	if err := s.call(ctx, http.MethodPost, "/api/v4/channels/direct", "channels/direct", []string{botID, user.ID}, &dm); err != nil {
		return err
	}
	body := map[string]any{"channel_id": dm.ID, "message": formatBatch(msgs)}
	return s.call(ctx, http.MethodPost, "/api/v4/posts", "posts", body, nil)
}

func (s *sender) resolveBotID(ctx context.Context) (string, error) {
	s.mu.Lock()

	// If already cached on success, return it
	if s.botID != "" {
		id := s.botID
		s.mu.Unlock()
		return id, nil
	}

	// If a wave is in progress, wait for it
	if s.wave != nil {
		w := s.wave
		s.mu.Unlock()

		// Wait for the wave to complete or context to cancel.
		// All waiters share the same result; no retries within this wave.
		select {
		case <-w.done:
			return w.id, w.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	// No wave in progress and not cached: become the wave owner.
	// All concurrent callers will join this wave and share its result.
	w := &wave{done: make(chan struct{})}
	s.wave = w
	s.mu.Unlock()

	// Do network call without holding lock.
	// A hung request holds all concurrent callers for its duration—acceptable
	// because they'd fail with the same timeout anyway.
	var me struct {
		ID string `json:"id"`
	}
	err := s.call(ctx, http.MethodGet, "/api/v4/users/me", "users/me", nil, &me)

	// Record result in the wave
	s.mu.Lock()
	if err == nil {
		// Success: cache and record in wave
		w.id = me.ID
		s.botID = me.ID
	}
	w.err = err
	// Clear the wave pointer so next call starts fresh if this one failed
	s.wave = nil
	close(w.done)
	s.mu.Unlock()

	return w.id, w.err
}

// formatBatch renders what accumulated for one addressee as a single post.
// A lone message looks exactly as it did before batching existed; several get a
// counted header and are then rendered by the same format, so there is one
// source of wording rather than two that drift.
func formatBatch(msgs []notifychannel.Message) string {
	if len(msgs) == 1 {
		return format(msgs[0])
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**%d %s**", len(msgs), updatesWord(len(msgs)))
	for _, m := range msgs {
		b.WriteString("\n\n")
		b.WriteString(format(m))
	}
	return b.String()
}

// updatesWord picks the Russian plural form for the header count.
func updatesWord(n int) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return "обновлений"
	}
	switch n % 10 {
	case 1:
		return "обновление"
	case 2, 3, 4:
		return "обновления"
	default:
		return "обновлений"
	}
}

// openLinkLabel is what the link to the product reads as in a message.
const openLinkLabel = "Открыть в трекере"

// format renders the message as Markdown: bold title, body, then the link.
// The core already produced the wording; this only adds Mattermost's syntax.
func format(m notifychannel.Message) string {
	var b strings.Builder
	b.WriteString("**")
	b.WriteString(m.Title)
	b.WriteString("**")
	if m.Body != "" {
		b.WriteString("\n")
		b.WriteString(m.Body)
	}
	if m.URL != "" {
		// A Markdown link, not a bare address: the reader gets something to click
		// with a label instead of a query string. The core supplies an absolute URL
		// or none at all — a relative one would resolve against the messenger's own
		// host, which is why it never sends one.
		b.WriteString("\n[")
		b.WriteString(openLinkLabel)
		b.WriteString("](")
		b.WriteString(m.URL)
		b.WriteString(")")
	}
	return b.String()
}

// call выполняет запрос к API.
//
// endpoint — устойчивая метка ручки для сообщений об ошибках, отдельная от path.
// Разделение обязательное: путь адресации содержит адрес почты получателя, а
// ошибка канала уходит вызывающему и попадает в лог. Отлавливать адрес из готового
// текста шаблоном ненадёжно — принимаемые формы адреса шире любого разумного
// шаблона (однобуквенный TLD, интернационализованные домены), — поэтому адрес
// просто не попадает в текст.
func (s *sender) call(ctx context.Context, method, path, endpoint string, in, out any) error {
	var body *bytes.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return permanent("mattermost: encode %s: %v", endpoint, err)
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return permanent("mattermost: build request %s: %v", endpoint, err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.http.Do(req)
	if err != nil {
		// Network failures are transient by nature — the batch is worth keeping.
		return fmt.Errorf("mattermost: %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return fmt.Errorf("mattermost: decode %s: %w", endpoint, err)
			}
		}
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		// Rate limiting and server errors are worth retrying.
		return fmt.Errorf("mattermost: %s: status %d", endpoint, resp.StatusCode)
	default:
		// 4xx: a missing addressee, a revoked token, a bad request. Retrying the
		// same call cannot change the outcome.
		return permanent("mattermost: %s: status %d", endpoint, resp.StatusCode)
	}
}
