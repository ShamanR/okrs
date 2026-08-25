// Package teamstatus is the team period status entity service. It touches exactly one repository and
// never writes the activity journal — anything orchestrating more is a usecase.
package teamstatus

import (
	"context"
	"time"

	"okrs/internal/core/domain"
)

// Service is the team period status entity service.
type Service struct {
	repo Repo
}

func New(repo Repo) *Service { return &Service{repo: repo} }

type Repo interface {
	GetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error)
	GetTeamPeriodStatusWithTime(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error)
	ListTeamPeriodStatuses(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error)
	SetTeamPeriodStatus(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus) error
	SetTeamPeriodStatuses(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64, status domain.TeamPeriodStatus) error
}

func (s *Service) Get(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	return s.repo.GetTeamPeriodStatus(ctx, scope, teamID, periodID)
}

// — Однострочные операции над сущностью, нужные сценариям слоя usecase. —

func (s *Service) GetWithTime(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error) {
	return s.repo.GetTeamPeriodStatusWithTime(ctx, scope, teamID, periodID)
}

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) List(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error) {
	return s.repo.ListTeamPeriodStatuses(ctx, scope, periodID, teamIDs)
}

func (s *Service) Set(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus) error {
	return s.repo.SetTeamPeriodStatus(ctx, scope, teamID, periodID, status)
}

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) SetMany(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64, status domain.TeamPeriodStatus) error {
	return s.repo.SetTeamPeriodStatuses(ctx, scope, periodID, teamIDs, status)
}
