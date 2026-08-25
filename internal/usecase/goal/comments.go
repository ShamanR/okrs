package goal

import (
	"context"

	"okrs/internal/core/domain"
)

func (s *UseCase) AddComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error {
	commentID, err := s.goals.AddComment(ctx, scope, goalID, text, authorUserID)
	if err != nil {
		return err
	}
	if g, gerr := s.goals.Get(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: authorUserID, Category: domain.ActivityDiscussion, Action: domain.ActionCommentAdded,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: g.Title, Payload: map[string]any{"text": text},
		})
	}
	return nil
}
func (s *UseCase) AddReply(ctx context.Context, scope domain.TenantScope, goalID, parentID int64, text string, authorUserID int64) error {
	replyID, err := s.goals.AddReply(ctx, scope, goalID, parentID, text, authorUserID)
	if err != nil {
		return err // includes goals.ErrNotFound for a bad/non-task parent
	}
	if g, gerr := s.goals.Get(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: authorUserID, Category: domain.ActivityDiscussion, Action: domain.ActionReplyAdded,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &replyID,
			EntityTitle: g.Title, Payload: map[string]any{"text": text},
		})
	}
	return nil
}
func (s *UseCase) SetCommentResolved(ctx context.Context, scope domain.TenantScope, goalID, commentID int64, resolved bool, userID int64) error {
	changed, err := s.goals.SetCommentResolved(ctx, scope, goalID, commentID, resolved, userID)
	if err != nil {
		return err
	}
	if !changed {
		return nil // already in the target state → no event, no re-stamp
	}
	if g, gerr := s.goals.Get(ctx, scope, goalID); gerr == nil {
		action := domain.ActionCommentReopened
		if resolved {
			action = domain.ActionCommentResolved
		}
		teamID, periodID := g.TeamID, g.PeriodID
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: userID, Category: domain.ActivityDiscussion, Action: action,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: g.Title,
			Payload:     map[string]any{"before": map[string]any{"resolved": !resolved}, "after": map[string]any{"resolved": resolved}},
		})
	}
	return nil
}

// DeleteComment removes a task (cascading its replies) or a reply. Authorization:
// the requesting user must be the author, or a tenant admin. Returns isTask so the
// caller/log distinguishes a task deletion (comment_deleted) from a reply (reply_deleted).
// A cascaded task deletion logs a single comment_deleted event (replies vanish silently).
func (s *UseCase) DeleteComment(ctx context.Context, scope domain.TenantScope, goalID, commentID, requestingUserID int64, isAdmin bool) (bool, error) {
	author, isTask, err := s.goals.GetCommentMeta(ctx, scope, goalID, commentID)
	if err != nil {
		return false, err // goals.ErrNotFound if absent
	}
	if author != requestingUserID && !isAdmin {
		return false, domain.ErrForbidden
	}
	if err := s.goals.DeleteComment(ctx, scope, goalID, commentID); err != nil {
		return false, err
	}
	action := domain.ActionReplyDeleted
	if isTask {
		action = domain.ActionCommentDeleted
	}
	// The goal is not deleted by removing a comment, so it is still readable for the
	// team/period/title snapshot of the journal entry.
	if g, gerr := s.goals.Get(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.activity.Record(ctx, scope, domain.ActivityEvent{
			ActorUserID: requestingUserID, Category: domain.ActivityDiscussion, Action: action,
			TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
			EntityTitle: g.Title,
		})
	}
	return isTask, nil
}
