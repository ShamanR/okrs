package progress

import (
	"math"
	"sort"

	"okrs/internal/core/domain"
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
// With checkpoints it linearly interpolates between points: start (0%), each
// checkpoint, and target (100%); outside the range it clamps to the nearest
// endpoint's percent.
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

	pts := make([]point, 0, len(checkpoints)+2)
	pts = append(pts, point{Value: start, Percent: 0})
	for _, cp := range checkpoints {
		pts = append(pts, point{Value: cp.Value, Percent: cp.ProgressPercent})
	}
	pts = append(pts, point{Value: target, Percent: 100})
	sort.Slice(pts, func(i, j int) bool { return pts[i].Value < pts[j].Value })

	if current <= pts[0].Value {
		return clampPercent(float64(pts[0].Percent))
	}
	last := pts[len(pts)-1]
	if current >= last.Value {
		return clampPercent(float64(last.Percent))
	}
	for i := 0; i < len(pts)-1; i++ {
		left, right := pts[i], pts[i+1]
		if current >= left.Value && current <= right.Value {
			return clampPercent(interpolate(left, right, current))
		}
	}
	return 0
}

type point struct {
	Value   float64
	Percent int
}

func interpolate(left, right point, current float64) float64 {
	if right.Value == left.Value {
		return float64(left.Percent)
	}
	pos := (current - left.Value) / (right.Value - left.Value)
	return float64(left.Percent) + pos*float64(right.Percent-left.Percent)
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
