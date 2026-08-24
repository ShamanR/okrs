package keyresult_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/service/keyresult"
	"okrs/internal/service/servicetest"
)

func TestUpdateKRHealthStatusSets(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthNotStarted}
	svc := keyresult.New(store)
	if err := svc.UpdateHealthStatus(context.Background(), domain.TenantScope{TenantID: 1}, 7, domain.KRHealthOnTrack); err != nil {
		t.Fatalf("update: %v", err)
	}
	if store.HealthUpdates[7] != domain.KRHealthOnTrack {
		t.Fatalf("expected on_track, got %q", store.HealthUpdates[7])
	}
}

func TestUpdateKRHealthStatusRejectsInvalid(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, Kind: domain.KRKindNumerical}
	svc := keyresult.New(store)
	if err := svc.UpdateHealthStatus(context.Background(), domain.TenantScope{TenantID: 1}, 7, domain.KRHealthStatus("bogus")); err == nil {
		t.Fatal("expected error for invalid health status")
	}
	if _, ok := store.HealthUpdates[7]; ok {
		t.Fatal("invalid status must not be written")
	}
}
