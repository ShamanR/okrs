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
