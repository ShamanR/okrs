package service

import (
	"context"
	"fmt"
	"sort"

	"okrs/internal/domain"
	"okrs/internal/okr"
	"okrs/internal/store"
)

type Store interface {
	ListTeams(ctx context.Context) ([]domain.Team, error)
	GetTeam(ctx context.Context, id int64) (domain.Team, error)
	ListGoalsByTeamQuarter(ctx context.Context, teamID int64, year, quarter int) ([]domain.Goal, error)
	ListGoalShares(ctx context.Context, goalID int64) ([]store.GoalShare, error)
	GetTeamQuarterStatus(ctx context.Context, teamID int64, year, quarter int) (domain.TeamQuarterStatus, error)
	UpdatePercentCurrent(ctx context.Context, krID int64, current float64) error
	UpdateLinearCurrent(ctx context.Context, krID int64, current float64) error
	UpdateBoolean(ctx context.Context, krID int64, done bool) error
	ListProjectStages(ctx context.Context, krID int64) ([]domain.KRProjectStage, error)
	UpdateProjectStageDone(ctx context.Context, stageID int64, done bool) error
	ReplaceGoalShares(ctx context.Context, goalID int64, shares []store.GoalShareInput) error
	UpdateGoalTeamWeight(ctx context.Context, goalID, teamID int64, weight int) error
	GetKeyResult(ctx context.Context, id int64) (domain.KeyResult, error)
	AddGoalComment(ctx context.Context, goalID int64, text string) error
	AddKeyResultComment(ctx context.Context, krID int64, text string) error
	GetGoal(ctx context.Context, id int64) (domain.Goal, error)
	UpdateGoal(ctx context.Context, input store.GoalUpdateInput) error
	CreateKeyResult(ctx context.Context, input store.KeyResultInput) (int64, error)
	UpdateKeyResult(ctx context.Context, input store.KeyResultUpdateInput) error
	UpsertPercentMeta(ctx context.Context, input store.PercentMetaInput) error
	UpsertLinearMeta(ctx context.Context, input store.LinearMetaInput) error
	UpsertBooleanMeta(ctx context.Context, krID int64, done bool) error
	ReplaceProjectStages(ctx context.Context, krID int64, stages []store.ProjectStageInput) error
	SetTeamQuarterStatus(ctx context.Context, teamID int64, year, quarter int, status domain.TeamQuarterStatus) error
}

type Service struct {
	store Store
}

func New(store Store) *Service {
	return &Service{store: store}
}

type TeamNode struct {
	Team     domain.Team
	Children []TeamNode
}

type TeamSummary struct {
	ID              int64
	Name            string
	Type            domain.TeamType
	Indent          int
	Status          domain.TeamQuarterStatus
	QuarterProgress int
	GoalsCount      int
	GoalsWeight     int
	Goals           []TeamGoalSummary
}

type TeamGoalSummary struct {
	ID         int64
	Title      string
	Weight     int
	Progress   int
	ShareTeams []TeamShareInfo
}

type TeamShareInfo struct {
	ID     int64
	Name   string
	Type   domain.TeamType
	Weight int
}

type TeamOKR struct {
	Team            domain.Team
	Year            int
	Quarter         int
	QuarterStatus   domain.TeamQuarterStatus
	QuarterProgress int
	GoalsCount      int
	GoalsWeight     int
	Goals           []GoalDetails
}

type GoalDetails struct {
	Goal       domain.Goal
	ShareTeams []TeamShareInfo
}

func (s *Service) GetHierarchy(ctx context.Context) ([]TeamNode, error) {
	teams, err := s.store.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	_, childrenMap, roots := buildTeamHierarchy(teams)
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

func (s *Service) GetTeamsWithQuarterSummary(ctx context.Context, year, quarter int, orgID *int64) ([]TeamSummary, error) {
	teams, err := s.store.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	teamsByID, childrenMap, roots := buildTeamHierarchy(teams)
	filteredRoots := roots
	if orgID != nil {
		if team, ok := teamsByID[*orgID]; ok {
			filteredRoots = []domain.Team{team}
		}
	}
	rows := make([]TeamSummary, 0, len(teams))
	for _, team := range filteredRoots {
		if err := s.appendTeamSummary(ctx, &rows, team, 0, year, quarter, childrenMap, teamsByID); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (s *Service) GetTeamOKR(ctx context.Context, teamID int64, year, quarter int) (TeamOKR, error) {
	team, err := s.store.GetTeam(ctx, teamID)
	if err != nil {
		return TeamOKR{}, err
	}
	goals, err := s.store.ListGoalsByTeamQuarter(ctx, teamID, year, quarter)
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
	quarterProgress := okr.QuarterProgress(goals)
	goalsWeight := 0
	for _, goal := range goals {
		goalsWeight += goal.Weight
	}
	status, err := s.store.GetTeamQuarterStatus(ctx, teamID, year, quarter)
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
		Team:            team,
		Year:            year,
		Quarter:         quarter,
		QuarterStatus:   status,
		QuarterProgress: quarterProgress,
		GoalsCount:      len(goals),
		GoalsWeight:     goalsWeight,
		Goals:           goalDetails,
	}, nil
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

func (s *Service) UpdateTeamQuarterStatus(ctx context.Context, teamID int64, year, quarter int, status domain.TeamQuarterStatus) error {
	return s.store.SetTeamQuarterStatus(ctx, teamID, year, quarter, status)
}

func (s *Service) appendTeamSummary(ctx context.Context, rows *[]TeamSummary, team domain.Team, level int, year, quarter int, childrenMap map[int64][]domain.Team, teamsByID map[int64]domain.Team) error {
	goals, err := s.store.ListGoalsByTeamQuarter(ctx, team.ID, year, quarter)
	if err != nil {
		return err
	}
	status, err := s.store.GetTeamQuarterStatus(ctx, team.ID, year, quarter)
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
		})
	}
	quarterProgress := okr.QuarterProgress(goals)
	goalsWeight := 0
	for _, goal := range goals {
		goalsWeight += goal.Weight
	}
	*rows = append(*rows, TeamSummary{
		ID:              team.ID,
		Name:            team.Name,
		Type:            team.Type,
		Indent:          level * 24,
		Status:          status,
		QuarterProgress: quarterProgress,
		GoalsCount:      len(goals),
		GoalsWeight:     goalsWeight,
		Goals:           goalRows,
	})
	children := childrenMap[team.ID]
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	for _, child := range children {
		if err := s.appendTeamSummary(ctx, rows, child, level+1, year, quarter, childrenMap, teamsByID); err != nil {
			return err
		}
	}
	return nil
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
		allTeams, err := s.store.ListTeams(ctx)
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
			childrenMap[*team.ParentID] = append(childrenMap[*team.ParentID], team)
		} else {
			roots = append(roots, team)
		}
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
