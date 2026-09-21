package settings

import (
	"context"
	"encoding/json"
	"errors"

	"okrs/internal/core/domain"
)

// Reader is the read-only settings port consumers depend on; *Service satisfies it.
type Reader interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
}

// ProgressSnapshotIntervalDaysKey is the tenant_settings (general) key controlling how
// often the progress snapshot job records a point for the period chart, in days (≥1).
const ProgressSnapshotIntervalDaysKey = "progress_snapshot_interval_days"

// LoadProgressSnapshotIntervalDays reads the per-tenant snapshot interval in days,
// defaulting to 1 (daily) when unset or invalid.
func LoadProgressSnapshotIntervalDays(ctx context.Context, scope domain.TenantScope, sr Reader) int {
	n, ok := readInt(ctx, scope, sr, ProgressSnapshotIntervalDaysKey)
	if !ok || n < 1 {
		return 1
	}
	return n
}

// Tenant product keys holding the progress evaluation thresholds.
const (
	ProgressStaleDaysKey       = "progress_stale_days"
	ProgressBehindMarginKey    = "progress_behind_margin"
	ProgressGreenThresholdKey  = "progress_green_threshold"
	ProgressWeightToleranceKey = "progress_weight_tolerance"
)

// ProgressThresholds are the tenant's progress evaluation thresholds.
type ProgressThresholds struct {
	// StaleDays: a goal of an in_progress team not updated for more days is "stale".
	StaleDays int `json:"stale_days"`
	// BehindMargin: percentage points of lag behind the expected pace tolerated before
	// team progress is highlighted as lagging.
	BehindMargin int `json:"behind_margin"`
	// GreenThreshold: progress percent (1..100) at or above which a goal/team is "in plan".
	GreenThreshold int `json:"green_threshold"`
	// WeightTolerance: allowed deviation of a team's goal weight sum from 100.
	WeightTolerance int `json:"weight_tolerance"`
}

// DefaultProgressThresholds apply until a tenant admin changes them.
var DefaultProgressThresholds = ProgressThresholds{StaleDays: 7, BehindMargin: 10, GreenThreshold: 80, WeightTolerance: 0}

// Validate reports the first out-of-range threshold.
func (t ProgressThresholds) Validate() error {
	switch {
	case t.StaleDays <= 0:
		return errors.New("stale_days must be > 0")
	case t.BehindMargin < 0:
		return errors.New("behind_margin must be >= 0")
	case t.GreenThreshold < 1 || t.GreenThreshold > 100:
		return errors.New("green_threshold must be between 1 and 100")
	case t.WeightTolerance < 0:
		return errors.New("weight_tolerance must be >= 0")
	}
	return nil
}

// LoadProgressThresholds reads the tenant's thresholds. A missing or invalid value is
// replaced by its default individually, so one bad key never resets the others.
func LoadProgressThresholds(ctx context.Context, scope domain.TenantScope, sr Reader) ProgressThresholds {
	t := DefaultProgressThresholds
	if n, ok := readInt(ctx, scope, sr, ProgressStaleDaysKey); ok && n > 0 {
		t.StaleDays = n
	}
	if n, ok := readInt(ctx, scope, sr, ProgressBehindMarginKey); ok && n >= 0 {
		t.BehindMargin = n
	}
	if n, ok := readInt(ctx, scope, sr, ProgressGreenThresholdKey); ok && n >= 1 && n <= 100 {
		t.GreenThreshold = n
	}
	if n, ok := readInt(ctx, scope, sr, ProgressWeightToleranceKey); ok && n >= 0 {
		t.WeightTolerance = n
	}
	return t
}

// readInt reads an integer setting; ok is false when it is unset, unreadable or not an int.
func readInt(ctx context.Context, scope domain.TenantScope, sr Reader, key string) (int, bool) {
	if sr == nil {
		return 0, false
	}
	raw, err := sr.GetTenant(ctx, scope, key)
	if err != nil || raw == nil {
		return 0, false
	}
	var n int
	if json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	return n, true
}
