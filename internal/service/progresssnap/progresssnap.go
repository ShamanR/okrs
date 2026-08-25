// Package progresssnap is the progress-snapshot entity service: the dated per-team
// progress points that back the period progress chart. It exists so that the usecase
// layer and the scheduler reach snapshots through a service like every other entity,
// instead of holding a repository directly — one door to the store, not two.
package progresssnap

import (
	"context"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/progresssnap"
)

type Repo interface {
	UpsertSnapshots(ctx context.Context, scope domain.TenantScope, periodID int64, day time.Time, snaps []progresssnap.Snapshot) error
	ListSnapshots(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) ([]progresssnap.SeriesRow, error)
	LatestSnapshotDate(ctx context.Context, scope domain.TenantScope, periodID int64) (time.Time, bool, error)
}

// Service is the progress-snapshot entity service.
type Service struct {
	repo Repo
}

func New(repo Repo) *Service { return &Service{repo: repo} }

// Upsert writes one day's points for a whole period in a single statement.
// Батчевая операция: не превращать в цикл по командам — это N+1.
func (s *Service) Upsert(ctx context.Context, scope domain.TenantScope, periodID int64, day time.Time, snaps []progresssnap.Snapshot) error {
	return s.repo.UpsertSnapshots(ctx, scope, periodID, day, snaps)
}

// List returns the full series for the given teams in one query.
// Батчевая операция: не превращать в цикл по командам — это N+1.
func (s *Service) List(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) ([]progresssnap.SeriesRow, error) {
	return s.repo.ListSnapshots(ctx, scope, periodID, teamIDs)
}

// LatestDate reports the newest recorded point for a period; ok is false when the
// period has no points yet. The scheduler uses it to decide whether a pass is due.
func (s *Service) LatestDate(ctx context.Context, scope domain.TenantScope, periodID int64) (time.Time, bool, error) {
	return s.repo.LatestSnapshotDate(ctx, scope, periodID)
}
