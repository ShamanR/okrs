package service_test

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store/grants"
	"okrs/internal/store/memberships"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"
	"okrs/internal/store/users"
)

func TestAttachMemberAppliesDefaultAccess(t *testing.T) {
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
	grantRepo := grants.NewGrantRepository(pool)
	prov := service.NewProvisioningService(
		tnRepo, tenants.NewTenantCache(tnRepo),
		memRepo, memberships.NewMembershipCache(memRepo),
		settingsSvc, grants.NewGrantsCache(grantRepo), newOnboardingForTest(t, pool), users.NewUserRepository(pool),
	)

	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Root') RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	scope := domain.TenantScope{TenantID: 1}
	_ = settingsSvc.SetTenantProduct(ctx, scope, "new_user_policy", "default_node")
	_ = settingsSvc.SetTenantProduct(ctx, scope, "default_hierarchy_node_id", teamID)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:b','github','b','B') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if _, err := prov.AttachMember(ctx, 1, uid, domain.RoleUser); err != nil {
		t.Fatalf("attach: %v", err)
	}
	gs, err := grantRepo.ListUserGrants(ctx, scope, uid)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if len(gs) != 1 || gs[0].TeamID != teamID {
		t.Fatalf("grants = %+v, want one grant on team %d", gs, teamID)
	}
}

func TestSetMemberRoleLastAdminGuard(t *testing.T) {
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
		settingsSvc, grants.NewGrantsCache(grants.NewGrantRepository(pool)), newOnboardingForTest(t, pool), users.NewUserRepository(pool),
	)

	var admin int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:sole','github','sole','Sole') RETURNING id`).Scan(&admin)
	_, _ = memRepo.Upsert(ctx, domain.Membership{UserID: admin, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive})

	// Demoting the only admin must be refused.
	if err := prov.SetMemberRole(ctx, 1, admin, domain.RoleUser); !errors.Is(err, service.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}

	// With a second admin present, demotion is allowed.
	var admin2 int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:two','github','two','Two') RETURNING id`).Scan(&admin2)
	_, _ = memRepo.Upsert(ctx, domain.Membership{UserID: admin2, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive})
	if err := prov.SetMemberRole(ctx, 1, admin, domain.RoleUser); err != nil {
		t.Fatalf("demote with 2 admins: %v", err)
	}
	m, _ := memRepo.Get(ctx, admin, 1)
	if m.Role != domain.RoleUser {
		t.Fatalf("role = %q, want user", m.Role)
	}

	// Unknown membership → ErrNotFound.
	if err := prov.SetMemberRole(ctx, 1, 999999, domain.RoleUser); !errors.Is(err, memberships.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSetSystemAdminGuards(t *testing.T) {
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
		settingsSvc, grants.NewGrantsCache(grants.NewGrantRepository(pool)), newOnboardingForTest(t, pool), users.NewUserRepository(pool),
	)

	mk := func(sub string) int64 {
		var id int64
		_ = pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
			VALUES ($1,'github',$2,$2) RETURNING id`, "github:"+sub, sub).Scan(&id)
		return id
	}
	a := mk("a")
	b := mk("b")

	// Grant b: allowed.
	if err := prov.SetSystemAdmin(ctx, a, b, true); err != nil {
		t.Fatalf("grant b: %v", err)
	}
	// Revoke sole system-admin b (caller a) → last-admin guard.
	if err := prov.SetSystemAdmin(ctx, a, b, false); !errors.Is(err, service.ErrLastSystemAdmin) {
		t.Fatalf("err = %v, want ErrLastSystemAdmin", err)
	}
	// Grant a too, then a revoking self → self-lockout guard.
	if err := prov.SetSystemAdmin(ctx, a, a, true); err != nil {
		t.Fatalf("grant a: %v", err)
	}
	if err := prov.SetSystemAdmin(ctx, a, a, false); !errors.Is(err, service.ErrSelfLockout) {
		t.Fatalf("err = %v, want ErrSelfLockout", err)
	}
	// Now a can revoke b (two admins, not self).
	if err := prov.SetSystemAdmin(ctx, a, b, false); err != nil {
		t.Fatalf("revoke b: %v", err)
	}

	// Revoking a non-admin must be a no-op even when only one real admin remains — the
	// last-admin guard must not fire because no admin is actually removed.
	c := mk("c") // never granted
	if err := prov.SetSystemAdmin(ctx, a, c, false); err != nil {
		t.Fatalf("revoke non-admin should be a no-op, got %v", err)
	}
	// Stale/unknown target → users.ErrNotFound (404), not a spurious last-admin conflict.
	if err := prov.SetSystemAdmin(ctx, a, 999999, false); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

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
	prov := service.NewProvisioningService(tnRepo, tenants.NewTenantCache(tnRepo), memRepo, memberships.NewMembershipCache(memRepo), settingsSvc, grantsCache, newOnboardingForTest(t, pool), users.NewUserRepository(pool))

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
	prov := service.NewProvisioningService(tnRepo, tenants.NewTenantCache(tnRepo), memRepo, memberships.NewMembershipCache(memRepo), settingsSvc, grants.NewGrantsCache(grants.NewGrantRepository(pool)), newOnboardingForTest(t, pool), users.NewUserRepository(pool))

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
		settingsSvc, grants.NewGrantsCache(grants.NewGrantRepository(pool)), newOnboardingForTest(t, pool), users.NewUserRepository(pool),
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
