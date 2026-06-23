// Package entitlements is the feature-gating seam. The box defines the interface
// and ships an OSS-unlimited implementation; a SaaS module registers a
// snapshot-reading implementation (consulting entitlement.* tenant_settings) via the
// registry. This package stays free of internal/store imports — it is a pure seam.
package entitlements

import (
	"sync"

	"okrs/internal/domain"
)

// Unlimited is the sentinel returned by Limit when a feature has no cap.
const Unlimited int64 = -1

// Entitlements answers per-tenant feature-gating questions. It takes an explicit
// domain.TenantScope (not context) so it obeys the layering rule.
type Entitlements interface {
	Has(scope domain.TenantScope, key string) bool
	Limit(scope domain.TenantScope, key string) int64
}

// UnlimitedEntitlements is the OSS implementation: every feature is on, every limit is ∞.
type UnlimitedEntitlements struct{}

func (UnlimitedEntitlements) Has(domain.TenantScope, string) bool    { return true }
func (UnlimitedEntitlements) Limit(domain.TenantScope, string) int64 { return Unlimited }

// Factory builds an Entitlements implementation.
type Factory func() Entitlements

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register makes an implementation available by name (same pattern as auth.Register).
func Register(name string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = f
}

// Get looks a registered factory up by name.
func Get(name string) (Factory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[name]
	return f, ok
}
