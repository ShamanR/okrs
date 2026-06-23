package settings

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type fakeSysBackend struct {
	calls int
	data  map[string]json.RawMessage
}

func (f *fakeSysBackend) ListAll(context.Context) (map[string]json.RawMessage, error) {
	f.calls++
	return f.data, nil
}

func TestSystemSettingsCacheGlobalSnapshot(t *testing.T) {
	b := &fakeSysBackend{data: map[string]json.RawMessage{"default_registration_tenant_id": json.RawMessage(`1`)}}
	c := newSystemSettingsCacheWithBackend(b, time.Minute)
	ctx := context.Background()
	if _, err := c.GetAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "default_registration_tenant_id"); err != nil {
		t.Fatal(err)
	}
	if b.calls != 1 {
		t.Fatalf("expected 1 backend call (snapshot reused), got %d", b.calls)
	}
	c.Invalidate()
	if _, err := c.GetAll(ctx); err != nil {
		t.Fatal(err)
	}
	if b.calls != 2 {
		t.Fatalf("expected reload after invalidate, got %d", b.calls)
	}
}
