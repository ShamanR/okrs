package servicetest

// TODO(refactoring): поля и типы-аргументы этого фейка экспортированы только потому,
// что он понадобился трём пакетам сразу (usecase/goal, usecase/keyresult, service/goal).
// Экспорт — шум: снаружи пакета servicetest эти имена никому не нужны. Альтернативы:
// (а) вернуть поля в приватные и добавить конструкторы-билдеры вида
//     NewGoalStore().WithGoals(...).WithStatuses(...);
// (б) оставить как есть — цена ровно косметическая.
// Не делалось в рамках рефакторинга: не относится к разделению слоёв.

import (
	"context"
	"errors"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/shares"
	storeteams "okrs/internal/store/teams"
)

// GoalStore is the goal-focused in-memory double: unlike Store it records the calls a
// goal scenario makes (status writes, share replacements, copies), so tests can assert
// orchestration rather than final state.

type GoalStore struct {
	Goals      map[int64]domain.Goal
	GoalShares map[int64][]shares.GoalShare
	Statuses   map[[2]int64]domain.TeamPeriodStatus
	// goals remaining after a delete (used to drive resetStatusIfNoGoals)
	GoalsAfterDelete map[int64]map[int64][]domain.Goal
	KeyResults       map[int64]domain.KeyResult
	NextGoalID       int64

	// call tracking
	SetStatusCalls       []SetStatusArg
	DeleteGoalCalls      []int64
	DeleteShareCalls     []DeleteShareArg
	UpdateOwnerCalls     []UpdateOwnerArg
	ReplaceSharesCalls   []ReplaceSharesArg
	UpsertNumericalCalls []krs.NumericalMetaInput
	UpsertBoolCalls      []UpsertBoolArg
	ReplaceStageCalls    []ReplaceStagesArg
	CopyGoalCalls        []goals.CopyGoalInput
	MissingTeams         map[int64]bool
	MissingPeriods       map[int64]bool

	// comment/reply fakes
	CommentAuthor      int64
	CommentIsTask      bool
	CommentMetaErr     error
	DeleteCommentErr   error
	AddReplyErr        error
	DeleteCommentCalls []int64
}

type SetStatusArg struct {
	TeamID, PeriodID int64
	Status           domain.TeamPeriodStatus
}
type DeleteShareArg struct{ GoalID, TeamID int64 }
type UpdateOwnerArg struct {
	GoalID, TeamID int64
	Weight         int
}
type ReplaceSharesArg struct {
	GoalID int64
	Shares []shares.GoalShareInput
}
type UpsertBoolArg struct {
	KRID int64
	Done bool
}
type ReplaceStagesArg struct {
	KRID   int64
	Stages []krs.ProjectStageInput
}

func NewGoalStore() *GoalStore {
	return &GoalStore{
		Goals:            make(map[int64]domain.Goal),
		GoalShares:       make(map[int64][]shares.GoalShare),
		Statuses:         make(map[[2]int64]domain.TeamPeriodStatus),
		GoalsAfterDelete: make(map[int64]map[int64][]domain.Goal),
		KeyResults:       make(map[int64]domain.KeyResult),
		NextGoalID:       1,
	}
}

// — Store interface implementation (tracking methods first) —

