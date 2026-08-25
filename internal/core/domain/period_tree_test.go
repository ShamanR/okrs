package domain

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func ids(vs []PeriodView) []int64 {
	out := make([]int64, len(vs))
	for i, v := range vs {
		out[i] = v.ID
	}
	return out
}

func TestBuildPeriodViews_ParentAndDepth(t *testing.T) {
	now := d(2026, time.July, 3)
	ps := []Period{
		{ID: 1, Name: "Y2026", StartDate: d(2026, time.January, 1), EndDate: d(2026, time.December, 31)},
		{ID: 2, Name: "Q1", StartDate: d(2026, time.January, 1), EndDate: d(2026, time.March, 31)},
		{ID: 3, Name: "Q3", StartDate: d(2026, time.July, 1), EndDate: d(2026, time.September, 30)},
	}
	views := BuildPeriodViews(ps, now)
	byID := map[int64]PeriodView{}
	for _, v := range views {
		byID[v.ID] = v
	}
	if byID[1].ParentID != nil {
		t.Fatalf("Y2026 must be root")
	}
	if byID[2].ParentID == nil || *byID[2].ParentID != 1 || byID[2].Depth != 1 {
		t.Fatalf("Q1 parent must be Y2026 at depth 1, got parent=%v depth=%d", byID[2].ParentID, byID[2].Depth)
	}
	if byID[3].Status != PeriodStatusActive {
		t.Fatalf("Q3 must be active, got %s", byID[3].Status)
	}
}

func TestBuildPeriodViews_EqualRangesAreSiblings(t *testing.T) {
	now := d(2026, time.July, 3)
	ps := []Period{
		{ID: 1, StartDate: d(2026, time.January, 1), EndDate: d(2026, time.December, 31)},
		{ID: 2, StartDate: d(2026, time.January, 1), EndDate: d(2026, time.December, 31)},
	}
	views := BuildPeriodViews(ps, now)
	for _, v := range views {
		if v.ParentID != nil {
			t.Fatalf("identical ranges must be siblings, period %d got parent %v", v.ID, v.ParentID)
		}
	}
}

func TestBuildPeriodViews_Order(t *testing.T) {
	now := d(2026, time.July, 3)
	ps := []Period{
		{ID: 10, Name: "Y2025", StartDate: d(2025, time.January, 1), EndDate: d(2025, time.December, 31)},
		{ID: 11, Name: "Y2025-Q3", StartDate: d(2025, time.July, 1), EndDate: d(2025, time.September, 30)},
		{ID: 12, Name: "Y2025-Q4", StartDate: d(2025, time.October, 1), EndDate: d(2025, time.December, 31)},
		{ID: 1, Name: "Y2026", StartDate: d(2026, time.January, 1), EndDate: d(2026, time.December, 31)},
		{ID: 2, Name: "Y2026-Q1", StartDate: d(2026, time.January, 1), EndDate: d(2026, time.March, 31)},
		{ID: 3, Name: "Y2026-Q3", StartDate: d(2026, time.July, 1), EndDate: d(2026, time.September, 30)},
	}
	got := ids(BuildPeriodViews(ps, now))
	// active year 2026 first (children ascending), then closed year 2025 (children descending).
	want := []int64{1, 2, 3, 10, 12, 11}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order mismatch: got %v want %v", got, want)
		}
	}
}
