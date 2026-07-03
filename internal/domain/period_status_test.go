package domain

import (
	"testing"
	"time"
)

func TestPeriodStatusFor(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	}
	now := d(2026, time.July, 3)
	archived := d(2026, time.July, 1)

	cases := []struct {
		name string
		p    Period
		want PeriodStatus
	}{
		{"future", Period{StartDate: d(2026, time.October, 1), EndDate: d(2026, time.December, 31)}, PeriodStatusFuture},
		{"active", Period{StartDate: d(2026, time.July, 1), EndDate: d(2026, time.September, 30)}, PeriodStatusActive},
		{"closed", Period{StartDate: d(2026, time.January, 1), EndDate: d(2026, time.March, 31)}, PeriodStatusClosed},
		{"boundary_start_is_active", Period{StartDate: now, EndDate: d(2026, time.August, 1)}, PeriodStatusActive},
		{"boundary_end_is_active", Period{StartDate: d(2026, time.January, 1), EndDate: now}, PeriodStatusActive},
		{"archived_overrides", Period{StartDate: d(2026, time.January, 1), EndDate: d(2026, time.March, 31), ArchivedAt: &archived}, PeriodStatusArchived},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PeriodStatusFor(c.p, now); got != c.want {
				t.Fatalf("PeriodStatusFor(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestPeriodStatusFor_LocalCalendarDayBoundary проверяет, что статус считается по
// календарному дню в зоне now, а не по UTC-округлению абсолютного момента: в +07 в
// 00:30 период, стартующий сегодня, должен быть active, а не future.
func TestPeriodStatusFor_LocalCalendarDayBoundary(t *testing.T) {
	loc := time.FixedZone("UTC+7", 7*60*60)
	// 03 июля 00:30 по локальной зоне (= 02 июля 17:30 UTC).
	now := time.Date(2026, time.July, 3, 0, 30, 0, 0, loc)

	start := time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC) // период стартует сегодня
	end := time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)
	if got := PeriodStatusFor(Period{StartDate: start, EndDate: end}, now); got != PeriodStatusActive {
		t.Fatalf("period starting today must be active at 00:30 local, got %q", got)
	}

	// Период, закончившийся вчера (02 июля), в те же локальные сутки → closed.
	endedYesterday := time.Date(2026, time.July, 2, 0, 0, 0, 0, time.UTC)
	if got := PeriodStatusFor(Period{StartDate: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC), EndDate: endedYesterday}, now); got != PeriodStatusClosed {
		t.Fatalf("period ended yesterday must be closed, got %q", got)
	}
}
