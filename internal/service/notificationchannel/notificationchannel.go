// Package notificationchannel owns everything about a channel's configuration in a
// tenant: which channels this build has, which of them the tenant may use, what is
// stored for each, and how to turn that into a ready Sender.
//
// Whether a tenant may use a channel is a two-level question, and the two levels
// answer different things (see channelAvailable):
//
//   - ChannelGrants says whether a system administrator explicitly assigned this
//     channel to this tenant — an administrative fact entered by hand, stored as a
//     tenant_settings entitlement.* key.
//   - entitlements.Entitlements says whether the tenant's tariff unlocks the class
//     of capability the channel belongs to. In the OSS build this answers true to
//     everything, which is correct for a tariff-class question ("sso", "subdomains")
//     but would make the gate decorative if it were also trusted to answer "was this
//     specific channel assigned" — that is not a question about the tariff at all.
//
// Both must hold for a channel to be available. Neither overrides the other.
//
// It is the only place where a channel secret exists in plaintext. The store keeps
// it encrypted, the API returns only a mask, and the channel receives it already
// decrypted — so no other layer has to be trusted with it.
package notificationchannel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"okrs/internal/core/domain"
	"okrs/internal/platform/entitlements"
	"okrs/internal/platform/secretbox"
	"okrs/internal/store/notificationchannels"
	"okrs/notifychannel"
)

var (
	// ErrUnknownChannel: the name is not in this build at all.
	//
	// An API surfaced to a tenant admin (task 7) must answer this and
	// ErrNotAvailable identically: telling them apart would let an admin
	// enumerate channel names to discover which ones exist but were not
	// granted to their tenant.
	ErrUnknownChannel = errors.New("notificationchannel: unknown channel")
	// ErrNotAvailable: the channel exists but this tenant has not been granted it.
	//
	// See ErrUnknownChannel: a tenant-facing API must collapse the two into one
	// response.
	ErrNotAvailable = errors.New("notificationchannel: channel not available for this tenant")
	// ErrNoSecretKey: the deployment has no encryption key, so a channel that
	// requires a secret cannot be configured.
	ErrNoSecretKey = errors.New("notificationchannel: no encryption key configured")
	// ErrNotConfigured: the tenant has never saved this channel's settings.
	ErrNotConfigured = errors.New("notificationchannel: channel not configured")
	// ErrSecretRequired: the channel needs a secret to run, and Save was asked to
	// turn it on with neither a new secret nor one already stored. Storing it
	// disabled with no secret is legitimate; storing it enabled is not — that
	// would sit in the database as "on" while unable to send anything, and the
	// gap would only surface at delivery time in the 2a-2 worker.
	ErrSecretRequired = errors.New("notificationchannel: secret required to enable this channel")
	// ErrFieldRequired: Save was asked to enable a channel while one of its
	// Descriptor.Fields marked Required is empty. Same reasoning as
	// ErrSecretRequired, applied to the rest of the form: a row stored as "on"
	// with no base URL is a channel that cannot send anything, and the gap would
	// surface only at delivery time. Errors carrying it are *FieldRequiredError,
	// which names the field so the admin is told what to fill in.
	ErrFieldRequired = errors.New("notificationchannel: required field is empty")
	// ErrInvalidConfig: the channel's own constructor refused what the tenant
	// stored. That is a configuration problem the admin can fix, not a server
	// fault, and the API layer answers it as such rather than as a 500.
	ErrInvalidConfig = errors.New("notificationchannel: stored configuration rejected by the channel")
)

// FieldRequiredError names the empty field, so the API can say which one to fill
// in instead of "something is missing". Unwraps to ErrFieldRequired, so callers
// that only care about the class keep using errors.Is.
type FieldRequiredError struct {
	Channel string
	Key     string
	Label   string
}

func (e *FieldRequiredError) Error() string {
	return fmt.Sprintf("notificationchannel: %s: field %q is required", e.Channel, e.Key)
}

func (e *FieldRequiredError) Unwrap() error { return ErrFieldRequired }

// Repo is the port this service needs, declared consumer-side per specs/010.
type Repo interface {
	List(ctx context.Context, scope domain.TenantScope) ([]notificationchannels.Config, error)
	Get(ctx context.Context, scope domain.TenantScope, channel string) (notificationchannels.Config, bool, error)
	Upsert(ctx context.Context, scope domain.TenantScope, c notificationchannels.Config, byUserID int64) error
}

