package auth

import (
	"context"
	"errors"

	"okrs/internal/domain"
)

var (
	ErrNoMembership = errors.New("auth: user has no active membership")
	ErrNotMember    = errors.New("auth: user is not a member of the tenant")
)

// TenantLookup loads a tenant by id (satisfied by store/tenants.TenantRepository).
type TenantLookup interface {
	GetByID(ctx context.Context, id int64) (*domain.Tenant, error)
}

// MembershipLookup lists a user's active memberships (satisfied by store/memberships.MembershipRepository).
type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}

// TenantResolver runs an ordered list of strategies; the first that resolves wins.
// SubdomainStrategy and friends are added later (premium) by registering more strategies,
// without changing this core.
type TenantResolver struct {
	strategies []ResolveStrategy
}

func NewTenantResolver(strategies ...ResolveStrategy) *TenantResolver {
	return &TenantResolver{strategies: strategies}
}

// Resolve returns the active tenant and the user's role in it, trying each strategy in order.
// ErrNoMembership if none resolves.
func (r *TenantResolver) Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, error) {
	for _, st := range r.strategies {
		tn, role, ok, err := st.Resolve(ctx, user, sess)
		if err != nil {
			return nil, "", err
		}
		if ok {
			return tn, role, nil
		}
	}
	return nil, "", ErrNoMembership
}
