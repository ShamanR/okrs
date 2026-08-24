package activity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/service/activity"
	storeactivity "okrs/internal/store/activity"
)

// recordingRepo is a minimal Repo: only Record carries behaviour, the reads are
// unused by these tests and return zero values.
type recordingRepo struct {
	recorded []domain.ActivityEvent
	failNext bool
}

func (f *recordingRepo) Record(_ context.Context, _ domain.TenantScope, ev domain.ActivityEvent) (int64, error) {
	if f.failNext {
		return 0, errors.New("boom")
	}
	f.recorded = append(f.recorded, ev)
	return int64(len(f.recorded)), nil
}

func (f *recordingRepo) RecordBatch(_ context.Context, _ domain.TenantScope, evs []domain.ActivityEvent) error {
	if f.failNext {
		return errors.New("boom")
	}
	f.recorded = append(f.recorded, evs...)
	return nil
}

func (f *recordingRepo) List(context.Context, domain.TenantScope, []int64, storeactivity.ListFilter) ([]domain.ActivityEvent, *storeactivity.Cursor, error) {
	return nil, nil, nil
}

func (f *recordingRepo) TreeCounts(context.Context, domain.TenantScope, []int64, *int64, *time.Time) (map[int64]int, error) {
	return nil, nil
}

func (f *recordingRepo) CategoryCounts(context.Context, domain.TenantScope, []int64, storeactivity.ListFilter) (map[string]int, error) {
	return nil, nil
}

func (f *recordingRepo) Purge(context.Context, domain.TenantScope, *time.Time) (int64, error) {
	return 0, nil
}

// A failing journal write must not surface to the caller: the business operation that
// triggered it has already succeeded. A nil logger must not panic either.
func TestRecordIsBestEffort(t *testing.T) {
	repo := &recordingRepo{failNext: true}
	svc := activity.New(repo, nil)

	svc.Record(context.Background(), domain.TenantScope{TenantID: 1}, domain.ActivityEvent{Action: domain.ActionStatusChanged})
	if len(repo.recorded) != 0 {
		t.Fatalf("expected no recorded event on failure, got %d", len(repo.recorded))
	}

	repo.failNext = false
	svc.Record(context.Background(), domain.TenantScope{TenantID: 1}, domain.ActivityEvent{Action: domain.ActionStatusChanged})
	if len(repo.recorded) != 1 {
		t.Fatalf("expected 1 recorded event, got %d", len(repo.recorded))
	}
}

// A nil repository is the "journal disabled" configuration and must be a no-op.
func TestRecordWithNilRepoIsNoOp(t *testing.T) {
	svc := activity.New(nil, nil)
	svc.Record(context.Background(), domain.TenantScope{TenantID: 1}, domain.ActivityEvent{Action: domain.ActionStatusChanged})
}

func TestDiffFieldsOnlyChanged(t *testing.T) {
	changed := activity.DiffFields(map[string][2]any{
		"title":       {"old", "new"},
		"description": {"same", "same"},
		"weight":      {10, 20},
	})
	if len(changed) != 2 {
		t.Fatalf("want 2 changed, got %d: %+v", len(changed), changed)
	}
	if _, ok := changed["description"]; ok {
		t.Fatalf("unchanged field leaked: %+v", changed)
	}
	if changed["title"].(map[string]any)["after"] != "new" {
		t.Fatalf("title diff wrong: %+v", changed["title"])
	}
	if len(activity.DiffFields(map[string][2]any{"a": {1, 1}})) != 0 {
		t.Fatalf("no-op edit should produce empty diff")
	}
}
