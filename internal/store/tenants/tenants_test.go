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

func TestTenantRename(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := NewTenantRepository(pool)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Rename(ctx, tn.ID, "Acme LLC"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := repo.GetByID(ctx, tn.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Acme LLC" {
		t.Fatalf("name = %q, want Acme LLC", got.Name)
	}
	if err := repo.Rename(ctx, tn.ID, "  "); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("blank name: want ErrInvalidName, got %v", err)
	}
	if err := repo.Rename(ctx, 999999, "X"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tenant: want ErrNotFound, got %v", err)
	}
}

func TestTenantUpdate(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := NewTenantRepository(pool)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "acme", "Acme Inc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Create(ctx, "globex", "Globex"); err != nil {
		t.Fatalf("create globex: %v", err)
	}

	upd, err := repo.Update(ctx, tn.ID, "Acme LLC", "acme-llc")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.Name != "Acme LLC" || upd.Slug != "acme-llc" {
		t.Fatalf("update result = %+v", upd)
	}
	// старый slug освобождён (жёсткая замена)
	if _, err := repo.GetBySlug(ctx, "acme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old slug still resolves: %v", err)
	}

	if _, err := repo.Update(ctx, tn.ID, "X", "ACME"); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("invalid slug: want ErrInvalidSlug, got %v", err)
	}
	if _, err := repo.Update(ctx, tn.ID, "  ", "acme-llc"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("blank name: want ErrInvalidName, got %v", err)
	}
	if _, err := repo.Update(ctx, tn.ID, "X", "globex"); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("taken slug: want ErrSlugTaken, got %v", err)
	}
	if _, err := repo.Update(ctx, 999999, "X", "free-slug"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tenant: want ErrNotFound, got %v", err)
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