// ChannelGrants is the port to the tenant's explicit channel assignments: the
// administrative data a system administrator entered by hand through the system
// panel, distinct in kind from the tariff-class answer entitlements.Entitlements
// gives (see the package doc comment). Declared consumer-side per specs/010.
//
// settings.Service.TenantEntitlements satisfies this directly: it already reads
// the entitlement.* keys with the prefix stripped, through the tenant snapshot
// cache, so this port asks for nothing settings does not already provide.
type ChannelGrants interface {
	TenantEntitlements(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error)
}

// ChannelState is one channel as the admin UI sees it: what it is, whether the
// tenant switched it on, and what is stored — with the secret reduced to a mask.
type ChannelState struct {
	Descriptor notifychannel.Descriptor
	Enabled    bool
	Configured bool
	Values     map[string]any
	SecretHint string
}

// SaveInput is one channel's configuration as submitted by a tenant admin.
// An empty Secret means "leave the stored one alone" — the form shows a mask,
// so a user editing only a URL must not lose the token.
type SaveInput struct {
	Channel string
	Enabled bool
	Values  map[string]any
	Secret  string
}

type Service struct {
	repo     Repo
	box      *secretbox.Box // nil when the deployment configured no key
	channels map[string]notifychannel.Channel
	order    []string // build order, so the UI lists channels deterministically
	ent      entitlements.Entitlements
	grants   ChannelGrants
}

// New assembles the service. A duplicate Descriptor.Name is an assembly error
// rather than a silent overwrite: two channels answering to one name would make
// which one you configured depend on map iteration order.
func New(repo Repo, box *secretbox.Box, channels []notifychannel.Channel, ent entitlements.Entitlements, grants ChannelGrants) (*Service, error) {
	if grants == nil {
		return nil, errors.New("notificationchannel: nil ChannelGrants")
	}
	s := &Service{repo: repo, box: box, channels: map[string]notifychannel.Channel{}, ent: ent, grants: grants}
	for _, ch := range channels {
		name := ch.Descriptor.Name
		if name == "" {
			return nil, errors.New("notificationchannel: channel with empty Descriptor.Name")
		}
		if ch.New == nil {
			return nil, fmt.Errorf("notificationchannel: channel %q has a nil constructor", name)
		}
		if _, dup := s.channels[name]; dup {
			return nil, fmt.Errorf("notificationchannel: duplicate channel name %q", name)
		}
		if err := checkSecretField(ch.Descriptor); err != nil {
			return nil, err
		}
		s.channels[name] = ch
		s.order = append(s.order, name)
	}
	return s, nil
}

// checkSecretField enforces the contract between Descriptor.SecretField and the
// fields declared with Kind == FieldSecret. Only SecretField is encrypted and
// stripped from config_json, so a second secret-kind field — or one whose key the
// descriptor never names — would be written to the database in plaintext. Nothing
// downstream would notice: the API strips every secret-kind field before the wire,
// so the leak would stay invisible while the row sits unencrypted. No channel in
// this build can hit it (Mattermost has exactly one), but Descriptor is a public
// contract for channels written elsewhere, and an assembly error is the cheapest
// possible place to say so.
func checkSecretField(d notifychannel.Descriptor) error {
	var secretKeys []string
	for _, f := range d.Fields {
		if f.Kind == notifychannel.FieldSecret {
			secretKeys = append(secretKeys, f.Key)
		}
	}
	switch {
	case len(secretKeys) > 1:
		return fmt.Errorf("notificationchannel: channel %q declares %d fields with Kind=secret (%v); only the one named by Descriptor.SecretField is encrypted, so at most one is allowed",
			d.Name, len(secretKeys), secretKeys)
	case d.SecretField == "" && len(secretKeys) == 1:
		return fmt.Errorf("notificationchannel: channel %q declares field %q with Kind=secret but leaves Descriptor.SecretField empty; it would be stored unencrypted",
			d.Name, secretKeys[0])
	case d.SecretField != "" && len(secretKeys) == 0:
		return fmt.Errorf("notificationchannel: channel %q names Descriptor.SecretField %q but declares no field with Kind=secret",
			d.Name, d.SecretField)
	case d.SecretField != "" && secretKeys[0] != d.SecretField:
		return fmt.Errorf("notificationchannel: channel %q names Descriptor.SecretField %q but its secret field is %q",
			d.Name, d.SecretField, secretKeys[0])
	}
	return nil
}

// entitlementKey builds the tariff-level key checked as the second condition of
// channelAvailable. Note the asymmetry with the system panel, which writes the
// bare form: provisioning.SetEntitlements adds the "entitlement." prefix, so what
// is written as "notifications.mattermost" is read back as
// "entitlement.notifications.mattermost".
func entitlementKey(name string) string { return "entitlement.notifications." + name }

