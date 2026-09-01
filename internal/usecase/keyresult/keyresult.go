// Package keyresult holds the key-result business scenarios: progress updates of all
// three kinds, create/update with per-kind metadata, deletion and notes. Each recomputes
// the parent goal's progress and publishes a domain event for the mutation, which is
// what makes them usecases rather than keyresult-service methods. The activity journal
// is one subscriber among possibly several; this package does not know it exists.
package keyresult

import (
	"context"
	"fmt"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/core/progress"
	goalsvc "okrs/internal/service/goal"
	keyresultsvc "okrs/internal/service/keyresult"
	"okrs/internal/store/krs"
)

// Publisher publishes domain events. *eventbus.Bus satisfies it.
// Narrow port on the consumer side: the usecase must not know that a journal,
// a notifier, or anything else is listening.
type Publisher interface {
	Publish(ctx context.Context, ev event.Event)
	PublishBatch(ctx context.Context, evs []event.Event)
}

// Deps are the entity services this usecase orchestrates.
type Deps struct {
	KRs    *keyresultsvc.Service
	Goals  *goalsvc.Service
	Events Publisher
}

type UseCase struct {
	krs    *keyresultsvc.Service
	goals  *goalsvc.Service
	events Publisher
}

func New(deps Deps) *UseCase {
	return &UseCase{krs: deps.KRs, goals: deps.Goals, events: deps.Events}
}

type ProjectStageUpdate struct {
	ID     int64
	IsDone bool
}

// CheckInInput carries what one check-in submits. A nil field was not part of this
// submission and must be left as it is — that is what lets the note endpoint and
// the progress endpoints share one operation without inventing a second event.
type CheckInInput struct {
	Numerical *float64
	Boolean   *bool
	Project   []ProjectStageUpdate
	Health    *domain.KRHealthStatus
	Note      *string
}

// CheckIn is the single operation behind "the user submitted the check-in form":
// progress, health status and note travel together and produce at most one
// KRCheckedIn event. It replaces what used to be up to two independent mutations
// (a progress call plus a note call), each publishing its own event.
//
// Progress is taken from whichever of Numerical/Boolean/Project is set — at most
// one is expected to be, matching the KR's own Kind. Nil fields elsewhere
// (Health, Note) mean "not part of this submission", not "clear this".
func (s *UseCase) CheckIn(ctx context.Context, scope domain.TenantScope, krID int64, in CheckInInput, actorUserID int64) error {
	kr, err := s.krs.Get(ctx, scope, krID)
	if err != nil {
		return err
	}
	// Kind mismatches are rejected before any store write or extra fetch.
	if in.Numerical != nil && kr.Kind != domain.KRKindNumerical {
		return fmt.Errorf("unsupported kr kind for numerical update: %s", kr.Kind)
	}
	if in.Boolean != nil && kr.Kind != domain.KRKindBoolean {
		return fmt.Errorf("unsupported kr kind for boolean update: %s", kr.Kind)
	}
	if in.Project != nil && kr.Kind != domain.KRKindProject {
		return fmt.Errorf("unsupported kr kind for project update: %s", kr.Kind)
	}

	// 1. Read every "before" value up front — after the mutations below there is no
	// way to recover what the note or progress used to be. Read unconditionally,
	// symmetric with checkInProgress below: the published event must carry the KR's
	// real current note in both fields when the note is not part of this
	// submission, not a pair of empty strings — the same "actual value, not a
	// zero-ish placeholder" fix as for progress. A GetNote failure is swallowed
	// (nil check), same as before, so it cannot block the rest of the check-in —
	// the cost is one extra SELECT, not a correctness risk.
	beforeNote := ""
	if note, nerr := s.krs.GetNote(ctx, scope, krID); nerr == nil && note != nil {
		beforeNote = note.Text
	}
	beforeHealth := kr.HealthStatus

	beforeProg, afterProg, projectUpdates, err := s.checkInProgress(ctx, scope, krID, kr, in)
	if err != nil {
		return err
	}

	// 2. Apply progress, health status and note.
	switch {
	case in.Numerical != nil:
		if err := s.krs.UpdateNumericalCurrent(ctx, scope, krID, *in.Numerical); err != nil {
			return err
		}
	case in.Boolean != nil:
		if err := s.krs.UpdateBoolean(ctx, scope, krID, *in.Boolean); err != nil {
			return err
		}
	case in.Project != nil:
		if err := s.krs.BatchUpdateProjectStagesDone(ctx, scope, krID, projectUpdates); err != nil {
			return err
		}
	}
	afterHealth := beforeHealth
	if in.Health != nil {
		afterHealth = *in.Health
		if err := s.krs.UpdateHealthStatus(ctx, scope, krID, *in.Health); err != nil {
			return err
		}
	}
	afterNote := beforeNote
	if in.Note != nil {
		afterNote = *in.Note
		if err := s.krs.UpsertNote(ctx, scope, krID, *in.Note, actorUserID); err != nil {
			return err
		}
	}

	// 3. Auto-complete on reaching 100% only applies when this check-in did not
	// also set health explicitly — otherwise the auto-complete's guard (which
	// reads the KR's health as it was BEFORE this call) could silently overwrite
	// the status the user just chose in the same request. A CheckIn call that only
	// sets progress (Health == nil) is unaffected by this guard. The returned
	// status/changed pair, not a re-check of the
	// condition, is what decides afterHealth: AutoCompleteHealth may have just
	// written KRHealthDone to the store, and the event must report that, not the
	// stale beforeHealth — otherwise a check-in that crosses 100% renders as if
	// the status never moved, when the status transition is the actual news.
	if in.Health == nil && (in.Numerical != nil || in.Boolean != nil || in.Project != nil) {
		if newHealth, changed := s.krs.AutoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg); changed {
			afterHealth = newHealth
		}
	}

	// 4. Nothing to publish if nothing actually changed — today's defect is a
	// progress event fired unconditionally even when only the health status (or
	// nothing at all) changed.
	if beforeProg == afterProg && beforeHealth == afterHealth && beforeNote == afterNote {
		return nil
	}

	// 5. Publish exactly one event. A failure to resolve the parent goal here is
	// swallowed, not propagated: the check-in itself already succeeded, and this
	// mirrors the historical behaviour of the methods CheckIn replaces.
	g, gerr := s.goals.Get(ctx, scope, kr.GoalID)
	if gerr != nil {
		return nil
	}
	teamID, periodID, goalID := g.TeamID, g.PeriodID, g.ID
	s.events.Publish(ctx, event.KRCheckedIn{
		Meta:   event.Meta{Scope: scope, ActorID: actorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()},
		GoalID: goalID, KRID: krID, KRTitle: kr.Title, GoalTitle: g.Title, KRKind: kr.Kind,
		ProgressBefore: beforeProg, ProgressAfter: afterProg,
		HealthBefore: beforeHealth, HealthAfter: afterHealth,
		NoteBefore: beforeNote, NoteAfter: afterNote,
	})
	return nil
}

