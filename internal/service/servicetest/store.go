// Package servicetest holds the shared in-memory fake repository used by the tests of
// every service package. It lives in a normal (non _test.go) package because Go cannot
// import _test.go files across packages, and duplicating 80 fake methods in each of the
// nine entity-service packages is not an option.
package servicetest

import (
	"context"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/goals"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/shares"
	storeteams "okrs/internal/store/teams"
)

type Store struct {
	Teams            []domain.Team
	Periods          []domain.Period
	CurrentPeriod    domain.Period
	GoalsByTeam      map[int64]map[int64][]domain.Goal
	Statuses         map[[2]int64]domain.TeamPeriodStatus
	KeyResults       map[int64]domain.KeyResult
	NumericalUpdates map[int64]float64
	HealthUpdates    map[int64]domain.KRHealthStatus
	BooleanUpdates   map[int64]bool
	ProjectStages    map[int64][]domain.KRProjectStage
	StageUpdates     map[int64]bool
	MovedGoals       map[int64]int
	MovedKRs         map[int64]int
	SoftDeleted      []int64
	Restored         []int64
	HardDeleted      []int64
	BulkSetTeamIDs   []int64
	BulkSetStatus    domain.TeamPeriodStatus
}

func NewStore() *Store {
	return &Store{
		GoalsByTeam:      make(map[int64]map[int64][]domain.Goal),
		Statuses:         make(map[[2]int64]domain.TeamPeriodStatus),
		KeyResults:       make(map[int64]domain.KeyResult),
		NumericalUpdates: make(map[int64]float64),
		HealthUpdates:    make(map[int64]domain.KRHealthStatus),
		BooleanUpdates:   make(map[int64]bool),
		ProjectStages:    make(map[int64][]domain.KRProjectStage),
		StageUpdates:     make(map[int64]bool),
		MovedGoals:       make(map[int64]int),
		MovedKRs:         make(map[int64]int),
	}
}

