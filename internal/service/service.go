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
)

type Store interface {
	ListTeams(ctx context.Context) ([]domain.Team, error)
	ListDeletedTeams(ctx context.Context) ([]domain.Team, error)
	ListAllTeams(ctx context.Context) ([]domain.Team, error)
	GetTeam(ctx context.Context, id int64) (domain.Team, error)
	CreateTeam(ctx context.Context, input store.TeamInput) (int64, error)
	UpdateTeam(ctx context.Context, input store.TeamInput, id int64) error
	ListPeriods(ctx context.Context) ([]domain.Period, error)
	GetPeriod(ctx context.Context, id int64) (domain.Period, error)
	FindPeriodForDate(ctx context.Context, date time.Time) (domain.Period, error)
	CreatePeriod(ctx context.Context, input store.PeriodInput) (int64, error)
	UpdatePeriod(ctx context.Context, periodID int64, input store.PeriodInput) error
	DeletePeriod(ctx context.Context, periodID int64) error
	MovePeriod(ctx context.Context, periodID int64, direction int) error
	ListGoalsByTeamPeriod(ctx context.Context, teamID, periodID int64) ([]domain.Goal, error)
	ListGoalsByTeamsPeriod(ctx context.Context, periodID int64, teamIDs []int64) (map[int64][]domain.Goal, error)
	ListTeamOverviewStats(ctx context.Context, periodID int64, teamIDs []int64) (map[int64]store.TeamOverviewStats, error)
	GetGoal(ctx context.Context, id int64) (domain.Goal, error)
	CreateGoal(ctx context.Context, input store.GoalInput) (int64, error)
	DeleteGoal(ctx context.Context, id int64) error
	UpdateGoal(ctx context.Context, input store.GoalUpdateInput) error
	UpdateGoalFields(ctx context.Context, input store.GoalFieldsUpdateInput) error
	UpdateGoalOwner(ctx context.Context, goalID, teamID int64, weight int) error
	MoveGoal(ctx context.Context, goalID int64, direction int) error
	AddGoalComment(ctx context.Context, goalID int64, text string) error
	ListGoalComments(ctx context.Context, goalID int64) ([]domain.GoalComment, error)
	ListGoalShares(ctx context.Context, goalID int64) ([]store.GoalShare, error)
	GetGoalShare(ctx context.Context, goalID, teamID int64) (store.GoalShare, error)
	ReplaceGoalShares(ctx context.Context, goalID int64, shares []store.GoalShareInput) error
	DeleteGoalShare(ctx context.Context, goalID, teamID int64) error
	UpdateGoalTeamWeight(ctx context.Context, goalID, teamID int64, weight int) error
	GetTeamPeriodStatus(ctx context.Context, teamID, periodID int64) (domain.TeamPeriodStatus, error)
	ListTeamPeriodStatuses(ctx context.Context, periodID int64, teamIDs []int64) (map[int64]domain.TeamPeriodStatus, error)
	SetTeamPeriodStatus(ctx context.Context, teamID, periodID int64, status domain.TeamPeriodStatus) error
	ListTeamLastGoalUpdateInPeriod(ctx context.Context, periodID int64, teamIDs []int64) (map[int64]time.Time, error)
	TeamHasGoals(ctx context.Context, id int64) (bool, error)
	TeamHasGoalsInPeriod(ctx context.Context, id, periodID int64) (bool, error)
	ListTeamIDsWithGoalsInPeriod(ctx context.Context, periodID int64) (map[int64]struct{}, error)
	SoftDeleteTeam(ctx context.Context, id int64) error
	RestoreTeam(ctx context.Context, id int64) error
	HardDeleteTeam(ctx context.Context, id int64) error
	GetKeyResult(ctx context.Context, id int64) (domain.KeyResult, error)
	CreateKeyResult(ctx context.Context, input store.KeyResultInput) (int64, error)
	UpdateKeyResult(ctx context.Context, input store.KeyResultUpdateInput) error
	DeleteKeyResult(ctx context.Context, id int64) error
	MoveKeyResult(ctx context.Context, krID int64, direction int) error
	AddKeyResultComment(ctx context.Context, krID int64, text string) error
	FindGoalIDByKR(ctx context.Context, krID int64) (int64, error)
	FindGoalIDByStage(ctx context.Context, stageID int64) (int64, error)
	UpdatePercentCurrent(ctx context.Context, krID int64, current float64) error
	UpdateLinearCurrent(ctx context.Context, krID int64, current float64) error
	UpdateBoolean(ctx context.Context, krID int64, done bool) error
	ListProjectStages(ctx context.Context, krID int64) ([]domain.KRProjectStage, error)
	UpdateProjectStageDone(ctx context.Context, stageID int64, done bool) error
	UpsertPercentMeta(ctx context.Context, input store.PercentMetaInput) error
	UpsertLinearMeta(ctx context.Context, input store.LinearMetaInput) error
	UpsertBooleanMeta(ctx context.Context, krID int64, done bool) error
	ReplaceProjectStages(ctx context.Context, krID int64, stages []store.ProjectStageInput) error
}

