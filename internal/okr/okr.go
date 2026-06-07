package okr

import (
	"math"
	"sort"

	"okrs/internal/domain"
)

func GoalProgress(keyResults []domain.KeyResult) int {
	if len(keyResults) == 0 {
		return 0
	}
	var sumWeight int
	var weighted float64
	for _, kr := range keyResults {
		sumWeight += kr.Weight
		weighted += float64(kr.Progress * kr.Weight)
	}
	if sumWeight == 0 {
		return 0
	}
	return int(math.Round(weighted / float64(sumWeight)))
}

func PeriodProgress(goals []domain.Goal) int {
	if len(goals) == 0 {
		return 0
	}
	var sumWeight int
	var weighted float64
	for _, goal := range goals {
		sumWeight += goal.Weight
		weighted += float64(goal.Progress * goal.Weight)
	}
	if sumWeight == 0 {
		return 0
	}
	return int(math.Round(weighted / float64(sumWeight)))
}

func ProjectProgress(stages []domain.KRProjectStage) int {
	var total int
	for _, stage := range stages {
		if stage.IsDone {
			total += stage.Weight
		}
	}
	if total < 0 {
		return 0
	}
	if total > 100 {
		return 100
	}
	return total
}

func BooleanProgress(done bool) int {
	if done {
		return 100
	}
	return 0
}

// NumericalProgress computes 0..100 progress for a numerical KR.
// With no checkpoints it is linear from start to target (either direction).
// With checkpoints it is a step function: the percent of the last reached step.
func NumericalProgress(start, target, current float64, checkpoints []domain.KRNumericalCheckpoint) int {
	if len(checkpoints) == 0 {
		if start == target {
			if current >= target {
				return 100
			}
			return 0
		}
		return clampPercent(linearPercent(start, target, current))
	}

	pts := make([]domain.KRNumericalCheckpoint, len(checkpoints))
	copy(pts, checkpoints)
	sort.Slice(pts, func(i, j int) bool { return pts[i].Value < pts[j].Value })

	if current < pts[0].Value {
		return clampPercent(float64(pts[0].ProgressPercent))
	}
	result := pts[0].ProgressPercent
	for _, p := range pts {
		if current >= p.Value {
			result = p.ProgressPercent
		} else {
			break
		}
	}
	return clampPercent(float64(result))
}

func linearPercent(start, target, current float64) float64 {
	return ((current - start) / (target - start)) * 100
}

func clampPercent(value float64) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return int(math.Round(value))
}
