// Package goallink is the goal-link entity service: reads over the goal_links
// repository. Anything that also touches goals (AttachGoalLinks, SetGoalParents)
// orchestrates two services and therefore belongs to the usecase layer, not here.
package goallink

import (
	"context"

	"okrs/internal/core/domain"
	"okrs/internal/store/goallinks"
)

// GoalProgressReader is the narrow port into the goal service: link rows carry the
// linked goal's progress, and progress is the goal service's business. Declared here,
// on the consumer side, so goallink does not depend on the whole goal package.
type GoalProgressReader interface {
	ProgressByIDs(ctx context.Context, scope domain.TenantScope, ids []int64) (map[int64]int, error)
}

// Service is the goal-link entity service.
type Service struct {
	repo  Repo
	goals GoalProgressReader
}

func New(repo Repo, goals GoalProgressReader) *Service {
	return &Service{repo: repo, goals: goals}
}

type Repo interface {
	ReplaceParents(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64) (added, removed []int64, err error)
	ListLinksForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (parents, children map[int64][]domain.GoalRef, err error)
	ListLinkable(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]goallinks.LinkableGoal, error)
}

func (s *Service) ListForGoals(ctx context.Context, scope domain.TenantScope, goalIDs, allowedTeamIDs []int64, adminAll bool) (map[int64][]domain.GoalRef, map[int64][]domain.GoalRef, error) {
	parents, children, err := s.repo.ListLinksForGoals(ctx, scope, goalIDs, allowedTeamIDs, adminAll)
	if err != nil {
		return nil, nil, err
	}
	if err := s.fillGoalRefProgress(ctx, scope, parents, children); err != nil {
		return nil, nil, err
	}
	return parents, children, nil
}
func (s *Service) ListLinkable(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, periodID *int64, excludeGoalID int64, q string) ([]goallinks.LinkableGoal, error) {
	items, err := s.repo.ListLinkable(ctx, scope, allowedTeamIDs, adminAll, periodID, excludeGoalID, q)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	ids := make([]int64, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	progressByID, err := s.goals.ProgressByIDs(ctx, scope, ids)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Progress = progressByID[items[i].ID]
	}
	return items, nil
}

func (s *Service) fillGoalRefProgress(ctx context.Context, scope domain.TenantScope, refMaps ...map[int64][]domain.GoalRef) error {
	idSet := make(map[int64]struct{})
	for _, m := range refMaps {
		for _, refs := range m {
			for _, r := range refs {
				idSet[r.ID] = struct{}{}
			}
		}
	}
	if len(idSet) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	progressByID, err := s.goals.ProgressByIDs(ctx, scope, ids)
	if err != nil {
		return err
	}
	for _, m := range refMaps {
		for key := range m {
			for i := range m[key] {
				m[key][i].Progress = progressByID[m[key][i].ID]
			}
		}
	}
	return nil
}

// — Однострочные операции над сущностью, нужные сценариям слоя usecase. —

func (s *Service) ReplaceParents(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64) (added, removed []int64, err error) {
	return s.repo.ReplaceParents(ctx, scope, allowedTeamIDs, adminAll, childID, parentIDs)
}
