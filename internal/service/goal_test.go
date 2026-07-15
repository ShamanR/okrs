package service

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/shares"
	storeteams "okrs/internal/store/teams"
)

// ── goalFakeStore ─────────────────────────────────────────────────────────────

type goalFakeStore struct {
	goals      map[int64]domain.Goal
	goalShares map[int64][]shares.GoalShare
	statuses   map[[2]int64]domain.TeamPeriodStatus
	// goals remaining after a delete (used to drive resetStatusIfNoGoals)
	goalsAfterDelete map[int64]map[int64][]domain.Goal
	keyResults       map[int64]domain.KeyResult
	nextGoalID       int64

	// call tracking
	setStatusCalls       []setStatusArg
	deleteGoalCalls      []int64
	deleteShareCalls     []deleteShareArg
	updateOwnerCalls     []updateOwnerArg
	replaceSharesCalls   []replaceSharesArg
	upsertNumericalCalls []krs.NumericalMetaInput
	upsertBoolCalls      []upsertBoolArg
	replaceStageCalls    []replaceStagesArg
}

type setStatusArg struct {
	teamID, periodID int64
	status           domain.TeamPeriodStatus
}
type deleteShareArg struct{ goalID, teamID int64 }
type updateOwnerArg struct {
	goalID, teamID int64
	weight         int
}
type replaceSharesArg struct {
	goalID int64
	shares []shares.GoalShareInput
}
type upsertBoolArg struct {
	krID int64
	done bool
}
type replaceStagesArg struct {
	krID   int64
	stages []krs.ProjectStageInput
}

func newGoalFakeStore() *goalFakeStore {
	return &goalFakeStore{
		goals:            make(map[int64]domain.Goal),
		goalShares:       make(map[int64][]shares.GoalShare),
		statuses:         make(map[[2]int64]domain.TeamPeriodStatus),
		goalsAfterDelete: make(map[int64]map[int64][]domain.Goal),
		keyResults:       make(map[int64]domain.KeyResult),
		nextGoalID:       1,
	}
}

func newGoalTestService(gf *goalFakeStore) *Service {
	return New(Deps{Teams: gf, Goals: gf, Shares: gf, Periods: gf, KRs: gf, Statuses: gf, Users: gf})
}

// — Store interface implementation (tracking methods first) —

func (f *goalFakeStore) GetGoal(_ context.Context, _ domain.TenantScope, id int64) (domain.Goal, error) {
	return f.goals[id], nil
}
func (f *goalFakeStore) CreateGoal(_ context.Context, _ domain.TenantScope, input goals.GoalInput) (int64, error) {
	id := f.nextGoalID
	f.nextGoalID++
	return id, nil
}
func (f *goalFakeStore) DeleteGoal(_ context.Context, _ domain.TenantScope, id int64) error {
	f.deleteGoalCalls = append(f.deleteGoalCalls, id)
	return nil
}
func (f *goalFakeStore) DeleteGoalShare(_ context.Context, _ domain.TenantScope, goalID, teamID int64) error {
	f.deleteShareCalls = append(f.deleteShareCalls, deleteShareArg{goalID, teamID})
	return nil
}
func (f *goalFakeStore) UpdateGoalOwner(_ context.Context, _ domain.TenantScope, goalID, teamID int64, weight int) error {
	f.updateOwnerCalls = append(f.updateOwnerCalls, updateOwnerArg{goalID, teamID, weight})
	return nil
}
func (f *goalFakeStore) SetTeamPeriodStatus(_ context.Context, _ domain.TenantScope, teamID, periodID int64, status domain.TeamPeriodStatus) error {
	f.setStatusCalls = append(f.setStatusCalls, setStatusArg{teamID, periodID, status})
	return nil
}
func (f *goalFakeStore) ReplaceGoalShares(_ context.Context, _ domain.TenantScope, goalID int64, shares []shares.GoalShareInput) error {
	f.replaceSharesCalls = append(f.replaceSharesCalls, replaceSharesArg{goalID, shares})
	return nil
}
func (f *goalFakeStore) UpsertNumericalMeta(_ context.Context, _ domain.TenantScope, input krs.NumericalMetaInput) error {
	f.upsertNumericalCalls = append(f.upsertNumericalCalls, input)
	return nil
}
func (f *goalFakeStore) UpsertBooleanMeta(_ context.Context, _ domain.TenantScope, krID int64, done bool) error {
	f.upsertBoolCalls = append(f.upsertBoolCalls, upsertBoolArg{krID, done})
	return nil
}
func (f *goalFakeStore) ReplaceProjectStages(_ context.Context, _ domain.TenantScope, krID int64, stages []krs.ProjectStageInput) error {
	f.replaceStageCalls = append(f.replaceStageCalls, replaceStagesArg{krID, stages})
	return nil
}
func (f *goalFakeStore) GetTeamPeriodStatus(_ context.Context, _ domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	if s, ok := f.statuses[[2]int64{teamID, periodID}]; ok {
		return s, nil
	}
	return domain.TeamPeriodStatusNoGoals, nil
}
func (f *goalFakeStore) ListGoalShares(_ context.Context, _ domain.TenantScope, goalID int64) ([]shares.GoalShare, error) {
	return f.goalShares[goalID], nil
}
func (f *goalFakeStore) ListGoalSharesByGoalIDs(_ context.Context, _ domain.TenantScope, goalIDs []int64) (map[int64][]shares.GoalShare, error) {
	result := make(map[int64][]shares.GoalShare, len(goalIDs))
	for _, id := range goalIDs {
		if sl := f.goalShares[id]; len(sl) > 0 {
			result[id] = sl
		}
	}
	return result, nil
}
func (f *goalFakeStore) ListGoalsByTeamPeriod(_ context.Context, _ domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	if m := f.goalsAfterDelete[teamID]; m != nil {
		return m[periodID], nil
	}
	return nil, nil
}
func (f *goalFakeStore) GetKeyResult(_ context.Context, _ domain.TenantScope, id int64) (domain.KeyResult, error) {
	return f.keyResults[id], nil
}
func (f *goalFakeStore) GetBooleanMeta(context.Context, domain.TenantScope, int64) (*domain.KRBoolean, error) {
	return nil, nil
}
func (f *goalFakeStore) GetKeyResultNote(context.Context, domain.TenantScope, int64) (*domain.KeyResultNote, error) {
	return nil, nil
}
func (f *goalFakeStore) CreateKeyResult(_ context.Context, _ domain.TenantScope, _ krs.KeyResultInput) (int64, error) {
	id := f.nextGoalID
	f.nextGoalID++
	return id, nil
}

