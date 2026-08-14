package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/shares"
	storeteams "okrs/internal/store/teams"
)

type fakeStore struct {
	teams            []domain.Team
	periods          []domain.Period
	currentPeriod    domain.Period
	goalsByTeam      map[int64]map[int64][]domain.Goal
	statuses         map[[2]int64]domain.TeamPeriodStatus
	keyResults       map[int64]domain.KeyResult
	numericalUpdates map[int64]float64
	healthUpdates    map[int64]domain.KRHealthStatus
	booleanUpdates   map[int64]bool
	projectStages    map[int64][]domain.KRProjectStage
	stageUpdates     map[int64]bool
	movedGoals       map[int64]int
	movedKRs         map[int64]int
	softDeleted      []int64
	restored         []int64
	hardDeleted      []int64
	bulkSetTeamIDs   []int64
	bulkSetStatus    domain.TeamPeriodStatus
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		goalsByTeam:      make(map[int64]map[int64][]domain.Goal),
		statuses:         make(map[[2]int64]domain.TeamPeriodStatus),
		keyResults:       make(map[int64]domain.KeyResult),
		numericalUpdates: make(map[int64]float64),
		healthUpdates:    make(map[int64]domain.KRHealthStatus),
		booleanUpdates:   make(map[int64]bool),
		projectStages:    make(map[int64][]domain.KRProjectStage),
		stageUpdates:     make(map[int64]bool),
		movedGoals:       make(map[int64]int),
		movedKRs:         make(map[int64]int),
	}
}

