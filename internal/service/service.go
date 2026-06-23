package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"okrs/internal/domain"
	"okrs/internal/okr"
	"okrs/internal/store"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	"okrs/internal/store/krs"
	"okrs/internal/store/periods"
	"okrs/internal/store/shares"
	"okrs/internal/store/teams"
)

// GrantsProvider gives the service access to the cached user_hierarchy_grants snapshot.
// *grants.GrantsCache satisfies this interface.
type GrantsProvider interface {
	AllGrants(ctx context.Context) (map[int64][]grants.HierarchyGrant, error)
}

// Per-entity repository interfaces used by the service layer.
// Each interface is satisfied by the corresponding store.*Repository type.

type TeamRepo interface {
	ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	ListDeletedTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	ListAllTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
	GetTeam(ctx context.Context, scope domain.TenantScope, id int64) (domain.Team, error)
	CreateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput) (int64, error)
	UpdateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput, id int64) error
	TeamHasGoals(ctx context.Context, scope domain.TenantScope, id int64) (bool, error)
	TeamHasGoalsInPeriod(ctx context.Context, scope domain.TenantScope, id, periodID int64) (bool, error)
	ListTeamIDsWithGoalsInPeriod(ctx context.Context, scope domain.TenantScope, periodID int64) (map[int64]struct{}, error)
	SoftDeleteTeam(ctx context.Context, scope domain.TenantScope, id int64) error
	RestoreTeam(ctx context.Context, scope domain.TenantScope, id int64) error
	HardDeleteTeam(ctx context.Context, scope domain.TenantScope, id int64) error
}

type GoalRepo interface {
	ListGoalsByTeamPeriod(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error)
	ListGoalsByTeamsPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error)
	GetGoal(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error)
	CreateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalInput) (int64, error)
	DeleteGoal(ctx context.Context, scope domain.TenantScope, id int64) error
	UpdateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalUpdateInput) error
	UpdateGoalFields(ctx context.Context, scope domain.TenantScope, input goals.GoalFieldsUpdateInput) error
	UpdateGoalOwner(ctx context.Context, scope domain.TenantScope, goalID, teamID int64, weight int) error
	MoveGoal(ctx context.Context, scope domain.TenantScope, goalID int64, direction int) error
	AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error
	ListGoalComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error)
	ListTeamLastGoalUpdateInPeriod(ctx context.Context, scope domain.TenantScope, periodID int64, teamIDs []int64) (map[int64]time.Time, error)
}

type GoalShareRepo interface {
	ListGoalShares(ctx context.Context, goalID int64) ([]shares.GoalShare, error)
	ListGoalSharesByGoalIDs(ctx context.Context, goalIDs []int64) (map[int64][]shares.GoalShare, error)
	GetGoalShare(ctx context.Context, goalID, teamID int64) (shares.GoalShare, error)
	ReplaceGoalShares(ctx context.Context, goalID int64, shares []shares.GoalShareInput) error
	DeleteGoalShare(ctx context.Context, goalID, teamID int64) error
	UpdateGoalTeamWeight(ctx context.Context, goalID, teamID int64, weight int) error
}

type PeriodRepo interface {
	ListPeriods(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error)
	GetPeriod(ctx context.Context, scope domain.TenantScope, id int64) (domain.Period, error)
	FindPeriodForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error)
	CreatePeriod(ctx context.Context, scope domain.TenantScope, input periods.PeriodInput) (int64, error)
	UpdatePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, input periods.PeriodInput) error
	DeletePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error
	MovePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, direction int) error
}

type KRRepo interface {
	GetKeyResult(ctx context.Context, id int64) (domain.KeyResult, error)
	CreateKeyResult(ctx context.Context, input krs.KeyResultInput) (int64, error)
	UpdateKeyResult(ctx context.Context, input krs.KeyResultUpdateInput) error
	DeleteKeyResult(ctx context.Context, id int64) error
	MoveKeyResult(ctx context.Context, krID int64, direction int) error
	UpsertKeyResultNote(ctx context.Context, krID int64, text string, authorUserID int64) error
	UpdateKeyResultDescription(ctx context.Context, krID int64, description string) error
	FindGoalIDByKR(ctx context.Context, krID int64) (int64, error)
	FindGoalIDByStage(ctx context.Context, stageID int64) (int64, error)
	UpdateNumericalCurrent(ctx context.Context, krID int64, current float64) error
	UpdateBoolean(ctx context.Context, krID int64, done bool) error
	ListProjectStages(ctx context.Context, krID int64) ([]domain.KRProjectStage, error)
	UpdateProjectStageDone(ctx context.Context, stageID int64, done bool) error
	BatchUpdateProjectStagesDone(ctx context.Context, krID int64, updates map[int64]bool) error
	UpsertNumericalMeta(ctx context.Context, input krs.NumericalMetaInput) error
	UpsertBooleanMeta(ctx context.Context, krID int64, done bool) error
	ReplaceProjectStages(ctx context.Context, krID int64, stages []krs.ProjectStageInput) error
}

type TeamStatusRepo interface {
	GetTeamPeriodStatus(ctx context.Context, teamID, periodID int64) (domain.TeamPeriodStatus, error)
	GetTeamPeriodStatusWithTime(ctx context.Context, teamID, periodID int64) (domain.TeamPeriodStatus, *time.Time, error)
	ListTeamPeriodStatuses(ctx context.Context, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error)
	SetTeamPeriodStatus(ctx context.Context, teamID, periodID int64, status domain.TeamPeriodStatus) error
}

