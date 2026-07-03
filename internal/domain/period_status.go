package domain

import "time"

// PeriodStatus — жизненный статус периода. future/active/closed выводятся из дат,
// archived выставляется вручную (Period.ArchivedAt).
type PeriodStatus string

const (
	PeriodStatusFuture   PeriodStatus = "future"
	PeriodStatusActive   PeriodStatus = "active"
	PeriodStatusClosed   PeriodStatus = "closed"
	PeriodStatusArchived PeriodStatus = "archived"
)

// PeriodStatusFor возвращает статус периода относительно now.
// Границы включительны: now == start и now == end → active.
func PeriodStatusFor(p Period, now time.Time) PeriodStatus {
	if p.ArchivedAt != nil {
		return PeriodStatusArchived
	}
	day := now.Truncate(24 * time.Hour)
	start := p.StartDate.Truncate(24 * time.Hour)
	end := p.EndDate.Truncate(24 * time.Hour)
	switch {
	case day.Before(start):
		return PeriodStatusFuture
	case day.After(end):
		return PeriodStatusClosed
	default:
		return PeriodStatusActive
	}
}
