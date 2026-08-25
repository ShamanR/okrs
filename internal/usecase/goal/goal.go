// Package goal holds the goal business scenarios: create/update/delete, copy and move
// between boards, sharing with other teams, comments and parent links. Each one
// orchestrates several entity services and records the activity journal — that is
// exactly what makes them usecases rather than goal-service methods.
package goal

import (
	"context"

	"okrs/internal/core/domain"
	activitysvc "okrs/internal/service/activity"
	goalsvc "okrs/internal/service/goal"
	goallinksvc "okrs/internal/service/goallink"
	goalsharesvc "okrs/internal/service/goalshare"
	periodsvc "okrs/internal/service/period"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
	"okrs/internal/store/goals"
	"okrs/internal/store/shares"
)

// Deps are the entity services this usecase orchestrates.
type Deps struct {
	Goals    *goalsvc.Service
	Shares   *goalsharesvc.Service
	Links    *goallinksvc.Service
	Statuses *teamstatussvc.Service
	Periods  *periodsvc.Service
	Teams    *teamsvc.Service
	Activity *activitysvc.Service
}

type UseCase struct {
	goals    *goalsvc.Service
	shares   *goalsharesvc.Service
	links    *goallinksvc.Service
	statuses *teamstatussvc.Service
	periods  *periodsvc.Service
	teams    *teamsvc.Service
	activity *activitysvc.Service
}

func New(deps Deps) *UseCase {
	return &UseCase{goals: deps.Goals, shares: deps.Shares, links: deps.Links, statuses: deps.Statuses,
		periods: deps.Periods, teams: deps.Teams, activity: deps.Activity}
}

type ShareTarget struct {
	TeamID int64
	Weight int
}

// CopyGoalParams are the inputs for CopyGoal.
type CopyGoalParams struct {
	SourceGoalID   int64
	TargetTeamID   int64
	TargetPeriodID int64
	Mode           CopyGoalMode
	WithProgress   bool
	WithComments   bool
}

// CopyGoalMode selects copy (keep source) or move (copy then hard-delete source).
type CopyGoalMode string

const (
	CopyGoalModeCopy CopyGoalMode = "copy"
	CopyGoalModeMove CopyGoalMode = "move"
)

// Create creates a goal and auto-advances status from NoGoals to Forming on first goal.
// Returns domain.ErrPeriodClosed if the team's period status is InProgress or Closed.
func (s *UseCase) Create(ctx context.Context, scope domain.TenantScope, input goals.GoalInput, actorUserID int64) (int64, error) {
	status, err := s.statuses.Get(ctx, scope, input.TeamID, input.PeriodID)
	if err != nil {
		return 0, err
	}
	if status == domain.TeamPeriodStatusClosed || status == domain.TeamPeriodStatusInProgress {
		return 0, domain.ErrPeriodClosed
	}
	goalID, err := s.goals.Create(ctx, scope, input)
	if err != nil {
		return 0, err
	}
	if status == domain.TeamPeriodStatusNoGoals {
		if err := s.statuses.Set(ctx, scope, input.TeamID, input.PeriodID, domain.TeamPeriodStatusForming); err != nil {
			return 0, err
		}
	}
	teamID, periodID := input.TeamID, input.PeriodID
	s.activity.Record(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalCreated,
		TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, EntityTitle: input.Title,
	})
	return goalID, nil
}
func (s *UseCase) Update(ctx context.Context, scope domain.TenantScope, input goals.GoalUpdateInput, actorUserID int64) error {
	before, _ := s.goals.Get(ctx, scope, input.ID)
	if err := s.goals.Update(ctx, scope, input); err != nil {
		return err
	}
	if after, aerr := s.goals.Get(ctx, scope, input.ID); aerr == nil {
		changed := activitysvc.DiffFields(map[string][2]any{
			"title":       {before.Title, after.Title},
			"description": {before.Description, after.Description},
			"priority":    {string(before.Priority), string(after.Priority)},
			"weight":      {before.Weight, after.Weight},
		})
		if len(changed) > 0 {
			teamID, periodID, gid := after.TeamID, after.PeriodID, after.ID
			s.activity.Record(ctx, scope, domain.ActivityEvent{
				ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalFieldsChanged,
				TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, EntityTitle: after.Title,
				Payload: map[string]any{"changed": changed},
			})
		}
	}
	return nil
}
func (s *UseCase) UpdateFields(ctx context.Context, scope domain.TenantScope, input goals.GoalFieldsUpdateInput, actorUserID int64) error {
	before, _ := s.goals.Get(ctx, scope, input.ID)
	if err := s.goals.UpdateFields(ctx, scope, input); err != nil {
		return err
	}
	if after, aerr := s.goals.Get(ctx, scope, input.ID); aerr == nil {
		changed := activitysvc.DiffFields(map[string][2]any{
			"title":       {before.Title, after.Title},
			"description": {before.Description, after.Description},
			"priority":    {string(before.Priority), string(after.Priority)},
		})
		if len(changed) > 0 {
			teamID, periodID, gid := after.TeamID, after.PeriodID, after.ID
			s.activity.Record(ctx, scope, domain.ActivityEvent{
				ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalFieldsChanged,
				TeamID: &teamID, PeriodID: &periodID, GoalID: &gid, EntityTitle: after.Title,
				Payload: map[string]any{"changed": changed},
			})
		}
	}
	return nil
}