type Service struct {
	store Store
}

var (
	ErrTeamHasGoals                = errors.New("team has goals")
	ErrTeamNotVisibleInPeriod      = errors.New("team not visible in period")
	ErrPeriodClosed                = errors.New("period is closed")
	ErrCannotShareWithClosedPeriod = errors.New("cannot share goal with team whose period is validated or closed")
)

func New(store Store) *Service {
	return &Service{store: store}
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
	Team         domain.Team
	Status       domain.TeamPeriodStatus
	HasGoals     bool
	Progress     int
	LastUpdateAt *time.Time
}

type TeamOverview struct {
	AverageProgress int
	TeamsWithGoals  int
	Priorities      TeamPrioritySummary
	WorkBalance     TeamWorkBalance
	ChildrenSummary []TeamChildSummary
}

type TeamPrioritySummary struct {
	P0 int
	P1 int
	P2 int
	P3 int
}

type TeamWorkBalance struct {
	Discovery int
	Delivery  int
}

type TeamOKR struct {
	Team           domain.Team
	Period         domain.Period
	PeriodStatus   domain.TeamPeriodStatus
	PeriodProgress int
	GoalsCount     int
	GoalsWeight    int
	Goals          []GoalDetails
}

type GoalDetails struct {
	Goal       domain.Goal
	ShareTeams []TeamShareInfo
}

func (s *Service) GetHierarchy(ctx context.Context, periodID *int64) ([]TeamNode, error) {
	teams, err := s.store.ListAllTeams(ctx)
	if err != nil {
		return nil, err
	}
	return s.getHierarchyFromTeams(ctx, teams, periodID)
}

