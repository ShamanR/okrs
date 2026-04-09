package teams

import (
	"okrs/internal/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

func newTeamOKRResponse(data service.TeamOKR) dto.TeamOKRResponse {
	goals := make([]dto.GoalDetails, 0, len(data.Goals))
	for _, goal := range data.Goals {
		goals = append(goals, v1.MapGoalDetails(goal, data.Period))
	}
	return dto.TeamOKRResponse{
		Team:           dto.TeamInfo{ID: data.Team.ID, Name: data.Team.Name, Type: string(data.Team.Type), TypeLabel: common.TeamTypeLabel(data.Team.Type), ParentID: data.Team.ParentID},
		Period:         v1.MapPeriodInfo(data.Period),
		PeriodStatus:   string(data.PeriodStatus),
		StatusLabel:    common.TeamPeriodStatusLabel(data.PeriodStatus),
		PeriodProgress: data.PeriodProgress,
		GoalsCount:     data.GoalsCount,
		GoalsWeight:    data.GoalsWeight,
		ProgressMeta:   v1.BuildProgressBarInfo(data.PeriodProgress, data.Period),
		Goals:          goals,
	}
}

func newTeamOverviewResponse(period domain.Period, overview service.TeamOverview) dto.TeamOverviewResponse {
	return dto.TeamOverviewResponse{
		AverageProgress: overview.AverageProgress,
		TeamsWithGoals:  overview.TeamsWithGoals,
		ProgressMeta:    v1.BuildProgressBarInfo(overview.AverageProgress, period),
		Priorities:      dto.PrioritySummaryInfo{P0: overview.Priorities.P0, P1: overview.Priorities.P1, P2: overview.Priorities.P2, P3: overview.Priorities.P3},
		WorkBalance:     dto.WorkBalanceInfo{Discovery: overview.WorkBalance.Discovery, Delivery: overview.WorkBalance.Delivery},
		ChildrenSummary: mapTeamChildrenSummaryResponse(period, overview.ChildrenSummary),
	}
}

func mapTeamChildrenSummaryResponse(period domain.Period, items []service.TeamChildSummary) dto.TeamChildrenSummaryResponse {
	rows := make([]dto.TeamChildSummaryResult, 0, len(items))
	for _, item := range items {
		var progressMeta *dto.ProgressBarInfo
		if item.HasGoals {
			meta := v1.BuildProgressBarInfo(item.Progress, period)
			progressMeta = &meta
		}
		rows = append(rows, dto.TeamChildSummaryResult{
			Team:         dto.TeamInfo{ID: item.Team.ID, Name: item.Team.Name, Type: string(item.Team.Type), TypeLabel: common.TeamTypeLabel(item.Team.Type), ParentID: item.Team.ParentID},
			Status:       string(item.Status),
			StatusLabel:  common.TeamPeriodStatusLabel(item.Status),
			HasGoals:     item.HasGoals,
			ProgressMeta: progressMeta,
			LastUpdated:  item.LastUpdateAt,
		})
	}
	return dto.TeamChildrenSummaryResponse{Period: v1.MapPeriodInfo(period), Items: rows}
}
