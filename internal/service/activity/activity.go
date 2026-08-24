// Package activity is the activity-journal entity service: append-only writes plus
// the scoped reads and counters the journal UI needs. Record deliberately swallows
// and logs errors — a journal write must never fail the operation that triggered it.
package activity

import (
	"context"
	"log/slog"
	"time"

	"okrs/internal/core/domain"
	storeactivity "okrs/internal/store/activity"
)

// Service is the activity entity service.
type Service struct {
	repo   Repo
	logger *slog.Logger
}

func New(repo Repo, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

type Repo interface {
	Record(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) (int64, error)
	RecordBatch(ctx context.Context, scope domain.TenantScope, evs []domain.ActivityEvent) error
	List(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f storeactivity.ListFilter) ([]domain.ActivityEvent, *storeactivity.Cursor, error)
	TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error)
	CategoryCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f storeactivity.ListFilter) (map[string]int, error)
	Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error)
}

// recordActivity persists one event best-effort: a failure is logged, never returned,
// so the activity journal can never break the user's mutation.
func (s *Service) Record(ctx context.Context, scope domain.TenantScope, ev domain.ActivityEvent) {
	if s.repo == nil {
		return
	}
	if _, err := s.repo.Record(ctx, scope, ev); err != nil && s.logger != nil {
		s.logger.Warn("activity: record failed", "action", string(ev.Action), "tenant", scope.TenantID, "err", err)
	}
}

// ListActivity returns a scoped page of journal events. allowedTeamIDs == nil = admin/unrestricted.
func (s *Service) List(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f storeactivity.ListFilter) ([]domain.ActivityEvent, *storeactivity.Cursor, error) {
	return s.repo.List(ctx, scope, allowedTeamIDs, f)
}

// ActivityTreeCounts returns direct per-team event counts for the sidebar tree.
func (s *Service) TreeCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, periodID *int64, since *time.Time) (map[int64]int, error) {
	return s.repo.TreeCounts(ctx, scope, allowedTeamIDs, periodID, since)
}

// ActivityCategoryCounts returns per-category event counts for the feed's tab counters,
// stable across the selected category (the filter excludes category).
func (s *Service) CategoryCounts(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f storeactivity.ListFilter) (map[string]int, error) {
	return s.repo.CategoryCounts(ctx, scope, allowedTeamIDs, f)
}

// PurgeActivity deletes journal rows for the caller's tenant. Authority (tenant-admin) is
// enforced by RequireTenantAdminMiddleware on the route; the system plane uses
// ProvisioningService.PurgeActivityForTenant instead.
func (s *Service) Purge(ctx context.Context, scope domain.TenantScope, olderThan *time.Time) (int64, error) {
	return s.repo.Purge(ctx, scope, olderThan)
}

// diffFields returns only the entries whose before != after, as {field: {"before":x,"after":y}}.
func DiffFields(pairs map[string][2]any) map[string]any {
	out := map[string]any{}
	for field, ba := range pairs {
		if ba[0] != ba[1] {
			out[field] = map[string]any{"before": ba[0], "after": ba[1]}
		}
	}
	return out
}

// RecordBatch writes many events in one statement and, unlike Record, reports the
// error: bulk callers decide whether a lost batch is worth logging or surfacing.
// Батчевая операция: не превращать в цикл Record — это N+1.
func (s *Service) RecordBatch(ctx context.Context, scope domain.TenantScope, evs []domain.ActivityEvent) error {
	if s.repo == nil {
		return nil
	}
	return s.repo.RecordBatch(ctx, scope, evs)
}
