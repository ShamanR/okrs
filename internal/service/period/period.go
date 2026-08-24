// Package period is the period entity service. It touches exactly one repository and
// never writes the activity journal — anything orchestrating more is a usecase.
package period

import (
	"context"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/periods"
)

// Service is the period entity service.
type Service struct {
	repo Repo
}

func New(repo Repo) *Service { return &Service{repo: repo} }

type Repo interface {
	ListPeriods(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error)
	GetPeriod(ctx context.Context, scope domain.TenantScope, id int64) (domain.Period, error)
	FindPeriodForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error)
	CreatePeriod(ctx context.Context, scope domain.TenantScope, input periods.PeriodInput) (int64, error)
	UpdatePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, input periods.PeriodInput) error
	DeletePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
	ArchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
	UnarchivePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
}

func (s *Service) List(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error) {
	return s.repo.ListPeriods(ctx, scope)
}
func (s *Service) ListViews(ctx context.Context, scope domain.TenantScope, includeArchived bool) ([]domain.PeriodView, error) {
	all, err := s.repo.ListPeriods(ctx, scope)
	if err != nil {
		return nil, err
	}
	src := all
	if !includeArchived {
		src = make([]domain.Period, 0, len(all))
		for _, p := range all {
			if p.ArchivedAt == nil {
				src = append(src, p)
			}
		}
	}
	return domain.BuildPeriodViews(src, time.Now()), nil
}
func (s *Service) Get(ctx context.Context, scope domain.TenantScope, periodID int64) (domain.Period, error) {
	return s.repo.GetPeriod(ctx, scope, periodID)
}
func (s *Service) FindForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error) {
	return s.repo.FindPeriodForDate(ctx, scope, date)
}
func (s *Service) Create(ctx context.Context, scope domain.TenantScope, input periods.PeriodInput) (int64, error) {
	return s.repo.CreatePeriod(ctx, scope, input)
}
func (s *Service) Update(ctx context.Context, scope domain.TenantScope, periodID int64, input periods.PeriodInput) error {
	return s.repo.UpdatePeriod(ctx, scope, periodID, input)
}
func (s *Service) Delete(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.repo.DeletePeriod(ctx, scope, periodID)
}
func (s *Service) Archive(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	p, err := s.repo.GetPeriod(ctx, scope, periodID)
	if err != nil {
		return err
	}
	if domain.PeriodStatusFor(p, time.Now()) != domain.PeriodStatusClosed {
		return domain.ErrPeriodNotClosed
	}
	return s.repo.ArchivePeriod(ctx, scope, periodID)
}
func (s *Service) Unarchive(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.repo.UnarchivePeriod(ctx, scope, periodID)
}
