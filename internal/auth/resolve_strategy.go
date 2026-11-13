package auth

import (
	"context"
	"sync"

	"okrs/internal/core/domain"
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
	RegisterResolveStrategy("header", func(d ResolveDeps) ResolveStrategy {
		return NewHeaderStrategy(d.Tenants, d.Memberships)
	})
}

// HeaderStrategy resolves the active tenant from the X-Tenant-ID request header,
// validated against the user's active memberships. Without the header a user with
// exactly one membership resolves to it; with several the client must choose
// (not resolved → the caller gates with no_membership). A header naming a tenant
// the user is not an active member of never resolves — the request is rejected
// downstream as tenant_forbidden. This is the OSS default (works everywhere).
type HeaderStrategy struct {
	tenants TenantLookup
	members MembershipLookup
}

func NewHeaderStrategy(t TenantLookup, m MembershipLookup) ResolveStrategy {
	return &HeaderStrategy{tenants: t, members: m}
}

func (s *HeaderStrategy) Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, bool, error) {
	memberships, err := s.members.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, "", false, err
	}
	if len(memberships) == 0 {
		return nil, "", false, nil // not resolved → next strategy / ErrNoMembership
	}
	var pick *domain.Membership
	if want, ok := HeaderTenantIDFromContext(ctx); ok {
		for i := range memberships {
			if memberships[i].TenantID == want {
				pick = &memberships[i]
				break
			}
		}
		if pick == nil {
			return nil, "", false, nil // foreign tenant → tenant_forbidden downstream
		}
	} else if len(memberships) == 1 {
		pick = &memberships[0]
	} else {
		return nil, "", false, nil // several memberships, no choice made
	}
	tn, err := s.tenants.GetByID(ctx, pick.TenantID)
	if err != nil {
		return nil, "", false, err
	}
	return tn, pick.Role, true, nil
}
