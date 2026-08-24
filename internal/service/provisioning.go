package service

import (
	"context"
	"errors"
	"strings"

	"okrs/internal/core/domain"
	"okrs/internal/store/memberships"
	"okrs/internal/store/tenants"
)

// ErrLastAdmin is returned when an operation would leave a tenant with zero active admins.
var ErrLastAdmin = errors.New("service: cannot remove the tenant's last admin")

var (
	// ErrLastSystemAdmin is returned when revoking would leave the instance with no system-admin.
	ErrLastSystemAdmin = errors.New("service: cannot revoke the last system admin")
	// ErrSelfLockout is returned when a system-admin tries to revoke their own privilege.
	ErrSelfLockout = errors.New("service: cannot revoke your own system-admin")
)

// ProvisioningService is the cross-tenant control surface used by the system-admin
// plane and (later) SaaS/service-desk control planes. It wraps the tenant and
// membership repositories, invalidating the resolve-hot-path caches after each write.
// grantRemover removes all of a user's grants in a tenant. *grants.GrantsCache satisfies it.
type grantRemover interface {
	RemoveAllUserGrants(ctx context.Context, scope domain.TenantScope, userID int64) error
}

// defaultAccessApplier applies a tenant's new-user policy to a member. *OnboardingService satisfies it.
type defaultAccessApplier interface {
	ApplyDefaultAccess(ctx context.Context, scope domain.TenantScope, userID int64) error
}

// systemAdminStore toggles/counts/reads the instance system-admin flag. *users.UserRepository satisfies it.
type systemAdminStore interface {
	SetSystemAdmin(ctx context.Context, userID int64, v bool) error
	CountSystemAdmins(ctx context.Context) (int, error)
	IsSystemAdmin(ctx context.Context, userID int64) (bool, error)
}

type ProvisioningService struct {
	tenants       *tenants.TenantRepository
	tenantCache   *tenants.TenantCache
	members       *memberships.MembershipRepository
	memberCache   *memberships.MembershipCache
	settings      *SettingsService
	grants        grantRemover
	defaultAccess defaultAccessApplier
	users         systemAdminStore
}

func NewProvisioningService(
	tnRepo *tenants.TenantRepository, tenantCache *tenants.TenantCache,
	memRepo *memberships.MembershipRepository, memberCache *memberships.MembershipCache,
	settings *SettingsService, grants grantRemover, defaultAccess defaultAccessApplier,
	users systemAdminStore,
) *ProvisioningService {
	return &ProvisioningService{
		tenants:       tnRepo,
		tenantCache:   tenantCache,
		members:       memRepo,
		memberCache:   memberCache,
		settings:      settings,
		grants:        grants,
		defaultAccess: defaultAccess,
		users:         users,
	}
}

// CreateTenant provisions a new tenant (slug validated by the repository).
func (p *ProvisioningService) CreateTenant(ctx context.Context, name, slug string) (*domain.Tenant, error) {
	return p.tenants.Create(ctx, slug, name)
}

// RenameTenant changes only the tenant's display name (tenant-admin path) and invalidates the cache.
func (p *ProvisioningService) RenameTenant(ctx context.Context, id int64, name string) error {
	if err := p.tenants.Rename(ctx, id, name); err != nil {
		return err
	}
	p.tenantCache.Invalidate(id)
	return nil
}

// UpdateTenant changes name and slug (system-admin path) and invalidates the cache.
func (p *ProvisioningService) UpdateTenant(ctx context.Context, id int64, name, slug string) (*domain.Tenant, error) {
	t, err := p.tenants.Update(ctx, id, name, slug)
	if err != nil {
		return nil, err
	}
	p.tenantCache.Invalidate(id)
	return t, nil
}

// AttachMember gives an existing global user an active membership in a tenant.
// Email→invitation onboarding is Plan 4; here the caller supplies a concrete user id.
func (p *ProvisioningService) AttachMember(ctx context.Context, tenantID, userID int64, role domain.Role) (*domain.Membership, error) {
	// Apply the default-access grant before creating the active membership so a failing grant
	// (e.g. a stale default_hierarchy_node_id) does not leave an active membership without access.
	if err := p.defaultAccess.ApplyDefaultAccess(ctx, domain.TenantScope{TenantID: tenantID}, userID); err != nil {
		return nil, err
	}
	m, err := p.members.Upsert(ctx, domain.Membership{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		Status:   domain.MembershipActive,
	})
	if err != nil {
		return nil, err
	}
	p.memberCache.InvalidateUser(userID)
	return m, nil
}

