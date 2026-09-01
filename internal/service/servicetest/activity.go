package servicetest

import (
	"context"
	"errors"
	"time"

	"okrs/internal/core/domain"
	storeactivity "okrs/internal/store/activity"
)

// ActivityRepo is the shared in-memory activity journal double. Recorded holds every
// event written via Record or RecordBatch (kept for the ~24 existing usecase-layer
// tests that assert on it); Events mirrors RecordBatch's writes only, and BatchCalls
// counts RecordBatch invocations — together they let a test distinguish one batched
// write from a loop of individual ones. BatchScopes records the TenantScope each
// RecordBatch call carried, in call order, so a test can catch a batch routed to the
// wrong tenant (or to no tenant at all) even though BatchCalls/Events alone would not
// notice. Setting FailNext makes the next write fail, which is how tests check that a
// journal failure never breaks the business operation that triggered it.
type ActivityRepo struct {
	Recorded    []domain.ActivityEvent
	Events      []domain.ActivityEvent
	BatchCalls  int
	BatchScopes []domain.TenantScope
	FailNext    bool
}

func (f *ActivityRepo) Record(_ context.Context, _ domain.TenantScope, ev domain.ActivityEvent) (int64, error) {
	if f.FailNext {
		return 0, errors.New("boom")
	}
	f.Recorded = append(f.Recorded, ev)
	return int64(len(f.Recorded)), nil
}
func (f *ActivityRepo) RecordBatch(_ context.Context, scope domain.TenantScope, evs []domain.ActivityEvent) error {
	if f.FailNext {
		return errors.New("boom")
	}
	f.BatchCalls++
	f.BatchScopes = append(f.BatchScopes, scope)
	f.Recorded = append(f.Recorded, evs...)
	f.Events = append(f.Events, evs...)
	return nil
}
func (f *ActivityRepo) List(context.Context, domain.TenantScope, []int64, storeactivity.ListFilter) ([]domain.ActivityEvent, *storeactivity.Cursor, error) {
	return nil, nil, nil
}
func (f *ActivityRepo) TreeCounts(context.Context, domain.TenantScope, []int64, *int64, *time.Time) (map[int64]int, error) {
	return nil, nil
}
func (f *ActivityRepo) CategoryCounts(context.Context, domain.TenantScope, []int64, storeactivity.ListFilter) (map[string]int, error) {
	return nil, nil
}
func (f *ActivityRepo) Purge(context.Context, domain.TenantScope, *time.Time) (int64, error) {
	return 0, nil
}
