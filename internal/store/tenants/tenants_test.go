package tenants

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/store/testutil"
)

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
