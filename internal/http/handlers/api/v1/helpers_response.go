package v1

import (
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
)

func MapPeriodInfo(period domain.Period) dto.PeriodInfo {
	return dto.PeriodInfo{
		ID:        period.ID,
		Name:      period.Name,
		StartDate: period.StartDate,
		EndDate:   period.EndDate,
		SortOrder: period.SortOrder,
	}
}

func BuildProgressBarInfo(actual int, period domain.Period) dto.ProgressBarInfo {
	forecast := CalculatePeriodForecast(period, time.Now())
	delta := actual - forecast
	status := "on_track"
	if delta > 10 {
		status = "above"
	} else if delta < -10 {
		status = "below"
	}
	return dto.ProgressBarInfo{Actual: actual, Forecast: forecast, Delta: delta, Status: status}
}

func CalculatePeriodForecast(period domain.Period, now time.Time) int {
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

func MapGoalDetails(detail service.GoalDetails, period domain.Period) dto.GoalDetails {
	krList := make([]dto.KeyResult, 0, len(detail.Goal.KeyResults))
	for _, kr := range detail.Goal.KeyResults {
		krList = append(krList, MapKeyResult(kr))
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
	comments := make([]dto.GoalComment, 0, len(detail.Goal.Comments))
	for _, c := range detail.Goal.Comments {
		comments = append(comments, dto.GoalComment{ID: c.ID, Text: c.Text, AuthorName: c.AuthorName, CreatedAt: c.CreatedAt})
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
		ProgressMeta: BuildProgressBarInfo(goal.Progress, period),
		KeyResults:   krList,
		ShareTeams:   shareTeams,
		Comments:     comments,
		CreatedAt:    goal.CreatedAt,
		UpdatedAt:    goal.UpdatedAt,
	}
}

func MapKeyResult(kr domain.KeyResult) dto.KeyResult {
	comments := make([]dto.KRComment, 0, len(kr.Comments))
	for _, comment := range kr.Comments {
		comments = append(comments, dto.KRComment{ID: comment.ID, Text: comment.Text, AuthorName: comment.AuthorName, CreatedAt: comment.CreatedAt})
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
			checkpoints = append(checkpoints, dto.PercentCheckpoint{ID: cp.ID, MetricValue: cp.MetricValue, Percent: cp.KRPercent})
		}
		return dto.Measure{Kind: string(kr.Kind), Percent: &dto.PercentMeasure{StartValue: kr.Percent.StartValue, TargetValue: kr.Percent.TargetValue, CurrentValue: kr.Percent.CurrentValue}, Checkpoints: checkpoints}
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
