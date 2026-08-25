package healthcheckin

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
)

type hcKeyProbe struct{ tenantID, periodID int64 }

func TestHealthCheckInCacheKeysByTenant(t *testing.T) {
	var calls []hcKeyProbe
	loader := func(_ context.Context, scope domain.TenantScope, periodID int64) (*PeriodData, error) {
		calls = append(calls, hcKeyProbe{scope.TenantID, periodID})
		return &PeriodData{PeriodID: periodID, CachedAt: time.Now()}, nil
	}
	c := NewCache(loader, time.Minute, nil)
	ctx := context.Background()

	// Same periodID, different tenants → two distinct loads (no cross-tenant cache hit).
	if _, err := c.Get(ctx, domain.TenantScope{TenantID: 1}, 100); err != nil {
		t.Fatalf("get t1: %v", err)
	}
	if _, err := c.Get(ctx, domain.TenantScope{TenantID: 2}, 100); err != nil {
		t.Fatalf("get t2: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 loads for distinct tenants, got %d", len(calls))
	}
	// Repeat for t1 within TTL → cache hit, no new load.
	if _, err := c.Get(ctx, domain.TenantScope{TenantID: 1}, 100); err != nil {
		t.Fatalf("get t1 again: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected cache hit, got %d loads", len(calls))
	}
}
