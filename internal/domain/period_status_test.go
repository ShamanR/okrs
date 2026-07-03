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
