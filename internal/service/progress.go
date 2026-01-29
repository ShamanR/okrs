package service

import (
	"okrs/internal/domain"
	"okrs/internal/okr"
)

func CalculateGoalProgress(goal *domain.Goal) int {
	for i := range goal.KeyResults {
		goal.KeyResults[i].Progress = CalculateKRProgress(goal.KeyResults[i])
	}
	return okr.GoalProgress(goal.KeyResults)
}

func CalculateKRProgress(kr domain.KeyResult) int {
	switch kr.Kind {
	case domain.KRKindProject:
		if kr.Project == nil {
			return 0
		}
		return okr.ProjectProgress(kr.Project.Stages)
	case domain.KRKindPercent:
		if kr.Percent == nil {
			return 0
		}
		return okr.PercentProgress(kr.Percent.StartValue, kr.Percent.TargetValue, kr.Percent.CurrentValue, kr.Percent.Checkpoints)
	case domain.KRKindLinear:
		if kr.Linear == nil {
			return 0
		}
		return okr.LinearProgress(kr.Linear.StartValue, kr.Linear.TargetValue, kr.Linear.CurrentValue)
	case domain.KRKindBoolean:
		if kr.Boolean == nil {
			return 0
		}
		return okr.BooleanProgress(kr.Boolean.IsDone)
	default:
		return 0
	}
}
