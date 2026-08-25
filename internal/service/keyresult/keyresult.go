// Package keyresult is the key result entity service. It touches exactly one repository and
// never writes the activity journal — anything orchestrating more is a usecase.
package keyresult

import (
	"context"
	"fmt"

	"okrs/internal/core/domain"
	"okrs/internal/store/krs"
)

// Service is the key result entity service.
type Service struct {
	repo Repo
}

func New(repo Repo) *Service { return &Service{repo: repo} }

// MetaInput carries the per-kind metadata a KR create/update applies.
type MetaInput struct {
	NumericalStart       float64
	NumericalTarget      float64
	NumericalCurrent     float64
	NumericalUnit        string
	NumericalCheckpoints []domain.KRNumericalCheckpoint
	BooleanDone          bool
	ProjectStages        []krs.ProjectStageInput
}

type Repo interface {
	GetKeyResult(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error)
	CreateKeyResult(ctx context.Context, scope domain.TenantScope, input krs.KeyResultInput) (int64, error)
	UpdateKeyResult(ctx context.Context, scope domain.TenantScope, input krs.KeyResultUpdateInput) error
	DeleteKeyResult(ctx context.Context, scope domain.TenantScope, id int64) error
	MoveKeyResult(ctx context.Context, scope domain.TenantScope, krID int64, direction int) error
	UpsertKeyResultNote(ctx context.Context, scope domain.TenantScope, krID int64, text string, authorUserID int64) error
	GetKeyResultNote(ctx context.Context, scope domain.TenantScope, krID int64) (*domain.KeyResultNote, error)
	BatchLoadNotes(ctx context.Context, scope domain.TenantScope, krIDs []int64) (map[int64]*domain.KeyResultNote, error)
	GetBooleanMeta(ctx context.Context, scope domain.TenantScope, krID int64) (*domain.KRBoolean, error)
	UpdateKeyResultDescription(ctx context.Context, scope domain.TenantScope, krID int64, description string) error
	FindGoalIDByKR(ctx context.Context, scope domain.TenantScope, krID int64) (int64, error)
	FindGoalIDByStage(ctx context.Context, scope domain.TenantScope, stageID int64) (int64, error)
	UpdateNumericalCurrent(ctx context.Context, scope domain.TenantScope, krID int64, current float64) error
	UpdateHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error
	UpdateBoolean(ctx context.Context, scope domain.TenantScope, krID int64, done bool) error
	ListProjectStages(ctx context.Context, scope domain.TenantScope, krID int64) ([]domain.KRProjectStage, error)
	UpdateProjectStageDone(ctx context.Context, scope domain.TenantScope, stageID int64, done bool) error
	BatchUpdateProjectStagesDone(ctx context.Context, scope domain.TenantScope, krID int64, updates map[int64]bool) error
	UpsertNumericalMeta(ctx context.Context, scope domain.TenantScope, input krs.NumericalMetaInput) error
	UpsertBooleanMeta(ctx context.Context, scope domain.TenantScope, krID int64, done bool) error
	ReplaceProjectStages(ctx context.Context, scope domain.TenantScope, krID int64, stages []krs.ProjectStageInput) error
}