// — no-op implementations for the remaining Store interface methods —

func (f *goalFakeStore) ListTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	return nil, nil
}
func (f *goalFakeStore) ListDeletedTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	return nil, nil
}
func (f *goalFakeStore) ListAllTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	return nil, nil
}
func (f *goalFakeStore) GetTeam(_ context.Context, _ domain.TenantScope, _ int64) (domain.Team, error) {
	return domain.Team{}, nil
}
func (f *goalFakeStore) CreateTeam(_ context.Context, _ domain.TenantScope, _ storeteams.TeamInput) (int64, error) {
	return 0, nil
}
func (f *goalFakeStore) UpdateTeam(_ context.Context, _ domain.TenantScope, _ storeteams.TeamInput, _ int64) error {
	return nil
}
func (f *goalFakeStore) ListPeriods(_ context.Context, _ domain.TenantScope) ([]domain.Period, error) {
	return nil, nil
}
func (f *goalFakeStore) GetPeriod(_ context.Context, _ domain.TenantScope, _ int64) (domain.Period, error) {
	return domain.Period{}, nil
}
func (f *goalFakeStore) FindPeriodForDate(_ context.Context, _ domain.TenantScope, _ time.Time) (domain.Period, error) {
	return domain.Period{}, nil
}
func (f *goalFakeStore) CreatePeriod(_ context.Context, _ domain.TenantScope, _ periods.PeriodInput) (int64, error) {
	return 0, nil
}
func (f *goalFakeStore) UpdatePeriod(_ context.Context, _ domain.TenantScope, _ int64, _ periods.PeriodInput) error {
	return nil
}
func (f *goalFakeStore) DeletePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *goalFakeStore) ArchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *goalFakeStore) UnarchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *goalFakeStore) ListGoalsByTeamsPeriod(_ context.Context, _ domain.TenantScope, _ int64, _ []int64) (map[int64][]domain.Goal, error) {
	return nil, nil
}
func (f *goalFakeStore) UpdateGoal(_ context.Context, _ domain.TenantScope, in goals.GoalUpdateInput) error {
	if g, ok := f.goals[in.ID]; ok {
		g.Title, g.Description, g.Priority, g.Weight = in.Title, in.Description, in.Priority, in.Weight
		f.goals[in.ID] = g
	}
	return nil
}
func (f *goalFakeStore) UpdateGoalFields(_ context.Context, _ domain.TenantScope, _ goals.GoalFieldsUpdateInput) error {
	return nil
}
func (f *goalFakeStore) MoveGoal(_ context.Context, _ domain.TenantScope, _ int64, _ int64, _ int) error {
	return nil
}
func (f *goalFakeStore) AddGoalComment(_ context.Context, _ domain.TenantScope, _ int64, _ string, _ int64) (int64, error) {
	return 1, nil
}
func (f *goalFakeStore) SetGoalCommentResolved(_ context.Context, _ domain.TenantScope, _, _ int64, _ bool, _ int64) (bool, error) {
	return true, nil
}
func (f *goalFakeStore) ListGoalComments(_ context.Context, _ domain.TenantScope, _ int64) ([]domain.GoalComment, error) {
	return nil, nil
}
func (f *goalFakeStore) GetGoalShare(_ context.Context, _ domain.TenantScope, _, _ int64) (shares.GoalShare, error) {
	return shares.GoalShare{}, nil
}
func (f *goalFakeStore) UpdateGoalTeamWeight(_ context.Context, _ domain.TenantScope, _, _ int64, _ int) error { return nil }
func (f *goalFakeStore) GetTeamPeriodStatusWithTime(_ context.Context, _ domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error) {
	s, err := f.GetTeamPeriodStatus(context.Background(), domain.TenantScope{}, teamID, periodID)
	return s, nil, err
}
func (f *goalFakeStore) ListTeamPeriodStatuses(_ context.Context, _ domain.TenantScope, _ int64, _ []int64) (map[int64]domain.TeamPeriodStatus, error) {
	return nil, nil
}
func (f *goalFakeStore) ListTeamLastGoalUpdateInPeriod(_ context.Context, _ domain.TenantScope, _ int64, _ []int64) (map[int64]time.Time, error) {
	return nil, nil
}
func (f *goalFakeStore) TeamHasGoals(_ context.Context, _ domain.TenantScope, _ int64) (bool, error) {
	return false, nil
}
func (f *goalFakeStore) TeamHasGoalsInPeriod(_ context.Context, _ domain.TenantScope, _, _ int64) (bool, error) {
	return false, nil
}
func (f *goalFakeStore) ListTeamIDsWithGoalsInPeriod(_ context.Context, _ domain.TenantScope, _ int64) (map[int64]struct{}, error) {
	return nil, nil
}
func (f *goalFakeStore) SoftDeleteTeam(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *goalFakeStore) RestoreTeam(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *goalFakeStore) HardDeleteTeam(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *goalFakeStore) UpdateKeyResult(_ context.Context, _ domain.TenantScope, _ krs.KeyResultUpdateInput) error {
	return nil
}
func (f *goalFakeStore) DeleteKeyResult(_ context.Context, _ domain.TenantScope, _ int64) error      { return nil }
func (f *goalFakeStore) MoveKeyResult(_ context.Context, _ domain.TenantScope, _ int64, _ int) error { return nil }
func (f *goalFakeStore) UpsertKeyResultNote(_ context.Context, _ domain.TenantScope, _ int64, _ string, _ int64) error {
	return nil
}
func (f *goalFakeStore) UpdateKeyResultDescription(_ context.Context, _ domain.TenantScope, _ int64, _ string) error {
	return nil
}
func (f *goalFakeStore) FindGoalIDByKR(_ context.Context, _ domain.TenantScope, _ int64) (int64, error)    { return 0, nil }
func (f *goalFakeStore) FindGoalIDByStage(_ context.Context, _ domain.TenantScope, _ int64) (int64, error) { return 0, nil }
func (f *goalFakeStore) UpdateNumericalCurrent(_ context.Context, _ domain.TenantScope, _ int64, _ float64) error {
	return nil
}
func (f *goalFakeStore) UpdateBoolean(_ context.Context, _ domain.TenantScope, _ int64, _ bool) error { return nil }
func (f *goalFakeStore) ListProjectStages(_ context.Context, _ domain.TenantScope, _ int64) ([]domain.KRProjectStage, error) {
	return nil, nil
}
func (f *goalFakeStore) UpdateProjectStageDone(_ context.Context, _ domain.TenantScope, _ int64, _ bool) error { return nil }
func (f *goalFakeStore) BatchUpdateProjectStagesDone(_ context.Context, _ domain.TenantScope, _ int64, _ map[int64]bool) error {
	return nil
}
func (f *goalFakeStore) GetUsersByDisplayNames(_ context.Context, _ []string) ([]*domain.User, error) {
	return nil, nil
}
func (f *goalFakeStore) SearchUsersUnrestricted(_ context.Context, _ string, _ int) ([]*domain.User, error) {
	return nil, nil
}
func (f *goalFakeStore) SearchUsersInSet(_ context.Context, _ []int64, _ []string, _ string, _ int) ([]*domain.User, error) {
	return nil, nil
}
func (f *goalFakeStore) GetUsersByUDIDs(_ context.Context, _ []string) ([]*domain.User, error) {
	return nil, nil
}
func (f *goalFakeStore) ListUserLeadTeams(_ context.Context) (map[string]string, error) {
	return nil, nil
}
func (f *goalFakeStore) ValidateUDIDsExist(_ context.Context, _ []string) ([]string, error) {
	return nil, nil
}