type UserRepo interface {
	GetUsersByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error)
	SearchUsersUnrestricted(ctx context.Context, q string, limit int) ([]*domain.User, error)
	SearchUsersInSet(ctx context.Context, userIDs []int64, leadUDIDs []string, q string, limit int) ([]*domain.User, error)
	GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error)
	ListUserLeadTeams(ctx context.Context) (map[string]string, error)
	ValidateUDIDsExist(ctx context.Context, udids []string) ([]string, error)
}

// Deps holds all repository dependencies for the service.
type Deps struct {
	Teams    TeamRepo
	Goals    GoalRepo
	Shares   GoalShareRepo
	Periods  PeriodRepo
	KRs      KRRepo
	Statuses TeamStatusRepo
	Users    UserRepo
	Grants   GrantsProvider
	HCCache  *HealthCheckInCache
}

type Service struct {
	teams    TeamRepo
	goals    GoalRepo
	shares   GoalShareRepo
	periods  PeriodRepo
	krs      KRRepo
	statuses TeamStatusRepo
	users    UserRepo
	grants   GrantsProvider
	hcCache  *HealthCheckInCache
}

var (
	ErrTeamHasGoals                = errors.New("team has goals")
	ErrTeamNotVisibleInPeriod      = errors.New("team not visible in period")
	ErrPeriodClosed                = errors.New("period is closed")
	ErrCannotShareWithClosedPeriod = errors.New("cannot share goal with team whose period is in_progress or closed")
)

// New constructs a Service from a Deps bundle.
func New(deps Deps) *Service {
	return &Service{
		teams:    deps.Teams,
		goals:    deps.Goals,
		shares:   deps.Shares,
		periods:  deps.Periods,
		krs:      deps.KRs,
		statuses: deps.Statuses,
		users:    deps.Users,
		grants:   deps.Grants,
		hcCache:  deps.HCCache,
	}
}

// NewFromStore constructs a Service from a *store.Store and a GrantsProvider.
// Use this at the wiring layer instead of building Deps manually.
func NewFromStore(st *store.Store, grantsProvider GrantsProvider, hcCache *HealthCheckInCache) *Service {
	return New(Deps{
		Teams:    st.Teams,
		Goals:    st.Goals,
		Shares:   st.Shares,
		Periods:  st.Periods,
		KRs:      st.KRs,
		Statuses: st.Statuses,
		Users:    st.Users,
		Grants:   grantsProvider,
		HCCache:  hcCache,
	})
}

type TeamNode struct {
	Team     domain.Team
	Children []TeamNode
}

type TeamSummary struct {
	ID             int64
	Name           string
	Type           domain.TeamType
	Indent         int
	Status         domain.TeamPeriodStatus
	PeriodProgress int
	GoalsCount     int
	GoalsWeight    int
	Goals          []TeamGoalSummary
}

type TeamGoalSummary struct {
	ID         int64
	Title      string
	Weight     int
	Progress   int
	ShareTeams []TeamShareInfo
	Priority   string
	WorkType   domain.WorkType
}

type TeamShareInfo struct {
	ID     int64
	Name   string
	Type   domain.TeamType
	Weight int
}

type TeamChildSummary struct {
	Team              domain.Team
	Status            domain.TeamPeriodStatus
	HasGoals          bool
	Progress          int
	GoalsCount        int
	HighPriorityCount int
	LastUpdateAt      *time.Time
}

type TeamOverview struct {
	AverageProgress int
	TeamsWithGoals  int
	ChildrenSummary []TeamChildSummary
}

type TeamOKR struct {
	Team            domain.Team
	Period          domain.Period
	PeriodStatus    domain.TeamPeriodStatus
	StatusChangedAt *time.Time
	PeriodProgress  int
	GoalsCount      int
	GoalsWeight     int
	Goals           []GoalDetails
}

type GoalDetails struct {
	Goal       domain.Goal
	ShareTeams []TeamShareInfo
}

func (s *Service) GetHierarchy(ctx context.Context, scope domain.TenantScope, periodID *int64) ([]TeamNode, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}
	return s.getHierarchyFromTeams(ctx, scope, allTeams, periodID)
}

