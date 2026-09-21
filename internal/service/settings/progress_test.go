package settings_test

import (
	"context"
	"encoding/json"
	"testing"

	"okrs/internal/core/domain"
	settingssvc "okrs/internal/service/settings"
)

// mapReader is an in-memory settings.Reader keyed by setting key.
type mapReader map[string]string

func (m mapReader) GetTenant(_ context.Context, _ domain.TenantScope, key string) (json.RawMessage, error) {
	v, ok := m[key]
	if !ok {
		return nil, nil
	}
	return json.RawMessage(v), nil
}

var scope1 = domain.TenantScope{TenantID: 1}

func TestLoadProgressSnapshotIntervalDays(t *testing.T) {
	ctx := context.Background()
	if got := settingssvc.LoadProgressSnapshotIntervalDays(ctx, scope1, mapReader{}); got != 1 {
		t.Fatalf("unset: want 1, got %d", got)
	}
	if got := settingssvc.LoadProgressSnapshotIntervalDays(ctx, scope1, mapReader{settingssvc.ProgressSnapshotIntervalDaysKey: `7`}); got != 7 {
		t.Fatalf("configured: want 7, got %d", got)
	}
	if got := settingssvc.LoadProgressSnapshotIntervalDays(ctx, scope1, mapReader{settingssvc.ProgressSnapshotIntervalDaysKey: `0`}); got != 1 {
		t.Fatalf("invalid: want 1, got %d", got)
	}
}

func TestLoadProgressThresholds_Defaults(t *testing.T) {
	got := settingssvc.LoadProgressThresholds(context.Background(), scope1, mapReader{})
	want := settingssvc.ProgressThresholds{StaleDays: 7, BehindMargin: 10, GreenThreshold: 80, WeightTolerance: 0}
	if got != want {
		t.Fatalf("defaults: want %+v, got %+v", want, got)
	}
}

func TestLoadProgressThresholds_Configured(t *testing.T) {
	r := mapReader{
		settingssvc.ProgressStaleDaysKey:       `14`,
		settingssvc.ProgressBehindMarginKey:    `0`,
		settingssvc.ProgressGreenThresholdKey:  `70`,
		settingssvc.ProgressWeightToleranceKey: `5`,
	}
	got := settingssvc.LoadProgressThresholds(context.Background(), scope1, r)
	want := settingssvc.ProgressThresholds{StaleDays: 14, BehindMargin: 0, GreenThreshold: 70, WeightTolerance: 5}
	if got != want {
		t.Fatalf("configured: want %+v, got %+v", want, got)
	}
}

func TestLoadProgressThresholds_InvalidFallsBackPerField(t *testing.T) {
	r := mapReader{
		settingssvc.ProgressStaleDaysKey:       `0`,
		settingssvc.ProgressBehindMarginKey:    `-1`,
		settingssvc.ProgressGreenThresholdKey:  `101`,
		settingssvc.ProgressWeightToleranceKey: `"x"`,
	}
	got := settingssvc.LoadProgressThresholds(context.Background(), scope1, r)
	want := settingssvc.DefaultProgressThresholds
	if got != want {
		t.Fatalf("invalid: want defaults %+v, got %+v", want, got)
	}

	// One invalid field does not reset the valid ones.
	r = mapReader{settingssvc.ProgressStaleDaysKey: `-3`, settingssvc.ProgressGreenThresholdKey: `60`}
	got = settingssvc.LoadProgressThresholds(context.Background(), scope1, r)
	if got.StaleDays != 7 || got.GreenThreshold != 60 {
		t.Fatalf("per-field fallback: got %+v", got)
	}
}

func TestProgressThresholdsValidate(t *testing.T) {
	ok := settingssvc.ProgressThresholds{StaleDays: 1, BehindMargin: 0, GreenThreshold: 100, WeightTolerance: 0}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid thresholds rejected: %v", err)
	}
	bad := []settingssvc.ProgressThresholds{
		{StaleDays: 0, BehindMargin: 10, GreenThreshold: 80},
		{StaleDays: 7, BehindMargin: -1, GreenThreshold: 80},
		{StaleDays: 7, BehindMargin: 10, GreenThreshold: 0},
		{StaleDays: 7, BehindMargin: 10, GreenThreshold: 101},
		{StaleDays: 7, BehindMargin: 10, GreenThreshold: 80, WeightTolerance: -1},
	}
	for _, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("expected validation error for %+v", b)
		}
	}
}