func (f *GoalStore) GetGoal(_ context.Context, _ domain.TenantScope, id int64) (domain.Goal, error) {
	return f.Goals[id], nil
}
func (f *GoalStore) CreateGoal(_ context.Context, _ domain.TenantScope, input goals.GoalInput) (int64, error) {
	id := f.NextGoalID
	f.NextGoalID++
	return id, nil
}
func (f *GoalStore) DeleteGoal(_ context.Context, _ domain.TenantScope, id int64) error {
	f.DeleteGoalCalls = append(f.DeleteGoalCalls, id)
	return nil
}
func (f *GoalStore) CopyGoal(_ context.Context, _ domain.TenantScope, in goals.CopyGoalInput) (int64, error) {
	f.CopyGoalCalls = append(f.CopyGoalCalls, in)
	id := f.NextGoalID
	f.NextGoalID++
	return id, nil
}
func (f *GoalStore) DeleteGoalShare(_ context.Context, _ domain.TenantScope, goalID, teamID int64) error {
	f.DeleteShareCalls = append(f.DeleteShareCalls, DeleteShareArg{goalID, teamID})
	return nil
}
func (f *GoalStore) UpdateGoalOwner(_ context.Context, _ domain.TenantScope, goalID, teamID int64, weight int) error {
	f.UpdateOwnerCalls = append(f.UpdateOwnerCalls, UpdateOwnerArg{goalID, teamID, weight})
	return nil
}
func (f *GoalStore) SetTeamPeriodStatus(_ context.Context, _ domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus) error {
	f.SetStatusCalls = append(f.SetStatusCalls, SetStatusArg{teamID, periodID, status})
	return nil
}
func (f *GoalStore) SetTeamPeriodStatuses(context.Context, domain.TenantScope, int64, []int64, domain.TeamPeriodStatus) error {
	return nil
}
func (f *GoalStore) ReplaceGoalShares(_ context.Context, _ domain.TenantScope, goalID int64, shares []shares.GoalShareInput) error {
	f.ReplaceSharesCalls = append(f.ReplaceSharesCalls, ReplaceSharesArg{goalID, shares})
	return nil
}
func (f *GoalStore) UpsertNumericalMeta(_ context.Context, _ domain.TenantScope, input krs.NumericalMetaInput) error {
	f.UpsertNumericalCalls = append(f.UpsertNumericalCalls, input)
	return nil
}
func (f *GoalStore) UpsertBooleanMeta(_ context.Context, _ domain.TenantScope, krID int64, done bool) error {
	f.UpsertBoolCalls = append(f.UpsertBoolCalls, UpsertBoolArg{krID, done})
	return nil
}
func (f *GoalStore) ReplaceProjectStages(_ context.Context, _ domain.TenantScope, krID int64, stages []krs.ProjectStageInput) error {
	f.ReplaceStageCalls = append(f.ReplaceStageCalls, ReplaceStagesArg{krID, stages})
	return nil
}
func (f *GoalStore) GetTeamPeriodStatus(_ context.Context, _ domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	if s, ok := f.Statuses[[2]int64{teamID, periodID}]; ok {
		return s, nil
	}
	return domain.TeamPeriodStatusNoGoals, nil
}
func (f *GoalStore) ListGoalShares(_ context.Context, _ domain.TenantScope, goalID int64) ([]shares.GoalShare, error) {
	return f.GoalShares[goalID], nil
}
func (f *GoalStore) ListGoalSharesByGoalIDs(_ context.Context, _ domain.TenantScope, goalIDs []int64) (map[int64][]shares.GoalShare, error) {
	result := make(map[int64][]shares.GoalShare, len(goalIDs))
	for _, id := range goalIDs {
		if sl := f.GoalShares[id]; len(sl) > 0 {
			result[id] = sl
		}
	}
	return result, nil
}
func (f *GoalStore) ListGoalsByTeamPeriod(_ context.Context, _ domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	if m := f.GoalsAfterDelete[teamID]; m != nil {
		return m[periodID], nil
	}
	return nil, nil
}
func (f *GoalStore) GetKeyResult(_ context.Context, _ domain.TenantScope, id int64) (domain.KeyResult, error) {
	return f.KeyResults[id], nil
}
func (f *GoalStore) GetBooleanMeta(context.Context, domain.TenantScope, int64) (*domain.KRBoolean, error) {
	return nil, nil
}
func (f *GoalStore) GetKeyResultNote(context.Context, domain.TenantScope, int64) (*domain.KeyResultNote, error) {
	return nil, nil
}
func (f *GoalStore) BatchLoadNotes(context.Context, domain.TenantScope, []int64) (map[int64]*domain.KeyResultNote, error) {
	return nil, nil
}
func (f *GoalStore) CreateKeyResult(_ context.Context, _ domain.TenantScope, _ krs.KeyResultInput) (int64, error) {
	id := f.NextGoalID
	f.NextGoalID++
	return id, nil
}

// — no-op implementations for the remaining Store interface methods —

