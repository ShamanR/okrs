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
func (s *Service) Set(ctx context.Context, scope domain.TenantScope, userID int64, p notificationprefs.Preference) error {
	if !slices.Contains(notificationprefs.AllTypes, p.Type) {
		return ErrInvalidType
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
			return ErrInvalidScope
		}
	}
	if len(p.Channels) == 0 {
		// An empty channel set means "nowhere to deliver". In phase 1b in_app is the
		// only channel, so fixing it quietly beats storing a useless preference.
		p.Channels = []string{"in_app"}
	}
	for _, c := range p.Channels {
		if !slices.Contains(AvailableChannels, c) {
			return ErrInvalidChannel
		}
	}
	return s.repo.Set(ctx, scope, userID, p)
}

// Батчевая операция: не превращать в цикл по событиям — это N+1.
func (s *Service) Resolve(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return s.repo.ResolveRecipients(ctx, scope, notifType, targets)
}

// Батчевая операция: не превращать в цикл — это N+1.
func (s *Service) ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error) {
	return s.repo.ResolveAddressed(ctx, scope, notifType, userIDs)
}
