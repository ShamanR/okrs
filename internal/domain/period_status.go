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
//
// Сравнение ведётся по календарным датам. Календарный день now берётся в его
// собственной зоне (now.Date()) — а не через Truncate(24h), который округляет
// абсолютный момент к UTC-границе и на нескольких первых часах локальных суток в
// не-UTC зонах давал бы неверный день (напр. период, стартующий сегодня, читался
// бы как future). Даты периода приходят из DATE-колонок (UTC-полночь), их
// календарный день корректен в любой зоне.
func PeriodStatusFor(p Period, now time.Time) PeriodStatus {
	if p.ArchivedAt != nil {
		return PeriodStatusArchived
	}
	day := dateOnly(now)
	start := dateOnly(p.StartDate)
	end := dateOnly(p.EndDate)
	switch {
	case day.Before(start):
		return PeriodStatusFuture
	case day.After(end):
		return PeriodStatusClosed
	default:
		return PeriodStatusActive
	}
}

// dateOnly сводит t к его календарной дате (год/месяц/день в зоне t),
// нормализованной к UTC-полуночи для устойчивого сравнения.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
