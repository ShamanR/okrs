package usersettings_test

import (
	"context"
	"encoding/json"
	"testing"

	"okrs/internal/store/testutil"
	"okrs/internal/store/usersettings"
)

func TestUserSettingsRoundTrip(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := usersettings.NewUserSettingsRepository(pool)

	// user id 1 (anonymous-local) exists from migrations.
	if err := repo.Set(ctx, 1, "default_landing_tenant_id", 1); err != nil {
		t.Fatalf("set: %v", err)
	}
	raw, err := repo.Get(ctx, 1, "default_landing_tenant_id")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var id int64
	_ = json.Unmarshal(raw, &id)
	if id != 1 {
		t.Fatalf("got %d, want 1", id)
	}
}