func (s *Service) getHierarchyFromTeams(ctx context.Context, teams []domain.Team, periodID *int64) ([]TeamNode, error) {
	teamIDsWithGoals := map[int64]struct{}{}
	if periodID != nil && *periodID > 0 {
		ids, err := s.store.ListTeamIDsWithGoalsInPeriod(ctx, *periodID)
		if err != nil {
			return nil, err
		}
		teamIDsWithGoals = ids
	}
	visibleTeams := make([]domain.Team, 0, len(teams))
	for _, team := range teams {
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

func (s *Service) GetTeam(ctx context.Context, teamID int64) (domain.Team, error) {
	return s.store.GetTeam(ctx, teamID)
}

func (s *Service) ListPeriods(ctx context.Context) ([]domain.Period, error) {
	return s.store.ListPeriods(ctx)
}

func (s *Service) GetPeriod(ctx context.Context, periodID int64) (domain.Period, error) {
	return s.store.GetPeriod(ctx, periodID)
}

func (s *Service) GetTeamsWithPeriodSummary(ctx context.Context, periodID int64, orgID *int64) ([]TeamSummary, error) {
	teams, err := s.store.ListAllTeams(ctx)
	if err != nil {
		return nil, err
	}
	return s.getTeamsWithPeriodSummaryFromTeams(ctx, teams, periodID, orgID)
}

func (s *Service) getTeamsWithPeriodSummaryFromTeams(ctx context.Context, teams []domain.Team, periodID int64, orgID *int64) ([]TeamSummary, error) {
	teamsByID, childrenMap, roots := buildTeamHierarchy(teams)
	filteredRoots := roots
	if orgID != nil {
		if team, ok := teamsByID[*orgID]; ok {
			filteredRoots = []domain.Team{team}
		}
	}
	rows := make([]TeamSummary, 0, len(teams))
	for _, team := range filteredRoots {
		if err := s.appendTeamSummary(ctx, &rows, team, 0, periodID, childrenMap, teamsByID); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (s *Service) GetTeamOKR(ctx context.Context, teamID, periodID int64, period domain.Period) (TeamOKR, error) {
	team, err := s.store.GetTeam(ctx, teamID)
	if err != nil {
		return TeamOKR{}, err
	}
	visible, err := s.isTeamVisibleInPeriod(ctx, team, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	if !visible {
		return TeamOKR{}, ErrTeamNotVisibleInPeriod
	}
	goals, err := s.store.ListGoalsByTeamPeriod(ctx, teamID, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	shareInfos := make(map[int64][]TeamShareInfo, len(goals))
	for i := range goals {
		goals[i].Progress = CalculateGoalProgress(&goals[i])
		shares, err := s.listGoalShareTeams(ctx, goals[i], nil)
		if err != nil {
			return TeamOKR{}, err
		}
		shareInfos[goals[i].ID] = shares
	}
	periodProgress := okr.PeriodProgress(goals)
	goalsWeight := 0
	for _, goal := range goals {
		goalsWeight += goal.Weight
	}
	status, err := s.store.GetTeamPeriodStatus(ctx, teamID, periodID)
	if err != nil {
		return TeamOKR{}, err
	}
	goalDetails := make([]GoalDetails, 0, len(goals))
	for _, goal := range goals {
		goalDetails = append(goalDetails, GoalDetails{
			Goal:       goal,
			ShareTeams: shareInfos[goal.ID],
		})
	}
	return TeamOKR{
		Team:           team,
		Period:         period,
		PeriodStatus:   status,
		PeriodProgress: periodProgress,
		GoalsCount:     len(goals),
		GoalsWeight:    goalsWeight,
		Goals:          goalDetails,
	}, nil
}

func (s *Service) GetDirectChildrenSummary(ctx context.Context, teamID, periodID int64) ([]TeamChildSummary, error) {
	teams, err := s.store.ListAllTeams(ctx)
	if err != nil {
		return nil, err
	}
	hierarchy, err := s.getHierarchyFromTeams(ctx, teams, &periodID)
	if err != nil {
		return nil, err
	}
	children := findDirectChildren(teamID, hierarchy)
	if len(children) == 0 {
		return []TeamChildSummary{}, nil
	}
	return s.buildDirectChildrenSummary(ctx, periodID, children, nil)
}

func (s *Service) buildDirectChildrenSummary(ctx context.Context, periodID int64, children []TeamNode, summaryByID map[int64]TeamSummary) ([]TeamChildSummary, error) {
	childIDs := make([]int64, 0, len(children))
	for _, child := range children {
		childIDs = append(childIDs, child.Team.ID)
	}
	statuses, err := s.store.ListTeamPeriodStatuses(ctx, periodID, childIDs)
	if err != nil {
		return nil, err
	}
	lastUpdates, err := s.store.ListTeamLastGoalUpdateInPeriod(ctx, periodID, childIDs)
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
		if summaryByID != nil {
			if summary, ok := summaryByID[child.Team.ID]; ok {
				item.HasGoals = summary.GoalsCount > 0
				item.Progress = summary.PeriodProgress
			}
		} else {
			goals, err := s.store.ListGoalsByTeamPeriod(ctx, child.Team.ID, periodID)
			if err != nil {
				return nil, err
			}
			item.HasGoals = len(goals) > 0
			if item.HasGoals {
				for i := range goals {
					goals[i].Progress = CalculateGoalProgress(&goals[i])
				}
				item.Progress = okr.PeriodProgress(goals)
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

func (s *Service) GetTeamOverview(ctx context.Context, teamID, periodID int64) (TeamOverview, error) {
	teams, err := s.store.ListAllTeams(ctx)
	if err != nil {
		return TeamOverview{}, err
	}
	hierarchy, err := s.getHierarchyFromTeams(ctx, teams, &periodID)
	if err != nil {
		return TeamOverview{}, err
	}
	children := findDirectChildren(teamID, hierarchy)
	descendantIDs := collectDescendantIDs(teamID, hierarchy)
	goalsByTeam, err := s.store.ListGoalsByTeamsPeriod(ctx, periodID, descendantIDs)
	if err != nil {
		return TeamOverview{}, err
	}
	overviewStats, err := s.store.ListTeamOverviewStats(ctx, periodID, descendantIDs)
	if err != nil {
		return TeamOverview{}, err
	}
	summaryByID := make(map[int64]TeamSummary, len(descendantIDs))
	for _, id := range descendantIDs {
		goals := goalsByTeam[id]
		if len(goals) == 0 {
			continue
		}
		for i := range goals {
			goals[i].Progress = CalculateGoalProgress(&goals[i])
		}
		summaryByID[id] = TeamSummary{
			ID:             id,
			GoalsCount:     len(goals),
			PeriodProgress: okr.PeriodProgress(goals),
		}
	}
	childrenSummary := []TeamChildSummary{}
	if len(children) > 0 {
		childrenSummary, err = s.buildDirectChildrenSummary(ctx, periodID, children, summaryByID)
		if err != nil {
			return TeamOverview{}, err
		}
	}

	totalProgress := 0
	teamsWithGoals := 0
	priorities := TeamPrioritySummary{}
	workBalance := TeamWorkBalance{}

	for _, id := range descendantIDs {
		summary, ok := summaryByID[id]
		if !ok || summary.GoalsCount == 0 {
			continue
		}
		teamsWithGoals++
		totalProgress += summary.PeriodProgress
		if stat, exists := overviewStats[id]; exists {
			priorities.P0 += stat.PriorityP0
			priorities.P1 += stat.PriorityP1
			priorities.P2 += stat.PriorityP2
			priorities.P3 += stat.PriorityP3
			workBalance.Discovery += stat.Discovery
			workBalance.Delivery += stat.Delivery
		}
	}

	avgProgress := 0
	if teamsWithGoals > 0 {
		avgProgress = totalProgress / teamsWithGoals
	}

	return TeamOverview{
		AverageProgress: avgProgress,
		TeamsWithGoals:  teamsWithGoals,
		Priorities:      priorities,
		WorkBalance:     workBalance,
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

func (s *Service) UpdateKRProgressPercent(ctx context.Context, krID int64, current float64) error {
	kr, err := s.store.GetKeyResult(ctx, krID)
	if err != nil {
		return err
	}
	switch kr.Kind {
	case domain.KRKindPercent:
		return s.store.UpdatePercentCurrent(ctx, krID, current)
	case domain.KRKindLinear:
		return s.store.UpdateLinearCurrent(ctx, krID, current)
	default:
		return fmt.Errorf("unsupported kr kind for percent update: %s", kr.Kind)
	}
}

func (s *Service) UpdateKRProgressBoolean(ctx context.Context, krID int64, done bool) error {
	kr, err := s.store.GetKeyResult(ctx, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindBoolean {
		return fmt.Errorf("unsupported kr kind for boolean update: %s", kr.Kind)
	}
	return s.store.UpdateBoolean(ctx, krID, done)
}

type ProjectStageUpdate struct {
	ID     int64
	IsDone bool
}

func (s *Service) UpdateKRProgressProject(ctx context.Context, krID int64, updates []ProjectStageUpdate) error {
	kr, err := s.store.GetKeyResult(ctx, krID)
	if err != nil {
		return err
	}
	if kr.Kind != domain.KRKindProject {
		return fmt.Errorf("unsupported kr kind for project update: %s", kr.Kind)
	}
	stages, err := s.store.ListProjectStages(ctx, krID)
	if err != nil {
		return err
	}
	updatesByID := make(map[int64]ProjectStageUpdate, len(updates))
	for _, update := range updates {
		updatesByID[update.ID] = update
	}
	for _, stage := range stages {
		if update, ok := updatesByID[stage.ID]; ok {
			if err := s.store.UpdateProjectStageDone(ctx, stage.ID, update.IsDone); err != nil {
				return err
			}
		}
	}
	return nil
}

type ShareTarget struct {
	TeamID int64
	Weight int
}

func (s *Service) ShareGoal(ctx context.Context, goalID int64, targets []ShareTarget) error {
	shares := make([]store.GoalShareInput, 0, len(targets))
	for _, target := range targets {
		shares = append(shares, store.GoalShareInput{TeamID: target.TeamID, Weight: target.Weight})
	}
	return s.store.ReplaceGoalShares(ctx, goalID, shares)
}

func (s *Service) UpdateGoalWeight(ctx context.Context, goalID, teamID int64, weight int) error {
	return s.store.UpdateGoalTeamWeight(ctx, goalID, teamID, weight)
}

func (s *Service) AddGoalComment(ctx context.Context, goalID int64, text string) error {
	return s.store.AddGoalComment(ctx, goalID, text)
}

func (s *Service) AddKeyResultComment(ctx context.Context, krID int64, text string) error {
	return s.store.AddKeyResultComment(ctx, krID, text)
}

func (s *Service) GetGoal(ctx context.Context, id int64) (domain.Goal, error) {
	goal, err := s.store.GetGoal(ctx, id)
	if err != nil {
		return domain.Goal{}, err
	}
	goal.Progress = CalculateGoalProgress(&goal)
	return goal, nil
}

type KeyResultMetaInput struct {
	PercentStart   float64
	PercentTarget  float64
	PercentCurrent float64
	LinearStart    float64
	LinearTarget   float64
	LinearCurrent  float64
	BooleanDone    bool
	ProjectStages  []store.ProjectStageInput
}

func (s *Service) UpdateGoal(ctx context.Context, input store.GoalUpdateInput) error {
	return s.store.UpdateGoal(ctx, input)
}

func (s *Service) MoveGoal(ctx context.Context, goalID int64, direction int) error {
	return s.store.MoveGoal(ctx, goalID, direction)
}

func (s *Service) MoveKeyResult(ctx context.Context, krID int64, direction int) error {
	return s.store.MoveKeyResult(ctx, krID, direction)
}

func (s *Service) CreateKeyResultWithMeta(ctx context.Context, input store.KeyResultInput, meta KeyResultMetaInput) (int64, error) {
	krID, err := s.store.CreateKeyResult(ctx, input)
	if err != nil {
		return 0, err
	}
	if err := s.applyKeyResultMeta(ctx, krID, input.Kind, meta); err != nil {
		return 0, err
	}
	return krID, nil
}

func (s *Service) UpdateKeyResultWithMeta(ctx context.Context, input store.KeyResultUpdateInput, meta KeyResultMetaInput) error {
	if err := s.store.UpdateKeyResult(ctx, input); err != nil {
		return err
	}
	return s.applyKeyResultMeta(ctx, input.ID, input.Kind, meta)
}

func (s *Service) applyKeyResultMeta(ctx context.Context, krID int64, kind domain.KRKind, meta KeyResultMetaInput) error {
	switch kind {
	case domain.KRKindPercent:
		return s.store.UpsertPercentMeta(ctx, store.PercentMetaInput{
			KeyResultID:  krID,
			StartValue:   meta.PercentStart,
			TargetValue:  meta.PercentTarget,
			CurrentValue: meta.PercentCurrent,
		})
	case domain.KRKindLinear:
		return s.store.UpsertLinearMeta(ctx, store.LinearMetaInput{
			KeyResultID:  krID,
			StartValue:   meta.LinearStart,
			TargetValue:  meta.LinearTarget,
			CurrentValue: meta.LinearCurrent,
		})
	case domain.KRKindBoolean:
		return s.store.UpsertBooleanMeta(ctx, krID, meta.BooleanDone)
	case domain.KRKindProject:
		return s.store.ReplaceProjectStages(ctx, krID, meta.ProjectStages)
	default:
		return nil
	}
}

func (s *Service) UpdateTeamPeriodStatus(ctx context.Context, teamID, periodID int64, status domain.TeamPeriodStatus) error {
	return s.store.SetTeamPeriodStatus(ctx, teamID, periodID, status)
}

func (s *Service) DeleteTeam(ctx context.Context, teamID int64) error {
	hasGoals, err := s.store.TeamHasGoals(ctx, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return s.store.SoftDeleteTeam(ctx, teamID)
	}
	return s.store.HardDeleteTeam(ctx, teamID)
}

func (s *Service) RestoreTeam(ctx context.Context, teamID int64) error {
	return s.store.RestoreTeam(ctx, teamID)
}

func (s *Service) HardDeleteTeam(ctx context.Context, teamID int64) error {
	hasGoals, err := s.store.TeamHasGoals(ctx, teamID)
	if err != nil {
		return err
	}
	if hasGoals {
		return ErrTeamHasGoals
	}
	return s.store.HardDeleteTeam(ctx, teamID)
}

func (s *Service) appendTeamSummary(ctx context.Context, rows *[]TeamSummary, team domain.Team, level int, periodID int64, childrenMap map[int64][]domain.Team, teamsByID map[int64]domain.Team) error {
	visible, err := s.isTeamVisibleInPeriod(ctx, team, periodID)
	if err != nil {
		return err
	}
	if visible {
		goals, err := s.store.ListGoalsByTeamPeriod(ctx, team.ID, periodID)
		if err != nil {
			return err
		}
		status, err := s.store.GetTeamPeriodStatus(ctx, team.ID, periodID)
		if err != nil {
			return err
		}
		goalRows := make([]TeamGoalSummary, 0, len(goals))
		for i := range goals {
			goals[i].Progress = CalculateGoalProgress(&goals[i])
			shareTeams, err := s.listGoalShareTeams(ctx, goals[i], teamsByID)
			if err != nil {
				return err
			}
			goalRows = append(goalRows, TeamGoalSummary{
				ID:         goals[i].ID,
				Title:      goals[i].Title,
				Weight:     goals[i].Weight,
				Progress:   goals[i].Progress,
				ShareTeams: shareTeams,
				Priority:   string(goals[i].Priority),
				WorkType:   goals[i].WorkType,
			})
		}
		periodProgress := okr.PeriodProgress(goals)
		goalsWeight := 0
		for _, goal := range goals {
			goalsWeight += goal.Weight
		}
		*rows = append(*rows, TeamSummary{
			ID:             team.ID,
			Name:           team.Name,
			Type:           team.Type,
			Indent:         level * 24,
			Status:         status,
			PeriodProgress: periodProgress,
			GoalsCount:     len(goals),
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
		if err := s.appendTeamSummary(ctx, rows, child, nextLevel, periodID, childrenMap, teamsByID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) isTeamVisibleInPeriod(ctx context.Context, team domain.Team, periodID int64) (bool, error) {
	return s.isTeamVisibleInPeriodWithCurrent(ctx, team, periodID)
}

func (s *Service) isTeamVisibleInPeriodWithCurrent(ctx context.Context, team domain.Team, periodID int64) (bool, error) {
	hasGoals, err := s.store.TeamHasGoalsInPeriod(ctx, team.ID, periodID)
	if err != nil {
		return false, err
	}
	if hasGoals {
		return true, nil
	}
	return team.DeletedAt == nil, nil
}

func (s *Service) listGoalShareTeams(ctx context.Context, goal domain.Goal, teamsByID map[int64]domain.Team) ([]TeamShareInfo, error) {
	shares, err := s.store.ListGoalShares(ctx, goal.ID)
	if err != nil {
		return nil, err
	}
	teamIDs := make(map[int64]struct{}, len(shares)+1)
	teamIDs[goal.TeamID] = struct{}{}
	for _, share := range shares {
		teamIDs[share.TeamID] = struct{}{}
	}
	teams := make([]TeamShareInfo, 0, len(teamIDs))
	if teamsByID == nil {
		teamsByID = make(map[int64]domain.Team)
		allTeams, err := s.store.ListAllTeams(ctx)
		if err != nil {
			return nil, err
		}
		for _, team := range allTeams {
			teamsByID[team.ID] = team
		}
	}
	for teamID := range teamIDs {
		team, ok := teamsByID[teamID]
		if !ok {
			continue
		}
		weight := goal.Weight
		for _, share := range shares {
			if share.TeamID == teamID {
				weight = share.Weight
				break
			}
		}
		teams = append(teams, TeamShareInfo{ID: team.ID, Name: team.Name, Type: team.Type, Weight: weight})
	}
	sort.Slice(teams, func(i, j int) bool { return teams[i].Name < teams[j].Name })
	return teams, nil
}

func buildTeamHierarchy(teams []domain.Team) (map[int64]domain.Team, map[int64][]domain.Team, []domain.Team) {
	teamsByID := make(map[int64]domain.Team, len(teams))
	childrenMap := make(map[int64][]domain.Team)
	roots := make([]domain.Team, 0)
	for _, team := range teams {
		teamsByID[team.ID] = team
	}
	for _, team := range teams {
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

func (s *Service) ListTeams(ctx context.Context) ([]domain.Team, error) {
	return s.store.ListTeams(ctx)
}

func (s *Service) ListDeletedTeams(ctx context.Context) ([]domain.Team, error) {
	return s.store.ListDeletedTeams(ctx)
}

func (s *Service) ListAllTeams(ctx context.Context) ([]domain.Team, error) {
	return s.store.ListAllTeams(ctx)
}

func (s *Service) CreateTeam(ctx context.Context, input store.TeamInput) (int64, error) {
	return s.store.CreateTeam(ctx, input)
}

func (s *Service) UpdateTeam(ctx context.Context, input store.TeamInput, id int64) error {
	return s.store.UpdateTeam(ctx, input, id)
}

// — Period passthroughs —

func (s *Service) FindPeriodForDate(ctx context.Context, date time.Time) (domain.Period, error) {
	return s.store.FindPeriodForDate(ctx, date)
}

func (s *Service) CreatePeriod(ctx context.Context, input store.PeriodInput) (int64, error) {
	return s.store.CreatePeriod(ctx, input)
}

func (s *Service) UpdatePeriod(ctx context.Context, periodID int64, input store.PeriodInput) error {
	return s.store.UpdatePeriod(ctx, periodID, input)
}

func (s *Service) DeletePeriod(ctx context.Context, periodID int64) error {
	return s.store.DeletePeriod(ctx, periodID)
}

func (s *Service) MovePeriod(ctx context.Context, periodID int64, direction int) error {
	return s.store.MovePeriod(ctx, periodID, direction)
}

// — Goal passthroughs —

func (s *Service) ListGoalsByTeamPeriod(ctx context.Context, teamID, periodID int64) ([]domain.Goal, error) {
	return s.store.ListGoalsByTeamPeriod(ctx, teamID, periodID)
}

func (s *Service) UpdateGoalFields(ctx context.Context, input store.GoalFieldsUpdateInput) error {
	return s.store.UpdateGoalFields(ctx, input)
}

func (s *Service) ListGoalComments(ctx context.Context, goalID int64) ([]domain.GoalComment, error) {
	return s.store.ListGoalComments(ctx, goalID)
}

func (s *Service) GetTeamPeriodStatus(ctx context.Context, teamID, periodID int64) (domain.TeamPeriodStatus, error) {
	return s.store.GetTeamPeriodStatus(ctx, teamID, periodID)
}

func (s *Service) GetGoalShare(ctx context.Context, goalID, teamID int64) (store.GoalShare, error) {
	return s.store.GetGoalShare(ctx, goalID, teamID)
}

func (s *Service) DeleteGoalShare(ctx context.Context, goalID, teamID int64) error {
	return s.store.DeleteGoalShare(ctx, goalID, teamID)
}

func (s *Service) ListGoalShares(ctx context.Context, goalID int64) ([]store.GoalShare, error) {
	return s.store.ListGoalShares(ctx, goalID)
}

// — Key result passthroughs —

func (s *Service) DeleteKeyResult(ctx context.Context, id int64) error {
	return s.store.DeleteKeyResult(ctx, id)
}

func (s *Service) FindGoalIDByKR(ctx context.Context, krID int64) (int64, error) {
	return s.store.FindGoalIDByKR(ctx, krID)
}

func (s *Service) FindGoalIDByStage(ctx context.Context, stageID int64) (int64, error) {
	return s.store.FindGoalIDByStage(ctx, stageID)
}

// — Business logic —

// CreateGoal creates a goal and auto-advances status from NoGoals to Forming on first goal.
// Returns ErrPeriodClosed if the team's period status is Closed.
func (s *Service) CreateGoal(ctx context.Context, input store.GoalInput) (int64, error) {
	status, err := s.store.GetTeamPeriodStatus(ctx, input.TeamID, input.PeriodID)
	if err != nil {
		return 0, err
	}
	if status == domain.TeamPeriodStatusClosed {
		return 0, ErrPeriodClosed
	}
	goalID, err := s.store.CreateGoal(ctx, input)
	if err != nil {
		return 0, err
	}
	if status == domain.TeamPeriodStatusNoGoals {
		if err := s.store.SetTeamPeriodStatus(ctx, input.TeamID, input.PeriodID, domain.TeamPeriodStatusForming); err != nil {
			return 0, err
		}
	}
	return goalID, nil
}

// DeleteGoal removes a goal or a team's share of it, transferring ownership when the owner deletes.
// Returns the effective requesting teamID and the goal's periodID for redirect.
// Returns ErrPeriodClosed if the owner tries to delete in a closed period with no shares.
func (s *Service) DeleteGoal(ctx context.Context, goalID, requestingTeamID int64) (effectiveTeamID, periodID int64, err error) {
	goal, err := s.store.GetGoal(ctx, goalID)
	if err != nil {
		return 0, 0, err
	}
	if requestingTeamID == 0 {
		requestingTeamID = goal.TeamID
	}
	if requestingTeamID != goal.TeamID {
		return requestingTeamID, goal.PeriodID, s.store.DeleteGoalShare(ctx, goalID, requestingTeamID)
	}
	shares, err := s.store.ListGoalShares(ctx, goalID)
	if err != nil {
		return 0, 0, err
	}
	if len(shares) > 0 {
		newOwner := shares[0]
		if err := s.store.UpdateGoalOwner(ctx, goalID, newOwner.TeamID, newOwner.Weight); err != nil {
			return 0, 0, err
		}
		if err := s.store.DeleteGoalShare(ctx, goalID, newOwner.TeamID); err != nil {
			return 0, 0, err
		}
		return requestingTeamID, goal.PeriodID, nil
	}
	status, err := s.store.GetTeamPeriodStatus(ctx, goal.TeamID, goal.PeriodID)
	if err != nil {
		return 0, 0, err
	}
	if status == domain.TeamPeriodStatusClosed {
		return 0, 0, ErrPeriodClosed
	}
	return requestingTeamID, goal.PeriodID, s.store.DeleteGoal(ctx, goalID)
}

// UpdateGoalOwnerAndShares updates goal ownership and sharing based on the selected team set.
// Returns ErrCannotShareWithClosedPeriod if any selected team has a validated or closed period.
func (s *Service) UpdateGoalOwnerAndShares(ctx context.Context, goalID int64, selectedTeamIDs []int64) (ownerID, periodID int64, err error) {
	goal, err := s.store.GetGoal(ctx, goalID)
	if err != nil {
		return 0, 0, err
	}
	shares, err := s.store.ListGoalShares(ctx, goalID)
	if err != nil {
		return 0, 0, err
	}
	shareWeights := make(map[int64]int, len(shares))
	for _, share := range shares {
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
	newShares := make([]store.GoalShareInput, 0, len(selectedTeamIDs))
	for _, teamID := range selectedTeamIDs {
		status, err := s.store.GetTeamPeriodStatus(ctx, teamID, goal.PeriodID)
		if err != nil {
			return 0, 0, err
		}
		if status == domain.TeamPeriodStatusValidated || status == domain.TeamPeriodStatusClosed {
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
			if err := s.store.UpdateGoalOwner(ctx, goalID, ownerID, ownerWeight); err != nil {
				return 0, 0, err
			}
			continue
		}
		weight := 0
		if w, ok := shareWeights[teamID]; ok {
			weight = w
		}
		newShares = append(newShares, store.GoalShareInput{TeamID: teamID, Weight: weight})
	}
	if err := s.store.ReplaceGoalShares(ctx, goalID, newShares); err != nil {
		return 0, 0, err
	}
	return ownerID, goal.PeriodID, nil
}
