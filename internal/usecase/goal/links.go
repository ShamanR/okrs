package goal

import (
	"context"
	"time"

	"errors"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/store/goallinks"
)

// SetParents replaces the full set of parents of childID. Validation: access to the
// child's owner team; tenant-membership and scope-access of every parent; no self-link; no
// cycle. Publishes GoalLinked/GoalUnlinked for the diff. Navigation-only: does not
// touch progress or team period status.
func (s *UseCase) SetParents(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64, actorUserID int64) error {
	child, err := s.goals.Get(ctx, scope, childID)
	if err != nil {
		return domain.ErrGoalLinkNotAccessible
	}
	if !adminAll && !containsID(allowedTeamIDs, child.TeamID) {
		return domain.ErrGoalLinkNotAccessible
	}

	// Dedup + self-link guard.
	seen := make(map[int64]bool, len(parentIDs))
	uniq := make([]int64, 0, len(parentIDs))
	for _, p := range parentIDs {
		if p == childID {
			return domain.ErrGoalLinkSelf
		}
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}

	// Every parent must exist in the tenant and its owner team must be in scope.
	if len(uniq) > 0 {
		owners, err := s.goals.ListOwnerTeamIDs(ctx, scope, uniq)
		if err != nil {
			return err
		}
		for _, p := range uniq {
			teamID, ok := owners[p]
			if !ok { // not in tenant
				return domain.ErrGoalLinkNotAccessible
			}
			if !adminAll && !containsID(allowedTeamIDs, teamID) {
				return domain.ErrGoalLinkNotAccessible
			}
		}
	}

	added, removed, err := s.links.ReplaceParents(ctx, scope, allowedTeamIDs, adminAll, childID, uniq)
	if err != nil {
		if errors.Is(err, goallinks.ErrCycle) {
			return domain.ErrGoalLinkCycle
		}
		return err
	}

	teamID, periodID := child.TeamID, child.PeriodID
	meta := event.Meta{Scope: scope, ActorID: actorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()}
	if len(added) > 0 {
		s.events.Publish(ctx, event.GoalLinked{
			Meta: meta, ChildGoalID: childID, Title: child.Title, ParentGoalIDs: added,
		})
	}
	if len(removed) > 0 {
		s.events.Publish(ctx, event.GoalUnlinked{
			Meta: meta, ChildGoalID: childID, Title: child.Title, ParentGoalIDs: removed,
		})
	}
	return nil
}
