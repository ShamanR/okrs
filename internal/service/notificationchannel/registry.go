package notificationchannel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"okrs/internal/store/notificationchannels"
	"okrs/notifychannel"
)

// replaceFlushTimeout bounds the flush of a channel being replaced after its
// settings changed. Nothing is waiting on that flush, and the caller that
// triggered the replacement is on the delivery path, so it must not be held by an
// unreachable external service.
const replaceFlushTimeout = 30 * time.Second

// channelKey identifies one live channel instance: a channel name inside one tenant.
type channelKey struct {
	tenantID int64
	channel  string
}

// liveChannel is a constructed channel together with a fingerprint of the stored
// configuration it was built from.
type liveChannel struct {
	sender      notifychannel.Sender
	fingerprint string
}

// live returns this tenant's instance of the channel, constructing one only when
// there is none yet or when the stored configuration has changed underneath it.
//
// The instance has to outlive a single delivery. A channel that batches keeps its
// pending messages inside itself, so rebuilding it per message would create a
// buffer and immediately discard it; the Mattermost channel's cached bot id is
// the same story one layer down — it was written to be resolved once and reused,
// which never happened while every call produced a fresh instance.
func (s *Service) live(
	ctx context.Context,
	key channelKey,
	ch notifychannel.Channel,
	settings notifychannel.Settings,
	fp string,
) (notifychannel.Sender, error) {
	s.mu.Lock()
	if cur, ok := s.instances[key]; ok && cur.fingerprint == fp {
		s.mu.Unlock()
		return cur.sender, nil
	}
	s.mu.Unlock()

	// Constructed outside the lock: a channel constructor is the tenant's code
	// path, not ours, and holding the registry lock across it would let one slow
	// channel stall every other tenant.
	built, err := ch.New(notifychannel.Deps{
		Settings: settings,
		// The channel reports delivery failures itself — Send has no caller left
		// by the time delivery happens. It gets a logger that cannot write the
		// secret, because the core is the only layer holding the plaintext.
		Logger: scrubbingLogger(s.logger, settings.Secret),
		Now:    s.now,
	})
	if err != nil {
		// Marked as a configuration problem, not passed through bare: the caller
		// cannot otherwise tell "this tenant stored something the channel will not
		// accept" from a genuine server fault, and the two deserve different answers.
		// The original error stays reachable through errors.Is/As.
		return nil, fmt.Errorf("notificationchannel: %s: %w: %w", key.channel, ErrInvalidConfig, err)
	}
	// maskSecretErrors is a second barrier, not a substitute for a channel's own
	// care: see notifychannel.Sender's doc comment and secretmask.go.
	wrapped := maskSecretErrors(built, settings.Secret)

	s.mu.Lock()
	// Another goroutine may have installed an equivalent instance while this one
	// was constructing. Prefer theirs: two live buffers for one channel would
	// split a tenant's pending updates across both.
	if cur, ok := s.instances[key]; ok && cur.fingerprint == fp {
		s.mu.Unlock()
		return cur.sender, nil
	}
	replaced := s.instances[key]
	if s.instances == nil {
		s.instances = map[channelKey]*liveChannel{}
	}
	s.instances[key] = &liveChannel{sender: wrapped, fingerprint: fp}
	s.mu.Unlock()

	if replaced != nil {
		s.flushReplaced(ctx, key, replaced)
	}
	return wrapped, nil
}

// flushReplaced delivers whatever the outgoing instance still holds, using the
// settings it was built with. Anything that slips into it between the swap and
// this call is not lost either: the instance's own window timer still holds a
// reference to it and fires on schedule.
func (s *Service) flushReplaced(ctx context.Context, key channelKey, replaced *liveChannel) {
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replaceFlushTimeout)
	defer cancel()
	if err := replaced.sender.Flush(flushCtx); err != nil && s.logger != nil {
		s.logger.Warn("notificationchannel: не удалось выгрузить накопленное перед сменой настроек канала",
			"channel", key.channel, "tenant_id", key.tenantID, "err", err)
	}
}

// Flush delivers everything every live channel still holds.
//
// The shutdown path calls it: a channel buffers in memory, so a replica that
// exits without this loses whatever its window had not yet sent. The caller
// supplies the deadline — an orderly shutdown has a budget, and an unreachable
// external service must not spend all of it.
func (s *Service) Flush(ctx context.Context) error {
	s.mu.Lock()
	pending := make([]*liveChannel, 0, len(s.instances))
	keys := make([]channelKey, 0, len(s.instances))
	for k, c := range s.instances {
		keys = append(keys, k)
		pending = append(pending, c)
	}
	s.mu.Unlock()

	var errs []error
	for i, c := range pending {
		if err := c.sender.Flush(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", keys[i].channel, err))
		}
	}
	return errors.Join(errs...)
}

// fingerprint summarises the stored configuration an instance was built from, so
// a later read can tell whether the tenant changed anything.
//
// Everything the constructor reads goes in. UpdatedAt alone would be tempting and
// almost always sufficient, but a row rewritten within the same timestamp
// resolution would then look unchanged, and the failure mode — a channel quietly
// running on a revoked token — is not one worth risking for a few bytes.
func fingerprint(c notificationchannels.Config) string {
	sum := sha256.New()
	// json.Marshal sorts map keys, so equal configurations hash equally.
	payload, err := json.Marshal(struct {
		Enabled   bool           `json:"enabled"`
		Values    map[string]any `json:"values"`
		SecretEnc []byte         `json:"secret_enc"`
		UpdatedAt time.Time      `json:"updated_at"`
	}{c.Enabled, c.Values, c.SecretEnc, c.UpdatedAt})
	if err != nil {
		// Unmarshalable settings cannot be fingerprinted, so treat the
		// configuration as changed every time rather than as unchanged: rebuilding
		// too often is wasteful, running on stale settings is wrong.
		return fmt.Sprintf("unhashable-%d", time.Now().UnixNano())
	}
	sum.Write(payload)
	return hex.EncodeToString(sum.Sum(nil))
}