func (s *Service) getHierarchyFromTeams(ctx context.Context, scope domain.TenantScope, allTeams []domain.Team, periodID *int64) ([]TeamNode, error) {
	teamIDsWithGoals := map[int64]struct{}{}
	if periodID != nil && *periodID > 0 {
		ids, err := s.teams.ListTeamIDsWithGoalsInPeriod(ctx, scope, *periodID)
		if err != nil {
			return nil, err
		}
		teamIDsWithGoals = ids
	}
	visibleTeams := make([]domain.Team, 0, len(allTeams))
	for _, team := range allTeams {
		if team.DeletedAt == nil {
			visibleTeams = append(visibleTeams, team)
			continue
		}
		if periodID == nil || *periodID == 0 {
			continue
		}
		_, hasGoals := teamIDsWithGoals[team.ID]
		if hasGoals {
			visibleTeams = append(visibleTeams, team)
		}
	}
	_, childrenMap, roots := buildTeamHierarchy(visibleTeams)
	nodes := make([]TeamNode, 0, len(roots))
	for _, team := range roots {
		node := buildTeamNode(team, childrenMap)
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (s *Service) GetTeam(ctx context.Context, scope domain.TenantScope, teamID int64) (domain.Team, error) {
	return s.teams.GetTeam(ctx, scope, teamID)
}

func (s *Service) ListPeriods(ctx context.Context, scope domain.TenantScope) ([]domain.Period, error) {
	return s.periods.ListPeriods(ctx, scope)
}

func (s *Service) GetPeriod(ctx context.Context, scope domain.TenantScope, periodID int64) (domain.Period, error) {
	return s.periods.GetPeriod(ctx, scope, periodID)
}

func (s *Service) GetTeamsWithPeriodSummary(ctx context.Context, scope domain.TenantScope, periodID int64, orgID *int64) ([]TeamSummary, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}
	return s.getTeamsWithPeriodSummaryFromTeams(ctx, scope, allTeams, periodID, orgID)
}

// teamSummaryBatch holds pre-loaded data for building TeamSummary without per-team DB queries.
type teamSummaryBatch struct {
	teamIDsWithGoals map[int64]struct{}
	goalsByTeam      map[int64][]domain.Goal
	statuses         map[int64]domain.TeamPeriodStatus
	sharesByGoal     map[int64][]shares.GoalShare
}

func (s *Service) getTeamsWithPeriodSummaryFromTeams(ctx context.Context, scope domain.TenantScope, allTeams []domain.Team, periodID int64, orgID *int64) ([]TeamSummary, error) {
	teamsByID, childrenMap, roots := buildTeamHierarchy(allTeams)

	allTeamIDs := make([]int64, len(allTeams))
	for i, t := range allTeams {
		allTeamIDs[i] = t.ID
	}

	teamIDsWithGoals, err := s.teams.ListTeamIDsWithGoalsInPeriod(ctx, scope, periodID)
	if err != nil {
		return nil, err
	}
	goalsByTeam, err := s.goals.ListGoalsByTeamsPeriod(ctx, scope, periodID, allTeamIDs)
	if err != nil {
		return nil, err
	}
	statuses, err := s.statuses.ListTeamPeriodStatuses(ctx, periodID, allTeamIDs)
	if err != nil {
		return nil, err
	}

	allGoalIDs := make([]int64, 0)
	for _, gList := range goalsByTeam {
		for _, g := range gList {
			allGoalIDs = append(allGoalIDs, g.ID)
		}
	}
	sharesByGoal, err := s.shares.ListGoalSharesByGoalIDs(ctx, allGoalIDs)
	if err != nil {
		return nil, err
	}

	batch := &teamSummaryBatch{
		teamIDsWithGoals: teamIDsWithGoals,
		goalsByTeam:      goalsByTeam,
		statuses:         statuses,
		sharesByGoal:     sharesByGoal,
	}

	filteredRoots := roots
	if orgID != nil {
		if team, ok := teamsByID[*orgID]; ok {
			filteredRoots = []domain.Team{team}
		}
	}
	rows := make([]TeamSummary, 0, len(allTeams))
	for _, team := range filteredRoots {
		s.appendTeamSummaryFromBatch(&rows, team, 0, childrenMap, teamsByID, batch)
	}
	return rows, nil
}

func (s *Service) appendTeamSummaryFromBatch(rows *[]TeamSummary, team domain.Team, level int, childrenMap map[int64][]domain.Team, teamsByID map[int64]domain.Team, batch *teamSummaryBatch) {
	_, hasGoals := batch.teamIDsWithGoals[team.ID]
	visible := team.DeletedAt == nil || hasGoals

	if visible {
		goalsList := batch.goalsByTeam[team.ID]

		status := domain.TeamPeriodStatusNoGoals
		if s, ok := batch.statuses[team.ID]; ok {
			status = s
		}

		goalRows := make([]TeamGoalSummary, 0, len(goalsList))
		goalsWeight := 0
		for i := range goalsList {
			goalsList[i].Progress = CalculateGoalProgress(&goalsList[i])
			goalsWeight += goalsList[i].Weight
			shareTeams := buildShareInfosFromBatch(goalsList[i], batch.sharesByGoal[goalsList[i].ID], teamsByID)
			goalRows = append(goalRows, TeamGoalSummary{
				ID:         goalsList[i].ID,
				Title:      goalsList[i].Title,
				Weight:     goalsList[i].Weight,
				Progress:   goalsList[i].Progress,
				ShareTeams: shareTeams,
				Priority:   string(goalsList[i].Priority),
				WorkType:   goalsList[i].WorkType,
			})
		}

		*rows = append(*rows, TeamSummary{
			ID:             team.ID,
			Name:           team.Name,
			Type:           team.Type,
			Indent:         level * 24,
			Status:         status,
			PeriodProgress: okr.PeriodProgress(goalsList),
			GoalsCount:     len(goalsList),
			GoalsWeight:    goalsWeight,
			Goals:          goalRows,
		})
	}

	nextLevel := level
	if visible {
		nextLevel = level + 1
	}
	children := childrenMap[team.ID]
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	for _, child := range children {
		s.appendTeamSummaryFromBatch(rows, child, nextLevel, childrenMap, teamsByID, batch)
	}
}

func (s *Service) GetTeamOKR(ctx context.Context, scope domain.TenantScope, teamID, periodID int64, period domain.Period) (TeamOKR, error) {
	team, err := s.teams.GetTeam(ctx, scope, teamID)
	if err != nil {
		return TeamOKR{}, err
	}
	visible, err := s.isTeamVisibleInPeriod(ctx, scope, team, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	if !visible {
		return TeamOKR{}, ErrTeamNotVisibleInPeriod
	}
	goalsList, err := s.goals.ListGoalsByTeamPeriod(ctx, scope, teamID, periodID)
	if err != nil {
		return TeamOKR{}, err
	}

	goalIDs := make([]int64, len(goalsList))
	for i, g := range goalsList {
		goalIDs[i] = g.ID
	}
	sharesByGoal, err := s.shares.ListGoalSharesByGoalIDs(ctx, goalIDs)
	if err != nil {
		return TeamOKR{}, err
	}
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return TeamOKR{}, err
	}
	teamsByID := make(map[int64]domain.Team, len(allTeams))
	for _, t := range allTeams {
		teamsByID[t.ID] = t
	}

	goalsWeight := 0
	goalDetails := make([]GoalDetails, 0, len(goalsList))
	for i := range goalsList {
		goalsList[i].Progress = CalculateGoalProgress(&goalsList[i])
		goalsWeight += goalsList[i].Weight
		shareTeams := buildShareInfosFromBatch(goalsList[i], sharesByGoal[goalsList[i].ID], teamsByID)
		goalDetails = append(goalDetails, GoalDetails{
			Goal:       goalsList[i],
			ShareTeams: shareTeams,
		})
	}

	periodProgress := okr.PeriodProgress(goalsList)
	status, statusChangedAt, err := s.statuses.GetTeamPeriodStatusWithTime(ctx, teamID, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	return TeamOKR{
		Team:            team,
		Period:          period,
		PeriodStatus:    status,
		StatusChangedAt: statusChangedAt,
		PeriodProgress:  periodProgress,
		GoalsCount:      len(goalsList),
		GoalsWeight:     goalsWeight,
		Goals:           goalDetails,
	}, nil
}

// buildShareInfosFromBatch builds TeamShareInfo slice from pre-loaded shares and teams.
// This avoids per-goal DB calls in loops.
func buildShareInfosFromBatch(goal domain.Goal, shareList []shares.GoalShare, teamsByID map[int64]domain.Team) []TeamShareInfo {
	teamIDs := make(map[int64]struct{}, len(shareList)+1)
	teamIDs[goal.TeamID] = struct{}{}
	for _, share := range shareList {
		teamIDs[share.TeamID] = struct{}{}
	}
	teamInfos := make([]TeamShareInfo, 0, len(teamIDs))
	for teamID := range teamIDs {
		team, ok := teamsByID[teamID]
		if !ok {
			continue
		}
		weight := goal.Weight
		for _, share := range shareList {
			if share.TeamID == teamID {
				weight = share.Weight
				break
			}
		}
		teamInfos = append(teamInfos, TeamShareInfo{ID: team.ID, Name: team.Name, Type: team.Type, Weight: weight})
	}
	sort.Slice(teamInfos, func(i, j int) bool { return teamInfos[i].Name < teamInfos[j].Name })
	return teamInfos
}

func (s *Service) GetDirectChildrenSummary(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]TeamChildSummary, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}
	hierarchy, err := s.getHierarchyFromTeams(ctx, scope, allTeams, &periodID)
	if err != nil {
		return nil, err
	}
	children := findDirectChildren(teamID, hierarchy)
	if len(children) == 0 {
		return []TeamChildSummary{}, nil
	}
	return s.buildDirectChildrenSummary(ctx, scope, periodID, children, nil)
}