func (f *fakeStore) ListTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	items := make([]domain.Team, 0, len(f.teams))
	for _, team := range f.teams {
		if team.DeletedAt == nil {
			items = append(items, team)
		}
	}
	return items, nil
}
func (f *fakeStore) ListDeletedTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	items := make([]domain.Team, 0, len(f.teams))
	for _, team := range f.teams {
		if team.DeletedAt != nil {
			items = append(items, team)
		}
	}
	return items, nil
}
func (f *fakeStore) ListAllTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	return f.teams, nil
}
func (f *fakeStore) GetTeam(_ context.Context, _ domain.TenantScope, id int64) (domain.Team, error) {
	for _, team := range f.teams {
		if team.ID == id {
			return team, nil
		}
	}
	return domain.Team{}, nil
}
func (f *fakeStore) ListPeriods(_ context.Context, _ domain.TenantScope) ([]domain.Period, error) {
	return f.periods, nil
}
func (f *fakeStore) GetPeriod(_ context.Context, _ domain.TenantScope, id int64) (domain.Period, error) {
	for _, period := range f.periods {
		if period.ID == id {
			return period, nil
		}
	}
	return domain.Period{}, nil
}
func (f *fakeStore) FindPeriodForDate(_ context.Context, _ domain.TenantScope, _ time.Time) (domain.Period, error) {
	return f.currentPeriod, nil
}
func (f *fakeStore) ListGoalsByTeamPeriod(_ context.Context, _ domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	if f.goalsByTeam[teamID] == nil {
		return nil, nil
	}
	return f.goalsByTeam[teamID][periodID], nil
}
func (f *fakeStore) ListGoalsByTeamsPeriod(_ context.Context, _ domain.TenantScope, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error) {
	result := make(map[int64][]domain.Goal, len(teamIDs))
	for _, teamID := range teamIDs {
		if f.goalsByTeam[teamID] == nil {
			continue
		}
		goals := f.goalsByTeam[teamID][periodID]
		copied := make([]domain.Goal, len(goals))
		copy(copied, goals)
		result[teamID] = copied
	}
	return result, nil
}
func (f *fakeStore) listTeamOverviewStatsUnused(_ context.Context, periodID int64, teamIDs []int64) (map[int64]goals.TeamOverviewStats, error) {
	result := make(map[int64]goals.TeamOverviewStats, len(teamIDs))
	for _, teamID := range teamIDs {
		item := goals.TeamOverviewStats{TeamID: teamID}
		for _, goal := range f.goalsByTeam[teamID][periodID] {
			item.Goals++
			switch goal.Priority {
			case domain.PriorityP0:
				item.PriorityP0++
			case domain.PriorityP1:
				item.PriorityP1++
			case domain.PriorityP2:
				item.PriorityP2++
			case domain.PriorityP3:
				item.PriorityP3++
			}
			switch goal.WorkType {
			case domain.WorkTypeDiscovery:
				item.Discovery++
			case domain.WorkTypeDelivery:
				item.Delivery++
			}
		}
		if item.Goals > 0 {
			result[teamID] = item
		}
	}
	return result, nil
}
func (f *fakeStore) ListGoalShares(context.Context, domain.TenantScope, int64) ([]shares.GoalShare, error) {
	return nil, nil
}
func (f *fakeStore) ListGoalSharesByGoalIDs(_ context.Context, _ domain.TenantScope, goalIDs []int64) (map[int64][]shares.GoalShare, error) {
	return make(map[int64][]shares.GoalShare, len(goalIDs)), nil
}
func (f *fakeStore) GetTeamPeriodStatus(_ context.Context, _ domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	if status, ok := f.statuses[[2]int64{teamID, periodID}]; ok {
		return status, nil
	}
	return domain.TeamPeriodStatusNoGoals, nil
}
func (f *fakeStore) GetTeamPeriodStatusWithTime(_ context.Context, _ domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error) {
	if status, ok := f.statuses[[2]int64{teamID, periodID}]; ok {
		return status, nil, nil
	}
	return domain.TeamPeriodStatusNoGoals, nil, nil
}
func (f *fakeStore) ListTeamPeriodStatuses(_ context.Context, _ domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error) {
	result := make(map[int64]domain.TeamPeriodStatus, len(teamIDs))
	for _, teamID := range teamIDs {
		if status, ok := f.statuses[[2]int64{teamID, periodID}]; ok {
			result[teamID] = status
		}
	}
	return result, nil
}
func (f *fakeStore) ListTeamLastGoalUpdateInPeriod(_ context.Context, _ domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]time.Time, error) {
	result := make(map[int64]time.Time)
	for _, teamID := range teamIDs {
		goals := f.goalsByTeam[teamID][periodID]
		var max time.Time
		for _, goal := range goals {
			if goal.UpdatedAt.After(max) {
				max = goal.UpdatedAt
			}
		}
		if !max.IsZero() {
			result[teamID] = max
		}
	}
	return result, nil
}
func (f *fakeStore) TeamHasGoals(_ context.Context, _ domain.TenantScope, id int64) (bool, error) {
	for _, goals := range f.goalsByTeam[id] {
		if len(goals) > 0 {
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeStore) TeamHasGoalsInPeriod(_ context.Context, _ domain.TenantScope, id, periodID int64) (bool, error) {
	return len(f.goalsByTeam[id][periodID]) > 0, nil
}
func (f *fakeStore) ListTeamIDsWithGoalsInPeriod(_ context.Context, _ domain.TenantScope, periodID int64) (map[int64]struct{}, error) {
	ids := make(map[int64]struct{})
	for teamID := range f.goalsByTeam {
		if len(f.goalsByTeam[teamID][periodID]) > 0 {
			ids[teamID] = struct{}{}
		}
	}
	return ids, nil
}
func (f *fakeStore) SoftDeleteTeam(_ context.Context, _ domain.TenantScope, id int64) error {
	f.softDeleted = append(f.softDeleted, id)
	return nil
}
func (f *fakeStore) RestoreTeam(_ context.Context, _ domain.TenantScope, id int64) error {
	f.restored = append(f.restored, id)
	return nil
}
func (f *fakeStore) HardDeleteTeam(_ context.Context, _ domain.TenantScope, id int64) error {
	f.hardDeleted = append(f.hardDeleted, id)
	return nil
}
func (f *fakeStore) UpdateNumericalCurrent(_ context.Context, _ domain.TenantScope, krID int64, current float64) error {
	f.numericalUpdates[krID] = current
	return nil
}
func (f *fakeStore) UpdateHealthStatus(_ context.Context, _ domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	f.healthUpdates[krID] = status
	kr := f.keyResults[krID]
	kr.HealthStatus = status
	f.keyResults[krID] = kr
	return nil
}
func (f *fakeStore) UpdateBoolean(_ context.Context, _ domain.TenantScope, krID int64, done bool) error {
	f.booleanUpdates[krID] = done
	return nil
}
func (f *fakeStore) ListProjectStages(_ context.Context, _ domain.TenantScope, krID int64) ([]domain.KRProjectStage, error) {
	return f.projectStages[krID], nil
}
func (f *fakeStore) UpdateProjectStageDone(_ context.Context, _ domain.TenantScope, stageID int64, done bool) error {
	f.stageUpdates[stageID] = done
	return nil
}
func (f *fakeStore) BatchUpdateProjectStagesDone(_ context.Context, _ domain.TenantScope, _ int64, updates map[int64]bool) error {
	for id, done := range updates {
		f.stageUpdates[id] = done
	}
	return nil
}
func (f *fakeStore) ReplaceGoalShares(context.Context, domain.TenantScope, int64, []shares.GoalShareInput) error {
	return nil
}
func (f *fakeStore) UpdateGoalTeamWeight(context.Context, domain.TenantScope, int64, int64, int) error {
	return nil
}
func (f *fakeStore) GetKeyResult(_ context.Context, _ domain.TenantScope, id int64) (domain.KeyResult, error) {
	return f.keyResults[id], nil
}
func (f *fakeStore) GetBooleanMeta(context.Context, domain.TenantScope, int64) (*domain.KRBoolean, error) {
	return nil, nil
}
func (f *fakeStore) GetKeyResultNote(context.Context, domain.TenantScope, int64) (*domain.KeyResultNote, error) {
	return nil, nil
}
func (f *fakeStore) BatchLoadNotes(context.Context, domain.TenantScope, []int64) (map[int64]*domain.KeyResultNote, error) {
	return nil, nil
}
func (f *fakeStore) AddGoalComment(context.Context, domain.TenantScope, int64, string, int64) (int64, error) {
	return 1, nil
}
func (f *fakeStore) SetGoalCommentResolved(context.Context, domain.TenantScope, int64, int64, bool, int64) (bool, error) {
	return true, nil
}
func (f *fakeStore) AddGoalReply(context.Context, domain.TenantScope, int64, int64, string, int64) (int64, error) {
	return 1, nil
}
func (f *fakeStore) GetGoalCommentMeta(context.Context, domain.TenantScope, int64, int64) (int64, bool, error) {
	return 0, true, nil
}
func (f *fakeStore) DeleteGoalComment(context.Context, domain.TenantScope, int64, int64) error {
	return nil
}
func (f *fakeStore) UpsertKeyResultNote(context.Context, domain.TenantScope, int64, string, int64) error {
	return nil
}
func (f *fakeStore) UpdateKeyResultDescription(context.Context, domain.TenantScope, int64, string) error {
	return nil
}
func (f *fakeStore) GetGoal(context.Context, domain.TenantScope, int64) (domain.Goal, error) {
	return domain.Goal{}, nil
}
func (f *fakeStore) UpdateGoal(context.Context, domain.TenantScope, goals.GoalUpdateInput) error {
	return nil
}
func (f *fakeStore) CreateKeyResult(context.Context, domain.TenantScope, krs.KeyResultInput) (int64, error) {
	return 0, nil
}
func (f *fakeStore) UpdateKeyResult(context.Context, domain.TenantScope, krs.KeyResultUpdateInput) error {
	return nil
}
func (f *fakeStore) MoveGoal(_ context.Context, _ domain.TenantScope, _ int64, goalID int64, direction int) error {
	f.movedGoals[goalID] = direction
	return nil
}
func (f *fakeStore) MoveKeyResult(_ context.Context, _ domain.TenantScope, krID int64, direction int) error {
	f.movedKRs[krID] = direction
	return nil
}
func (f *fakeStore) UpsertNumericalMeta(context.Context, domain.TenantScope, krs.NumericalMetaInput) error {
	return nil
}
func (f *fakeStore) UpsertBooleanMeta(context.Context, domain.TenantScope, int64, bool) error {
	return nil
}
func (f *fakeStore) ReplaceProjectStages(context.Context, domain.TenantScope, int64, []krs.ProjectStageInput) error {
	return nil
}
func (f *fakeStore) SetTeamPeriodStatus(context.Context, domain.TenantScope, int64, int64, domain.TeamPeriodStatus) error {
	return nil
}
func (f *fakeStore) SetTeamPeriodStatuses(_ context.Context, _ domain.TenantScope, periodID int64, teamIDs []int64, status domain.TeamPeriodStatus) error {
	f.bulkSetTeamIDs = append([]int64(nil), teamIDs...)
	f.bulkSetStatus = status
	for _, id := range teamIDs {
		f.statuses[[2]int64{id, periodID}] = status
	}
	return nil
}
func (f *fakeStore) CreateTeam(_ context.Context, _ domain.TenantScope, _ storeteams.TeamInput) (int64, error) {
	return 0, nil
}
func (f *fakeStore) UpdateTeam(_ context.Context, _ domain.TenantScope, _ storeteams.TeamInput, _ int64) error {
	return nil
}
func (f *fakeStore) CreatePeriod(_ context.Context, _ domain.TenantScope, _ periods.PeriodInput) (int64, error) {
	return 0, nil
}
func (f *fakeStore) UpdatePeriod(_ context.Context, _ domain.TenantScope, _ int64, _ periods.PeriodInput) error {
	return nil
}
func (f *fakeStore) DeletePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *fakeStore) ArchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *fakeStore) UnarchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *fakeStore) CreateGoal(context.Context, domain.TenantScope, goals.GoalInput) (int64, error) {
	return 0, nil
}
func (f *fakeStore) DeleteGoal(context.Context, domain.TenantScope, int64) error { return nil }
func (f *fakeStore) CopyGoal(context.Context, domain.TenantScope, goals.CopyGoalInput) (int64, error) {
	return 0, nil
}
func (f *fakeStore) UpdateGoalFields(context.Context, domain.TenantScope, goals.GoalFieldsUpdateInput) error {
	return nil
}
func (f *fakeStore) UpdateGoalOwner(context.Context, domain.TenantScope, int64, int64, int) error {
	return nil
}
func (f *fakeStore) ListGoalCommentsByGoals(context.Context, domain.TenantScope, []int64) (map[int64][]domain.GoalComment, error) {
	return nil, nil
}
func (f *fakeStore) ListGoalOwnerTeamIDs(context.Context, domain.TenantScope, []int64) (map[int64]int64, error) {
	return nil, nil
}
func (f *fakeStore) ListGoalComments(context.Context, domain.TenantScope, int64) ([]domain.GoalComment, error) {
	return nil, nil
}
func (f *fakeStore) GetGoalShare(context.Context, domain.TenantScope, int64, int64) (shares.GoalShare, error) {
	return shares.GoalShare{}, nil
}
func (f *fakeStore) DeleteGoalShare(context.Context, domain.TenantScope, int64, int64) error {
	return nil
}
func (f *fakeStore) DeleteKeyResult(context.Context, domain.TenantScope, int64) error { return nil }
func (f *fakeStore) FindGoalIDByKR(context.Context, domain.TenantScope, int64) (int64, error) {
	return 0, nil
}
func (f *fakeStore) FindGoalIDByStage(context.Context, domain.TenantScope, int64) (int64, error) {
	return 0, nil
}
func (f *fakeStore) GetUsersByDisplayNames(context.Context, []string) ([]*domain.User, error) {
	return nil, nil
}
func (f *fakeStore) SearchUsersUnrestricted(context.Context, string, int) ([]*domain.User, error) {
	return nil, nil
}
func (f *fakeStore) SearchUsersInSet(context.Context, []int64, []string, string, int) ([]*domain.User, error) {
	return nil, nil
}
func (f *fakeStore) GetUsersByUDIDs(context.Context, []string) ([]*domain.User, error) {
	return nil, nil
}
func (f *fakeStore) ListUserLeadTeams(context.Context) (map[string]string, error) {
	return nil, nil
}
func (f *fakeStore) ValidateUDIDsExist(_ context.Context, _ []string) ([]string, error) {
	return nil, nil
}

