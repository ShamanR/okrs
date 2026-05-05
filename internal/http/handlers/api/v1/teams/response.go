package teams

import (
	"okrs/internal/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

func newTeamOKRResponse(data service.TeamOKR, userRefs map[string]*dto.UserRef) dto.TeamOKRResponse {
	goals := make([]dto.GoalDetails, 0, len(data.Goals))
	for _, goal := range data.Goals {
		goals = append(goals, v1.MapGoalDetails(goal, data.Period, userRefs))
	}
	return dto.TeamOKRResponse{
		Team:            dto.TeamInfo{ID: data.Team.ID, Name: data.Team.Name, Type: string(data.Team.Type), TypeLabel: common.TeamTypeLabel(data.Team.Type), Lead: v1.ResolveUserRef(data.Team.Lead, userRefs), ParentID: data.Team.ParentID},
		Period:          v1.MapPeriodInfo(data.Period),
		PeriodStatus:    string(data.PeriodStatus),
		StatusLabel:     common.TeamPeriodStatusLabel(data.PeriodStatus),
		StatusChangedAt: data.StatusChangedAt,
		PeriodProgress:  data.PeriodProgress,
		GoalsCount:      data.GoalsCount,
		GoalsWeight:     data.GoalsWeight,
		ProgressMeta:    v1.BuildProgressBarInfo(data.PeriodProgress, data.Period),
		Goals:           goals,
	}
}

func newTeamOverviewResponse(period domain.Period, overview service.TeamOverview, userRefs map[string]*dto.UserRef) dto.TeamOverviewResponse {
	return dto.TeamOverviewResponse{
		AverageProgress: overview.AverageProgress,
		TeamsWithGoals:  overview.TeamsWithGoals,
		ProgressMeta:    v1.BuildProgressBarInfo(overview.AverageProgress, period),
		ChildrenSummary: mapTeamChildrenSummaryResponse(period, overview.ChildrenSummary, userRefs),
	}
}

func mapTeamChildrenSummaryResponse(period domain.Period, items []service.TeamChildSummary, userRefs map[string]*dto.UserRef) dto.TeamChildrenSummaryResponse {
	rows := make([]dto.TeamChildSummaryResult, 0, len(items))
	for _, item := range items {
		var progressMeta *dto.ProgressBarInfo
		if item.HasGoals {
			meta := v1.BuildProgressBarInfo(item.Progress, period)
			progressMeta = &meta
		}
		rows = append(rows, dto.TeamChildSummaryResult{
			Team:              dto.TeamInfo{ID: item.Team.ID, Name: item.Team.Name, Type: string(item.Team.Type), TypeLabel: common.TeamTypeLabel(item.Team.Type), Lead: v1.ResolveUserRef(item.Team.Lead, userRefs), ParentID: item.Team.ParentID},
			Status:            string(item.Status),
			StatusLabel:       common.TeamPeriodStatusLabel(item.Status),
			HasGoals:          item.HasGoals,
			GoalsCount:        item.GoalsCount,
			HighPriorityCount: item.HighPriorityCount,
			ProgressMeta:      progressMeta,
			LastUpdated:       item.LastUpdateAt,
		})
	}
	return dto.TeamChildrenSummaryResponse{Period: v1.MapPeriodInfo(period), Items: rows}
}