// Delete removes a goal or a team's share of it, transferring ownership when the owner deletes.
// Returns the effective requesting teamID and the goal's periodID for redirect.
// Returns domain.ErrPeriodClosed if the owner tries to delete in a closed period with no shares.
func (s *UseCase) Delete(ctx context.Context, scope domain.TenantScope, goalID, requestingTeamID int64, actorUserID int64) (effectiveTeamID, periodID int64, err error) {
	goal, err := s.goals.Get(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	if requestingTeamID == 0 {
		requestingTeamID = goal.TeamID
	}
	if requestingTeamID != goal.TeamID {
		if err := s.shares.Delete(ctx, scope, goalID, requestingTeamID); err != nil {
			return 0, 0, err
		}
		// A shared team declined the goal — record it (anchored to the owner team, whose feed
		// the owner watches; payload carries the team that left).
		ownerTeam, pID, gid, decliner := goal.TeamID, goal.PeriodID, goalID, requestingTeamID
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnshared,
			TeamID: &ownerTeam, PeriodID: &pID, GoalID: &gid, EntityTitle: goal.Title,
			Payload: map[string]any{"declined_by_team_id": decliner},
		})
		_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
		return requestingTeamID, goal.PeriodID, nil
	}
	shareList, err := s.shares.List(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	if len(shareList) > 0 {
		newOwner := shareList[0]
		if err := s.goals.UpdateOwner(ctx, scope, goalID, newOwner.TeamID, newOwner.Weight); err != nil {
			return 0, 0, err
		}
		if err := s.shares.Delete(ctx, scope, goalID, newOwner.TeamID); err != nil {
			return 0, 0, err
		}
		// Owner "deleted" a shared goal → ownership transferred to a shared team; log the composition change.
		oldOwner, pID, gid, newOwnerTeam := goal.TeamID, goal.PeriodID, goalID, newOwner.TeamID
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalOwnerChanged,
			TeamID: &newOwnerTeam, PeriodID: &pID, GoalID: &gid, EntityTitle: goal.Title,
			Payload: map[string]any{"before": map[string]any{"owner_team_id": oldOwner}, "after": map[string]any{"owner_team_id": newOwnerTeam}},
		})
		_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
		return requestingTeamID, goal.PeriodID, nil
	}
	status, err := s.statuses.Get(ctx, scope, goal.TeamID, goal.PeriodID)
	if err != nil {
		return 0, 0, err
	}
	if status == domain.TeamPeriodStatusClosed || status == domain.TeamPeriodStatusInProgress {
		return 0, 0, domain.ErrPeriodClosed
	}
	if err := s.goals.Delete(ctx, scope, goalID); err != nil {
		return 0, 0, err
	}
	teamID, pID := goal.TeamID, goal.PeriodID
	s.activity.Record(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalDeleted,
		TeamID: &teamID, PeriodID: &pID, GoalID: &goalID, EntityTitle: goal.Title,
	})
	_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
	return requestingTeamID, goal.PeriodID, nil
}