func newTestService(st *fakeStore, grants GrantsProvider) *Service {
	return New(Deps{Teams: st, Goals: st, Shares: st, Periods: st, KRs: st, Statuses: st, Users: st, Grants: grants})
}

// ShareGoal must reject targets that don't belong to the active tenant, so a caller can't attach
// a goal to a foreign/global team ID (cross-tenant reference).
func TestShareGoalRejectsForeignTeamTarget(t *testing.T) {
	store := newFakeStore()
	store.teams = []domain.Team{{ID: 1}, {ID: 2}} // only these belong to the tenant
	svc := newTestService(store, nil)
	scope := domain.TenantScope{TenantID: 1}

	err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 2, Weight: 50}, {TeamID: 99, Weight: 50}}, 1)
	if err != ErrShareTargetNotInTenant {
		t.Fatalf("foreign target (99) must be rejected with ErrShareTargetNotInTenant, got %v", err)
	}

	if err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 1, Weight: 100}}, 1); err != nil {
		t.Fatalf("all-valid targets must be accepted, got %v", err)
	}
}

// A team whose period is already in_progress or closed must not be newly added as a share target —
// its OKR set for the period is locked. Only newly added teams are guarded; a team already sharing
// the goal stays untouched even if its period has since advanced.
func TestShareGoalRejectsStartedPeriodTarget(t *testing.T) {
	store := newFakeStore()
	store.teams = []domain.Team{{ID: 1}, {ID: 2}, {ID: 3}}
	svc := newTestService(store, nil)
	scope := domain.TenantScope{TenantID: 1}
	// fakeStore.GetGoal returns a zero goal, so the goal's period is 0; status is keyed on period 0.
	store.statuses[[2]int64{2, 0}] = domain.TeamPeriodStatusInProgress
	store.statuses[[2]int64{3, 0}] = domain.TeamPeriodStatusClosed

	if err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 2, Weight: 50}}, 1); err != ErrCannotShareWithClosedPeriod {
		t.Fatalf("in_progress target must be rejected with ErrCannotShareWithClosedPeriod, got %v", err)
	}
	if err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 3, Weight: 50}}, 1); err != ErrCannotShareWithClosedPeriod {
		t.Fatalf("closed target must be rejected with ErrCannotShareWithClosedPeriod, got %v", err)
	}
	// A team with an open (forming) period is still addable.
	store.statuses[[2]int64{1, 0}] = domain.TeamPeriodStatusForming
	if err := svc.ShareGoal(context.Background(), scope, 10, []ShareTarget{{TeamID: 1, Weight: 100}}, 1); err != nil {
		t.Fatalf("forming target must be accepted, got %v", err)
	}
}

