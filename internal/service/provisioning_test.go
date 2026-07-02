package service_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store/grants"
	"okrs/internal/store/memberships"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"
)

func TestProvisioningRemoveMember(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	tnRepo := tenants.NewTenantRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	grantRepo := grants.NewGrantRepository(pool)
	grantsCache := grants.NewGrantsCache(grantRepo)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	prov := service.NewProvisioningService(tnRepo, tenants.NewTenantCache(tnRepo), memRepo, memberships.NewMembershipCache(memRepo), settingsSvc, grantsCache)

	scope := domain.TenantScope{TenantID: 1}
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, tenant_id) VALUES ('Root',1) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := memRepo.Upsert(ctx, domain.Membership{UserID: 1, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if err := grantRepo.AddUserGrant(ctx, scope, 1, teamID, 1); err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	if err := prov.RemoveMember(ctx, 1, 1); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := memRepo.Get(ctx, 1, 1); err != memberships.ErrNotFound {
		t.Fatalf("membership should be gone, got %v", err)
	}
	g, _ := grantRepo.ListUserGrants(ctx, scope, 1)
	if len(g) != 0 {
		t.Fatalf("grants should be gone, got %d", len(g))
	}
}

func TestProvisioningDenyMember(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	tnRepo := tenants.NewTenantRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	prov := service.NewProvisioningService(tnRepo, tenants.NewTenantCache(tnRepo), memRepo, memberships.NewMembershipCache(memRepo), settingsSvc, grants.NewGrantsCache(grants.NewGrantRepository(pool)))

	if _, err := memRepo.Upsert(ctx, domain.Membership{UserID: 1, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested}); err != nil {
		t.Fatalf("seed requested: %v", err)
	}
	if err := prov.DenyMember(ctx, 1, 1); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if _, err := memRepo.Get(ctx, 1, 1); err != memberships.ErrNotFound {
		t.Fatalf("requested membership should be gone, got %v", err)
	}
}

func TestProvisioningLifecycle(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	tnRepo := tenants.NewTenantRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	prov := service.NewProvisioningService(
		tnRepo, tenants.NewTenantCache(tnRepo),
		memRepo, memberships.NewMembershipCache(memRepo),
		settingsSvc, grants.NewGrantsCache(grants.NewGrantRepository(pool)),
	)

	// Create tenant.
	tn, err := prov.CreateTenant(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if tn.Slug != "acme" {
		t.Fatalf("slug = %q", tn.Slug)
	}

	// Attach the anonymous-local user (id 1) as admin.
	m, err := prov.AttachMember(ctx, tn.ID, 1, domain.RoleAdmin)
	if err != nil {
		t.Fatalf("attach member: %v", err)
	}
	if m.Role != domain.RoleAdmin || m.Status != domain.MembershipActive {
		t.Fatalf("membership = %+v", m)
	}

	// Set entitlements (bare keys get the entitlement. prefix).
	if err := prov.SetEntitlements(ctx, tn.ID, map[string]any{"sso": true}); err != nil {
		t.Fatalf("set entitlements: %v", err)
	}
	scope := domain.TenantScope{TenantID: tn.ID}
	if raw, _ := settingsSvc.GetTenant(ctx, scope, "entitlement.sso"); raw == nil {
		t.Fatalf("entitlement.sso not written")
	}

	// Suspend + restore.
	if err := prov.Suspend(ctx, tn.ID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	got, _ := tnRepo.GetByID(ctx, tn.ID)
	if got.Status != domain.TenantSuspended {
		t.Fatalf("status = %q, want suspended", got.Status)
	}
	if err := prov.Restore(ctx, tn.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ = tnRepo.GetByID(ctx, tn.ID)
	if got.Status != domain.TenantActive {
		t.Fatalf("status = %q, want active", got.Status)
	}
}