// Copy copies (or moves) a goal into a target team/period.
// It rejects a target whose team period status is InProgress/Closed (domain.ErrPeriodClosed),
// and a move whose target equals the source pair (domain.ErrTransferTargetSameAsSource).
// Shares are never carried. On move, the source is hard-deleted (cascade).
func (s *UseCase) Copy(ctx context.Context, scope domain.TenantScope, p CopyGoalParams, actorUserID int64) (int64, error) {
	src, err := s.goals.Get(ctx, scope, p.SourceGoalID)
	if err != nil {
		return 0, err
	}
	if p.Mode == CopyGoalModeMove && p.TargetTeamID == src.TeamID && p.TargetPeriodID == src.PeriodID {
		return 0, domain.ErrTransferTargetSameAsSource
	}
	// Validate both target records live in the caller's tenant. The goals FK only enforces the
	// global period/team id, so without these scoped lookups a caller could copy into another
	// tenant's period/team (or a nonexistent one surfaces as an opaque insert error).
	if _, err := s.teams.Get(ctx, scope, p.TargetTeamID); err != nil {
		return 0, domain.ErrTransferTargetNotFound
	}
	if _, err := s.periods.Get(ctx, scope, p.TargetPeriodID); err != nil {
		return 0, domain.ErrTransferTargetNotFound
	}
	targetStatus, err := s.statuses.Get(ctx, scope, p.TargetTeamID, p.TargetPeriodID)
	if err != nil {
		return 0, err
	}
	if targetStatus == domain.TeamPeriodStatusClosed || targetStatus == domain.TeamPeriodStatusInProgress {
		return 0, domain.ErrPeriodClosed
	}

	// For a move, the copy and the source deletion run in one store transaction so the move
	// cannot partially succeed (copy committed, source left behind).
	newGoalID, err := s.goals.Copy(ctx, scope, goals.CopyGoalInput{
		SourceGoalID:   p.SourceGoalID,
		TargetTeamID:   p.TargetTeamID,
		TargetPeriodID: p.TargetPeriodID,
		WithProgress:   p.WithProgress,
		WithComments:   p.WithComments,
		DeleteSource:   p.Mode == CopyGoalModeMove,
	})
	if err != nil {
		return 0, err
	}

	if targetStatus == domain.TeamPeriodStatusNoGoals {
		if err := s.statuses.Set(ctx, scope, p.TargetTeamID, p.TargetPeriodID, domain.TeamPeriodStatusForming); err != nil {
			return 0, err
		}
	}

	action := domain.ActionGoalCopied
	if p.Mode == CopyGoalModeMove {
		action = domain.ActionGoalMoved
	}
	tt, tp, ng := p.TargetTeamID, p.TargetPeriodID, newGoalID
	s.activity.Record(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: action,
		TeamID: &tt, PeriodID: &tp, GoalID: &ng, EntityTitle: src.Title,
		Payload: map[string]any{
			"source_goal_id":   src.ID,
			"source_team_id":   src.TeamID,
			"source_period_id": src.PeriodID,
			"with_progress":    p.WithProgress,
			"with_comments":    p.WithComments,
		},
	})

	if p.Mode == CopyGoalModeMove {
		// Source already deleted inside the copy transaction above; reset its team status
		// if that removal left the source team with no goals in the source period.
		_ = s.resetStatusIfNoGoals(ctx, scope, src.TeamID, src.PeriodID)
	}
	return newGoalID, nil
}
func (s *UseCase) Share(ctx context.Context, scope domain.TenantScope, goalID int64, targets []ShareTarget, actorUserID int64) error {
	goal, err := s.goals.Get(ctx, scope, goalID)
	if err != nil {
		return err
	}
	// Validate every target team belongs to the active tenant before writing shares. The share
	// repository only scopes the goal, so an unchecked target team_id could attach the goal to a
	// team in another tenant. One scoped lookup builds the allow-set (no per-target query).
	if len(targets) > 0 {
		tenantTeams, err := s.teams.ListAll(ctx, scope)
		if err != nil {
			return err
		}
		valid := make(map[int64]struct{}, len(tenantTeams))
		for _, t := range tenantTeams {
			valid[t.ID] = struct{}{}
		}
		for _, target := range targets {
			if _, ok := valid[target.TeamID]; !ok {
				return domain.ErrShareTargetNotInTenant
			}
		}
	}
	// The /share endpoint replaces the whole goal_shares set, so diff the current set against the
	// new targets to log ADDING teams (goal_shared) and REMOVING teams (goal_unshared) separately,
	// and to guard only NEWLY added teams below. The read error must propagate: swallowing it would
	// leave beforeSet empty, misclassify every target as newly added, and could reject an unchanged
	// save with 409 just because an existing participant is already in_progress/closed.
	cur, err := s.shares.List(ctx, scope, goalID)
	if err != nil {
		return err
	}
	beforeSet := make(map[int64]bool, len(cur))
	for _, sh := range cur {
		beforeSet[sh.TeamID] = true
	}
	shareInputs := make([]shares.GoalShareInput, 0, len(targets))
	newSet := map[int64]bool{}
	for _, target := range targets {
		shareInputs = append(shareInputs, shares.GoalShareInput{TeamID: target.TeamID, Weight: target.Weight})
		newSet[target.TeamID] = true
	}
	var added, removed []int64
	for _, target := range targets {
		if !beforeSet[target.TeamID] {
			added = append(added, target.TeamID)
		}
	}
	for teamID := range beforeSet {
		if !newSet[teamID] {
			removed = append(removed, teamID)
		}
	}
	// Guard: a team whose period is already in_progress or closed cannot be NEWLY added as a share
	// target — its OKR set for the period is locked, so a shared goal must not appear after the
	// fact. Only newly added teams are checked (one batched status lookup); teams already sharing
	// the goal are left untouched even if their period has since advanced. This mirrors the UI,
	// which greys out such teams and blocks selection, but is enforced server-side as source of truth.
	if len(added) > 0 {
		statuses, serr := s.statuses.List(ctx, scope, goal.PeriodID, added)
		if serr != nil {
			return serr
		}
		for _, teamID := range added {
			switch statuses[teamID] {
			case domain.TeamPeriodStatusInProgress, domain.TeamPeriodStatusClosed:
				return domain.ErrCannotShareWithClosedPeriod
			}
		}
	}
	if err := s.shares.Replace(ctx, scope, goalID, shareInputs); err != nil {
		return err
	}
	teamID, periodID := goal.TeamID, goal.PeriodID
	if len(added) > 0 {
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalShared,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, EntityTitle: goal.Title,
			Payload: map[string]any{"shared_with_team_ids": added},
		})
	}
	if len(removed) > 0 {
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnshared,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, EntityTitle: goal.Title,
			Payload: map[string]any{"unshared_team_ids": removed},
		})
	}
	return nil
}

