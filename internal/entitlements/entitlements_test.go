package entitlements_test

import (
	"testing"

	"okrs/internal/domain"
	"okrs/internal/entitlements"
)

func TestUnlimitedEntitlements(t *testing.T) {
	var e entitlements.Entitlements = entitlements.UnlimitedEntitlements{}
	scope := domain.TenantScope{TenantID: 1}
	if !e.Has(scope, "entitlement.sso") {
		t.Fatal("OSS must allow every feature")
	}
	if e.Limit(scope, "entitlement.max_users") != entitlements.Unlimited {
		t.Fatal("OSS limit must be Unlimited")
	}
}

func TestRegistry(t *testing.T) {
	entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })
	f, ok := entitlements.Get("unlimited")
	if !ok || f() == nil {
		t.Fatal("registry round-trip failed")
	}
}
