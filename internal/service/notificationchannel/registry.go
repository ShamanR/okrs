package notificationchannel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationchannels"
	"okrs/notifychannel"
)

// replaceFlushTimeout bounds the flush of a channel being replaced after its
// settings changed. Nothing is waiting on that flush, and the caller that
// triggered the replacement is on the delivery path, so it must not be held by an
// unreachable external service.
const replaceFlushTimeout = 30 * time.Second

// channelKey identifies one channel inside one tenant.
type channelKey struct {
	tenantID int64
	channel  string
}

// channelSlot is everything the registry keeps for one channel of one tenant.
//
// The slot, not the sender inside it, is the channel's identity here. It outlives
// any particular instance: retire empties a slot instead of removing it. That is
// what keeps build a stable lock — a goroutine already waiting on it must not
// find that a newcomer created a second mutex for the same channel, because two
// mutexes exclude nobody.
//
// Two locks guard a slot, and they differ in how long they are held, which is the
// whole reason they are separate:
//
//   - build is held across a row read and across ch.New — the tenant's own code.
//     Neither may run under a lock that unrelated tenants need.
//   - sender and fingerprint are guarded by Service.mu and touched only for the
//     moment it takes to read or swap them, so the read path never waits behind a
//     construction that is still in flight.
type channelSlot struct {
	build sync.Mutex

	// Guarded by Service.mu. A nil sender means the slot is empty: never built,
	// or retired after a save.
	sender      notifychannel.Sender
	fingerprint string
}

// slotLocked returns the channel's slot, creating it on first use.
// The caller holds Service.mu.
func (s *Service) slotLocked(key channelKey) *channelSlot {
	if s.slots == nil {
		s.slots = map[channelKey]*channelSlot{}
	}
	slot, ok := s.slots[key]
	if !ok {
		slot = &channelSlot{}
		s.slots[key] = slot
	}
	return slot
}

func (s *Service) slotFor(key channelKey) *channelSlot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.slotLocked(key)
}

// lookup returns the channel's slot and, when it already holds an instance built
// from this very configuration, that instance.
func (s *Service) lookup(key channelKey, fp string) (*channelSlot, notifychannel.Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.slotLocked(key)
	if slot.sender != nil && slot.fingerprint == fp {
		return slot, slot.sender
	}
	return slot, nil
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
	scope domain.TenantScope,
	key channelKey,
	ch notifychannel.Channel,
	fp string,
	requireEnabled bool,
) (notifychannel.Sender, error) {
	slot, cached := s.lookup(key, fp)
	if cached != nil {
		return cached, nil
	}

	sender, replaced, err := s.construct(ctx, scope, key, ch, slot, requireEnabled)
	if err != nil {
		return nil, err
	}
	// Outside the construction lock on purpose: a flush is network I/O with a
	// 30-second budget, and holding the lock across it would stall any Save for
	// this channel — retire waits on the same lock — for as long as an
	// unreachable server takes to time out.
	if replaced != nil {
		s.flushReplaced(ctx, key, replaced)
	}
	return sender, nil
}

