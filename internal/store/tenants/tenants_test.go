package tenants

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/testutil"
)

func TestTenantSetStatus(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := NewTenantRepository(pool)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.SetStatus(ctx, tn.ID, domain.TenantSuspended); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, err := repo.GetByID(ctx, tn.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != domain.TenantSuspended {
		t.Fatalf("status = %q, want suspended", got.Status)
	}
	if err := repo.SetStatus(ctx, 999999, domain.TenantActive); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing tenant, got %v", err)
	}
}

func TestTenantRepositoryCreateAndGet(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()

	repo := NewTenantRepository(pool)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tn.Slug != "acme" || tn.Name != "Acme Inc" {
		t.Fatalf("unexpected tenant: %+v", tn)
	}

	got, err := repo.GetBySlug(ctx, "acme")
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	if got.ID != tn.ID {
		t.Fatalf("id mismatch: %d != %d", got.ID, tn.ID)
	}

	if _, err := repo.Create(ctx, "ACME", "bad"); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("expected ErrInvalidSlug, got %v", err)
	}

	if _, err := repo.Create(ctx, "acme", "dup"); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}

	// default tenant created by migration 027 is reachable.
	def, err := repo.GetByID(ctx, 1)
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if def.Slug != "default" {
		t.Fatalf("default slug = %q", def.Slug)
	}
}
