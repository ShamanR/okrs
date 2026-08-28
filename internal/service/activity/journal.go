package activity

import (
	"context"
	"errors"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
)

// Handle is the bus subscriber that turns domain events into journal rows. It is the
// only place that knows the shape of an activity_events row — publishers do not.
//
// Registered synchronously (eventbus.Sync), so a mutation's event is durable before
// the HTTP response, exactly as it was when usecases called Record directly.
func (s *Service) Handle(ctx context.Context, evs []event.Event) error {
	// A batch may span tenants: one instance serves many requests concurrently.
	// Group first, then one write per tenant.
	// Батчевая операция: не превращать в цикл Record — это N+1.
	byTenant := make(map[int64][]domain.ActivityEvent)
	for _, ev := range evs {
		row, ok := toRow(ev)
		if !ok {
			continue
		}
		tenantID := ev.Context().Scope.TenantID
		byTenant[tenantID] = append(byTenant[tenantID], row)
	}
	// Every tenant's write is attempted even if an earlier one fails: one bad
	// tenant must not cost the others their rows. Errors are collected, not
	// returned on the first failure.
	var errs []error
	for tenantID, rows := range byTenant {
		if err := s.RecordBatch(ctx, domain.TenantScope{TenantID: tenantID}, rows); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ToRowForTest exposes toRow to the package's external test. Keeping toRow itself
// unexported preserves the rule that only this file knows the journal row shape.
func ToRowForTest(ev event.Event) (domain.ActivityEvent, bool) { return toRow(ev) }

// base fills the fields every row shares, from the event's Meta.
func base(m event.Meta, category domain.ActivityCategory, action domain.ActivityAction, title string) domain.ActivityEvent {
	return domain.ActivityEvent{
		ActorUserID: m.ActorID,
		Category:    category,
		Action:      action,
		TeamID:      m.TeamID,
		PeriodID:    m.PeriodID,
		EntityTitle: title,
		Payload:     map[string]any{},
	}
}

// changedPayload renders {field: {before, after}} in the journal's historical wire
// shape. The event carries typed pairs; the wire shape belongs to the journal.
func changedPayload(changed map[string][2]any) map[string]any {
	out := make(map[string]any, len(changed))
	for field, ba := range changed {
		out[field] = map[string]any{"before": ba[0], "after": ba[1]}
	}
	return map[string]any{"changed": out}
}

// toRow maps a domain event onto a journal row. Payloads reproduce the shapes written
// before the bus existed, byte for byte — the activity feed reads stored rows and must
// not notice the refactor.
func toRow(ev event.Event) (domain.ActivityEvent, bool) {
	switch e := ev.(type) {

	case event.GoalCreated:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalCreated, e.Title)
		r.GoalID = &e.GoalID
		return r, true

	case event.GoalCopied:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalCopied, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = copyPayload(e.SourceGoalID, e.SourceTeamID, e.SourcePeriodID, e.WithProgress, e.WithComments)
		return r, true

	case event.GoalMoved:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalMoved, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = copyPayload(e.SourceGoalID, e.SourceTeamID, e.SourcePeriodID, e.WithProgress, e.WithComments)
		return r, true

	case event.GoalDeleted:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalDeleted, e.Title)
		r.GoalID = &e.GoalID
		return r, true

	case event.GoalFieldsChanged:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalFieldsChanged, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = changedPayload(e.Changed)
		return r, true

	case event.GoalOwnerChanged:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalOwnerChanged, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = map[string]any{
			"before": map[string]any{"owner_team_id": e.BeforeTeamID},
			"after":  map[string]any{"owner_team_id": e.AfterTeamID},
		}
		return r, true

	case event.GoalShared:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalShared, e.Title)
		r.GoalID = &e.GoalID
		r.Payload = map[string]any{"shared_with_team_ids": e.SharedWithTeamIDs}
		return r, true

	case event.GoalUnshared:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalUnshared, e.Title)
		r.GoalID = &e.GoalID
		// Three historical shapes; exactly one field is set. See the type's doc comment.
		switch {
		case e.DeclinedByTeamID != 0:
			r.Payload = map[string]any{"declined_by_team_id": e.DeclinedByTeamID}
		case e.UnsharedTeamID != 0:
			r.Payload = map[string]any{"unshared_team_id": e.UnsharedTeamID}
		default:
			r.Payload = map[string]any{"unshared_team_ids": e.UnsharedTeamIDs}
		}
		return r, true

	case event.GoalLinked:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalLinked, e.Title)
		r.GoalID = &e.ChildGoalID
		r.Payload = map[string]any{"linked_parent_goal_ids": e.ParentGoalIDs}
		return r, true

	case event.GoalUnlinked:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionGoalUnlinked, e.Title)
		r.GoalID = &e.ChildGoalID
		r.Payload = map[string]any{"unlinked_parent_goal_ids": e.ParentGoalIDs}
		return r, true

	case event.KRCreated:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionKRCreated, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		return r, true

	case event.KRDeleted:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionKRDeleted, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		return r, true

	case event.KRFieldsChanged:
		r := base(e.Meta, domain.ActivityComposition, domain.ActionKRFieldsChanged, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		r.Payload = changedPayload(e.Changed)
		return r, true

	case event.KRProgressUpdated:
		r := base(e.Meta, domain.ActivityProgress, domain.ActionKRProgress, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		r.Payload = map[string]any{
			"before":     map[string]any{"progress": e.Before},
			"after":      map[string]any{"progress": e.After},
			"kind":       string(e.KRKind),
			"goal_title": e.GoalTitle,
		}
		return r, true

	case event.KRNoteUpdated:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionKRNoteUpdated, e.KRTitle)
		r.GoalID, r.KRID = &e.GoalID, &e.KRID
		r.Payload = map[string]any{
			"before": map[string]any{"note": e.BeforeText},
			"after":  map[string]any{"note": e.AfterText},
		}
		return r, true

	case event.StatusChanged:
		r := base(e.Meta, domain.ActivityStatus, domain.ActionStatusChanged, e.TeamTitle)
		r.Payload = map[string]any{
			"before": map[string]any{"status": string(e.Before)},
			"after":  map[string]any{"status": string(e.After)},
		}
		if e.Bulk {
			r.Payload["bulk"] = true
		}
		return r, true

	case event.CommentAdded:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionCommentAdded, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		r.Payload = map[string]any{"text": e.Text}
		return r, true

	case event.CommentResolved:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionCommentResolved, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		r.Payload = resolvedPayload(true)
		return r, true

	case event.CommentReopened:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionCommentReopened, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		r.Payload = resolvedPayload(false)
		return r, true

	case event.CommentDeleted:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionCommentDeleted, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		return r, true

	case event.ReplyAdded:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionReplyAdded, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		r.Payload = map[string]any{"text": e.Text}
		return r, true

	case event.ReplyDeleted:
		r := base(e.Meta, domain.ActivityDiscussion, domain.ActionReplyDeleted, e.GoalTitle)
		r.GoalID, r.CommentID = &e.GoalID, &e.CommentID
		return r, true
	}
	return domain.ActivityEvent{}, false
}

func copyPayload(srcGoal, srcTeam, srcPeriod int64, withProgress, withComments bool) map[string]any {
	return map[string]any{
		"source_goal_id":   srcGoal,
		"source_team_id":   srcTeam,
		"source_period_id": srcPeriod,
		"with_progress":    withProgress,
		"with_comments":    withComments,
	}
}

func resolvedPayload(resolved bool) map[string]any {
	return map[string]any{
		"before": map[string]any{"resolved": !resolved},
		"after":  map[string]any{"resolved": resolved},
	}
}
