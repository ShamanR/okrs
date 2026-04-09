package v1

import (
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

type hierarchyResponse struct {
	Items []teamNode `json:"items"`
}

type teamNode struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	TypeLabel string     `json:"type_label"`
	Lead      string     `json:"lead"`
	HasGoals  bool       `json:"has_goals"`
	Progress  *int       `json:"progress,omitempty"`
	Children  []teamNode `json:"children"`
}

type periodsResponse struct {
	Items []periodInfo `json:"items"`
}

type periodInfo struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	SortOrder int       `json:"sort_order"`
}

type shareTeam struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	Weight    int    `json:"weight"`
}

type teamOKRResponse struct {
	Team           teamInfo        `json:"team"`
	Period         periodInfo      `json:"period"`
	PeriodStatus   string          `json:"period_status"`
	StatusLabel    string          `json:"status_label"`
	PeriodProgress int             `json:"period_progress"`
	GoalsCount     int             `json:"goals_count"`
	GoalsWeight    int             `json:"goals_weight"`
	ProgressMeta   progressBarInfo `json:"progress_meta"`
	Goals          []goalDetails   `json:"goals"`
}

type teamOverviewResponse struct {
	AverageProgress int                         `json:"average_progress"`
	TeamsWithGoals  int                         `json:"teams_with_goals"`
	ProgressMeta    progressBarInfo             `json:"progress_meta"`
	Priorities      prioritySummaryInfo         `json:"priorities"`
	WorkBalance     workBalanceInfo             `json:"work_balance"`
	ChildrenSummary teamChildrenSummaryResponse `json:"children_summary"`
}

type teamChildrenSummaryResponse struct {
	Period periodInfo               `json:"period"`
	Items  []teamChildSummaryResult `json:"items"`
}

type teamChildSummaryResult struct {
	Team         teamInfo         `json:"team"`
	Status       string           `json:"status"`
	StatusLabel  string           `json:"status_label"`
	HasGoals     bool             `json:"has_goals"`
	ProgressMeta *progressBarInfo `json:"progress_meta,omitempty"`
	LastUpdated  *time.Time       `json:"last_updated,omitempty"`
}

type prioritySummaryInfo struct {
	P0 int `json:"p0"`
	P1 int `json:"p1"`
	P2 int `json:"p2"`
	P3 int `json:"p3"`
}

type workBalanceInfo struct {
	Discovery int `json:"discovery"`
	Delivery  int `json:"delivery"`
}

type progressBarInfo struct {
	Actual   int    `json:"actual"`
	Forecast int    `json:"forecast"`
	Delta    int    `json:"delta"`
	Status   string `json:"status"`
}

type teamInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	TypeLabel string `json:"type_label"`
	ParentID  *int64 `json:"parent_id,omitempty"`
}