// ── CreateGoal tests ──────────────────────────────────────────────────────────

func TestCreateGoalBlockedByClosedPeriod(t *testing.T) {
	st := newGoalFakeStore()
	st.statuses[[2]int64{1, 10}] = domain.TeamPeriodStatusClosed
	svc := newGoalTestService(st)

	_, err := svc.CreateGoal(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 1, PeriodID: 10}, 1)
	if err != ErrPeriodClosed {
		t.Fatalf("expected ErrPeriodClosed, got %v", err)
	}
}

func TestCreateGoalBlockedByInProgressPeriod(t *testing.T) {
	st := newGoalFakeStore()
	st.statuses[[2]int64{1, 10}] = domain.TeamPeriodStatusInProgress
	svc := newGoalTestService(st)

	_, err := svc.CreateGoal(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 1, PeriodID: 10}, 1)
	if err != ErrPeriodClosed {
		t.Fatalf("expected ErrPeriodClosed for in_progress, got %v", err)
	}
}

func TestCreateGoalAdvancesStatusFromNoGoals(t *testing.T) {
	st := newGoalFakeStore()
	// no entry in statuses → defaults to NoGoals
	svc := newGoalTestService(st)

	goalID, err := svc.CreateGoal(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 2, PeriodID: 5}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if goalID == 0 {
		t.Fatal("expected non-zero goal ID")
	}
	if len(st.setStatusCalls) != 1 {
		t.Fatalf("expected SetTeamPeriodStatus called once, got %d", len(st.setStatusCalls))
	}
	call := st.setStatusCalls[0]
	if call.teamID != 2 || call.periodID != 5 || call.status != domain.TeamPeriodStatusForming {
		t.Fatalf("unexpected SetTeamPeriodStatus call: %+v", call)
	}
}

