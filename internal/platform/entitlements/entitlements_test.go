package entitlements_test

import (
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/platform/entitlements"
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

// The OSS "unlimited" implementation must be registered by the package itself (via init), so an
// external embedder using app.New with the default config gets it without registering anything.
// Declared before TestRegistry so the registry isn't already populated by that test's Register.
func TestUnlimitedRegisteredByDefault(t *testing.T) {
	f, ok := entitlements.Get("unlimited")
	if !ok {
		t.Fatal(`built-in "unlimited" entitlements must be registered by default`)
	}
	if _, isUnlimited := f().(entitlements.UnlimitedEntitlements); !isUnlimited {
		t.Fatal(`"unlimited" must map to UnlimitedEntitlements`)
	}
}

func TestRegistry(t *testing.T) {
	entitlements.Register("unlimited", func() entitlements.Entitlements { return entitlements.UnlimitedEntitlements{} })
	f, ok := entitlements.Get("unlimited")
	if !ok || f() == nil {
		t.Fatal("registry round-trip failed")
	}
}
