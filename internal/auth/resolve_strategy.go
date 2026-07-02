package auth

import (
	"context"
	"sync"

	"okrs/internal/domain"
)

// ResolveStrategy resolves the active tenant for a request. The bool reports whether this
// strategy handled it; false means "let the next strategy try".
type ResolveStrategy interface {
	Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, bool, error)
}

// ResolveDeps are the lookups a strategy factory may need.
type ResolveDeps struct {
	Tenants     TenantLookup
	Memberships MembershipLookup
}

// ResolveStrategyFactory builds a strategy from shared deps.
type ResolveStrategyFactory func(ResolveDeps) ResolveStrategy

var (
	resolveRegistryMu sync.RWMutex
	resolveRegistry   = map[string]ResolveStrategyFactory{}
)

// RegisterResolveStrategy adds a strategy factory to the registry (call from init()).
func RegisterResolveStrategy(name string, f ResolveStrategyFactory) {
	resolveRegistryMu.Lock()
	defer resolveRegistryMu.Unlock()
	resolveRegistry[name] = f
}

// ResolveStrategyFactoryByName looks a registered factory up by name.
func ResolveStrategyFactoryByName(name string) (ResolveStrategyFactory, bool) {
	resolveRegistryMu.RLock()
	defer resolveRegistryMu.RUnlock()
	f, ok := resolveRegistry[name]
	return f, ok
}

func init() {
	RegisterResolveStrategy("session", func(d ResolveDeps) ResolveStrategy {
		return NewSessionStrategy(d.Tenants, d.Memberships)
	})
}

// SessionStrategy resolves from auth_sessions.active_tenant_id, falling back to the first
// active membership. This is the OSS default (works everywhere).
type SessionStrategy struct {
	tenants TenantLookup
	members MembershipLookup
}

func NewSessionStrategy(t TenantLookup, m MembershipLookup) ResolveStrategy {
	return &SessionStrategy{tenants: t, members: m}
}

func (s *SessionStrategy) Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, bool, error) {
	memberships, err := s.members.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, "", false, err
	}
	if len(memberships) == 0 {
		return nil, "", false, nil // not resolved → next strategy / ErrNoMembership
	}
	pick := memberships[0]
	if sess != nil && sess.ActiveTenantID != nil {
		for _, m := range memberships {
			if m.TenantID == *sess.ActiveTenantID {
				pick = m
				break
			}
		}
	}
	tn, err := s.tenants.GetByID(ctx, pick.TenantID)
	if err != nil {
		return nil, "", false, err
	}
	return tn, pick.Role, true, nil
}