// grantPrefix marks the bare (already entitlement.-stripped) tenant_settings keys
// that record an explicit channel assignment, e.g. "notifications.mattermost".
const grantPrefix = "notifications."

// grantedChannels reads the tenant's explicit channel assignments once and
// returns the set of channel names granted true. Fetched once per call so a
// screen that renders every channel (Available, and List through it) does not
// turn into one settings read per channel.
//
// A value other than JSON true — false, or anything malformed — is read as "not
// granted": an admin who once granted a channel and later switched the toggle off
// must see it disappear, not keep it available because the key still exists.
func (s *Service) grantedChannels(ctx context.Context, scope domain.TenantScope) (map[string]bool, error) {
	raw, err := s.grants.TenantEntitlements(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(raw))
	for k, v := range raw {
		name, ok := strings.CutPrefix(k, grantPrefix)
		if !ok {
			continue
		}
		var on bool
		if err := json.Unmarshal(v, &on); err == nil && on {
			out[name] = true
		}
	}
	return out, nil
}

// channelAvailable is the two-level gate itself: a channel is available to a
// tenant only if BOTH hold —
//
//  1. it was explicitly granted (an administrative assignment a system
//     administrator entered by hand, read through ChannelGrants), and
//  2. the tenant's tariff does not forbid it (entitlements.Entitlements.Has).
//
// See the package doc comment for why these are kept as two independent
// conditions rather than collapsed into one: they answer different questions, and
// a permissive tariff implementation (OSS "unlimited") must not make an
// assignment nobody made appear granted.
func (s *Service) channelAvailable(ctx context.Context, scope domain.TenantScope, name string) (bool, error) {
	granted, err := s.grantedChannels(ctx, scope)
	if err != nil {
		return false, err
	}
	return granted[name] && s.ent.Has(scope, entitlementKey(name)), nil
}

// Descriptors returns every channel compiled into this build, granted or not.
// The system panel needs the full list — that is what it grants from.
func (s *Service) Descriptors() []notifychannel.Descriptor {
	out := make([]notifychannel.Descriptor, 0, len(s.order))
	for _, name := range s.order {
		out = append(out, s.channels[name].Descriptor)
	}
	return out
}

// IsAvailable reports whether the tenant may use this channel at all.
func (s *Service) IsAvailable(ctx context.Context, scope domain.TenantScope, name string) (bool, error) {
	if _, ok := s.channels[name]; !ok {
		return false, nil
	}
	return s.channelAvailable(ctx, scope, name)
}

// Available returns only the channels this tenant was granted and whose tariff
// permits them. The tenant admin screen renders from this, so a channel the
// tenant does not have never appears — no locked cards, no upsell (design spec
// §13.4).
func (s *Service) Available(ctx context.Context, scope domain.TenantScope) ([]notifychannel.Descriptor, error) {
	granted, err := s.grantedChannels(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make([]notifychannel.Descriptor, 0, len(s.order))
	for _, name := range s.order {
		if granted[name] && s.ent.Has(scope, entitlementKey(name)) {
			out = append(out, s.channels[name].Descriptor)
		}
	}
	return out, nil
}

// List returns the available channels together with what the tenant stored.
func (s *Service) List(ctx context.Context, scope domain.TenantScope) ([]ChannelState, error) {
	rows, err := s.repo.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]notificationchannels.Config, len(rows))
	for _, r := range rows {
		stored[r.Channel] = r
	}

	avail, err := s.Available(ctx, scope)
	if err != nil {
		return nil, err
	}
	var out []ChannelState
	for _, d := range avail {
		st := ChannelState{Descriptor: d, Values: map[string]any{}}
		if row, ok := stored[d.Name]; ok {
			st.Configured = true
			st.Enabled = row.Enabled
			st.SecretHint = row.SecretHint
			st.Values = sanitize(row.Values, d)
		}
		out = append(out, st)
	}
	return out, nil
}

// sanitize drops the secret field from the values that leave the server. The
// secret is stored encrypted in its own column, but a channel could also have
// written it into config_json by mistake; stripping it here means one bug cannot
// become a leak.
func sanitize(values map[string]any, d notifychannel.Descriptor) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		if d.SecretField != "" && k == d.SecretField {
			continue
		}
		out[k] = v
	}
	return out
}

// EnabledNames returns the channels the tenant has both been granted and switched
// on. Phase 2a-2's fan-out and the user settings screen read this.
func (s *Service) EnabledNames(ctx context.Context, scope domain.TenantScope) ([]string, error) {
	states, err := s.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, st := range states {
		if st.Enabled {
			out = append(out, st.Descriptor.Name)
		}
	}
	return out, nil
}

