package tenants

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
)

type fakeTenantBackend struct {
	byID    map[int64]*domain.Tenant
	idCalls int
}

func (f *fakeTenantBackend) GetByID(_ context.Context, id int64) (*domain.Tenant, error) {
	f.idCalls++
	return f.byID[id], nil
}
func (f *fakeTenantBackend) GetBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	for _, t := range f.byID {
		if t.Slug == slug {
			return t, nil
		}
	}
	return nil, nil
}

func TestTenantCacheCachesByID(t *testing.T) {
	backend := &fakeTenantBackend{byID: map[int64]*domain.Tenant{
		2: {ID: 2, Slug: "acme", Status: domain.TenantActive},
	}}
	c := newTenantCacheWithBackend(backend, time.Minute)
	ctx := context.Background()

	if _, err := c.GetByID(ctx, 2); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := c.GetByID(ctx, 2); err != nil {
		t.Fatalf("second: %v", err)
	}
	if backend.idCalls != 1 {
		t.Fatalf("expected 1 backend call (cache hit), got %d", backend.idCalls)
	}

	c.Invalidate(2)
	if _, err := c.GetByID(ctx, 2); err != nil {
		t.Fatalf("after invalidate: %v", err)
	}
	if backend.idCalls != 2 {
		t.Fatalf("expected reload after invalidate, got %d", backend.idCalls)
	}
}