func (f *GoalStore) ListTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	return nil, nil
}
func (f *GoalStore) ListDeletedTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	return nil, nil
}
func (f *GoalStore) ListAllTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	return nil, nil
}
func (f *GoalStore) GetTeam(_ context.Context, _ domain.TenantScope, id int64) (domain.Team, error) {
	if f.MissingTeams[id] {
		return domain.Team{}, errors.New("team not found")
	}
	return domain.Team{}, nil
}
func (f *GoalStore) CreateTeam(_ context.Context, _ domain.TenantScope, _ storeteams.TeamInput) (int64, error) {
	return 0, nil
}
func (f *GoalStore) UpdateTeam(_ context.Context, _ domain.TenantScope, _ storeteams.TeamInput, _ int64) error {
	return nil
}
func (f *GoalStore) ListPeriods(_ context.Context, _ domain.TenantScope) ([]domain.Period, error) {
	return nil, nil
}
func (f *GoalStore) GetPeriod(_ context.Context, _ domain.TenantScope, id int64) (domain.Period, error) {
	if f.MissingPeriods[id] {
		return domain.Period{}, errors.New("period not found")
	}
	return domain.Period{}, nil
}
func (f *GoalStore) FindPeriodForDate(_ context.Context, _ domain.TenantScope, _ time.Time) (domain.Period, error) {
	return domain.Period{}, nil
}
func (f *GoalStore) CreatePeriod(_ context.Context, _ domain.TenantScope, _ periods.PeriodInput) (int64, error) {
	return 0, nil
}
func (f *GoalStore) UpdatePeriod(_ context.Context, _ domain.TenantScope, _ int64, _ periods.PeriodInput) error {
	return nil
}
func (f *GoalStore) DeletePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *GoalStore) ArchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *GoalStore) UnarchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *GoalStore) ListGoalsByTeamsPeriod(_ context.Context, _ domain.TenantScope, _ int64, _ []int64) (map[int64][]domain.Goal, error) {
	return nil, nil
}
func (f *GoalStore) UpdateGoal(_ context.Context, _ domain.TenantScope, in goals.GoalUpdateInput) error {
	if g, ok := f.Goals[in.ID]; ok {
		g.Title, g.Description, g.Priority, g.Weight = in.Title, in.Description, in.Priority, in.Weight
		f.Goals[in.ID] = g
	}
	return nil
}
func (f *GoalStore) UpdateGoalFields(_ context.Context, _ domain.TenantScope, _ goals.GoalFieldsUpdateInput) error {
	return nil
}
func (f *GoalStore) MoveGoal(_ context.Context, _ domain.TenantScope, _ int64, _ int64, _ int) error {
	return nil
}
func (f *GoalStore) AddGoalComment(_ context.Context, _ domain.TenantScope, _ int64, _ string, _ int64) (int64, error) {
	return 1, nil
}
func (f *GoalStore) SetGoalCommentResolved(_ context.Context, _ domain.TenantScope, _, _ int64, _ bool, _ int64) (bool, error) {
	return true, nil
}
func (f *GoalStore) ListGoalComments(_ context.Context, _ domain.TenantScope, _ int64) ([]domain.GoalComment, error) {
	return nil, nil
}
func (f *GoalStore) ListGoalCommentsByGoals(_ context.Context, _ domain.TenantScope, _ []int64) (map[int64][]domain.GoalComment, error) {
	return nil, nil
}
func (f *GoalStore) ListGoalOwnerTeamIDs(_ context.Context, _ domain.TenantScope, _ []int64) (map[int64]int64, error) {
	return nil, nil
}
func (f *GoalStore) ListGoalsByIDs(_ context.Context, _ domain.TenantScope, _ []int64) ([]domain.Goal, error) {
	return nil, nil
}
func (f *GoalStore) AddGoalReply(_ context.Context, _ domain.TenantScope, _, _ int64, _ string, _ int64) (int64, error) {
	if f.AddReplyErr != nil {
		return 0, f.AddReplyErr
	}
	return 1, nil
}
func (f *GoalStore) GetGoalCommentMeta(_ context.Context, _ domain.TenantScope, _, _ int64) (int64, bool, error) {
	if f.CommentMetaErr != nil {
		return 0, false, f.CommentMetaErr
	}
	return f.CommentAuthor, f.CommentIsTask, nil
}
func (f *GoalStore) DeleteGoalComment(_ context.Context, _ domain.TenantScope, _, commentID int64) error {
	if f.DeleteCommentErr != nil {
		return f.DeleteCommentErr
	}
	f.DeleteCommentCalls = append(f.DeleteCommentCalls, commentID)
	return nil
}
func (f *GoalStore) GetGoalShare(_ context.Context, _ domain.TenantScope, _, _ int64) (shares.GoalShare, error) {
	return shares.GoalShare{}, nil
}
func (f *GoalStore) UpdateGoalTeamWeight(_ context.Context, _ domain.TenantScope, _, _ int64, _ int) error {
	return nil
}
func (f *GoalStore) GetTeamPeriodStatusWithTime(_ context.Context, _ domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error) {
	s, err := f.GetTeamPeriodStatus(context.Background(), domain.TenantScope{}, teamID, periodID)
	return s, nil, err
}
func (f *GoalStore) ListTeamPeriodStatuses(_ context.Context, _ domain.TenantScope, _ int64, _ []int64) (map[int64]domain.TeamPeriodStatus, error) {
	return nil, nil
}
func (f *GoalStore) ListTeamLastGoalUpdateInPeriod(_ context.Context, _ domain.TenantScope, _ int64, _ []int64) (map[int64]time.Time, error) {
	return nil, nil
}
func (f *GoalStore) ListGoalsForPeriods(_ context.Context, _ domain.TenantScope, _ []int64, _ []int64, _ bool) ([]domain.Goal, error) {
	return nil, nil
}
func (f *GoalStore) TeamHasGoals(_ context.Context, _ domain.TenantScope, _ int64) (bool, error) {
	return false, nil
}
func (f *GoalStore) TeamHasGoalsInPeriod(_ context.Context, _ domain.TenantScope, _, _ int64) (bool, error) {
	return false, nil
}
func (f *GoalStore) ListTeamIDsWithGoalsInPeriod(_ context.Context, _ domain.TenantScope, _ int64) (map[int64]struct{}, error) {
	return nil, nil
}
func (f *GoalStore) SoftDeleteTeam(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *GoalStore) RestoreTeam(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *GoalStore) HardDeleteTeam(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *GoalStore) UpdateKeyResult(_ context.Context, _ domain.TenantScope, _ krs.KeyResultUpdateInput) error {
	return nil
}
func (f *GoalStore) DeleteKeyResult(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *GoalStore) MoveKeyResult(_ context.Context, _ domain.TenantScope, _ int64, _ int) error {
	return nil
}
func (f *GoalStore) UpsertKeyResultNote(_ context.Context, _ domain.TenantScope, _ int64, _ string, _ int64) error {
	return nil
}
func (f *GoalStore) UpdateKeyResultDescription(_ context.Context, _ domain.TenantScope, _ int64, _ string) error {
	return nil
}
func (f *GoalStore) FindGoalIDByKR(_ context.Context, _ domain.TenantScope, _ int64) (int64, error) {
	return 0, nil
}
func (f *GoalStore) FindGoalIDByStage(_ context.Context, _ domain.TenantScope, _ int64) (int64, error) {
	return 0, nil
}
func (f *GoalStore) UpdateNumericalCurrent(_ context.Context, _ domain.TenantScope, _ int64, _ float64) error {
	return nil
}

