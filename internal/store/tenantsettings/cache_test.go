package tenantsettings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"okrs/internal/domain"
)

type fakeBackend struct {
	calls int
	data  map[int64]map[string]json.RawMessage
}

func (f *fakeBackend) GetAll(_ context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	f.calls++
	return f.data[scope.TenantID], nil
}

func TestTenantSettingsCacheCachesPerTenant(t *testing.T) {
	b := &fakeBackend{data: map[int64]map[string]json.RawMessage{
		1: {"documentation_url": json.RawMessage(`"https://a"`)},
	}}
	c := newTenantSettingsCacheWithBackend(b, time.Minute)
	ctx := context.Background()
	s1 := domain.TenantScope{TenantID: 1}

	if _, err := c.GetAll(ctx, s1); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetAll(ctx, s1); err != nil {
		t.Fatal(err)
	}
	if b.calls != 1 {
		t.Fatalf("expected 1 backend call, got %d", b.calls)
	}
	c.Invalidate(1)
	if _, err := c.GetAll(ctx, s1); err != nil {
		t.Fatal(err)
	}
	if b.calls != 2 {
		t.Fatalf("expected reload after invalidate, got %d", b.calls)
	}
}
