package tenantsettings_test

import (
	"context"
	"encoding/json"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"
)

func TestTenantSettingsScopedByTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	repo := tenantsettings.NewTenantSettingsRepository(pool)
	s1 := domain.TenantScope{TenantID: 1}
	s2 := domain.TenantScope{TenantID: 2}

	if err := repo.Set(ctx, s1, "documentation_url", "https://a"); err != nil {
		t.Fatalf("set t1: %v", err)
	}
	if err := repo.Set(ctx, s2, "documentation_url", "https://b"); err != nil {
		t.Fatalf("set t2: %v", err)
	}

	got, err := repo.Get(ctx, s1, "documentation_url")
	if err != nil {
		t.Fatalf("get t1: %v", err)
	}
	var url string
	_ = json.Unmarshal(got, &url)
	if url != "https://a" {
		t.Fatalf("t1 saw %q, want https://a", url)
	}

	all2, err := repo.GetAll(ctx, s2)
	if err != nil {
		t.Fatalf("getall t2: %v", err)
	}
	if len(all2) != 1 {
		t.Fatalf("t2 snapshot = %v, want 1 key", all2)
	}
}
