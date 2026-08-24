package service

import (
	"context"
	"errors"

	"okrs/internal/core/domain"
	"okrs/internal/store/goallinks"
)

// SetGoalParents replaces the full set of parents of childID. Validation: access to the
// child's owner team; tenant-membership and scope-access of every parent; no self-link; no
// cycle. Records goal_linked/goal_unlinked activity for the diff. Navigation-only: does not
// touch progress or team period status.
func (s *Service) SetGoalParents(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, adminAll bool, childID int64, parentIDs []int64, actorUserID int64) error {
	child, err := s.goals.GetGoal(ctx, scope, childID)
	if err != nil {
		return ErrGoalLinkNotAccessible
	}
	if !adminAll && !containsID(allowedTeamIDs, child.TeamID) {
		return ErrGoalLinkNotAccessible
	}

	// Dedup + self-link guard.
	seen := make(map[int64]bool, len(parentIDs))
	uniq := make([]int64, 0, len(parentIDs))
	for _, p := range parentIDs {
		if p == childID {
			return ErrGoalLinkSelf
		}
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}

	// Every parent must exist in the tenant and its owner team must be in scope.
	if len(uniq) > 0 {
		owners, err := s.goals.ListGoalOwnerTeamIDs(ctx, scope, uniq)
		if err != nil {
			return err
		}
		for _, p := range uniq {
			teamID, ok := owners[p]
			if !ok { // not in tenant
				return ErrGoalLinkNotAccessible
			}
			if !adminAll && !containsID(allowedTeamIDs, teamID) {
				return ErrGoalLinkNotAccessible
			}
		}
	}

	added, removed, err := s.goalLinks.ReplaceParents(ctx, scope, allowedTeamIDs, adminAll, childID, uniq)
	if err != nil {
		if errors.Is(err, goallinks.ErrCycle) {
			return ErrGoalLinkCycle
		}
		return err
	}

	teamID, periodID := child.TeamID, child.PeriodID
	if len(added) > 0 {
		s.activitySvc.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalLinked,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &childID, EntityTitle: child.Title,
			Payload: map[string]any{"linked_parent_goal_ids": added},
		})
	}
	if len(removed) > 0 {
		s.activitySvc.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnlinked,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &childID, EntityTitle: child.Title,
			Payload: map[string]any{"unlinked_parent_goal_ids": removed},
		})
	}
	return nil
}

// ListLinksForGoals returns scope-filtered parent and child summaries for the given goals,
// with each summary's Progress computed (goals has no stored progress column).

// AttachGoalLinks fills Parents/Children (scope-filtered, with progress) on each board goal
// in place. Navigation-only; leaves progress/status untouched.
func (s *Service) AttachGoalLinks(ctx context.Context, scope domain.TenantScope, details []GoalDetails, allowedTeamIDs []int64, adminAll bool) error {
	if len(details) == 0 {
		return nil
	}
	goalIDs := make([]int64, len(details))
	for i := range details {
		goalIDs[i] = details[i].Goal.ID
	}
	parents, children, err := s.ListLinksForGoals(ctx, scope, goalIDs, allowedTeamIDs, adminAll)
	if err != nil {
		return err
	}
	for i := range details {
		details[i].Goal.Parents = parents[details[i].Goal.ID]
		details[i].Goal.Children = children[details[i].Goal.ID]
	}
	return nil
}

// goalProgressByIDs batch-loads the given goals once (no N+1) and computes each one's progress
// from its KRs via the shared formula (goals has no stored progress column).

// fillGoalRefProgress computes Progress for every distinct linked goal referenced in the maps,
// batch-loading their KRs once (no N+1) and applying the shared progress formula.

// ListLinkableGoals returns candidate goals for the parent picker, each with its Progress
// computed from KRs (the store query returns navigation fields only; progress is filled here,
// batch-loaded, so the picker shows real percentages instead of 0%).

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