func (s *Service) buildDirectChildrenSummary(ctx context.Context, scope domain.TenantScope, periodID int64, children []TeamNode, goalsByTeam map[int64][]domain.Goal) ([]TeamChildSummary, error) {
	childIDs := make([]int64, 0, len(children))
	for _, child := range children {
		childIDs = append(childIDs, child.Team.ID)
	}
	statuses, err := s.statuses.ListTeamPeriodStatuses(ctx, periodID, childIDs)
	if err != nil {
		return nil, err
	}
	lastUpdates, err := s.goals.ListTeamLastGoalUpdateInPeriod(ctx, scope, periodID, childIDs)
	if err != nil {
		return nil, err
	}
	result := make([]TeamChildSummary, 0, len(childIDs))
	for _, child := range children {
		item := TeamChildSummary{
			Team:   child.Team,
			Status: domain.TeamPeriodStatusNoGoals,
		}
		if status, ok := statuses[child.Team.ID]; ok {
			item.Status = status
		}
		var goalsList []domain.Goal
		if goalsByTeam != nil {
			goalsList = goalsByTeam[child.Team.ID]
		} else {
			goalsList, err = s.goals.ListGoalsByTeamPeriod(ctx, scope, child.Team.ID, periodID)
			if err != nil {
				return nil, err
			}
		}
		item.GoalsCount = len(goalsList)
		item.HasGoals = item.GoalsCount > 0
		if item.HasGoals {
			for i := range goalsList {
				goalsList[i].Progress = CalculateGoalProgress(&goalsList[i])
			}
			item.Progress = okr.PeriodProgress(goalsList)
			for _, g := range goalsList {
				if g.Priority == domain.PriorityP0 || g.Priority == domain.PriorityP1 {
					item.HighPriorityCount++
				}
			}
		}
		if updatedAt, ok := lastUpdates[child.Team.ID]; ok {
			value := updatedAt
			item.LastUpdateAt = &value
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) GetTeamOverview(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) (TeamOverview, error) {
	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return TeamOverview{}, err
	}
	hierarchy, err := s.getHierarchyFromTeams(ctx, scope, allTeams, &periodID)
	if err != nil {
		return TeamOverview{}, err
	}
	children := findDirectChildren(teamID, hierarchy)
	descendantIDs := collectDescendantIDs(teamID, hierarchy)
	goalsByTeam, err := s.goals.ListGoalsByTeamsPeriod(ctx, scope, periodID, descendantIDs)
	if err != nil {
		return TeamOverview{}, err
	}
	summaryByID := make(map[int64]TeamSummary, len(descendantIDs))
	for _, id := range descendantIDs {
		goalsList := goalsByTeam[id]
		if len(goalsList) == 0 {
			continue
		}
		for i := range goalsList {
			goalsList[i].Progress = CalculateGoalProgress(&goalsList[i])
		}
		summaryByID[id] = TeamSummary{
			ID:             id,
			GoalsCount:     len(goalsList),
			PeriodProgress: okr.PeriodProgress(goalsList),
		}
	}
	childrenSummary := []TeamChildSummary{}
	if len(children) > 0 {
		childrenSummary, err = s.buildDirectChildrenSummary(ctx, scope, periodID, children, goalsByTeam)
		if err != nil {
			return TeamOverview{}, err
		}
	}

	totalProgress := 0
	teamsWithGoals := 0

	for _, id := range descendantIDs {
		summary, ok := summaryByID[id]
		if !ok || summary.GoalsCount == 0 {
			continue
		}
		teamsWithGoals++
		totalProgress += summary.PeriodProgress
	}

	avgProgress := 0
	if teamsWithGoals > 0 {
		avgProgress = totalProgress / teamsWithGoals
	}

	return TeamOverview{
		AverageProgress: avgProgress,
		TeamsWithGoals:  teamsWithGoals,
		ChildrenSummary: childrenSummary,
	}, nil
}

func findDirectChildren(targetID int64, nodes []TeamNode) []TeamNode {
	var children []TeamNode
	var walk func(items []TeamNode) bool
	walk = func(items []TeamNode) bool {
		for _, node := range items {
			if node.Team.ID == targetID {
				children = node.Children
				return true
			}
			if walk(node.Children) {
				return true
			}
		}
		return false
	}
	_ = walk(nodes)
	return children
}

func collectDescendantIDs(targetID int64, nodes []TeamNode) []int64 {
	var descendants []int64
	var walk func(items []TeamNode, collect bool)
	walk = func(items []TeamNode, collect bool) {
		for _, node := range items {
			nextCollect := collect || node.Team.ID == targetID
			if collect {
				descendants = append(descendants, node.Team.ID)
			}
			if len(node.Children) > 0 {
				walk(node.Children, nextCollect)
			}
		}
	}
	walk(nodes, false)
	return descendants
}

func (s *Service) UpdateKRProgressNumerical(ctx context.Context, krID int64, current float64) error {
	kr, err := s.krs.GetKeyResult(ctx, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindNumerical {
		return fmt.Errorf("unsupported kr kind for numerical update: %s", kr.Kind)
	}
	return s.krs.UpdateNumericalCurrent(ctx, krID, current)
}

func (s *Service) UpdateKRProgressBoolean(ctx context.Context, krID int64, done bool) error {
	kr, err := s.krs.GetKeyResult(ctx, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindBoolean {
		return fmt.Errorf("unsupported kr kind for boolean update: %s", kr.Kind)
	}
	return s.krs.UpdateBoolean(ctx, krID, done)
}

type ProjectStageUpdate struct {
	ID     int64
	IsDone bool
}

func (s *Service) UpdateKRProgressProject(ctx context.Context, krID int64, updates []ProjectStageUpdate) error {
	kr, err := s.krs.GetKeyResult(ctx, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindProject {
		return fmt.Errorf("unsupported kr kind for project update: %s", kr.Kind)
	}
	stages, err := s.krs.ListProjectStages(ctx, krID)
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
	return s.krs.BatchUpdateProjectStagesDone(ctx, krID, validUpdates)
}

type ShareTarget struct {
	TeamID int64
	Weight int
}

func (s *Service) ShareGoal(ctx context.Context, goalID int64, targets []ShareTarget) error {
	shareInputs := make([]shares.GoalShareInput, 0, len(targets))
	for _, target := range targets {
		shareInputs = append(shareInputs, shares.GoalShareInput{TeamID: target.TeamID, Weight: target.Weight})
	}
	return s.shares.ReplaceGoalShares(ctx, goalID, shareInputs)
}

func (s *Service) UpdateGoalWeight(ctx context.Context, goalID, teamID int64, weight int) error {
	return s.shares.UpdateGoalTeamWeight(ctx, goalID, teamID, weight)
}

func (s *Service) AddGoalComment(ctx context.Context, scope domain.TenantScope, goalID int64, text string, authorUserID int64) error {
	return s.goals.AddGoalComment(ctx, scope, goalID, text, authorUserID)
}

func (s *Service) UpsertKeyResultNote(ctx context.Context, krID int64, text string, authorUserID int64) error {
	return s.krs.UpsertKeyResultNote(ctx, krID, text, authorUserID)
}

func (s *Service) UpdateKeyResultDescription(ctx context.Context, krID int64, description string) error {
	return s.krs.UpdateKeyResultDescription(ctx, krID, description)
}

func (s *Service) GetKeyResult(ctx context.Context, id int64) (domain.KeyResult, error) {
	return s.krs.GetKeyResult(ctx, id)
}

func (s *Service) GetGoal(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error) {
	goal, err := s.goals.GetGoal(ctx, scope, id)
	if err != nil {
		return domain.Goal{}, err
	}
	goal.Progress = CalculateGoalProgress(&goal)
	return goal, nil
}

type KeyResultMetaInput struct {
	NumericalStart       float64
	NumericalTarget      float64
	NumericalCurrent     float64
	NumericalUnit        string
	NumericalCheckpoints []domain.KRNumericalCheckpoint
	ZeroingCriteria      string
	BooleanDone          bool
	ProjectStages        []krs.ProjectStageInput
}

func (s *Service) UpdateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalUpdateInput) error {
	return s.goals.UpdateGoal(ctx, scope, input)
}

func (s *Service) MoveGoal(ctx context.Context, scope domain.TenantScope, goalID int64, direction int) error {
	return s.goals.MoveGoal(ctx, scope, goalID, direction)
}

func (s *Service) MoveKeyResult(ctx context.Context, krID int64, direction int) error {
	return s.krs.MoveKeyResult(ctx, krID, direction)
}

func (s *Service) CreateKeyResultWithMeta(ctx context.Context, input krs.KeyResultInput, meta KeyResultMetaInput) (int64, error) {
	krID, err := s.krs.CreateKeyResult(ctx, input)
	if err != nil {
		return 0, err
	}
	if err := s.applyKeyResultMeta(ctx, krID, input.Kind, meta); err != nil {
		return 0, err
	}
	return krID, nil
}

func (s *Service) UpdateKeyResultWithMeta(ctx context.Context, input krs.KeyResultUpdateInput, meta KeyResultMetaInput) error {
	if err := s.krs.UpdateKeyResult(ctx, input); err != nil {
		return err
	}
	return s.applyKeyResultMeta(ctx, input.ID, input.Kind, meta)
}

func (s *Service) applyKeyResultMeta(ctx context.Context, krID int64, kind domain.KRKind, meta KeyResultMetaInput) error {
	switch kind {
	case domain.KRKindNumerical:
		return s.krs.UpsertNumericalMeta(ctx, krs.NumericalMetaInput{
			KeyResultID:     krID,
			StartValue:      meta.NumericalStart,
			TargetValue:     meta.NumericalTarget,
			CurrentValue:    meta.NumericalCurrent,
			Unit:            meta.NumericalUnit,
			Checkpoints:     meta.NumericalCheckpoints,
			ZeroingCriteria: meta.ZeroingCriteria,
		})
	case domain.KRKindBoolean:
		return s.krs.UpsertBooleanMeta(ctx, krID, meta.BooleanDone)
	case domain.KRKindProject:
		return s.krs.ReplaceProjectStages(ctx, krID, meta.ProjectStages)
	default:
		return nil
	}
}

func (s *Service) UpdateTeamPeriodStatus(ctx context.Context, teamID, periodID int64, status domain.TeamPeriodStatus) error {
	return s.statuses.SetTeamPeriodStatus(ctx, teamID, periodID, status)
}

func (s *Service) DeleteTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	hasGoals, err := s.teams.TeamHasGoals(ctx, scope, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return s.teams.SoftDeleteTeam(ctx, scope, teamID)
	}
	return s.teams.HardDeleteTeam(ctx, scope, teamID)
}

func (s *Service) RestoreTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	return s.teams.RestoreTeam(ctx, scope, teamID)
}

func (s *Service) HardDeleteTeam(ctx context.Context, scope domain.TenantScope, teamID int64) error {
	hasGoals, err := s.teams.TeamHasGoals(ctx, scope, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return ErrTeamHasGoals
	}
	return s.teams.HardDeleteTeam(ctx, scope, teamID)
}

func (s *Service) isTeamVisibleInPeriod(ctx context.Context, scope domain.TenantScope, team domain.Team, periodID int64) (bool, error) {
	hasGoals, err := s.teams.TeamHasGoalsInPeriod(ctx, scope, team.ID, periodID)
	if err != nil {
		return false, err
	}
	if hasGoals {
		return true, nil
	}
	return team.DeletedAt == nil, nil
}

func buildTeamHierarchy(allTeams []domain.Team) (map[int64]domain.Team, map[int64][]domain.Team, []domain.Team) {
	teamsByID := make(map[int64]domain.Team, len(allTeams))
	childrenMap := make(map[int64][]domain.Team)
	roots := make([]domain.Team, 0)
	for _, team := range allTeams {
		teamsByID[team.ID] = team
	}
	for _, team := range allTeams {
		if team.ParentID != nil {
			if _, ok := teamsByID[*team.ParentID]; ok {
				childrenMap[*team.ParentID] = append(childrenMap[*team.ParentID], team)
				continue
			}
		}
		roots = append(roots, team)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	return teamsByID, childrenMap, roots
}

func buildTeamNode(team domain.Team, childrenMap map[int64][]domain.Team) TeamNode {
	children := childrenMap[team.ID]
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	node := TeamNode{Team: team}
	for _, child := range children {
		node.Children = append(node.Children, buildTeamNode(child, childrenMap))
	}
	return node
}

// — Team passthroughs —

func (s *Service) ListTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teams.ListTeams(ctx, scope)
}

func (s *Service) ListDeletedTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teams.ListDeletedTeams(ctx, scope)
}

