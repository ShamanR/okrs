package goal

import (
	"context"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
)

func (s *UseCase) AddComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error {
	commentID, err := s.goals.AddComment(ctx, scope, goalID, text, authorUserID)
	if err != nil {
		return err
	}
	if g, gerr := s.goals.Get(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		s.events.Publish(ctx, event.CommentAdded{
			Meta:   event.Meta{Scope: scope, ActorID: authorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()},
			GoalID: goalID, CommentID: commentID, GoalTitle: g.Title, Text: text,
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
		s.events.Publish(ctx, event.ReplyAdded{
			Meta:   event.Meta{Scope: scope, ActorID: authorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()},
			GoalID: goalID, CommentID: replyID, ParentCommentID: parentID, GoalTitle: g.Title, Text: text,
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
	// A comment's author never changes, so there is no ordering requirement against
	// SetCommentResolved above (unlike DeleteComment, where the author gates the
	// authorization check and must be read first). Loaded here, after the no-op
	// guard, so this query — needed only by the notification fan-out (task 9), the
	// journal itself never used it — is not paid on the already-resolved/-reopened
	// no-op path. One query, not in a loop; on error the service already returns
	// author == 0.
	author, _, _ := s.goals.GetCommentMeta(ctx, scope, goalID, commentID)
	if g, gerr := s.goals.Get(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		meta := event.Meta{Scope: scope, ActorID: userID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()}
		var ev event.Event = event.CommentReopened{
			Meta: meta, GoalID: goalID, CommentID: commentID, GoalTitle: g.Title, AuthorUserID: author,
		}
		if resolved {
			ev = event.CommentResolved{
				Meta: meta, GoalID: goalID, CommentID: commentID, GoalTitle: g.Title, AuthorUserID: author,
			}
		}
		s.events.Publish(ctx, ev)
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
	// The goal is not deleted by removing a comment, so it is still readable for the
	// team/period/title snapshot of the event.
	if g, gerr := s.goals.Get(ctx, scope, goalID); gerr == nil {
		teamID, periodID := g.TeamID, g.PeriodID
		meta := event.Meta{Scope: scope, ActorID: requestingUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()}
		var ev event.Event = event.ReplyDeleted{Meta: meta, GoalID: goalID, CommentID: commentID, GoalTitle: g.Title}
		if isTask {
			ev = event.CommentDeleted{Meta: meta, GoalID: goalID, CommentID: commentID, GoalTitle: g.Title}
		}
		s.events.Publish(ctx, ev)
	}
	return isTask, nil
}
