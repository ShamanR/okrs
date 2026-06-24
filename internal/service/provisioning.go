package service

import (
	"context"
	"strings"

	"okrs/internal/domain"
	"okrs/internal/store/memberships"
	"okrs/internal/store/tenants"
)

// ProvisioningService is the cross-tenant control surface used by the system-admin
// plane and (later) SaaS/service-desk control planes. It wraps the tenant and
// membership repositories, invalidating the resolve-hot-path caches after each write.
type ProvisioningService struct {
	tenants     *tenants.TenantRepository
	tenantCache *tenants.TenantCache
	members     *memberships.MembershipRepository
	memberCache *memberships.MembershipCache
	settings    *SettingsService
}

func NewProvisioningService(
	tnRepo *tenants.TenantRepository, tenantCache *tenants.TenantCache,
	memRepo *memberships.MembershipRepository, memberCache *memberships.MembershipCache,
	settings *SettingsService,
) *ProvisioningService {
	return &ProvisioningService{
		tenants:     tnRepo,
		tenantCache: tenantCache,
		members:     memRepo,
		memberCache: memberCache,
		settings:    settings,
	}
}

// CreateTenant provisions a new tenant (slug validated by the repository).
func (p *ProvisioningService) CreateTenant(ctx context.Context, name, slug string) (*domain.Tenant, error) {
	return p.tenants.Create(ctx, slug, name)
}

// AttachMember gives an existing global user an active membership in a tenant.
// Email→invitation onboarding is Plan 4; here the caller supplies a concrete user id.
func (p *ProvisioningService) AttachMember(ctx context.Context, tenantID, userID int64, role domain.Role) (*domain.Membership, error) {
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