func TestCreateGoalKeepsStatusWhenAlreadyForming(t *testing.T) {
	st := newGoalFakeStore()
	st.statuses[[2]int64{2, 5}] = domain.TeamPeriodStatusForming
	svc := newGoalTestService(st)

	_, err := svc.CreateGoal(context.Background(), domain.TenantScope{TenantID: 1}, goals.GoalInput{TeamID: 2, PeriodID: 5}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.setStatusCalls) != 0 {
		t.Fatalf("expected no SetTeamPeriodStatus call when already Forming, got %d", len(st.setStatusCalls))
	}
}

// ── DeleteGoal tests ──────────────────────────────────────────────────────────

func TestDeleteGoalBySharedTeamRemovesShareOnly(t *testing.T) {
	st := newGoalFakeStore()
	st.goals[7] = domain.Goal{ID: 7, TeamID: 1, PeriodID: 5}
	svc := newGoalTestService(st)

	effectiveTeam, periodID, err := svc.DeleteGoal(context.Background(), domain.TenantScope{TenantID: 1}, 7, 2, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if effectiveTeam != 2 || periodID != 5 {
		t.Fatalf("expected effectiveTeam=2 periodID=5, got %d %d", effectiveTeam, periodID)
	}
	if len(st.deleteShareCalls) != 1 || st.deleteShareCalls[0].goalID != 7 || st.deleteShareCalls[0].teamID != 2 {
		t.Fatalf("expected share deletion for goal 7 / team 2, got %+v", st.deleteShareCalls)
	}
	if len(st.deleteGoalCalls) != 0 {
		t.Fatal("goal itself should not be deleted when shared team removes share")
	}
}

func TestDeleteGoalByOwnerTransfersOwnershipWhenShared(t *testing.T) {
	st := newGoalFakeStore()
	st.goals[8] = domain.Goal{ID: 8, TeamID: 1, PeriodID: 5, Weight: 50}
	st.goalShares[8] = []shares.GoalShare{{GoalID: 8, TeamID: 3, Weight: 30}}
	svc := newGoalTestService(st)

	_, _, err := svc.DeleteGoal(context.Background(), domain.TenantScope{TenantID: 1}, 8, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Ownership transferred to team 3
	if len(st.updateOwnerCalls) != 1 || st.updateOwnerCalls[0].teamID != 3 {
		t.Fatalf("expected ownership transfer to team 3, got %+v", st.updateOwnerCalls)
	}
	// Old share for team 3 removed
	if len(st.deleteShareCalls) != 1 || st.deleteShareCalls[0].teamID != 3 {
		t.Fatalf("expected share deletion for team 3, got %+v", st.deleteShareCalls)
	}
	// Goal itself not deleted
	if len(st.deleteGoalCalls) != 0 {
		t.Fatal("goal itself should not be deleted when ownership transfers")
	}
}

func TestDeleteGoalByOwnerDeletesGoalWhenNoSharesAndPeriodOpen(t *testing.T) {
	st := newGoalFakeStore()
	st.goals[9] = domain.Goal{ID: 9, TeamID: 1, PeriodID: 5}
	// statuses defaults to NoGoals → open period
	svc := newGoalTestService(st)

	_, _, err := svc.DeleteGoal(context.Background(), domain.TenantScope{TenantID: 1}, 9, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.deleteGoalCalls) != 1 || st.deleteGoalCalls[0] != 9 {
		t.Fatalf("expected goal 9 to be deleted, got %+v", st.deleteGoalCalls)
	}
}

func TestDeleteGoalByOwnerBlockedByClosedPeriodWithNoShares(t *testing.T) {
	st := newGoalFakeStore()
	st.goals[10] = domain.Goal{ID: 10, TeamID: 1, PeriodID: 5}
	st.statuses[[2]int64{1, 5}] = domain.TeamPeriodStatusClosed
	svc := newGoalTestService(st)

	_, _, err := svc.DeleteGoal(context.Background(), domain.TenantScope{TenantID: 1}, 10, 1, 1)
	if err != ErrPeriodClosed {
		t.Fatalf("expected ErrPeriodClosed, got %v", err)
	}
	if len(st.deleteGoalCalls) != 0 {
		t.Fatal("goal should not be deleted when period is closed")
	}
}

func TestDeleteGoalResetsStatusWhenLastGoalRemoved(t *testing.T) {
	st := newGoalFakeStore()
	st.goals[11] = domain.Goal{ID: 11, TeamID: 1, PeriodID: 5}
	st.statuses[[2]int64{1, 5}] = domain.TeamPeriodStatusForming
	// goalsAfterDelete is empty → no goals remain after deletion
	svc := newGoalTestService(st)

	_, _, err := svc.DeleteGoal(context.Background(), domain.TenantScope{TenantID: 1}, 11, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// resetStatusIfNoGoals should set status to NoGoals
	var resetCall *setStatusArg
	for i := range st.setStatusCalls {
		if st.setStatusCalls[i].status == domain.TeamPeriodStatusNoGoals {
			resetCall = &st.setStatusCalls[i]
		}
	}
	if resetCall == nil {
		t.Fatal("expected status reset to NoGoals after last goal deleted")
	}
}

// ── UpdateGoalOwnerAndShares tests ────────────────────────────────────────────

func TestUpdateGoalOwnerAndSharesBlockedByInProgressPeriod(t *testing.T) {
	st := newGoalFakeStore()
	st.goals[20] = domain.Goal{ID: 20, TeamID: 1, PeriodID: 10, Weight: 40}
	// team 2 is in_progress
	st.statuses[[2]int64{2, 10}] = domain.TeamPeriodStatusInProgress
	svc := newGoalTestService(st)

	_, _, err := svc.UpdateGoalOwnerAndShares(context.Background(), domain.TenantScope{TenantID: 1}, 20, []int64{2}, 1)
	if err != ErrCannotShareWithClosedPeriod {
		t.Fatalf("expected ErrCannotShareWithClosedPeriod, got %v", err)
	}
}

func TestUpdateGoalOwnerAndSharesChangesOwnerWhenCurrentOwnerNotSelected(t *testing.T) {
	st := newGoalFakeStore()
	st.goals[21] = domain.Goal{ID: 21, TeamID: 1, PeriodID: 10, Weight: 40}
	// team 3 has open period (defaults to NoGoals)
	svc := newGoalTestService(st)

	ownerID, periodID, err := svc.UpdateGoalOwnerAndShares(context.Background(), domain.TenantScope{TenantID: 1}, 21, []int64{3}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ownerID != 3 {
		t.Fatalf("expected new owner 3, got %d", ownerID)
	}
	if periodID != 10 {
		t.Fatalf("expected period 10, got %d", periodID)
	}
	if len(st.updateOwnerCalls) != 1 || st.updateOwnerCalls[0].teamID != 3 {
		t.Fatalf("expected UpdateGoalOwner call for team 3, got %+v", st.updateOwnerCalls)
	}
}

// ── Unsupported KR kind errors ────────────────────────────────────────────────

func TestUpdateKRProgressNumericalRejectsUnsupportedKind(t *testing.T) {
	st := newGoalFakeStore()
	st.keyResults[1] = domain.KeyResult{ID: 1, Kind: domain.KRKindBoolean}
	svc := newGoalTestService(st)

	if err := svc.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 1, 50, 1); err == nil {
		t.Fatal("expected error for boolean KR with numerical update")
	}
}

func TestUpdateKRProgressBooleanRejectsUnsupportedKind(t *testing.T) {
	st := newGoalFakeStore()
	st.keyResults[2] = domain.KeyResult{ID: 2, Kind: domain.KRKindNumerical}
	svc := newGoalTestService(st)

	if err := svc.UpdateKRProgressBoolean(context.Background(), domain.TenantScope{TenantID: 1}, 2, true, 1); err == nil {
		t.Fatal("expected error for numerical KR with boolean update")
	}
}

func TestUpdateKRProgressProjectRejectsUnsupportedKind(t *testing.T) {
	st := newGoalFakeStore()
	st.keyResults[3] = domain.KeyResult{ID: 3, Kind: domain.KRKindNumerical}
	svc := newGoalTestService(st)

	if err := svc.UpdateKRProgressProject(context.Background(), domain.TenantScope{TenantID: 1}, 3, nil, 1); err == nil {
		t.Fatal("expected error for numerical KR with project update")
	}
}

// ── CreateKeyResultWithMeta tests ─────────────────────────────────────────────

func TestCreateKeyResultWithMetaAppliesNumericalMeta(t *testing.T) {
	st := newGoalFakeStore()
	svc := newGoalTestService(st)

	_, err := svc.CreateKeyResultWithMeta(context.Background(), domain.TenantScope{TenantID: 1},
		krs.KeyResultInput{Kind: domain.KRKindNumerical},
		KeyResultMetaInput{NumericalStart: 0, NumericalTarget: 100, NumericalCurrent: 30, NumericalUnit: "%"},
		1,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.upsertNumericalCalls) != 1 {
		t.Fatalf("expected UpsertNumericalMeta called once, got %d", len(st.upsertNumericalCalls))
	}
	meta := st.upsertNumericalCalls[0]
	if meta.StartValue != 0 || meta.TargetValue != 100 || meta.CurrentValue != 30 || meta.Unit != "%" {
		t.Fatalf("unexpected numerical meta values: %+v", meta)
	}
}

func TestCreateKeyResultWithMetaAppliesBooleanMeta(t *testing.T) {
	st := newGoalFakeStore()
	svc := newGoalTestService(st)

	_, err := svc.CreateKeyResultWithMeta(context.Background(), domain.TenantScope{TenantID: 1},
		krs.KeyResultInput{Kind: domain.KRKindBoolean},
		KeyResultMetaInput{BooleanDone: true},
		1,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.upsertBoolCalls) != 1 || !st.upsertBoolCalls[0].done {
		t.Fatalf("expected UpsertBooleanMeta(done=true), got %+v", st.upsertBoolCalls)
	}
}

func TestCreateKeyResultWithMetaAppliesProjectStages(t *testing.T) {
	st := newGoalFakeStore()
	svc := newGoalTestService(st)

	stages := []krs.ProjectStageInput{{Title: "Step 1", Weight: 60}, {Title: "Step 2", Weight: 40}}
	_, err := svc.CreateKeyResultWithMeta(context.Background(), domain.TenantScope{TenantID: 1},
		krs.KeyResultInput{Kind: domain.KRKindProject},
		KeyResultMetaInput{ProjectStages: stages},
		1,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.replaceStageCalls) != 1 || len(st.replaceStageCalls[0].stages) != 2 {
		t.Fatalf("expected ReplaceProjectStages with 2 stages, got %+v", st.replaceStageCalls)
	}
}