// checkInProgress computes the KR's progress percent before and after this
// check-in. For numerical KRs it always returns the real current progress — free
// to compute, since krs.Get already loaded kr.Numerical — even when this check-in
// does not touch progress at all, so before == after in that case.
//
// Boolean and project progress are NOT populated by krs.Get, so reporting the real
// before value costs an actual query (GetBooleanMeta / ListProjectStages), run
// unconditionally now — even for a note-only or health-only check-in — so the
// published event carries the KR's real current progress in both fields rather
// than a pair of zeros. A zero pair is harmless for the journal and the
// notification renderer — both only compare before to after — but it is wrong as
// an absolute value, and the event's payload is persisted verbatim into every
// recipient's notifications.payload_json (internal/usecase/notification's
// payloadOf). That stored row is a record of what the KR actually stood at, read
// back long after the check-in; writing a zero there records something that never
// happened. No consumer needs the absolute number today — the notification
// channels (notifychannel/, internal/service/notificationchannel/) do not read the
// payload at all — so this is about the record being true, not about a caller
// breaking.
//
// A ListProjectStages failure does not fail the whole check-in when progress was
// not part of this submission (in.Project == nil): the caller may be, say, an
// unrelated note edit on the same project-kind KR, and that write must still
// reach the store. before/after fall back to 0/0 in that case, which still
// satisfies the equality CheckIn's "did anything change" check relies on. When
// progress WAS part of the submission, the failure is real and is propagated, same
// as before this change.
//
// For a project check-in it also returns the per-stage updates to apply, computed
// here so the caller does not fetch stages twice.
func (s *UseCase) checkInProgress(ctx context.Context, scope domain.TenantScope, krID int64, kr domain.KeyResult, in CheckInInput) (before, after int, projectUpdates map[int64]bool, err error) {
	switch kr.Kind {
	case domain.KRKindNumerical:
		if n := kr.Numerical; n != nil {
			before = progress.NumericalProgress(n.StartValue, n.TargetValue, n.CurrentValue, n.Checkpoints)
			after = before
			if in.Numerical != nil {
				after = progress.NumericalProgress(n.StartValue, n.TargetValue, *in.Numerical, n.Checkpoints)
			}
		}
	case domain.KRKindBoolean:
		beforeDone := false
		if bm, berr := s.krs.GetBooleanMeta(ctx, scope, krID); berr == nil && bm != nil {
			beforeDone = bm.IsDone
		}
		before = progress.BooleanProgress(beforeDone)
		after = before
		if in.Boolean != nil {
			after = progress.BooleanProgress(*in.Boolean)
		}
	case domain.KRKindProject:
		stages, serr := s.krs.ListProjectStages(ctx, scope, krID)
		if serr != nil {
			if in.Project == nil {
				return 0, 0, nil, nil
			}
			return 0, 0, nil, serr
		}
		before = progress.ProjectProgress(stages)
		after = before
		if in.Project != nil {
			updatesByID := make(map[int64]bool, len(in.Project))
			for _, u := range in.Project {
				updatesByID[u.ID] = u.IsDone
			}
			projectUpdates = make(map[int64]bool, len(in.Project))
			afterStages := make([]domain.KRProjectStage, len(stages))
			copy(afterStages, stages)
			for i, stage := range afterStages {
				if done, ok := updatesByID[stage.ID]; ok {
					projectUpdates[stage.ID] = done
					afterStages[i].IsDone = done
				}
			}
			after = progress.ProjectProgress(afterStages)
		}
	}
	return before, after, projectUpdates, nil
}

