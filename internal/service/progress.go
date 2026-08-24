package service

import (
	"okrs/internal/core/domain"
	"okrs/internal/core/progress"
)

func CalculateGoalProgress(goal *domain.Goal) int {
	for i := range goal.KeyResults {
		goal.KeyResults[i].Progress = CalculateKRProgress(goal.KeyResults[i])
	}
	return progress.GoalProgress(goal.KeyResults)
}

func CalculateKRProgress(kr domain.KeyResult) int {
	switch kr.Kind {
	case domain.KRKindProject:
		if kr.Project == nil {
			return 0
		}
		return progress.ProjectProgress(kr.Project.Stages)
	case domain.KRKindNumerical:
		if kr.Numerical == nil {
			return 0
		}
		return progress.NumericalProgress(kr.Numerical.StartValue, kr.Numerical.TargetValue, kr.Numerical.CurrentValue, kr.Numerical.Checkpoints)
	case domain.KRKindBoolean:
		if kr.Boolean == nil {
			return 0
		}
		return progress.BooleanProgress(kr.Boolean.IsDone)
	default:
		return 0
	}
}