// construct builds and installs the instance, returning the one it displaced so
// the caller can flush it without holding the construction lock.
//
// The stored row is read again HERE, inside the serialization, and that — not the
// serialization by itself — is what makes the outcome well-defined. Serializing
// alone would not: two callers can read rows in one order and reach the lock in
// the other, so the one holding the OLDER row could still install last and leave
// the registry on revoked credentials. Nothing corrects that until the next
// lookup, and for a channel that was switched off there is no next lookup.
// Re-reading under the lock means whoever installs last also read last.
func (s *Service) construct(
	ctx context.Context,
	scope domain.TenantScope,
	key channelKey,
	ch notifychannel.Channel,
	slot *channelSlot,
	requireEnabled bool,
) (sender, replaced notifychannel.Sender, err error) {
	slot.build.Lock()
	defer slot.build.Unlock()

	row, ok, err := s.repo.Get(ctx, scope, key.channel)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrNotConfigured
	}
	// The authoritative read has to re-check this too, not only the caller's
	// earlier one: the whole point of reading again under the construction lock is
	// that the row may have moved since. A channel switched off in that window
	// must not get a freshly built instance to buffer new work into.
	if requireEnabled && !row.Enabled {
		return nil, nil, ErrNotEnabled
	}
	// live's fingerprint was only a hint for the lock-free fast path; this read is
	// the authoritative one.
	fresh := fingerprint(row)
	settings, err := s.settingsFor(ch, row, key.channel)
	if err != nil {
		return nil, nil, err
	}

	// Someone may have installed exactly this configuration while this call was
	// waiting for the construction lock.
	s.mu.Lock()
	if slot.sender != nil && slot.fingerprint == fresh {
		cur := slot.sender
		s.mu.Unlock()
		return cur, nil, nil
	}
	s.mu.Unlock()

	// Constructed while holding only the construction lock, never the registry
	// lock: a channel constructor is the tenant's code path, not ours, and holding
	// the registry across it would let one slow channel stall every other tenant.
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
		return nil, nil, fmt.Errorf("notificationchannel: %s: %w: %w", key.channel, ErrInvalidConfig, err)
	}
	// maskSecretErrors is a second barrier, not a substitute for a channel's own
	// care: see notifychannel.Sender's doc comment and secretmask.go.
	wrapped := maskSecretErrors(built, settings.Secret)

	s.mu.Lock()
	replaced = slot.sender
	slot.sender, slot.fingerprint = wrapped, fresh
	s.mu.Unlock()

	return wrapped, replaced, nil
}

// flushReplaced delivers whatever the outgoing instance still holds, using the
// settings it was built with. Anything that slips into it between the swap and
// this call is not lost either: the instance's own window timer still holds a
// reference to it and fires on schedule.
func (s *Service) flushReplaced(ctx context.Context, key channelKey, replaced notifychannel.Sender) {
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replaceFlushTimeout)
	defer cancel()
	if err := replaced.Flush(flushCtx); err != nil && s.logger != nil {
		s.logger.Warn("notificationchannel: не удалось выгрузить накопленное перед сменой настроек канала",
			"channel", key.channel, "tenant_id", key.tenantID, "err", err)
	}
}

// retire empties this tenant's slot for a channel and delivers whatever the
// instance in it still held.
//
// Called when the configuration is saved. Replacement in live() is lazy — it
// happens the next time someone asks for a sender — and that is enough only while
// the channel keeps being asked for. A channel the administrator just switched
// off never is: nothing replaces the instance, and its window timer still fires
// and posts the buffer under settings that were already revoked.
//
// The buffer is delivered rather than dropped, per the channel spec: updates
// already accepted must not vanish because a setting changed a moment later.
func (s *Service) retire(ctx context.Context, key channelKey) {
	slot := s.slotFor(key)

	// The construction lock, not only the registry lock. A construction that
	// started before this save must finish and install before the slot is
	// emptied — otherwise it fills the slot we just cleared and the registry is
	// left holding settings that were already replaced. Any construction that
	// starts after this point reads the row this save wrote.
	slot.build.Lock()
	s.mu.Lock()
	outgoing := slot.sender
	slot.sender, slot.fingerprint = nil, ""
	s.mu.Unlock()
	slot.build.Unlock()

	// Flushed outside both locks: an unreachable external service must not hold
	// up the next construction for this channel.
	if outgoing != nil {
		s.flushReplaced(ctx, key, outgoing)
	}
}

// Flush delivers everything every live channel still holds.
//
// The shutdown path calls it: a channel buffers in memory, so a replica that
// exits without this loses whatever its window had not yet sent. The caller
// supplies the deadline — an orderly shutdown has a budget, and an unreachable
// external service must not spend all of it.
func (s *Service) Flush(ctx context.Context) error {
	type pending struct {
		key    channelKey
		sender notifychannel.Sender
	}

	s.mu.Lock()
	live := make([]pending, 0, len(s.slots))
	for key, slot := range s.slots {
		if slot.sender != nil {
			live = append(live, pending{key: key, sender: slot.sender})
		}
	}
	s.mu.Unlock()

	var errs []error
	for _, p := range live {
		if err := p.sender.Flush(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.key.channel, err))
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
