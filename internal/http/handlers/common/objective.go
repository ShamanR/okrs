package common

import (
	"math"
	"time"

	"okrs/internal/domain"
)

type ObjectiveStatus struct {
	Label      string
	BadgeClass string
}

func QuarterRange(year, quarter int, zone *time.Location) (time.Time, time.Time) {
	if quarter < 1 || quarter > 4 || year <= 0 {
		return time.Time{}, time.Time{}
	}
	month := time.Month((quarter-1)*3 + 1)
	start := time.Date(year, month, 1, 0, 0, 0, 0, zone)
	end := start.AddDate(0, 3, 0)
	return start, end
}

func ComputeObjectiveStatus(goal domain.Goal, now time.Time, zone *time.Location) ObjectiveStatus {
	if len(goal.KeyResults) == 0 {
		return ObjectiveStatus{Label: "Нет данных", BadgeClass: "text-bg-secondary"}
	}

	start, end := QuarterRange(goal.Year, goal.Quarter, zone)
	if start.IsZero() || end.IsZero() {
		return ObjectiveStatus{Label: "Нет данных", BadgeClass: "text-bg-secondary"}
	}

	planned := plannedProgress(now, start, end)
	delta := planned - float64(goal.Progress)

	switch {
	case delta <= 10:
		return ObjectiveStatus{Label: "В норме", BadgeClass: "text-bg-success"}
	case delta <= 25:
		return ObjectiveStatus{Label: "Риск", BadgeClass: "text-bg-warning"}
	default:
		return ObjectiveStatus{Label: "Отставание", BadgeClass: "text-bg-danger"}
	}
}

func plannedProgress(now, start, end time.Time) float64 {
	if now.Before(start) {
		return 0
	}
	if !now.Before(end) {
		return 100
	}
	total := end.Sub(start).Hours()
	if total <= 0 {
		return 0
	}
	elapsed := now.Sub(start).Hours()
	percent := (elapsed / total) * 100
	return math.Max(0, math.Min(100, percent))
}