func (s *Service) ListAllTeams(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error) {
	return s.teams.ListAllTeams(ctx, scope)
}

func (s *Service) CreateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput) (int64, error) {
	return s.teams.CreateTeam(ctx, scope, input)
}

func (s *Service) UpdateTeam(ctx context.Context, scope domain.TenantScope, input teams.TeamInput, id int64) error {
	return s.teams.UpdateTeam(ctx, scope, input, id)
}

// — Period passthroughs —

func (s *Service) FindPeriodForDate(ctx context.Context, scope domain.TenantScope, date time.Time) (domain.Period, error) {
	return s.periods.FindPeriodForDate(ctx, scope, date)
}

func (s *Service) CreatePeriod(ctx context.Context, scope domain.TenantScope, input periods.PeriodInput) (int64, error) {
	return s.periods.CreatePeriod(ctx, scope, input)
}

func (s *Service) UpdatePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, input periods.PeriodInput) error {
	return s.periods.UpdatePeriod(ctx, scope, periodID, input)
}

func (s *Service) DeletePeriod(ctx context.Context, scope domain.TenantScope, periodID int64) error {
	return s.periods.DeletePeriod(ctx, scope, periodID)
}

func (s *Service) MovePeriod(ctx context.Context, scope domain.TenantScope, periodID int64, direction int) error {
	return s.periods.MovePeriod(ctx, scope, periodID, direction)
}

