package admincommon

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"okrs/internal/core/domain"
)

type mapSettings map[string]string

func (m mapSettings) GetTenant(_ context.Context, _ domain.TenantScope, key string) (json.RawMessage, error) {
	v, ok := m[key]
	if !ok {
		return nil, nil
	}
	return json.RawMessage(v), nil
}

func TestWeightToleranceReadsProgressThresholds(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	scope := domain.TenantScope{TenantID: 1}

	if got := WeightTolerance(r, mapSettings{}, scope); got != 0 {
		t.Fatalf("default: want 0, got %d", got)
	}
	if got := WeightTolerance(r, mapSettings{"progress_weight_tolerance": `5`}, scope); got != 5 {
		t.Fatalf("configured: want 5, got %d", got)
	}
	// The legacy health_checkin_config key is no longer consulted.
	if got := WeightTolerance(r, mapSettings{"health_checkin_config": `{"weight_tolerance":5}`}, scope); got != 0 {
		t.Fatalf("legacy key: want 0, got %d", got)
	}
}