// ListPeriodViews must filter out archived periods for the public caller (includeArchived=false)
// before building parent/depth views, so a public parent_id never points at a hidden period,
// while the admin caller (includeArchived=true) sees everything.
func TestListPeriodViews_ExcludesArchivedForPublic(t *testing.T) {
	now := time.Now()
	arch := now.AddDate(0, 0, -2)
	store := newFakeStore()
	store.periods = []domain.Period{
		{ID: 1, StartDate: now.AddDate(0, 0, -30), EndDate: now.AddDate(0, 0, -1)},                    // closed
		{ID: 2, StartDate: now.AddDate(0, 0, -30), EndDate: now.AddDate(0, 0, -1), ArchivedAt: &arch}, // archived
	}
	svc := newTestService(store, nil)

	pub, err := svc.ListPeriodViews(context.Background(), domain.TenantScope{TenantID: 1}, false)
	if err != nil {
		t.Fatalf("list period views (public): %v", err)
	}
	if len(pub) != 1 || pub[0].ID != 1 {
		t.Fatalf("public must exclude archived, got %+v", pub)
	}

	all, err := svc.ListPeriodViews(context.Background(), domain.TenantScope{TenantID: 1}, true)
	if err != nil {
		t.Fatalf("list period views (admin): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("admin must include archived, got %d", len(all))
	}
}

func TestUpdateKRProgressNumerical(t *testing.T) {
	store := newFakeStore()
	store.keyResults[1] = domain.KeyResult{ID: 1, Kind: domain.KRKindNumerical}
	service := newTestService(store, nil)

	if err := service.UpdateKRProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 1, 42, 1); err != nil {
		t.Fatalf("update numerical: %v", err)
	}
	if store.numericalUpdates[1] != 42 {
		t.Fatalf("expected numerical update")
	}
}

