package progress

import "okrs/internal/core/domain"

// ForGoal fills in each KR's progress and returns the goal's aggregate. It mutates
// goal.KeyResults[i].Progress, so callers working on cached data must copy first.
func ForGoal(goal *domain.Goal) int {
	for i := range goal.KeyResults {
		goal.KeyResults[i].Progress = ForKR(goal.KeyResults[i])
	}
	return GoalProgress(goal.KeyResults)
}

// ForKR dispatches to the per-kind calculation. A KR whose meta is missing scores 0.
func ForKR(kr domain.KeyResult) int {
	switch kr.Kind {
	case domain.KRKindProject:
		if kr.Project == nil {
			return 0
		}
		return ProjectProgress(kr.Project.Stages)
	case domain.KRKindNumerical:
		if kr.Numerical == nil {
			return 0
		}
		return NumericalProgress(kr.Numerical.StartValue, kr.Numerical.TargetValue, kr.Numerical.CurrentValue, kr.Numerical.Checkpoints)
	case domain.KRKindBoolean:
		if kr.Boolean == nil {
			return 0
		}
		return BooleanProgress(kr.Boolean.IsDone)
	default:
		return 0
	}
}