func (s *Service) GetUsersByDisplayNames(ctx context.Context, names []string) ([]*domain.User, error) {
	return s.users.GetUsersByDisplayNames(ctx, names)
}

func (s *Service) GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error) {
	return s.users.GetUsersByUDIDs(ctx, udids)
}

func (s *Service) ListUserLeadTeams(ctx context.Context) (map[string]string, error) {
	return s.users.ListUserLeadTeams(ctx)
}

func (s *Service) ValidateUserUDIDsExist(ctx context.Context, udids []string) ([]string, error) {
	return s.users.ValidateUDIDsExist(ctx, udids)
}

// SearchUsersInScope returns up to 20 non-system users visible in the given scope.
//   - scopeTeamIDs == nil → admin/unrestricted: all users
//   - scopeTeamIDs != nil → users with a hierarchy grant to any team related to the scope nodes:
//     ancestors (access from above), the nodes themselves, or descendants (access from below).
//
// Uses the GrantsProvider cache; falls back to empty result when cache is unavailable.
func (s *Service) SearchUsersInScope(ctx context.Context, scope domain.TenantScope, scopeTeamIDs []int64, q string, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 20
	}
	if scopeTeamIDs == nil {
		return s.users.SearchUsersUnrestricted(ctx, q, limit)
	}
	if len(scopeTeamIDs) == 0 || s.grants == nil {
		return nil, nil
	}

	allTeams, err := s.teams.ListAllTeams(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Build both maps for bidirectional tree traversal.
	parentMap := make(map[int64]int64, len(allTeams))
	childrenMap := make(map[int64][]int64, len(allTeams))
	for _, t := range allTeams {
		if t.ParentID != nil {
			parentMap[t.ID] = *t.ParentID
			childrenMap[*t.ParentID] = append(childrenMap[*t.ParentID], t.ID)
		}
	}

	// Related set: scope nodes + all their ancestors + all their descendants.
	relatedSet := make(map[int64]struct{})
	for _, id := range scopeTeamIDs {
		// Walk up.
		cur := id
		for {
			relatedSet[cur] = struct{}{}
			parent, ok := parentMap[cur]
			if !ok {
				break
			}
			cur = parent
		}
		// Walk down via BFS.
		queue := []int64{id}
		for len(queue) > 0 {
			cur, queue = queue[0], queue[1:]
			for _, child := range childrenMap[cur] {
				if _, visited := relatedSet[child]; !visited {
					relatedSet[child] = struct{}{}
					queue = append(queue, child)
				}
			}
		}
	}

	allGrants, err := s.grants.AllGrants(ctx)
	if err != nil {
		return nil, err
	}

	// Collect IDs of users whose grants intersect the related set.
	eligibleIDs := make([]int64, 0)
	seen := make(map[int64]struct{})
	for userID, userGrants := range allGrants {
		for _, g := range userGrants {
			if _, ok := relatedSet[g.TeamID]; ok {
				if _, dup := seen[userID]; !dup {
					seen[userID] = struct{}{}
					eligibleIDs = append(eligibleIDs, userID)
				}
				break
			}
		}
	}

	// Team leads of all related nodes are eligible regardless of explicit grants.
	leadUDIDs := make([]string, 0)
	for _, t := range allTeams {
		if _, ok := relatedSet[t.ID]; ok && t.LeadUDID != nil && t.DeletedAt == nil {
			leadUDIDs = append(leadUDIDs, *t.LeadUDID)
		}
	}

	return s.users.SearchUsersInSet(ctx, eligibleIDs, leadUDIDs, q, limit)
}

