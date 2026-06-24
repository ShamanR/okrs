package service_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store/memberships"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"
)

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
		settingsSvc,
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