func (s *Service) Get(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error) {
	return s.repo.GetKeyResult(ctx, scope, id)
}
func (s *Service) Move(ctx context.Context, scope domain.TenantScope, krID int64, direction int) error {
	return s.repo.MoveKeyResult(ctx, scope, krID, direction)
}
func (s *Service) UpdateDescription(ctx context.Context, scope domain.TenantScope, krID int64, description string) error {
	return s.repo.UpdateKeyResultDescription(ctx, scope, krID, description)
}
func (s *Service) UpdateHealthStatus(ctx context.Context, scope domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	if !domain.IsValidKRHealthStatus(string(status)) {
		return fmt.Errorf("invalid health status: %s", status)
	}
	return s.repo.UpdateHealthStatus(ctx, scope, krID, status)
}
func (s *Service) FindGoalIDByKR(ctx context.Context, scope domain.TenantScope, krID int64) (int64, error) {
	return s.repo.FindGoalIDByKR(ctx, scope, krID)
}
func (s *Service) FindGoalIDByStage(ctx context.Context, scope domain.TenantScope, stageID int64) (int64, error) {
	return s.repo.FindGoalIDByStage(ctx, scope, stageID)
}
func (s *Service) ApplyMeta(ctx context.Context, scope domain.TenantScope, krID int64, kind domain.KRKind, meta MetaInput) error {
	switch kind {
	case domain.KRKindNumerical:
		return s.repo.UpsertNumericalMeta(ctx, scope, krs.NumericalMetaInput{
			KeyResultID:  krID,
			StartValue:   meta.NumericalStart,
			TargetValue:  meta.NumericalTarget,
			CurrentValue: meta.NumericalCurrent,
			Unit:         meta.NumericalUnit,
			Checkpoints:  meta.NumericalCheckpoints,
		})
	case domain.KRKindBoolean:
		return s.repo.UpsertBooleanMeta(ctx, scope, krID, meta.BooleanDone)
	case domain.KRKindProject:
		return s.repo.ReplaceProjectStages(ctx, scope, krID, meta.ProjectStages)
	default:
		return nil
	}
}
func (s *Service) AutoCompleteHealth(ctx context.Context, scope domain.TenantScope, krID int64, kr domain.KeyResult, before, after int) {
	if before < 100 && after == 100 && kr.HealthStatus != domain.KRHealthDone {
		// best-effort: an auto-complete failure must not fail the progress mutation
		_ = s.repo.UpdateHealthStatus(ctx, scope, krID, domain.KRHealthDone)
	}
}

// — Однострочные операции над сущностью, нужные сценариям слоя usecase. —

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) BatchLoadNotes(ctx context.Context, scope domain.TenantScope, krIDs []int64) (map[int64]*domain.KeyResultNote, error) {
	return s.repo.BatchLoadNotes(ctx, scope, krIDs)
}

// Батчевая операция: один запрос на весь набор. Не превращать в цикл — это N+1.
func (s *Service) BatchUpdateProjectStagesDone(ctx context.Context, scope domain.TenantScope, krID int64, updates map[int64]bool) error {
	return s.repo.BatchUpdateProjectStagesDone(ctx, scope, krID, updates)
}

func (s *Service) Create(ctx context.Context, scope domain.TenantScope, input krs.KeyResultInput) (int64, error) {
	return s.repo.CreateKeyResult(ctx, scope, input)
}

func (s *Service) Delete(ctx context.Context, scope domain.TenantScope, id int64) error {
	return s.repo.DeleteKeyResult(ctx, scope, id)
}

func (s *Service) GetBooleanMeta(ctx context.Context, scope domain.TenantScope, krID int64) (*domain.KRBoolean, error) {
	return s.repo.GetBooleanMeta(ctx, scope, krID)
}

func (s *Service) GetNote(ctx context.Context, scope domain.TenantScope, krID int64) (*domain.KeyResultNote, error) {
	return s.repo.GetKeyResultNote(ctx, scope, krID)
}

func (s *Service) ListProjectStages(ctx context.Context, scope domain.TenantScope, krID int64) ([]domain.KRProjectStage, error) {
	return s.repo.ListProjectStages(ctx, scope, krID)
}

func (s *Service) Update(ctx context.Context, scope domain.TenantScope, input krs.KeyResultUpdateInput) error {
	return s.repo.UpdateKeyResult(ctx, scope, input)
}

func (s *Service) UpdateBoolean(ctx context.Context, scope domain.TenantScope, krID int64, done bool) error {
	return s.repo.UpdateBoolean(ctx, scope, krID, done)
}

func (s *Service) UpdateNumericalCurrent(ctx context.Context, scope domain.TenantScope, krID int64, current float64) error {
	return s.repo.UpdateNumericalCurrent(ctx, scope, krID, current)
}

func (s *Service) UpsertNote(ctx context.Context, scope domain.TenantScope, krID int64, text string, authorUserID int64) error {
	return s.repo.UpsertKeyResultNote(ctx, scope, krID, text, authorUserID)
}
