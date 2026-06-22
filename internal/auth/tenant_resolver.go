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

// TenantResolver resolves the active tenant for a request (SessionStrategy).
// Subdomain/email-domain strategies are added later (premium) without changing this core.
type TenantResolver struct {
	tenants TenantLookup
	members MembershipLookup
}

func NewTenantResolver(t TenantLookup, m MembershipLookup) *TenantResolver {
	return &TenantResolver{tenants: t, members: m}
}

// Resolve returns the active tenant and the user's role in it.
// Preference: session.ActiveTenantID (if the user is an active member); otherwise the
// first active membership. ErrNoMembership if the user has none.
func (r *TenantResolver) Resolve(ctx context.Context, user *domain.User, sess *domain.AuthSession) (*domain.Tenant, domain.Role, error) {
	memberships, err := r.members.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}
	if len(memberships) == 0 {
		return nil, "", ErrNoMembership
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

	tn, err := r.tenants.GetByID(ctx, pick.TenantID)
	if err != nil {
		return nil, "", err
	}
	return tn, pick.Role, nil
}