// ArchivePeriod must only allow archiving a closed period, so an active/future period can't be
// hidden from the tree via archive.
func TestArchivePeriod_RejectsNonClosed(t *testing.T) {
	now := time.Now()
	store := newFakeStore()
	store.periods = []domain.Period{{
		ID:        1,
		StartDate: now.AddDate(0, 0, -1),
		EndDate:   now.AddDate(0, 0, 10), // active
	}}
	svc := newTestService(store, nil)

	err := svc.ArchivePeriod(context.Background(), domain.TenantScope{TenantID: 1}, 1)
	if !errors.Is(err, ErrPeriodNotClosed) {
		t.Fatalf("expected ErrPeriodNotClosed, got %v", err)
	}
}

func TestArchivePeriod_AllowsClosed(t *testing.T) {
	now := time.Now()
	store := newFakeStore()
	store.periods = []domain.Period{{
		ID:        1,
		StartDate: now.AddDate(0, 0, -30),
		EndDate:   now.AddDate(0, 0, -1), // closed
	}}
	svc := newTestService(store, nil)

	if err := svc.ArchivePeriod(context.Background(), domain.TenantScope{TenantID: 1}, 1); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestUpdateKRProgressBoolean(t *testing.T) {
	store := newFakeStore()
	store.keyResults[3] = domain.KeyResult{ID: 3, Kind: domain.KRKindBoolean}
	service := newTestService(store, nil)

	if err := service.UpdateKRProgressBoolean(context.Background(), domain.TenantScope{TenantID: 1}, 3, true, 1); err != nil {
		t.Fatalf("update boolean: %v", err)
	}
	if !store.booleanUpdates[3] {
		t.Fatalf("expected boolean update")
	}
}

func TestUpdateKRProgressProject(t *testing.T) {
	store := newFakeStore()
	store.keyResults[4] = domain.KeyResult{ID: 4, Kind: domain.KRKindProject}
	store.projectStages[4] = []domain.KRProjectStage{{ID: 100, IsDone: false}, {ID: 101, IsDone: true}}
	service := newTestService(store, nil)

	updates := []ProjectStageUpdate{{ID: 100, IsDone: true}}
	if err := service.UpdateKRProgressProject(context.Background(), domain.TenantScope{TenantID: 1}, 4, updates, 1); err != nil {
		t.Fatalf("update project: %v", err)
	}
	if !store.stageUpdates[100] {
		t.Fatalf("expected stage update")
	}
}

func TestMoveGoal(t *testing.T) {
	store := newFakeStore()
	service := newTestService(store, nil)

	if err := service.MoveGoal(context.Background(), domain.TenantScope{TenantID: 1}, 5, 10, -1); err != nil {
		t.Fatalf("move goal: %v", err)
	}
	if store.movedGoals[10] != -1 {
		t.Fatalf("expected goal move direction")
	}
}

func TestMoveKeyResult(t *testing.T) {
	store := newFakeStore()
	service := newTestService(store, nil)

	if err := service.MoveKeyResult(context.Background(), domain.TenantScope{TenantID: 1}, 20, 1); err != nil {
		t.Fatalf("move kr: %v", err)
	}
	if store.movedKRs[20] != 1 {
		t.Fatalf("expected key result move direction")
	}
}

func TestDeleteTeamUsesSoftDeleteWhenTeamHasGoals(t *testing.T) {
	store := newFakeStore()
	store.goalsByTeam[10] = map[int64][]domain.Goal{1: {{ID: 1, TeamID: 10, PeriodID: 1}}}
	service := newTestService(store, nil)

	if err := service.DeleteTeam(context.Background(), domain.TenantScope{TenantID: 1}, 10); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	if len(store.softDeleted) != 1 || store.softDeleted[0] != 10 {
		t.Fatalf("expected soft delete for team with goals")
	}
	if len(store.hardDeleted) != 0 {
		t.Fatalf("did not expect hard delete")
	}
}

func TestDeleteTeamUsesHardDeleteWhenTeamHasNoGoals(t *testing.T) {
	store := newFakeStore()
	service := newTestService(store, nil)

	if err := service.DeleteTeam(context.Background(), domain.TenantScope{TenantID: 1}, 10); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	if len(store.hardDeleted) != 1 || store.hardDeleted[0] != 10 {
		t.Fatalf("expected hard delete for team without goals")
	}
}

func TestHardDeleteTeamRejectsTeamsWithGoals(t *testing.T) {
	store := newFakeStore()
	store.goalsByTeam[10] = map[int64][]domain.Goal{1: {{ID: 1, TeamID: 10, PeriodID: 1}}}
	service := newTestService(store, nil)

	if err := service.HardDeleteTeam(context.Background(), domain.TenantScope{TenantID: 1}, 10); err != ErrTeamHasGoals {
		t.Fatalf("expected ErrTeamHasGoals, got %v", err)
	}
}

func TestGetTeamsWithPeriodSummaryKeepsActiveTeamsWithoutHistoricalGoalsVisible(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{
		{ID: 1, Name: "Active parent", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted child", Type: domain.TeamTypeTeam, ParentID: ptr(1), DeletedAt: &deletedAt},
		{ID: 3, Name: "New active child", Type: domain.TeamTypeTeam, ParentID: ptr(1)},
	}
	store.goalsByTeam[2] = map[int64][]domain.Goal{1: {{ID: 100, TeamID: 2, PeriodID: 1, Title: "Historic"}}}
	store.statuses[[2]int64{2, 1}] = domain.TeamPeriodStatusClosed
	service := newTestService(store, nil)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 1, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected active teams plus deleted historical team, got %d", len(rows))
	}
	if rows[0].ID != 1 || rows[1].ID != 2 || rows[2].ID != 3 {
		t.Fatalf("expected active parent, deleted child with goals, and active child, got %+v", rows)
	}
	if rows[1].Indent != 24 || rows[2].Indent != 24 {
		t.Fatalf("expected children to remain nested under active parent, got %+v", rows)
	}
}

func TestGetTeamsWithPeriodSummaryShowsOnlyActiveTeamsInCurrentPeriod(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	service := newTestService(store, nil)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 2, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 1 {
		t.Fatalf("expected only active team in current period, got %+v", rows)
	}
}

func TestGetTeamsWithPeriodSummaryKeepsDeletedTeamsWithCurrentGoalsVisible(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	store.goalsByTeam[2] = map[int64][]domain.Goal{2: {{ID: 200, TeamID: 2, PeriodID: 2, Title: "Current"}}}
	service := newTestService(store, nil)

	rows, err := service.GetTeamsWithPeriodSummary(context.Background(), domain.TenantScope{TenantID: 1}, 2, nil)
	if err != nil {
		t.Fatalf("get team summaries: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected active team and deleted team with current goals, got %+v", rows)
	}
	if rows[1].ID != 2 {
		t.Fatalf("expected deleted team with current goals to remain visible, got %+v", rows)
	}
}

func TestGetTeamOKRAllowsDeletedTeamInHistoricalPeriodWithGoals(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	store.goalsByTeam[2] = map[int64][]domain.Goal{1: {{ID: 100, TeamID: 2, PeriodID: 1, Title: "Historic"}}}
	service := newTestService(store, nil)

	okr, err := service.GetTeamOKR(context.Background(), domain.TenantScope{TenantID: 1}, 2, 1, domain.Period{ID: 1, Name: "2024 Q4"})
	if err != nil {
		t.Fatalf("get team okr: %v", err)
	}
	if okr.Team.ID != 2 || len(okr.Goals) != 1 {
		t.Fatalf("expected historical deleted team okr to load")
	}
}

func TestGetTeamOKRAllowsActiveTeamWithoutHistoricalGoals(t *testing.T) {
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{{ID: 1, Name: "Active", Type: domain.TeamTypeTeam}}
	service := newTestService(store, nil)

	okr, err := service.GetTeamOKR(context.Background(), domain.TenantScope{TenantID: 1}, 1, 1, domain.Period{ID: 1, Name: "2024 Q4"})
	if err != nil {
		t.Fatalf("expected active team without historical goals to stay visible, got %v", err)
	}
	if okr.Team.ID != 1 || len(okr.Goals) != 0 {
		t.Fatalf("expected empty okr page for active team without goals")
	}
}

func TestGetTeamOKRRejectsDeletedTeamInCurrentPeriod(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	service := newTestService(store, nil)

	_, err := service.GetTeamOKR(context.Background(), domain.TenantScope{TenantID: 1}, 2, 2, domain.Period{ID: 2, Name: "2025 Q1"})
	if err != ErrTeamNotVisibleInPeriod {
		t.Fatalf("expected ErrTeamNotVisibleInPeriod, got %v", err)
	}
}

func TestGetTeamOKRAllowsDeletedTeamInCurrentPeriodWhenGoalsExist(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.currentPeriod = domain.Period{ID: 2}
	store.teams = []domain.Team{{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt}}
	store.goalsByTeam[2] = map[int64][]domain.Goal{2: {{ID: 200, TeamID: 2, PeriodID: 2, Title: "Current"}}}
	service := newTestService(store, nil)

	okr, err := service.GetTeamOKR(context.Background(), domain.TenantScope{TenantID: 1}, 2, 2, domain.Period{ID: 2, Name: "2025 Q1"})
	if err != nil {
		t.Fatalf("expected deleted team with current goals to be visible, got %v", err)
	}
	if okr.Team.ID != 2 || len(okr.Goals) != 1 {
		t.Fatalf("expected current deleted team okr to load")
	}
}

func TestGetHierarchyWithoutPeriodHidesDeletedTeams(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	store.goalsByTeam[2] = map[int64][]domain.Goal{
		1: {{ID: 100, TeamID: 2, PeriodID: 1, Title: "Historical"}},
	}
	service := newTestService(store, nil)

	nodes, err := service.GetHierarchy(context.Background(), domain.TenantScope{TenantID: 1}, nil)
	if err != nil {
		t.Fatalf("get hierarchy: %v", err)
	}
	ids := flattenNodeIDs(nodes)
	if _, ok := ids[1]; !ok {
		t.Fatalf("expected active team in hierarchy, got %+v", ids)
	}
	if _, ok := ids[2]; ok {
		t.Fatalf("expected deleted team to be hidden without period filter, got %+v", ids)
	}
}

func TestGetHierarchyWithPeriodIncludesDeletedTeamsWithGoals(t *testing.T) {
	deletedAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.teams = []domain.Team{
		{ID: 1, Name: "Active", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Deleted with goals", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
		{ID: 3, Name: "Deleted no goals", Type: domain.TeamTypeTeam, DeletedAt: &deletedAt},
	}
	store.goalsByTeam[2] = map[int64][]domain.Goal{
		5: {{ID: 200, TeamID: 2, PeriodID: 5, Title: "Current"}},
	}
	service := newTestService(store, nil)
	periodID := int64(5)

	nodes, err := service.GetHierarchy(context.Background(), domain.TenantScope{TenantID: 1}, &periodID)
	if err != nil {
		t.Fatalf("get hierarchy: %v", err)
	}
	ids := flattenNodeIDs(nodes)
	if _, ok := ids[1]; !ok {
		t.Fatalf("expected active team in hierarchy, got %+v", ids)
	}
	if _, ok := ids[2]; !ok {
		t.Fatalf("expected deleted team with period goals in hierarchy, got %+v", ids)
	}
	if _, ok := ids[3]; ok {
		t.Fatalf("expected deleted team without period goals to be hidden, got %+v", ids)
	}
}

func TestGetTeamOverview(t *testing.T) {
	store := newFakeStore()
	store.teams = []domain.Team{
		{ID: 1, Name: "Parent", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Child", Type: domain.TeamTypeTeam, ParentID: ptr(1)},
	}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store.goalsByTeam[2] = map[int64][]domain.Goal{
		10: {{
			ID:        200,
			TeamID:    2,
			PeriodID:  10,
			Title:     "Ship feature",
			Priority:  domain.PriorityP1,
			WorkType:  domain.WorkTypeDelivery,
			UpdatedAt: now,
		}},
	}
	store.statuses[[2]int64{2, 10}] = domain.TeamPeriodStatusInProgress
	svc := newTestService(store, nil)
	overview, err := svc.GetTeamOverview(context.Background(), domain.TenantScope{TenantID: 1}, 1, 10)
	if err != nil {
		t.Fatalf("get team overview: %v", err)
	}
	if overview.TeamsWithGoals != 1 {
		t.Fatalf("expected teams_with_goals=1, got %d", overview.TeamsWithGoals)
	}
	if overview.AverageProgress != 0 {
		t.Fatalf("expected average progress=0 for zero-progress goal, got %d", overview.AverageProgress)
	}
	if len(overview.ChildrenSummary) != 1 {
		t.Fatalf("expected one direct child summary row, got %d", len(overview.ChildrenSummary))
	}
	if !overview.ChildrenSummary[0].HasGoals {
		t.Fatalf("expected child has_goals=true")
	}
}

func TestFindDirectChildren(t *testing.T) {
	nodes := []TeamNode{
		{
			Team: domain.Team{ID: 1, Name: "Root"},
			Children: []TeamNode{
				{Team: domain.Team{ID: 2, Name: "Child A"}},
				{
					Team: domain.Team{ID: 3, Name: "Child B"},
					Children: []TeamNode{
						{Team: domain.Team{ID: 4, Name: "Grandchild"}},
					},
				},
			},
		},
	}

	children := findDirectChildren(1, nodes)
	if len(children) != 2 {
		t.Fatalf("expected 2 direct children, got %d", len(children))
	}
	if children[0].Team.ID != 2 || children[1].Team.ID != 3 {
		t.Fatalf("unexpected direct children ids: %d, %d", children[0].Team.ID, children[1].Team.ID)
	}

	missing := findDirectChildren(999, nodes)
	if len(missing) != 0 {
		t.Fatalf("expected no children for missing team, got %d", len(missing))
	}
}

func TestCollectDescendantIDs(t *testing.T) {
	nodes := []TeamNode{
		{
			Team: domain.Team{ID: 1},
			Children: []TeamNode{
				{Team: domain.Team{ID: 2}},
				{
					Team: domain.Team{ID: 3},
					Children: []TeamNode{
						{Team: domain.Team{ID: 4}},
					},
				},
			},
		},
	}

	got := collectDescendantIDs(1, nodes)
	want := []int64{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("expected %d descendants, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descendant mismatch at %d: want %d, got %d", i, want[i], got[i])
		}
	}
}

func TestBuildDirectChildrenSummaryWithoutSummaryMap(t *testing.T) {
	store := newFakeStore()
	store.teams = []domain.Team{
		{ID: 1, Name: "Parent"},
		{ID: 2, Name: "Child", ParentID: ptr(1)},
	}
	store.statuses[[2]int64{2, 11}] = domain.TeamPeriodStatusClosed
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	store.goalsByTeam[2] = map[int64][]domain.Goal{
		11: {{
			ID:        300,
			TeamID:    2,
			PeriodID:  11,
			Title:     "Child goal",
			UpdatedAt: now,
		}},
	}
	svc := newTestService(store, nil)
	children := []TeamNode{{Team: domain.Team{ID: 2, Name: "Child", ParentID: ptr(1)}}}

	rows, err := svc.buildDirectChildrenSummary(context.Background(), domain.TenantScope{TenantID: 1}, 11, children, nil)
	if err != nil {
		t.Fatalf("build summary: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !rows[0].HasGoals {
		t.Fatalf("expected has_goals=true")
	}
	if rows[0].Status != domain.TeamPeriodStatusClosed {
		t.Fatalf("expected status closed, got %s", rows[0].Status)
	}
	if rows[0].LastUpdateAt == nil {
		t.Fatalf("expected last_updated to be set")
	}
}

func flattenNodeIDs(nodes []TeamNode) map[int64]struct{} {
	ids := make(map[int64]struct{})
	var walk func(items []TeamNode)
	walk = func(items []TeamNode) {
		for _, node := range items {
			ids[node.Team.ID] = struct{}{}
			if len(node.Children) > 0 {
				walk(node.Children)
			}
		}
	}
	walk(nodes)
	return ids
}

func ptr(id int64) *int64 { return &id }