// isBlank decides whether a submitted value counts as "not filled in". Values
// arrive as decoded JSON, so a text field can be absent, null, or a string of
// spaces — all three are the same thing to the admin looking at an empty input.
// Any other type (a bool, a number) is a value the form did produce, false and 0
// included, so it is not blank.
func isBlank(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	default:
		return false
	}
}

// Save validates and stores one channel's configuration.
func (s *Service) Save(ctx context.Context, scope domain.TenantScope, in SaveInput, byUserID int64) error {
	ch, ok := s.channels[in.Channel]
	if !ok {
		return ErrUnknownChannel
	}
	avail, err := s.channelAvailable(ctx, scope, in.Channel)
	if err != nil {
		return err
	}
	if !avail {
		return ErrNotAvailable
	}
	if in.Enabled {
		// Checked before touching the database: an incomplete form cannot become a
		// stored row, and there is nothing to read for a request that is already
		// rejected. The secret is excluded here because it has its own rule — an
		// empty secret means "keep the stored one", so emptiness alone says nothing;
		// that case is decided further down and reported as ErrSecretRequired.
		for _, f := range ch.Descriptor.Fields {
			if !f.Required || f.Kind == notifychannel.FieldSecret {
				continue
			}
			if isBlank(in.Values[f.Key]) {
				return &FieldRequiredError{Channel: in.Channel, Key: f.Key, Label: f.Label}
			}
		}
	}

	prev, hadPrev, err := s.repo.Get(ctx, scope, in.Channel)
	if err != nil {
		return err
	}

	row := notificationchannels.Config{
		Channel: in.Channel,
		Enabled: in.Enabled,
		Values:  sanitize(in.Values, ch.Descriptor),
	}

	switch {
	case ch.Descriptor.SecretField == "":
		// Channel has no secret; nothing to encrypt.
	case in.Secret != "":
		if s.box == nil {
			return ErrNoSecretKey
		}
		enc, err := s.box.Seal(in.Secret)
		if err != nil {
			return err
		}
		row.SecretEnc = enc
		row.SecretHint = secretbox.Hint(in.Secret)
	case hadPrev && len(prev.SecretEnc) > 0:
		// Empty secret, but the stored row has a real one: keep it. The form
		// shows a mask, so a user editing only a URL must not silently lose
		// the token. A previous row that exists but was itself saved without a
		// secret (disabled) does NOT count here — otherwise Enabled: false
		// then Enabled: true, both with an empty Secret, would enable a channel
		// that never had a secret at all.
		row.SecretEnc = prev.SecretEnc
		row.SecretHint = prev.SecretHint
	case in.Enabled:
		// No new secret, none stored, and the caller wants it on: refuse. Storing
		// it enabled anyway would sit in the database as "on" while unable to
		// send anything, and the gap would only surface at delivery time.
		return ErrSecretRequired
	default:
		// No secret at all, but also not being enabled: storing it disabled is
		// legitimate, so nothing to do here.
	}

	return s.repo.Upsert(ctx, scope, row, byUserID)
}

// Sender builds a ready-to-use Sender from the tenant's stored configuration.
func (s *Service) Sender(ctx context.Context, scope domain.TenantScope, name string) (notifychannel.Sender, error) {
	ch, ok := s.channels[name]
	if !ok {
		return nil, ErrUnknownChannel
	}
	avail, err := s.channelAvailable(ctx, scope, name)
	if err != nil {
		return nil, err
	}
	if !avail {
		return nil, ErrNotAvailable
	}
	row, ok, err := s.repo.Get(ctx, scope, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotConfigured
	}

	settings := notifychannel.Settings{Values: row.Values}
	if ch.Descriptor.SecretField != "" && len(row.SecretEnc) > 0 {
		if s.box == nil {
			return nil, ErrNoSecretKey
		}
		secret, err := s.box.Open(row.SecretEnc)
		if err != nil {
			return nil, fmt.Errorf("notificationchannel: decrypt %s: %w", name, err)
		}
		settings.Secret = secret
	}
	sender, err := ch.New(settings)
	if err != nil {
		// Marked as a configuration problem, not passed through bare: the caller
		// cannot otherwise tell "this tenant stored something the channel will not
		// accept" from a genuine server fault, and the two deserve different answers.
		// The original error stays reachable through errors.Is/As.
		return nil, fmt.Errorf("notificationchannel: %s: %w: %w", name, ErrInvalidConfig, err)
	}
	// maskSecretErrors is a second barrier, not a substitute for a channel's own
	// care: see notifychannel.Sender's doc comment and secretmask.go.
	return maskSecretErrors(sender, settings.Secret), nil
}