// UpdateOwnerAndShares updates goal ownership and sharing based on the selected team set.
// Returns domain.ErrCannotShareWithClosedPeriod if any selected team has an in_progress or closed period.
func (s *UseCase) UpdateOwnerAndShares(ctx context.Context, scope domain.TenantScope, goalID int64, selectedTeamIDs []int64, actorUserID int64) (ownerID, periodID int64, err error) {
	goal, err := s.goals.Get(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	oldOwner := goal.TeamID
	shareList, err := s.shares.List(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	shareWeights := make(map[int64]int, len(shareList))
	for _, share := range shareList {
		shareWeights[share.TeamID] = share.Weight
	}
	selectedSet := make(map[int64]struct{}, len(selectedTeamIDs))
	for _, id := range selectedTeamIDs {
		selectedSet[id] = struct{}{}
	}
	ownerID = goal.TeamID
	if _, ok := selectedSet[ownerID]; !ok && len(selectedTeamIDs) > 0 {
		ownerID = selectedTeamIDs[0]
	}
	newShares := make([]shares.GoalShareInput, 0, len(selectedTeamIDs))
	for _, teamID := range selectedTeamIDs {
		status, err := s.statuses.Get(ctx, scope, teamID, goal.PeriodID)
		if err != nil {
			return 0, 0, err
		}
		if status == domain.TeamPeriodStatusInProgress || status == domain.TeamPeriodStatusClosed {
			return 0, 0, domain.ErrCannotShareWithClosedPeriod
		}
		if teamID == ownerID {
			ownerWeight := goal.Weight
			if ownerID != goal.TeamID {
				if w, ok := shareWeights[ownerID]; ok {
					ownerWeight = w
				} else {
					ownerWeight = 0
				}
			}
			if err := s.goals.UpdateOwner(ctx, scope, goalID, ownerID, ownerWeight); err != nil {
				return 0, 0, err
			}
			continue
		}
		weight := 0
		if w, ok := shareWeights[teamID]; ok {
			weight = w
		}
		newShares = append(newShares, shares.GoalShareInput{TeamID: teamID, Weight: weight})
	}
	if err := s.shares.Replace(ctx, scope, goalID, newShares); err != nil {
		return 0, 0, err
	}
	// Only log an owner change when the owner actually changed (avoid X→X noise).
	if ownerID != oldOwner {
		gid, pID := goalID, goal.PeriodID
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalOwnerChanged,
			TeamID: &ownerID, PeriodID: &pID, GoalID: &gid, EntityTitle: goal.Title,
			Payload: map[string]any{"before": map[string]any{"owner_team_id": oldOwner}, "after": map[string]any{"owner_team_id": ownerID}},
		})
	}
	return ownerID, goal.PeriodID, nil
}
func (s *UseCase) DeleteShare(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, actorUserID int64) error {
	g, _ := s.goals.Get(ctx, scope, goalID)
	if err := s.shares.Delete(ctx, scope, goalID, teamID); err != nil {
		return err
	}
	ownerTeam, periodID, shareTeam := g.TeamID, g.PeriodID, teamID
	s.activity.Record(ctx, scope, domain.ActivityEvent{
		ActorUserID: actorUserID, Category: domain.ActivityComposition, Action: domain.ActionGoalUnshared,
		TeamID: &ownerTeam, PeriodID: &periodID, GoalID: &goalID, EntityTitle: g.Title,
		Payload: map[string]any{"unshared_team_id": shareTeam},
	})
	return nil
}
func (s *UseCase) resetStatusIfNoGoals(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) error {
	goalsList, err := s.goals.ListByTeamPeriod(ctx, scope, teamID, periodID)
	if err != nil || len(goalsList) > 0 {
		return err
	}
	status, err := s.statuses.Get(ctx, scope, teamID, periodID)
	if err != nil || status == domain.TeamPeriodStatusNoGoals {
		return nil
	}
	return s.statuses.Set(ctx, scope, teamID, periodID, domain.TeamPeriodStatusNoGoals)
}

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