// — Goal passthroughs —

func (s *Service) ListGoalsByTeamPeriod(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) ([]domain.Goal, error) {
	return s.goals.ListGoalsByTeamPeriod(ctx, scope, teamID, periodID)
}

func (s *Service) UpdateGoalFields(ctx context.Context, scope domain.TenantScope, input goals.GoalFieldsUpdateInput) error {
	return s.goals.UpdateGoalFields(ctx, scope, input)
}

func (s *Service) ListGoalComments(ctx context.Context, scope domain.TenantScope, goalID int64) ([]domain.GoalComment, error) {
	return s.goals.ListGoalComments(ctx, scope, goalID)
}

func (s *Service) GetTeamPeriodStatus(ctx context.Context, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	return s.statuses.GetTeamPeriodStatus(ctx, teamID, periodID)
}

func (s *Service) GetGoalShare(ctx context.Context, goalID, teamID int64) (shares.GoalShare, error) {
	return s.shares.GetGoalShare(ctx, goalID, teamID)
}

func (s *Service) DeleteGoalShare(ctx context.Context, goalID, teamID int64) error {
	return s.shares.DeleteGoalShare(ctx, goalID, teamID)
}

func (s *Service) ListGoalShares(ctx context.Context, goalID int64) ([]shares.GoalShare, error) {
	return s.shares.ListGoalShares(ctx, goalID)
}

// — Key result passthroughs —

func (s *Service) DeleteKeyResult(ctx context.Context, id int64) error {
	return s.krs.DeleteKeyResult(ctx, id)
}

func (s *Service) FindGoalIDByKR(ctx context.Context, krID int64) (int64, error) {
	return s.krs.FindGoalIDByKR(ctx, krID)
}

func (s *Service) FindGoalIDByStage(ctx context.Context, stageID int64) (int64, error) {
	return s.krs.FindGoalIDByStage(ctx, stageID)
}

// — Business logic —

