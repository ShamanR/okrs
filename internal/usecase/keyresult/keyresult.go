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

func (s *UseCase) UpdateProgressNumerical(ctx context.Context, scope domain.TenantScope, krID int64, current float64, actorUserID int64) error {
	kr, err := s.krs.Get(ctx, scope, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindNumerical {
		return fmt.Errorf("unsupported kr kind for numerical update: %s", kr.Kind)
	}
	if err := s.krs.UpdateNumericalCurrent(ctx, scope, krID, current); err != nil {
		return err
	}
	if n := kr.Numerical; n != nil {
		beforeProg := progress.NumericalProgress(n.StartValue, n.TargetValue, n.CurrentValue, n.Checkpoints)
		afterProg := progress.NumericalProgress(n.StartValue, n.TargetValue, current, n.Checkpoints)
		s.publishKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
		s.krs.AutoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
	}
	return nil
}
func (s *UseCase) UpdateProgressBoolean(ctx context.Context, scope domain.TenantScope, krID int64, done bool, actorUserID int64) error {
	kr, err := s.krs.Get(ctx, scope, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindBoolean {
		return fmt.Errorf("unsupported kr kind for boolean update: %s", kr.Kind)
	}
	beforeDone := false
	if bm, berr := s.krs.GetBooleanMeta(ctx, scope, krID); berr == nil && bm != nil {
		beforeDone = bm.IsDone
	}
	if err := s.krs.UpdateBoolean(ctx, scope, krID, done); err != nil {
		return err
	}
	beforeProg := progress.BooleanProgress(beforeDone)
	afterProg := progress.BooleanProgress(done)
	s.publishKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
	s.krs.AutoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
	return nil
}
func (s *UseCase) UpdateProgressProject(ctx context.Context, scope domain.TenantScope, krID int64, updates []ProjectStageUpdate, actorUserID int64) error {
	kr, err := s.krs.Get(ctx, scope, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindProject {
		return fmt.Errorf("unsupported kr kind for project update: %s", kr.Kind)
	}
	stages, err := s.krs.ListProjectStages(ctx, scope, krID)
	if err != nil {
		return err
	}
	updatesByID := make(map[int64]bool, len(updates))
	for _, u := range updates {
		updatesByID[u.ID] = u.IsDone
	}
	validUpdates := make(map[int64]bool, len(updates))
	for _, stage := range stages {
		if done, ok := updatesByID[stage.ID]; ok {
			validUpdates[stage.ID] = done
		}
	}
	beforeProg := progress.ProjectProgress(stages)
	if err := s.krs.BatchUpdateProjectStagesDone(ctx, scope, krID, validUpdates); err != nil {
		return err
	}
	afterStages := make([]domain.KRProjectStage, len(stages))
	copy(afterStages, stages)
	for i := range afterStages {
		if done, ok := validUpdates[afterStages[i].ID]; ok {
			afterStages[i].IsDone = done
		}
	}
	afterProg := progress.ProjectProgress(afterStages)
	s.publishKRProgress(ctx, scope, krID, kr, beforeProg, afterProg, actorUserID)
	s.krs.AutoCompleteHealth(ctx, scope, krID, kr, beforeProg, afterProg)
	return nil
}

// publishKRProgress publishes a KRProgressUpdated event with explicit before/after
// percent (0..100). The caller computes the percentages from the KR's meta because
// store.GetKeyResult does not populate the computed KeyResult.Progress field.
func (s *UseCase) publishKRProgress(ctx context.Context, scope domain.TenantScope, krID int64, kr domain.KeyResult, beforeProg, afterProg int, actorUserID int64) {
	g, gerr := s.goals.Get(ctx, scope, kr.GoalID)
	if gerr != nil {
		return
	}
	teamID, periodID, goalID, krRef := g.TeamID, g.PeriodID, g.ID, krID
	s.events.Publish(ctx, event.KRProgressUpdated{
		Meta:   event.Meta{Scope: scope, ActorID: actorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()},
		GoalID: goalID, KRID: krRef, KRTitle: kr.Title, GoalTitle: g.Title, KRKind: kr.Kind,
		Before: beforeProg, After: afterProg,
	})
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
func (s *UseCase) UpsertNote(ctx context.Context, scope domain.TenantScope, krID int64, text string, authorUserID int64) error {
	beforeText := ""
	if before, berr := s.krs.GetNote(ctx, scope, krID); berr == nil && before != nil {
		beforeText = before.Text
	}
	if err := s.krs.UpsertNote(ctx, scope, krID, text, authorUserID); err != nil {
		return err
	}
	if beforeText != text {
		if kr, kerr := s.krs.Get(ctx, scope, krID); kerr == nil {
			if g, gerr := s.goals.Get(ctx, scope, kr.GoalID); gerr == nil {
				teamID, periodID, goalID, krRef := g.TeamID, g.PeriodID, g.ID, krID
				s.events.Publish(ctx, event.KRNoteUpdated{
					Meta:   event.Meta{Scope: scope, ActorID: authorUserID, TeamID: &teamID, PeriodID: &periodID, OccurredAt: time.Now()},
					GoalID: goalID, KRID: krRef, KRTitle: kr.Title, BeforeText: beforeText, AfterText: text,
				})
			}
		}
	}
	return nil
}