// SetMemberRole changes a member's tenant role. Refuses to demote the tenant's last active admin
// (ErrLastAdmin) so a tenant never ends up with zero admins. Unknown membership → memberships.ErrNotFound.
func (p *ProvisioningService) SetMemberRole(ctx context.Context, tenantID, userID int64, role domain.Role) error {
	scope := domain.TenantScope{TenantID: tenantID}
	cur, err := p.members.Get(ctx, userID, tenantID)
	if err != nil {
		return err // memberships.ErrNotFound bubbles up
	}
	if cur.Role == domain.RoleAdmin && role != domain.RoleAdmin && cur.Status == domain.MembershipActive {
		n, err := p.members.CountActiveAdmins(ctx, scope)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastAdmin
		}
	}
	if err := p.members.SetRole(ctx, scope, userID, role); err != nil {
		return err
	}
	p.memberCache.InvalidateUser(userID)
	return nil
}

// SetSystemAdmin grants/revokes the instance system-admin flag. Missing user → users.ErrNotFound.
// The guards apply only to a real state change that removes an admin: setting the flag to its
// current value is an idempotent no-op, and revoking a user who is not an admin never triggers the
// last-admin guard. Refuses to revoke the last remaining system-admin (ErrLastSystemAdmin) or the
// caller's own flag (ErrSelfLockout). callerID may be 0 for a machine (provisioning-token) caller.
func (p *ProvisioningService) SetSystemAdmin(ctx context.Context, callerID, targetID int64, v bool) error {
	cur, err := p.users.IsSystemAdmin(ctx, targetID)
	if err != nil {
		return err // users.ErrNotFound bubbles up (404)
	}
	if cur == v {
		return nil // already in the desired state — idempotent no-op
	}
	if !v { // revoking a real admin
		if callerID != 0 && callerID == targetID {
			return ErrSelfLockout
		}
		n, err := p.users.CountSystemAdmins(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastSystemAdmin
		}
	}
	return p.users.SetSystemAdmin(ctx, targetID, v)
}

// SetEntitlements writes entitlement.* keys for a tenant (system/provisioning authority).
// Bare keys (e.g. "sso") are namespaced to "entitlement.sso".
func (p *ProvisioningService) SetEntitlements(ctx context.Context, tenantID int64, entitlements map[string]any) error {
	scope := domain.TenantScope{TenantID: tenantID}
	for key, val := range entitlements {
		if !strings.HasPrefix(key, EntitlementPrefix) {
			key = EntitlementPrefix + key
		}
		if err := p.settings.SetTenantEntitlement(ctx, scope, key, val); err != nil {
			return err
		}
	}
	return nil
}

// RemoveMember severs a user's access to a tenant: delete their grants there, then their
// membership (any status). Idempotent.
func (p *ProvisioningService) RemoveMember(ctx context.Context, tenantID, userID int64) error {
	scope := domain.TenantScope{TenantID: tenantID}
	if err := p.grants.RemoveAllUserGrants(ctx, scope, userID); err != nil {
		return err
	}
	if err := p.members.Delete(ctx, scope, userID); err != nil {
		return err
	}
	p.memberCache.InvalidateUser(userID)
	return nil
}

// DenyMember removes a pending (requested) membership in a tenant.
func (p *ProvisioningService) DenyMember(ctx context.Context, tenantID, userID int64) error {
	scope := domain.TenantScope{TenantID: tenantID}
	if err := p.members.DeleteRequested(ctx, scope, userID); err != nil {
		return err
	}
	p.memberCache.InvalidateUser(userID)
	return nil
}

// Suspend blocks access to a tenant (data is retained).
func (p *ProvisioningService) Suspend(ctx context.Context, tenantID int64) error {
	return p.setStatus(ctx, tenantID, domain.TenantSuspended)
}

// Restore re-activates a suspended tenant.
func (p *ProvisioningService) Restore(ctx context.Context, tenantID int64) error {
	return p.setStatus(ctx, tenantID, domain.TenantActive)
}

func (p *ProvisioningService) setStatus(ctx context.Context, tenantID int64, status domain.TenantStatus) error {
	if err := p.tenants.SetStatus(ctx, tenantID, status); err != nil {
		return err
	}
	p.tenantCache.Invalidate(tenantID)
	return nil
}