type goalDetails struct {
	ID           int64           `json:"id"`
	TeamID       int64           `json:"team_id"`
	PeriodID     int64           `json:"period_id"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Priority     string          `json:"priority"`
	Weight       int             `json:"weight"`
	WorkType     string          `json:"work_type"`
	FocusType    string          `json:"focus_type"`
	OwnerText    string          `json:"owner_text"`
	Progress     int             `json:"progress"`
	ProgressMeta progressBarInfo `json:"progress_meta"`
	KeyResults   []keyResult     `json:"key_results"`
	ShareTeams   []shareTeam     `json:"share_teams"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type keyResult struct {
	ID          int64       `json:"id"`
	GoalID      int64       `json:"goal_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Weight      int         `json:"weight"`
	Kind        string      `json:"kind"`
	Progress    int         `json:"progress"`
	Measure     measure     `json:"measure"`
	Comments    []krComment `json:"comments"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type krComment struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type goalComment struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type goalResponse struct {
	Goal     goalDetails   `json:"goal"`
	Comments []goalComment `json:"comments"`
}

type measure struct {
	Kind        string              `json:"kind"`
	Percent     *percentMeasure     `json:"percent,omitempty"`
	Linear      *linearMeasure      `json:"linear,omitempty"`
	Boolean     *booleanMeasure     `json:"boolean,omitempty"`
	Project     *projectMeasure     `json:"project,omitempty"`
	Checkpoints []percentCheckpoint `json:"checkpoints,omitempty"`
}

type percentMeasure struct {
	StartValue   float64 `json:"start_value"`
	TargetValue  float64 `json:"target_value"`
	CurrentValue float64 `json:"current_value"`
}

type linearMeasure struct {
	StartValue   float64 `json:"start_value"`
	TargetValue  float64 `json:"target_value"`
	CurrentValue float64 `json:"current_value"`
}

type booleanMeasure struct {
	IsDone bool `json:"is_done"`
}

type projectMeasure struct {
	Stages []projectStage `json:"stages"`
}

type projectStage struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Weight int    `json:"weight"`
	IsDone bool   `json:"is_done"`
}

type percentCheckpoint struct {
	ID          int64   `json:"id"`
	MetricValue float64 `json:"metric_value"`
	Percent     int     `json:"percent"`
}

func mapHierarchy(nodes []service.TeamNode) []teamNode {
	return mapHierarchyWithMetrics(nodes, nil)
}

func mapHierarchyWithMetrics(nodes []service.TeamNode, metrics map[int64]service.TeamSummary) []teamNode {
	result := make([]teamNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, mapTeamNode(node, metrics))
	}
	return result
}

func mapTeamNode(node service.TeamNode, metrics map[int64]service.TeamSummary) teamNode {
	children := make([]teamNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, mapTeamNode(child, metrics))
	}
	var progress *int
	hasGoals := false
	if summary, ok := metrics[node.Team.ID]; ok {
		hasGoals = summary.GoalsCount > 0
		if hasGoals {
			value := summary.PeriodProgress
			progress = &value
		}
	}
	return teamNode{
		ID:        node.Team.ID,
		Name:      node.Team.Name,
		Type:      string(node.Team.Type),
		TypeLabel: common.TeamTypeLabel(node.Team.Type),
		Lead:      node.Team.Lead,
		HasGoals:  hasGoals,
		Progress:  progress,
		Children:  children,
	}
}

func mapPeriodInfo(period domain.Period) periodInfo {
	return periodInfo{
		ID:        period.ID,
		Name:      period.Name,
		StartDate: period.StartDate,
		EndDate:   period.EndDate,
		SortOrder: period.SortOrder,
	}
}

func mapTeamOKRResponse(data service.TeamOKR) teamOKRResponse {
	goals := make([]goalDetails, 0, len(data.Goals))
	for _, goal := range data.Goals {
		goals = append(goals, mapGoalDetails(goal, data.Period))
	}
	return teamOKRResponse{
		Team: teamInfo{
			ID:        data.Team.ID,
			Name:      data.Team.Name,
			Type:      string(data.Team.Type),
			TypeLabel: common.TeamTypeLabel(data.Team.Type),
			ParentID:  data.Team.ParentID,
		},
		Period:         mapPeriodInfo(data.Period),
		PeriodStatus:   string(data.PeriodStatus),
		StatusLabel:    common.TeamPeriodStatusLabel(data.PeriodStatus),
		PeriodProgress: data.PeriodProgress,
		GoalsCount:     data.GoalsCount,
		GoalsWeight:    data.GoalsWeight,
		ProgressMeta:   buildProgressBarInfo(data.PeriodProgress, data.Period),
		Goals:          goals,
	}
}