func (f *Store) ListTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	items := make([]domain.Team, 0, len(f.Teams))
	for _, team := range f.Teams {
		if team.DeletedAt == nil {
			items = append(items, team)
		}
	}
	return items, nil
}
func (f *Store) ListDeletedTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	items := make([]domain.Team, 0, len(f.Teams))
	for _, team := range f.Teams {
		if team.DeletedAt != nil {
			items = append(items, team)
		}
	}
	return items, nil
}
func (f *Store) ListAllTeams(_ context.Context, _ domain.TenantScope) ([]domain.Team, error) {
	return f.Teams, nil
}
func (f *Store) GetTeam(_ context.Context, _ domain.TenantScope, id int64) (domain.Team, error) {
	for _, team := range f.Teams {
		if team.ID == id {
			return team, nil
		}
	}
	return domain.Team{}, nil
}
func (f *Store) ListPeriods(_ context.Context, _ domain.TenantScope) ([]domain.Period, error) {
	return f.Periods, nil
}
func (f *Store) GetPeriod(_ context.Context, _ domain.TenantScope, id int64) (domain.Period, error) {
	for _, period := range f.Periods {
		if period.ID == id {
			return period, nil
		}
	}
	return domain.Period{}, nil
}
func (f *Store) FindPeriodForDate(_ context.Context, _ domain.TenantScope, _ time.Time) (domain.Period, error) {
	return f.CurrentPeriod, nil
}
func (f *Store) ListGoalsByTeamPeriod(_ context.Context, _ domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	if f.GoalsByTeam[teamID] == nil {
		return nil, nil
	}
	return f.GoalsByTeam[teamID][periodID], nil
}
func (f *Store) ListGoalsByTeamsPeriod(_ context.Context, _ domain.TenantScope, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error) {
	result := make(map[int64][]domain.Goal, len(teamIDs))
	for _, teamID := range teamIDs {
		if f.GoalsByTeam[teamID] == nil {
			continue
		}
		goals := f.GoalsByTeam[teamID][periodID]
		copied := make([]domain.Goal, len(goals))
		copy(copied, goals)
		result[teamID] = copied
	}
	return result, nil
}
func (f *Store) listTeamOverviewStatsUnused(_ context.Context, periodID int64, teamIDs []int64) (map[int64]goals.TeamOverviewStats, error) {
	result := make(map[int64]goals.TeamOverviewStats, len(teamIDs))
	for _, teamID := range teamIDs {
		item := goals.TeamOverviewStats{TeamID: teamID}
		for _, goal := range f.GoalsByTeam[teamID][periodID] {
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
func (f *Store) ListGoalShares(context.Context, domain.TenantScope, int64) ([]shares.GoalShare, error) {
	return nil, nil
}
func (f *Store) ListGoalSharesByGoalIDs(_ context.Context, _ domain.TenantScope, goalIDs []int64) (map[int64][]shares.GoalShare, error) {
	return make(map[int64][]shares.GoalShare, len(goalIDs)), nil
}
func (f *Store) GetTeamPeriodStatus(_ context.Context, _ domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	if status, ok := f.Statuses[[2]int64{teamID, periodID}]; ok {
		return status, nil
	}
	return domain.TeamPeriodStatusNoGoals, nil
}
func (f *Store) GetTeamPeriodStatusWithTime(_ context.Context, _ domain.TenantScope, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error) {
	if status, ok := f.Statuses[[2]int64{teamID, periodID}]; ok {
		return status, nil, nil
	}
	return domain.TeamPeriodStatusNoGoals, nil, nil
}
func (f *Store) ListTeamPeriodStatuses(_ context.Context, _ domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error) {
	result := make(map[int64]domain.TeamPeriodStatus, len(teamIDs))
	for _, teamID := range teamIDs {
		if status, ok := f.Statuses[[2]int64{teamID, periodID}]; ok {
			result[teamID] = status
		}
	}
	return result, nil
}
func (f *Store) ListTeamLastGoalUpdateInPeriod(_ context.Context, _ domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]time.Time, error) {
	result := make(map[int64]time.Time)
	for _, teamID := range teamIDs {
		goals := f.GoalsByTeam[teamID][periodID]
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
func (f *Store) TeamHasGoals(_ context.Context, _ domain.TenantScope, id int64) (bool, error) {
	for _, goals := range f.GoalsByTeam[id] {
		if len(goals) > 0 {
			return true, nil
		}
	}
	return false, nil
}
func (f *Store) TeamHasGoalsInPeriod(_ context.Context, _ domain.TenantScope, id, periodID int64) (bool, error) {
	return len(f.GoalsByTeam[id][periodID]) > 0, nil
}
func (f *Store) ListTeamIDsWithGoalsInPeriod(_ context.Context, _ domain.TenantScope, periodID int64) (map[int64]struct{}, error) {
	ids := make(map[int64]struct{})
	for teamID := range f.GoalsByTeam {
		if len(f.GoalsByTeam[teamID][periodID]) > 0 {
			ids[teamID] = struct{}{}
		}
	}
	return ids, nil
}
func (f *Store) SoftDeleteTeam(_ context.Context, _ domain.TenantScope, id int64) error {
	f.SoftDeleted = append(f.SoftDeleted, id)
	return nil
}
func (f *Store) RestoreTeam(_ context.Context, _ domain.TenantScope, id int64) error {
	f.Restored = append(f.Restored, id)
	return nil
}
func (f *Store) HardDeleteTeam(_ context.Context, _ domain.TenantScope, id int64) error {
	f.HardDeleted = append(f.HardDeleted, id)
	return nil
}
func (f *Store) UpdateNumericalCurrent(_ context.Context, _ domain.TenantScope, krID int64, current float64) error {
	f.NumericalUpdates[krID] = current
	return nil
}
func (f *Store) UpdateHealthStatus(_ context.Context, _ domain.TenantScope, krID int64, status domain.KRHealthStatus) error {
	f.HealthUpdates[krID] = status
	kr := f.KeyResults[krID]
	kr.HealthStatus = status
	f.KeyResults[krID] = kr
	return nil
}
func (f *Store) UpdateBoolean(_ context.Context, _ domain.TenantScope, krID int64, done bool) error {
	f.BooleanUpdates[krID] = done
	return nil
}
func (f *Store) ListProjectStages(_ context.Context, _ domain.TenantScope, krID int64) ([]domain.KRProjectStage, error) {
	return f.ProjectStages[krID], nil
}
func (f *Store) UpdateProjectStageDone(_ context.Context, _ domain.TenantScope, stageID int64, done bool) error {
	f.StageUpdates[stageID] = done
	return nil
}
func (f *Store) BatchUpdateProjectStagesDone(_ context.Context, _ domain.TenantScope, _ int64, updates map[int64]bool) error {
	for id, done := range updates {
		f.StageUpdates[id] = done
	}
	return nil
}
func (f *Store) ReplaceGoalShares(context.Context, domain.TenantScope, int64, []shares.GoalShareInput) error {
	return nil
}
func (f *Store) UpdateGoalTeamWeight(context.Context, domain.TenantScope, int64, int64, int) error {
	return nil
}
func (f *Store) GetKeyResult(_ context.Context, _ domain.TenantScope, id int64) (domain.KeyResult, error) {
	return f.KeyResults[id], nil
}
func (f *Store) GetBooleanMeta(context.Context, domain.TenantScope, int64) (*domain.KRBoolean, error) {
	return nil, nil
}
func (f *Store) GetKeyResultNote(context.Context, domain.TenantScope, int64) (*domain.KeyResultNote, error) {
	return nil, nil
}
func (f *Store) BatchLoadNotes(context.Context, domain.TenantScope, []int64) (map[int64]*domain.KeyResultNote, error) {
	return nil, nil
}
func (f *Store) AddGoalComment(context.Context, domain.TenantScope, int64, string, int64) (int64, error) {
	return 1, nil
}
func (f *Store) SetGoalCommentResolved(context.Context, domain.TenantScope, int64, int64, bool, int64) (bool, error) {
	return true, nil
}
func (f *Store) AddGoalReply(context.Context, domain.TenantScope, int64, int64, string, int64) (int64, error) {
	return 1, nil
}
func (f *Store) GetGoalCommentMeta(context.Context, domain.TenantScope, int64, int64) (int64, bool, error) {
	return 0, true, nil
}
func (f *Store) DeleteGoalComment(context.Context, domain.TenantScope, int64, int64) error {
	return nil
}
func (f *Store) UpsertKeyResultNote(context.Context, domain.TenantScope, int64, string, int64) error {
	return nil
}
func (f *Store) UpdateKeyResultDescription(context.Context, domain.TenantScope, int64, string) error {
	return nil
}
func (f *Store) GetGoal(context.Context, domain.TenantScope, int64) (domain.Goal, error) {
	return domain.Goal{}, nil
}
func (f *Store) UpdateGoal(context.Context, domain.TenantScope, goals.GoalUpdateInput) error {
	return nil
}
func (f *Store) CreateKeyResult(context.Context, domain.TenantScope, krs.KeyResultInput) (int64, error) {
	return 0, nil
}
func (f *Store) UpdateKeyResult(context.Context, domain.TenantScope, krs.KeyResultUpdateInput) error {
	return nil
}
func (f *Store) MoveGoal(_ context.Context, _ domain.TenantScope, _ int64, goalID int64, direction int) error {
	f.MovedGoals[goalID] = direction
	return nil
}
func (f *Store) MoveKeyResult(_ context.Context, _ domain.TenantScope, krID int64, direction int) error {
	f.MovedKRs[krID] = direction
	return nil
}
func (f *Store) UpsertNumericalMeta(context.Context, domain.TenantScope, krs.NumericalMetaInput) error {
	return nil
}
func (f *Store) UpsertBooleanMeta(context.Context, domain.TenantScope, int64, bool) error {
	return nil
}
func (f *Store) ReplaceProjectStages(context.Context, domain.TenantScope, int64, []krs.ProjectStageInput) error {
	return nil
}
func (f *Store) SetTeamPeriodStatus(context.Context, domain.TenantScope, int64, int64, domain.TeamPeriodStatus) error {
	return nil
}
func (f *Store) SetTeamPeriodStatuses(_ context.Context, _ domain.TenantScope, periodID int64, teamIDs []int64, status domain.TeamPeriodStatus) error {
	f.BulkSetTeamIDs = append([]int64(nil), teamIDs...)
	f.BulkSetStatus = status
	for _, id := range teamIDs {
		f.Statuses[[2]int64{id, periodID}] = status
	}
	return nil
}
func (f *Store) CreateTeam(_ context.Context, _ domain.TenantScope, _ storeteams.TeamInput) (int64, error) {
	return 0, nil
}
func (f *Store) UpdateTeam(_ context.Context, _ domain.TenantScope, _ storeteams.TeamInput, _ int64) error {
	return nil
}
func (f *Store) CreatePeriod(_ context.Context, _ domain.TenantScope, _ periods.PeriodInput) (int64, error) {
	return 0, nil
}
func (f *Store) UpdatePeriod(_ context.Context, _ domain.TenantScope, _ int64, _ periods.PeriodInput) error {
	return nil
}
func (f *Store) DeletePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *Store) ArchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *Store) UnarchivePeriod(_ context.Context, _ domain.TenantScope, _ int64) error {
	return nil
}
func (f *Store) CreateGoal(context.Context, domain.TenantScope, goals.GoalInput) (int64, error) {
	return 0, nil
}
func (f *Store) DeleteGoal(context.Context, domain.TenantScope, int64) error { return nil }
func (f *Store) CopyGoal(context.Context, domain.TenantScope, goals.CopyGoalInput) (int64, error) {
	return 0, nil
}
func (f *Store) UpdateGoalFields(context.Context, domain.TenantScope, goals.GoalFieldsUpdateInput) error {
	return nil
}
func (f *Store) UpdateGoalOwner(context.Context, domain.TenantScope, int64, int64, int) error {
	return nil
}
func (f *Store) ListGoalCommentsByGoals(context.Context, domain.TenantScope, []int64) (map[int64][]domain.GoalComment, error) {
	return nil, nil
}
func (f *Store) ListGoalOwnerTeamIDs(context.Context, domain.TenantScope, []int64) (map[int64]int64, error) {
	return nil, nil
}
func (f *Store) ListGoalsByIDs(context.Context, domain.TenantScope, []int64) ([]domain.Goal, error) {
	return nil, nil
}
func (f *Store) ListGoalsForPeriods(context.Context, domain.TenantScope, []int64, []int64, bool) ([]domain.Goal, error) {
	return nil, nil
}
func (f *Store) ListGoalComments(context.Context, domain.TenantScope, int64) ([]domain.GoalComment, error) {
	return nil, nil
}
func (f *Store) GetGoalShare(context.Context, domain.TenantScope, int64, int64) (shares.GoalShare, error) {
	return shares.GoalShare{}, nil
}
func (f *Store) DeleteGoalShare(context.Context, domain.TenantScope, int64, int64) error {
	return nil
}
func (f *Store) DeleteKeyResult(context.Context, domain.TenantScope, int64) error { return nil }
func (f *Store) FindGoalIDByKR(context.Context, domain.TenantScope, int64) (int64, error) {
	return 0, nil
}
func (f *Store) FindGoalIDByStage(context.Context, domain.TenantScope, int64) (int64, error) {
	return 0, nil
}
func (f *Store) GetUsersByDisplayNames(context.Context, []string) ([]*domain.User, error) {
	return nil, nil
}
func (f *Store) SearchUsersUnrestricted(context.Context, string, int) ([]*domain.User, error) {
	return nil, nil
}
func (f *Store) SearchUsersInSet(context.Context, []int64, []string, string, int) ([]*domain.User, error) {
	return nil, nil
}
func (f *Store) GetUsersByUDIDs(context.Context, []string) ([]*domain.User, error) {
	return nil, nil
}
func (f *Store) ListUserLeadTeams(context.Context) (map[string]string, error) {
	return nil, nil
}
func (f *Store) ValidateUDIDsExist(_ context.Context, _ []string) ([]string, error) {
	return nil, nil
}
