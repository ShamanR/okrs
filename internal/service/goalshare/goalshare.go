// Package goalshare is the goal share entity service. It touches exactly one repository and
// never writes the activity journal — anything orchestrating more is a usecase.
package goalshare

import (
	"context"

	"okrs/internal/core/domain"
	"okrs/internal/store/shares"
)

// Service is the goal share entity service.
type Service struct {
	repo Repo
}

func New(repo Repo) *Service { return &Service{repo: repo} }

type Repo interface {
	ListGoalShares(ctx context.Context, scope domain.TenantScope, goalID int64) ([]shares.GoalShare, error)
	ListGoalSharesByGoalIDs(ctx context.Context, scope domain.TenantScope, goalIDs []int64) (map[int64][]shares.GoalShare, error)
	GetGoalShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) (shares.GoalShare, error)
	ReplaceGoalShares(ctx context.Context, scope domain.TenantScope, goalID int64, shares []shares.GoalShareInput) error
	DeleteGoalShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) error
	UpdateGoalTeamWeight(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error
}

func (s *Service) Get(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) (shares.GoalShare, error) {
	return s.repo.GetGoalShare(ctx, scope, goalID, teamID)
}
func (s *Service) List(ctx context.Context, scope domain.TenantScope, goalID int64) ([]shares.GoalShare, error) {
	return s.repo.ListGoalShares(ctx, scope, goalID)
}
func (s *Service) UpdateWeight(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error {
	return s.repo.UpdateGoalTeamWeight(ctx, scope, goalID, teamID, weight)
}
