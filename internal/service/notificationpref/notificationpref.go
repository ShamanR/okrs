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

// AvailableChannels are the channels this build can deliver to. Phase 1b has only
// in-app; phase 2 replaces this with the tenant's entitled channel list. Set
// validates a caller-supplied Channels list against exactly this slice, and the
// preferences handler reports it verbatim as the API's "channels" field — one
// source of truth for what this build can actually deliver to, so a hand-crafted
// PUT cannot persist a channel (e.g. "telegram") that nothing yet honours.
var AvailableChannels = []string{"in_app"}

// Repo is the port this service needs. Declared consumer-side, per specs/010.
type Repo interface {
	GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error)
	Set(ctx context.Context, scope domain.TenantScope, userID int64, p notificationprefs.Preference) error
	ResolveRecipients(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error)
	ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error)
}

type Service struct{ repo Repo }

func New(repo Repo) *Service { return &Service{repo: repo} }

func (s *Service) GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error) {
	return s.repo.GetAll(ctx, scope, userID)
}

// Set validates before writing. The DB CHECK constraints are a backstop, not the
// place a user-facing error should come from.
// normalize validates one preference and fills in the defaults the client may omit.
// Pure: it writes nothing, which is what lets SetAll check a whole matrix before
// touching the store.
func normalize(p notificationprefs.Preference) (notificationprefs.Preference, error) {
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
	if len(p.Channels) == 0 {
		// An empty channel set means "nowhere to deliver". In phase 1b in_app is the
		// only channel, so fixing it quietly beats storing a useless preference.
		p.Channels = []string{"in_app"}
	}
	for _, c := range p.Channels {
		if !slices.Contains(AvailableChannels, c) {
			return p, ErrInvalidChannel
		}
	}
	return p, nil
}

func (s *Service) Set(ctx context.Context, scope domain.TenantScope, userID int64, p notificationprefs.Preference) error {
	p, err := normalize(p)
	if err != nil {
		return err
	}
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
func (s *Service) SetAll(ctx context.Context, scope domain.TenantScope, userID int64, ps []notificationprefs.Preference) error {
	checked := make([]notificationprefs.Preference, 0, len(ps))
	for _, p := range ps {
		n, err := normalize(p)
		if err != nil {
			return err
		}
		checked = append(checked, n)
	}
	for _, p := range checked {
		if err := s.repo.Set(ctx, scope, userID, p); err != nil {
			return err
		}
	}
	return nil
}

// Батчевая операция: не превращать в цикл по событиям — это N+1.
func (s *Service) Resolve(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return s.repo.ResolveRecipients(ctx, scope, notifType, targets)
}

// Батчевая операция: не превращать в цикл — это N+1.
func (s *Service) ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error) {
	return s.repo.ResolveAddressed(ctx, scope, notifType, userIDs)
}
