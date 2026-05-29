package periods_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/store/periods"
	"okrs/internal/store/testutil"
)

func TestPeriodsCRUD(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := periods.NewPeriodRepository(pool)

	id, err := r.CreatePeriod(ctx, periods.PeriodInput{
		Name:      "2025 Q1",
		StartDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	p, err := r.GetPeriod(ctx, id)
	if err != nil {
		t.Fatalf("GetPeriod: %v", err)
	}
	if p.Name != "2025 Q1" {
		t.Fatalf("expected name 2025 Q1, got %s", p.Name)
	}

	if err := r.UpdatePeriod(ctx, id, periods.PeriodInput{
		Name:      "2025 Q1 updated",
		StartDate: p.StartDate,
		EndDate:   p.EndDate,
	}); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}
	p2, _ := r.GetPeriod(ctx, id)
	if p2.Name != "2025 Q1 updated" {
		t.Fatalf("expected updated name, got %s", p2.Name)
	}

	list, err := r.ListPeriods(ctx)
	if err != nil {
		t.Fatalf("ListPeriods: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one period")
	}

	if err := r.DeletePeriod(ctx, id); err != nil {
		t.Fatalf("DeletePeriod: %v", err)
	}
	if _, err := r.GetPeriod(ctx, id); err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestFindPeriodForDate(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := periods.NewPeriodRepository(pool)

	id, err := r.CreatePeriod(ctx, periods.PeriodInput{
		Name:      "2025 Q2",
		StartDate: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	p, err := r.FindPeriodForDate(ctx, time.Date(2025, 5, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FindPeriodForDate inside: %v", err)
	}
	if p.ID != id {
		t.Fatalf("expected period %d, got %d", id, p.ID)
	}

	_, err = r.FindPeriodForDate(ctx, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for date outside all periods")
	}
}

func TestMovePeriodSwapsSortOrder(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := periods.NewPeriodRepository(pool)

	id1, _ := r.CreatePeriod(ctx, periods.PeriodInput{
		Name:      "P1",
		StartDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	id2, _ := r.CreatePeriod(ctx, periods.PeriodInput{
		Name:      "P2",
		StartDate: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
	})

	before, _ := r.ListPeriods(ctx)
	var idx1, idx2 int
	for i, p := range before {
		if p.ID == id1 {
			idx1 = i
		}
		if p.ID == id2 {
			idx2 = i
		}
	}
	if idx1 >= idx2 {
		t.Fatalf("expected P1 before P2 initially, idx1=%d idx2=%d", idx1, idx2)
	}

	// Move P1 down (direction > 0 means toward higher sort_order).
	if err := r.MovePeriod(ctx, id1, 1); err != nil {
		t.Fatalf("MovePeriod: %v", err)
	}

	after, _ := r.ListPeriods(ctx)
	var aidx1, aidx2 int
	for i, p := range after {
		if p.ID == id1 {
			aidx1 = i
		}
		if p.ID == id2 {
			aidx2 = i
		}
	}
	if aidx1 <= aidx2 {
		t.Fatalf("expected P1 after P2 after move, aidx1=%d aidx2=%d", aidx1, aidx2)
	}
}