func mapTeamOverviewResponse(period domain.Period, overview service.TeamOverview) teamOverviewResponse {
	return teamOverviewResponse{
		AverageProgress: overview.AverageProgress,
		TeamsWithGoals:  overview.TeamsWithGoals,
		ProgressMeta:    buildProgressBarInfo(overview.AverageProgress, period),
		Priorities: prioritySummaryInfo{
			P0: overview.Priorities.P0,
			P1: overview.Priorities.P1,
			P2: overview.Priorities.P2,
			P3: overview.Priorities.P3,
		},
		WorkBalance: workBalanceInfo{
			Discovery: overview.WorkBalance.Discovery,
			Delivery:  overview.WorkBalance.Delivery,
		},
		ChildrenSummary: mapTeamChildrenSummaryResponse(period, overview.ChildrenSummary),
	}
}

func mapTeamChildrenSummaryResponse(period domain.Period, items []service.TeamChildSummary) teamChildrenSummaryResponse {
	rows := make([]teamChildSummaryResult, 0, len(items))
	for _, item := range items {
		var progressMeta *progressBarInfo
		if item.HasGoals {
			meta := buildProgressBarInfo(item.Progress, period)
			progressMeta = &meta
		}
		rows = append(rows, teamChildSummaryResult{
			Team: teamInfo{
				ID:        item.Team.ID,
				Name:      item.Team.Name,
				Type:      string(item.Team.Type),
				TypeLabel: common.TeamTypeLabel(item.Team.Type),
				ParentID:  item.Team.ParentID,
			},
			Status:       string(item.Status),
			StatusLabel:  common.TeamPeriodStatusLabel(item.Status),
			HasGoals:     item.HasGoals,
			ProgressMeta: progressMeta,
			LastUpdated:  item.LastUpdateAt,
		})
	}
	return teamChildrenSummaryResponse{
		Period: mapPeriodInfo(period),
		Items:  rows,
	}
}

func buildProgressBarInfo(actual int, period domain.Period) progressBarInfo {
	forecast := calculatePeriodForecast(period, time.Now())
	delta := actual - forecast
	status := "on_track"
	if delta > 10 {
		status = "above"
	} else if delta < -10 {
		status = "below"
	}
	return progressBarInfo{
		Actual:   actual,
		Forecast: forecast,
		Delta:    delta,
		Status:   status,
	}
}