// ApplyCheckIn: GoalStore — узкий фейк для целевых сценариев, чек-ин в них не
// участвует, поэтому запись не имитируется. Делать иначе значило бы завести здесь
// второе состояние KR, которого у этого фейка нет.
func (f *GoalStore) ApplyCheckIn(context.Context, domain.TenantScope, int64, krs.CheckInWrites) error {
	return nil
}

func (f *GoalStore) UpdateHealthStatus(_ context.Context, _ domain.TenantScope, _ int64, _ domain.KRHealthStatus) error {
	return nil
}
func (f *GoalStore) UpdateBoolean(_ context.Context, _ domain.TenantScope, _ int64, _ bool) error {
	return nil
}
func (f *GoalStore) ListProjectStages(_ context.Context, _ domain.TenantScope, _ int64) ([]domain.KRProjectStage, error) {
	return nil, nil
}
func (f *GoalStore) UpdateProjectStageDone(_ context.Context, _ domain.TenantScope, _ int64, _ bool) error {
	return nil
}
func (f *GoalStore) BatchUpdateProjectStagesDone(_ context.Context, _ domain.TenantScope, _ int64, _ map[int64]bool) error {
	return nil
}
func (f *GoalStore) GetUsersByDisplayNames(_ context.Context, _ []string) ([]*domain.User, error) {
	return nil, nil
}
func (f *GoalStore) SearchUsersUnrestricted(_ context.Context, _ string, _ int) ([]*domain.User, error) {
	return nil, nil
}
func (f *GoalStore) SearchUsersInSet(_ context.Context, _ []int64, _ []string, _ string, _ int) ([]*domain.User, error) {
	return nil, nil
}
func (f *GoalStore) GetUsersByUDIDs(_ context.Context, _ []string) ([]*domain.User, error) {
	return nil, nil
}
func (f *GoalStore) ListUserLeadTeams(_ context.Context) (map[string]string, error) {
	return nil, nil
}
func (f *GoalStore) ValidateUDIDsExist(_ context.Context, _ []string) ([]string, error) {
	return nil, nil
}

// ── CreateGoal tests ──────────────────────────────────────────────────────────