func (s *UseCase) CreateWithMeta(ctx context.Context, scope domain.TenantScope, input krs.KeyResultInput, meta keyresultsvc.MetaInput, actorUserID int64) (int64, error) {
	krID, err := s.krs.Create(ctx, scope, input)
	if err != nil {
		return 0, err
	}
	if err := s.krs.ApplyMeta(ctx, scope, krID, input.Kind, meta); err != nil {
		return 0, err
	}
	if g, gerr := s.goals.Get(ctx, scope, input.GoalID); gerr == nil {
		teamID, periodID, goalID := g.TeamID, g.PeriodID, input.GoalID
		s.events.Publish(ctx, event.KRCreated{
			Meta:   event.Meta{Scope: scope, ActorID: actorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()},
			GoalID: goalID, KRID: krID, KRTitle: input.Title,
		})
	}
	return krID, nil
}
func (s *UseCase) UpdateWithMeta(ctx context.Context, scope domain.TenantScope, input krs.KeyResultUpdateInput, meta keyresultsvc.MetaInput, actorUserID int64) error {
	before, _ := s.krs.Get(ctx, scope, input.ID)
	if err := s.krs.Update(ctx, scope, input); err != nil {
		return err
	}
	if err := s.krs.ApplyMeta(ctx, scope, input.ID, input.Kind, meta); err != nil {
		return err
	}
	if after, aerr := s.krs.Get(ctx, scope, input.ID); aerr == nil {
		changed := event.Diff(map[string][2]any{
			"title":       {before.Title, after.Title},
			"description": {before.Description, after.Description},
			"weight":      {before.Weight, after.Weight},
		})
		if len(changed) > 0 {
			if g, gerr := s.goals.Get(ctx, scope, after.GoalID); gerr == nil {
				teamID, periodID, gid, krID := g.TeamID, g.PeriodID, g.ID, input.ID
				s.events.Publish(ctx, event.KRFieldsChanged{
					Meta:   event.Meta{Scope: scope, ActorID: actorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()},
					GoalID: gid, KRID: krID, KRTitle: after.Title, Changed: changed,
				})
			}
		}
	}
	return nil
}
func (s *UseCase) Delete(ctx context.Context, scope domain.TenantScope, id int64, actorUserID int64) error {
	kr, _ := s.krs.Get(ctx, scope, id)
	if err := s.krs.Delete(ctx, scope, id); err != nil {
		return err
	}
	var g domain.Goal
	if kr.GoalID != 0 {
		g, _ = s.goals.Get(ctx, scope, kr.GoalID)
	}
	teamID, periodID, goalID, krID := g.TeamID, g.PeriodID, g.ID, id
	s.events.Publish(ctx, event.KRDeleted{
		Meta:   event.Meta{Scope: scope, ActorID: actorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()},
		GoalID: goalID, KRID: krID, KRTitle: kr.Title,
	})
	return nil
}
