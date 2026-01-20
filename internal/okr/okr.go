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

func QuarterProgress(goals []domain.Goal) int {
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

func PercentProgress(start, target, current float64, checkpoints []domain.KRPercentCheckpoint) int {
	if start == target {
		return 0
	}
	if len(checkpoints) == 0 {
		return clampPercent(linearPercent(start, target, current))
	}

	points := make([]point, 0, len(checkpoints)+2)
	points = append(points, point{Value: start, Percent: 0})
	for _, cp := range checkpoints {
		points = append(points, point{Value: cp.MetricValue, Percent: cp.KRPercent})
	}
	points = append(points, point{Value: target, Percent: 100})

	sort.Slice(points, func(i, j int) bool { return points[i].Value < points[j].Value })

	if current <= points[0].Value {
		return 0
	}
	last := points[len(points)-1]
	if current >= last.Value {
		return 100
	}

	for i := 0; i < len(points)-1; i++ {
		left := points[i]
		right := points[i+1]
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

func linearPercent(start, target, current float64) float64 {
	return ((current - start) / (target - start)) * 100
}

func interpolate(left, right point, current float64) float64 {
	if right.Value == left.Value {
		return float64(left.Percent)
	}
	position := (current - left.Value) / (right.Value - left.Value)
	return float64(left.Percent) + position*float64(right.Percent-left.Percent)
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
