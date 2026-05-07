package settings_test

import (
	"context"
	"encoding/json"
	"testing"

	"okrs/internal/store/settings"
	"okrs/internal/store/testutil"
)

func TestSettingsGetSetRoundTrip(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := settings.NewSettingsRepository(pool)

	// Missing key returns nil without error.
	raw, err := r.GetSetting(ctx, "no_such_key")
	if err != nil {
		t.Fatalf("GetSetting missing key: %v", err)
	}
	if raw != nil {
		t.Fatalf("expected nil for missing key, got %s", raw)
	}

	// Set a string value and read it back.
	if err := r.SetSetting(ctx, "greeting", "hello"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	raw, err = r.GetSetting(ctx, "greeting")
	if err != nil {
		t.Fatalf("GetSetting after set: %v", err)
	}
	if string(raw) != `"hello"` {
		t.Fatalf("expected %q got %s", `"hello"`, raw)
	}

	// Overwrite (ON CONFLICT) returns the latest value.
	if err := r.SetSetting(ctx, "greeting", "world"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	raw, err = r.GetSetting(ctx, "greeting")
	if err != nil {
		t.Fatalf("GetSetting after overwrite: %v", err)
	}
	if string(raw) != `"world"` {
		t.Fatalf("expected %q got %s", `"world"`, raw)
	}
}

func TestSettingsJsonTypes(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := settings.NewSettingsRepository(pool)

	// Integer value round-trip.
	if err := r.SetSetting(ctx, "limit", 42); err != nil {
		t.Fatalf("SetSetting int: %v", err)
	}
	raw, err := r.GetSetting(ctx, "limit")
	if err != nil {
		t.Fatalf("GetSetting int: %v", err)
	}
	if string(raw) != "42" {
		t.Fatalf("expected 42 got %s", raw)
	}

	// Struct value round-trip.
	type cfg struct {
		Mode string `json:"mode"`
	}
	if err := r.SetSetting(ctx, "auth_cfg", cfg{Mode: "oidc"}); err != nil {
		t.Fatalf("SetSetting struct: %v", err)
	}
	raw, err = r.GetSetting(ctx, "auth_cfg")
	if err != nil {
		t.Fatalf("GetSetting struct: %v", err)
	}
	// Postgres may normalise JSON spacing; compare parsed content.
	var got struct{ Mode string `json:"mode"` }
	if err := json.Unmarshal(raw, &got); err != nil || got.Mode != "oidc" {
		t.Fatalf("expected mode=oidc, got %s", raw)
	}
}