func calculatePeriodForecast(period domain.Period, now time.Time) int {
	if period.EndDate.Before(period.StartDate) {
		return 0
	}
	if now.Before(period.StartDate) {
		return 0
	}
	if now.After(period.EndDate) {
		return 100
	}
	duration := period.EndDate.Sub(period.StartDate)
	if duration <= 0 {
		return 100
	}
	elapsed := now.Sub(period.StartDate)
	value := int((elapsed * 100) / duration)
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func mapGoalDetails(detail service.GoalDetails, period domain.Period) goalDetails {
	krList := make([]keyResult, 0, len(detail.Goal.KeyResults))
	for _, kr := range detail.Goal.KeyResults {
		krList = append(krList, mapKeyResult(kr))
	}
	shareTeams := make([]shareTeam, 0, len(detail.ShareTeams))
	for _, share := range detail.ShareTeams {
		shareTeams = append(shareTeams, shareTeam{
			ID:        share.ID,
			Name:      share.Name,
			Type:      string(share.Type),
			TypeLabel: common.TeamTypeLabel(share.Type),
			Weight:    share.Weight,
		})
	}
	goal := detail.Goal
	return goalDetails{
		ID:           goal.ID,
		TeamID:       goal.TeamID,
		PeriodID:     goal.PeriodID,
		Title:        goal.Title,
		Description:  goal.Description,
		Priority:     string(goal.Priority),
		Weight:       goal.Weight,
		WorkType:     string(goal.WorkType),
		FocusType:    string(goal.FocusType),
		OwnerText:    goal.OwnerText,
		Progress:     goal.Progress,
		ProgressMeta: buildProgressBarInfo(goal.Progress, period),
		KeyResults:   krList,
		ShareTeams:   shareTeams,
		CreatedAt:    goal.CreatedAt,
		UpdatedAt:    goal.UpdatedAt,
	}
}

func mapGoalResponse(goal domain.Goal) goalResponse {
	comments := make([]goalComment, 0, len(goal.Comments))
	for _, comment := range goal.Comments {
		comments = append(comments, goalComment{ID: comment.ID, Text: comment.Text, CreatedAt: comment.CreatedAt})
	}
	krList := make([]keyResult, 0, len(goal.KeyResults))
	for _, kr := range goal.KeyResults {
		krList = append(krList, mapKeyResult(kr))
	}
	goalDetail := goalDetails{
		ID:          goal.ID,
		TeamID:      goal.TeamID,
		PeriodID:    goal.PeriodID,
		Title:       goal.Title,
		Description: goal.Description,
		Priority:    string(goal.Priority),
		Weight:      goal.Weight,
		WorkType:    string(goal.WorkType),
		FocusType:   string(goal.FocusType),
		OwnerText:   goal.OwnerText,
		Progress:    goal.Progress,
		KeyResults:  krList,
		CreatedAt:   goal.CreatedAt,
		UpdatedAt:   goal.UpdatedAt,
	}
	return goalResponse{Goal: goalDetail, Comments: comments}
}

func mapKeyResult(kr domain.KeyResult) keyResult {
	comments := make([]krComment, 0, len(kr.Comments))
	for _, comment := range kr.Comments {
		comments = append(comments, krComment{ID: comment.ID, Text: comment.Text, CreatedAt: comment.CreatedAt})
	}
	return keyResult{
		ID:          kr.ID,
		GoalID:      kr.GoalID,
		Title:       kr.Title,
		Description: kr.Description,
		Weight:      kr.Weight,
		Kind:        string(kr.Kind),
		Progress:    kr.Progress,
		Measure:     buildMeasure(kr),
		Comments:    comments,
		CreatedAt:   kr.CreatedAt,
		UpdatedAt:   kr.UpdatedAt,
	}
}

func buildMeasure(kr domain.KeyResult) measure {
	switch kr.Kind {
	case domain.KRKindPercent:
		if kr.Percent == nil {
			return measure{Kind: string(kr.Kind)}
		}
		checkpoints := make([]percentCheckpoint, 0, len(kr.Percent.Checkpoints))
		for _, cp := range kr.Percent.Checkpoints {
			checkpoints = append(checkpoints, percentCheckpoint{
				ID:          cp.ID,
				MetricValue: cp.MetricValue,
				Percent:     cp.KRPercent,
			})
		}
		return measure{
			Kind:        string(kr.Kind),
			Percent:     &percentMeasure{StartValue: kr.Percent.StartValue, TargetValue: kr.Percent.TargetValue, CurrentValue: kr.Percent.CurrentValue},
			Checkpoints: checkpoints,
		}
	case domain.KRKindLinear:
		if kr.Linear == nil {
			return measure{Kind: string(kr.Kind)}
		}
		return measure{Kind: string(kr.Kind), Linear: &linearMeasure{StartValue: kr.Linear.StartValue, TargetValue: kr.Linear.TargetValue, CurrentValue: kr.Linear.CurrentValue}}
	case domain.KRKindBoolean:
		if kr.Boolean == nil {
			return measure{Kind: string(kr.Kind)}
		}
		return measure{Kind: string(kr.Kind), Boolean: &booleanMeasure{IsDone: kr.Boolean.IsDone}}
	case domain.KRKindProject:
		if kr.Project == nil {
			return measure{Kind: string(kr.Kind)}
		}
		stages := make([]projectStage, 0, len(kr.Project.Stages))
		for _, stage := range kr.Project.Stages {
			stages = append(stages, projectStage{ID: stage.ID, Title: stage.Title, Weight: stage.Weight, IsDone: stage.IsDone})
		}
		return measure{Kind: string(kr.Kind), Project: &projectMeasure{Stages: stages}}
	default:
		return measure{Kind: string(kr.Kind)}
	}
}
