package v1

import (
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

func mapHierarchy(nodes []service.TeamNode) []dto.TeamNode {
	return mapHierarchyWithMetrics(nodes, nil)
}

func NewHierarchyResponse(nodes []service.TeamNode, metrics map[int64]service.TeamSummary) any {
	return dto.HierarchyResponse{Items: mapHierarchyWithMetrics(nodes, metrics)}
}

func mapHierarchyWithMetrics(nodes []service.TeamNode, metrics map[int64]service.TeamSummary) []dto.TeamNode {
	result := make([]dto.TeamNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, mapTeamNode(node, metrics))
	}
	return result
}

func mapTeamNode(node service.TeamNode, metrics map[int64]service.TeamSummary) dto.TeamNode {
	children := make([]dto.TeamNode, 0, len(node.Children))
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
	return dto.TeamNode{
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

func mapPeriodInfo(period domain.Period) dto.PeriodInfo {
	return dto.PeriodInfo{
		ID:        period.ID,
		Name:      period.Name,
		StartDate: period.StartDate,
		EndDate:   period.EndDate,
		SortOrder: period.SortOrder,
	}
}

func NewPeriodsResponse(periods []domain.Period) any {
	items := make([]dto.PeriodInfo, 0, len(periods))
	for _, period := range periods {
		items = append(items, mapPeriodInfo(period))
	}
	return dto.PeriodsResponse{Items: items}
}

func mapTeamOKRResponse(data service.TeamOKR) dto.TeamOKRResponse {
	goals := make([]dto.GoalDetails, 0, len(data.Goals))
	for _, goal := range data.Goals {
		goals = append(goals, mapGoalDetails(goal, data.Period))
	}
	return dto.TeamOKRResponse{
		Team: dto.TeamInfo{
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

func NewTeamOKRResponse(data service.TeamOKR) any {
	return mapTeamOKRResponse(data)
}

func mapTeamOverviewResponse(period domain.Period, overview service.TeamOverview) dto.TeamOverviewResponse {
	return dto.TeamOverviewResponse{
		AverageProgress: overview.AverageProgress,
		TeamsWithGoals:  overview.TeamsWithGoals,
		ProgressMeta:    buildProgressBarInfo(overview.AverageProgress, period),
		Priorities: dto.PrioritySummaryInfo{
			P0: overview.Priorities.P0,
			P1: overview.Priorities.P1,
			P2: overview.Priorities.P2,
			P3: overview.Priorities.P3,
		},
		WorkBalance: dto.WorkBalanceInfo{
			Discovery: overview.WorkBalance.Discovery,
			Delivery:  overview.WorkBalance.Delivery,
		},
		ChildrenSummary: mapTeamChildrenSummaryResponse(period, overview.ChildrenSummary),
	}
}

func NewTeamOverviewResponse(period domain.Period, overview service.TeamOverview) any {
	return mapTeamOverviewResponse(period, overview)
}

func mapTeamChildrenSummaryResponse(period domain.Period, items []service.TeamChildSummary) dto.TeamChildrenSummaryResponse {
	rows := make([]dto.TeamChildSummaryResult, 0, len(items))
	for _, item := range items {
		var progressMeta *dto.ProgressBarInfo
		if item.HasGoals {
			meta := buildProgressBarInfo(item.Progress, period)
			progressMeta = &meta
		}
		rows = append(rows, dto.TeamChildSummaryResult{
			Team: dto.TeamInfo{
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
	return dto.TeamChildrenSummaryResponse{
		Period: mapPeriodInfo(period),
		Items:  rows,
	}
}

func buildProgressBarInfo(actual int, period domain.Period) dto.ProgressBarInfo {
	forecast := calculatePeriodForecast(period, time.Now())
	delta := actual - forecast
	status := "on_track"
	if delta > 10 {
		status = "above"
	} else if delta < -10 {
		status = "below"
	}
	return dto.ProgressBarInfo{
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

func mapGoalDetails(detail service.GoalDetails, period domain.Period) dto.GoalDetails {
	krList := make([]dto.KeyResult, 0, len(detail.Goal.KeyResults))
	for _, kr := range detail.Goal.KeyResults {
		krList = append(krList, mapKeyResult(kr))
	}
	shareTeams := make([]dto.ShareTeam, 0, len(detail.ShareTeams))
	for _, share := range detail.ShareTeams {
		shareTeams = append(shareTeams, dto.ShareTeam{
			ID:        share.ID,
			Name:      share.Name,
			Type:      string(share.Type),
			TypeLabel: common.TeamTypeLabel(share.Type),
			Weight:    share.Weight,
		})
	}
	goal := detail.Goal
	return dto.GoalDetails{
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

func mapGoalResponse(goal domain.Goal) dto.GoalResponse {
	comments := make([]dto.GoalComment, 0, len(goal.Comments))
	for _, comment := range goal.Comments {
		comments = append(comments, dto.GoalComment{ID: comment.ID, Text: comment.Text, CreatedAt: comment.CreatedAt})
	}
	krList := make([]dto.KeyResult, 0, len(goal.KeyResults))
	for _, kr := range goal.KeyResults {
		krList = append(krList, mapKeyResult(kr))
	}
	goalDetail := dto.GoalDetails{
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
	return dto.GoalResponse{Goal: goalDetail, Comments: comments}
}

func NewGoalResponse(goal domain.Goal) any {
	return mapGoalResponse(goal)
}

func mapKeyResult(kr domain.KeyResult) dto.KeyResult {
	comments := make([]dto.KRComment, 0, len(kr.Comments))
	for _, comment := range kr.Comments {
		comments = append(comments, dto.KRComment{ID: comment.ID, Text: comment.Text, CreatedAt: comment.CreatedAt})
	}
	return dto.KeyResult{
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

func buildMeasure(kr domain.KeyResult) dto.Measure {
	switch kr.Kind {
	case domain.KRKindPercent:
		if kr.Percent == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		checkpoints := make([]dto.PercentCheckpoint, 0, len(kr.Percent.Checkpoints))
		for _, cp := range kr.Percent.Checkpoints {
			checkpoints = append(checkpoints, dto.PercentCheckpoint{
				ID:          cp.ID,
				MetricValue: cp.MetricValue,
				Percent:     cp.KRPercent,
			})
		}
		return dto.Measure{
			Kind:        string(kr.Kind),
			Percent:     &dto.PercentMeasure{StartValue: kr.Percent.StartValue, TargetValue: kr.Percent.TargetValue, CurrentValue: kr.Percent.CurrentValue},
			Checkpoints: checkpoints,
		}
	case domain.KRKindLinear:
		if kr.Linear == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		return dto.Measure{Kind: string(kr.Kind), Linear: &dto.LinearMeasure{StartValue: kr.Linear.StartValue, TargetValue: kr.Linear.TargetValue, CurrentValue: kr.Linear.CurrentValue}}
	case domain.KRKindBoolean:
		if kr.Boolean == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		return dto.Measure{Kind: string(kr.Kind), Boolean: &dto.BooleanMeasure{IsDone: kr.Boolean.IsDone}}
	case domain.KRKindProject:
		if kr.Project == nil {
			return dto.Measure{Kind: string(kr.Kind)}
		}
		stages := make([]dto.ProjectStage, 0, len(kr.Project.Stages))
		for _, stage := range kr.Project.Stages {
			stages = append(stages, dto.ProjectStage{ID: stage.ID, Title: stage.Title, Weight: stage.Weight, IsDone: stage.IsDone})
		}
		return dto.Measure{Kind: string(kr.Kind), Project: &dto.ProjectMeasure{Stages: stages}}
	default:
		return dto.Measure{Kind: string(kr.Kind)}
	}
}
