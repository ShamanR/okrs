// Package notificationpref is the notification-preferences entity service: reads,
// validated writes, and recipient resolution for the fan-out.
package notificationpref

import (
	"context"
	"errors"
	"slices"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationprefs"
)

var (
	ErrInvalidType    = errors.New("notificationpref: unknown notification type")
	ErrInvalidScope   = errors.New("notificationpref: unknown scope")
	ErrInvalidChannel = errors.New("notificationpref: unknown channel")
)

// Repo is the port this service needs. Declared consumer-side, per specs/010.
type Repo interface {
	GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error)
	Set(ctx context.Context, scope domain.TenantScope, userID int64, p notificationprefs.Preference) error
	ResolveRecipients(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error)
	ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error)
}

// Channels is the port to the tenant's external delivery channels, declared
// consumer-side per specs/010: channel name → "on by default for staff who never
// chose".
//
// A plain map rather than a shared struct on purpose. service/notificationchannel
// satisfies this method as written, so neither service has to import the other's
// types for a question this small. nil is a legitimate implementation for a build
// with no channels at all — the bell needs none of this.
type Channels interface {
	DeliveryChannelDefaults(ctx context.Context, scope domain.TenantScope) (map[string]bool, error)
}

type Service struct {
	repo     Repo
	channels Channels
}

func New(repo Repo, channels Channels) *Service {
	return &Service{repo: repo, channels: channels}
}

// DeliveryDefaults returns every channel this tenant delivers to, mapped to
// whether it is on for a user who never chose.
//
// The bell is always present and always defaults to on: it costs nothing, it is
// where notifications have always appeared, and a tenant cannot be left with a
// notification system that delivers nowhere by default.
func (s *Service) DeliveryDefaults(ctx context.Context, scope domain.TenantScope) (map[string]bool, error) {
	out := map[string]bool{notificationprefs.ChannelInApp: true}
	if s.channels == nil {
		return out, nil
	}
	external, err := s.channels.DeliveryChannelDefaults(ctx, scope)
	if err != nil {
		return nil, err
	}
	for name, on := range external {
		if name == notificationprefs.ChannelInApp {
			// A build must not shadow the bell with an external channel of the
			// same name: the two behave differently everywhere downstream.
			continue
		}
		out[name] = on
	}
	return out, nil
}

// EffectiveChannels resolves where one recipient's notification actually goes.
// Thin re-export of the store-level rule so callers of this service need not
// reach past it; the rule itself lives next to the types it resolves.
func EffectiveChannels(defaults map[string]bool, overrides map[string]bool) []string {
	return notificationprefs.EffectiveChannels(defaults, overrides)
}

func (s *Service) GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error) {
	return s.repo.GetAll(ctx, scope, userID)
}

// normalize validates one preference and fills in the defaults the client may
// omit. Pure: it writes nothing, which is what lets SetAll check a whole matrix
// before touching the store.
//
// allowed is the tenant's delivery channels. An override naming anything else is
// refused rather than stored: a preference about a channel the tenant cannot use
// would sit in the database looking meaningful and silently start applying if
// that name ever became real.
func normalize(p notificationprefs.Preference, allowed map[string]bool) (notificationprefs.Preference, error) {
	if !slices.Contains(notificationprefs.AllTypes, p.Type) {
		return p, ErrInvalidType
	}
	if notificationprefs.IsAddressed(p.Type) {
		// Scope is meaningless for an addressed type; drop whatever the client sent.
		p.Scope = ""
	} else {
		if p.Scope == "" {
			p.Scope = notificationprefs.ScopeOwn
		}
		valid := []string{notificationprefs.ScopeOwn, notificationprefs.ScopeOwnAndChildren, notificationprefs.ScopeSubtree}
		if !slices.Contains(valid, p.Scope) {
			return p, ErrInvalidScope
		}
	}
	for name := range p.ChannelOverrides {
		if _, ok := allowed[name]; !ok {
			return p, ErrInvalidChannel
		}
	}
	if p.ChannelOverrides == nil {
		p.ChannelOverrides = map[string]bool{}
	}
	return p, nil
}

func (s *Service) Set(ctx context.Context, scope domain.TenantScope, userID int64, p notificationprefs.Preference) error {
	allowed, err := s.DeliveryDefaults(ctx, scope)
	if err != nil {
		return err
	}
	p, err = normalize(p, allowed)
	if err != nil {
		return err
	}
	// Same reduction as SetAll: a choice that matches the tenant default is stored
	// as no choice at all, so a later change of that default still reaches this
	// user. The two entry points must not disagree about what "on" means.
	p.ChannelOverrides = deviations(p.ChannelOverrides, allowed)
	return s.repo.Set(ctx, scope, userID, p)
}

// SetAll writes a whole preferences matrix, validating every row BEFORE writing any
// of them. Validating as it writes would leave the earlier rows applied when a later
// one is rejected: the caller is told the matrix was refused while half of it already
// took effect, and the settings screen then shows a state the user never asked for.
//
// The writes themselves are still separate statements, so a store failure midway can
// still land a partial matrix — that needs a transactional repository method and is
// recorded as debt. What this closes is the reachable-from-the-client half: a bad
// type, scope or channel anywhere in the payload now changes nothing at all.
//
// Rows are stored as deviations from the tenant's current defaults: a cell that
// agrees with the default is not recorded at all. That is what keeps a channel
// connected later reaching people who saved their settings before it existed —
// they never expressed a choice about it, so there is nothing to override.
func (s *Service) SetAll(ctx context.Context, scope domain.TenantScope, userID int64, ps []notificationprefs.Preference) error {
	allowed, err := s.DeliveryDefaults(ctx, scope)
	if err != nil {
		return err
	}
	checked := make([]notificationprefs.Preference, 0, len(ps))
	for _, p := range ps {
		n, err := normalize(p, allowed)
		if err != nil {
			return err
		}
		n.ChannelOverrides = deviations(n.ChannelOverrides, allowed)
		checked = append(checked, n)
	}
	for _, p := range checked {
		if err := s.repo.Set(ctx, scope, userID, p); err != nil {
			return err
		}
	}
	return nil
}

// deviations keeps only the choices that differ from the tenant's defaults.
//
// A cell the user left at its default stays "no opinion", so a later change of
// that default still reaches them. It also gives "reset to default" for free:
// putting a switch back where it started removes the override.
func deviations(chosen map[string]bool, defaults map[string]bool) map[string]bool {
	out := make(map[string]bool, len(chosen))
	for name, on := range chosen {
		if def, known := defaults[name]; known && def == on {
			continue
		}
		out[name] = on
	}
	return out
}

// Батчевая операция: не превращать в цикл по событиям — это N+1.
func (s *Service) Resolve(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return s.repo.ResolveRecipients(ctx, scope, notifType, targets)
}

// Батчевая операция: не превращать в цикл — это N+1.
func (s *Service) ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error) {
	return s.repo.ResolveAddressed(ctx, scope, notifType, userIDs)
}