// CreateGoal creates a goal and auto-advances status from NoGoals to Forming on first goal.
// Returns ErrPeriodClosed if the team's period status is InProgress or Closed.
func (s *Service) CreateGoal(ctx context.Context, scope domain.TenantScope, input goals.GoalInput) (int64, error) {
	status, err := s.statuses.GetTeamPeriodStatus(ctx, input.TeamID, input.PeriodID)
	if err != nil {
		return 0, err
	}
	if status == domain.TeamPeriodStatusClosed || status == domain.TeamPeriodStatusInProgress {
		return 0, ErrPeriodClosed
	}
	goalID, err := s.goals.CreateGoal(ctx, scope, input)
	if err != nil {
		return 0, err
	}
	if status == domain.TeamPeriodStatusNoGoals {
		if err := s.statuses.SetTeamPeriodStatus(ctx, input.TeamID, input.PeriodID, domain.TeamPeriodStatusForming); err != nil {
			return 0, err
		}
	}
	return goalID, nil
}

// DeleteGoal removes a goal or a team's share of it, transferring ownership when the owner deletes.
// Returns the effective requesting teamID and the goal's periodID for redirect.
// Returns ErrPeriodClosed if the owner tries to delete in a closed period with no shares.
func (s *Service) DeleteGoal(ctx context.Context, scope domain.TenantScope, goalID, requestingTeamID int64) (effectiveTeamID, periodID int64, err error) {
	goal, err := s.goals.GetGoal(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	if requestingTeamID == 0 {
		requestingTeamID = goal.TeamID
	}
	if requestingTeamID != goal.TeamID {
		if err := s.shares.DeleteGoalShare(ctx, goalID, requestingTeamID); err != nil {
			return 0, 0, err
		}
		_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
		return requestingTeamID, goal.PeriodID, nil
	}
	shareList, err := s.shares.ListGoalShares(ctx, goalID)
	if err != nil {
		return 0, 0, err
	}
	if len(shareList) > 0 {
		newOwner := shareList[0]
		if err := s.goals.UpdateGoalOwner(ctx, scope, goalID, newOwner.TeamID, newOwner.Weight); err != nil {
			return 0, 0, err
		}
		if err := s.shares.DeleteGoalShare(ctx, goalID, newOwner.TeamID); err != nil {
			return 0, 0, err
		}
		_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
		return requestingTeamID, goal.PeriodID, nil
	}
	status, err := s.statuses.GetTeamPeriodStatus(ctx, goal.TeamID, goal.PeriodID)
	if err != nil {
		return 0, 0, err
	}
	if status == domain.TeamPeriodStatusClosed || status == domain.TeamPeriodStatusInProgress {
		return 0, 0, ErrPeriodClosed
	}
	if err := s.goals.DeleteGoal(ctx, scope, goalID); err != nil {
		return 0, 0, err
	}
	_ = s.resetStatusIfNoGoals(ctx, scope, requestingTeamID, goal.PeriodID)
	return requestingTeamID, goal.PeriodID, nil
}

func (s *Service) resetStatusIfNoGoals(ctx context.Context, scope domain.TenantScope, teamID, periodID int64) error {
	goalsList, err := s.goals.ListGoalsByTeamPeriod(ctx, scope, teamID, periodID)
	if err != nil || len(goalsList) > 0 {
		return err
	}
	status, err := s.statuses.GetTeamPeriodStatus(ctx, teamID, periodID)
	if err != nil || status == domain.TeamPeriodStatusNoGoals {
		return nil
	}
	return s.statuses.SetTeamPeriodStatus(ctx, teamID, periodID, domain.TeamPeriodStatusNoGoals)
}

// UpdateGoalOwnerAndShares updates goal ownership and sharing based on the selected team set.
// Returns ErrCannotShareWithClosedPeriod if any selected team has an in_progress or closed period.
func (s *Service) UpdateGoalOwnerAndShares(ctx context.Context, scope domain.TenantScope, goalID int64, selectedTeamIDs []int64) (ownerID, periodID int64, err error) {
	goal, err := s.goals.GetGoal(ctx, scope, goalID)
	if err != nil {
		return 0, 0, err
	}
	shareList, err := s.shares.ListGoalShares(ctx, goalID)
	if err != nil {
		return 0, 0, err
	}
	shareWeights := make(map[int64]int, len(shareList))
	for _, share := range shareList {
		shareWeights[share.TeamID] = share.Weight
	}
	selectedSet := make(map[int64]struct{}, len(selectedTeamIDs))
	for _, id := range selectedTeamIDs {
		selectedSet[id] = struct{}{}
	}
	ownerID = goal.TeamID
	if _, ok := selectedSet[ownerID]; !ok && len(selectedTeamIDs) > 0 {
		ownerID = selectedTeamIDs[0]
	}
	newShares := make([]shares.GoalShareInput, 0, len(selectedTeamIDs))
	for _, teamID := range selectedTeamIDs {
		status, err := s.statuses.GetTeamPeriodStatus(ctx, teamID, goal.PeriodID)
		if err != nil {
			return 0, 0, err
		}
		if status == domain.TeamPeriodStatusInProgress || status == domain.TeamPeriodStatusClosed {
			return 0, 0, ErrCannotShareWithClosedPeriod
		}
		if teamID == ownerID {
			ownerWeight := goal.Weight
			if ownerID != goal.TeamID {
				if w, ok := shareWeights[ownerID]; ok {
					ownerWeight = w
				} else {
					ownerWeight = 0
				}
			}
			if err := s.goals.UpdateGoalOwner(ctx, scope, goalID, ownerID, ownerWeight); err != nil {
				return 0, 0, err
			}
			continue
		}
		weight := 0
		if w, ok := shareWeights[teamID]; ok {
			weight = w
		}
		newShares = append(newShares, shares.GoalShareInput{TeamID: teamID, Weight: weight})
	}
	if err := s.shares.ReplaceGoalShares(ctx, goalID, newShares); err != nil {
		return 0, 0, err
	}
	return ownerID, goal.PeriodID, nil
}
