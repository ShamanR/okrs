// Package mattermost delivers notifications as Mattermost direct messages.
//
// It addresses by email, so it needs no account-linking step and does not
// implement notifychannel.Linker. Public on purpose: channels are wired through
// app.Config.NotificationChannels next to main, and this package doubles as the
// worked example for a channel written in another repository.
package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"okrs/notifychannel"
)

// permanentError marks a failure that retrying cannot fix — an addressee with no
// Mattermost account, a malformed request. The delivery worker uses this to stop
// retrying instead of burning six attempts on a certainty.
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
			},
		},
		New: newSender,
	}
}

type sender struct {
	baseURL string
	token   string
	http    *http.Client

	// botID is resolved once on success and reused: the delivery worker sends in batches, and
	// re-asking who we are on every message is an N+1 over the network.
	// If resolution fails (temporary error), retry on next Send.
	// Multiple concurrent Send calls coalesce on the same wave: the first fetches, others wait.
	// On error, all waiters share the error and complete immediately—no sequential queueing.
	// On success, botID is cached; the wave is discarded and next Send starts fresh if needed.
	mu    sync.Mutex
	botID string // cached only on success
	wave  *wave  // current in-flight resolution, if any
}

func newSender(s notifychannel.Settings) (notifychannel.Sender, error) {
	raw, _ := s.Values["base_url"].(string)
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
	if s.Secret == "" {
		return nil, notifychannel.ErrMissingSecret
	}
	return &sender{
		baseURL: raw,
		token:   s.Secret,
		http:    &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (s *sender) Send(ctx context.Context, target notifychannel.Target, msg notifychannel.Message) error {
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
	body := map[string]any{"channel_id": dm.ID, "message": format(msg)}
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
		b.WriteString("\n")
		b.WriteString(m.URL)
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
		// Network failures are transient by nature — the worker should retry.
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
