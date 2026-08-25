package settings_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	settingssvc "okrs/internal/service/settings"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"
)

func TestTenantEntitlementsStripsPrefix(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	svc := settingssvc.New(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	scope := domain.TenantScope{TenantID: 1}

	if err := svc.SetTenantEntitlement(ctx, scope, "entitlement.sso", true); err != nil {
		t.Fatalf("set entitlement: %v", err)
	}
	if err := svc.SetTenantProduct(ctx, scope, "documentation_url", "https://x"); err != nil {
		t.Fatalf("set product: %v", err)
	}

	ent, err := svc.TenantEntitlements(ctx, scope)
	if err != nil {
		t.Fatalf("entitlements: %v", err)
	}
	if _, ok := ent["sso"]; !ok {
		t.Fatalf("expected stripped key 'sso', got %v", ent)
	}
	if _, leaked := ent["documentation_url"]; leaked {
		t.Fatalf("product key leaked into entitlements: %v", ent)
	}
}

func TestSettingsServiceWriteAuthority(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	svc := settingssvc.New(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	scope := domain.TenantScope{TenantID: 1}

	// Tenant-admin product write is allowed and visible via snapshot.
	if err := svc.SetTenantProduct(ctx, scope, "documentation_url", "https://x"); err != nil {
		t.Fatalf("product write: %v", err)
	}
	if raw, _ := svc.GetTenant(ctx, scope, "documentation_url"); raw == nil {
		t.Fatalf("product key not visible after write")
	}

	// Tenant-admin cannot write entitlement.* via the product path.
	if err := svc.SetTenantProduct(ctx, scope, "entitlement.sso", true); err != settingssvc.ErrEntitlementNamespace {
		t.Fatalf("expected ErrEntitlementNamespace, got %v", err)
	}

	// System/provisioning path can write entitlement.*.
	if err := svc.SetTenantEntitlement(ctx, scope, "entitlement.sso", true); err != nil {
		t.Fatalf("entitlement write: %v", err)
	}
}
