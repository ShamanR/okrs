package periods_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/periods"
	"okrs/internal/store/testutil"
)

// sc1 is the default-tenant scope used across the existing single-tenant period tests.
var sc1 = domain.TenantScope{TenantID: 1}

func TestPeriodsCRUD(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := periods.NewPeriodRepository(pool)

	id, err := r.CreatePeriod(ctx, sc1, periods.PeriodInput{
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

	p, err := r.GetPeriod(ctx, sc1, id)
	if err != nil {
		t.Fatalf("GetPeriod: %v", err)
	}
	if p.Name != "2025 Q1" {
		t.Fatalf("expected name 2025 Q1, got %s", p.Name)
	}

	if err := r.UpdatePeriod(ctx, sc1, id, periods.PeriodInput{
		Name:      "2025 Q1 updated",
		StartDate: p.StartDate,
		EndDate:   p.EndDate,
	}); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}
	p2, _ := r.GetPeriod(ctx, sc1, id)
	if p2.Name != "2025 Q1 updated" {
		t.Fatalf("expected updated name, got %s", p2.Name)
	}

	list, err := r.ListPeriods(ctx, sc1)
	if err != nil {
		t.Fatalf("ListPeriods: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one period")
	}

	if err := r.ArchivePeriod(ctx, sc1, id); err != nil {
		t.Fatalf("ArchivePeriod: %v", err)
	}
	pa, _ := r.GetPeriod(ctx, sc1, id)
	if pa.ArchivedAt == nil {
		t.Fatal("expected archived_at set")
	}
	if err := r.UnarchivePeriod(ctx, sc1, id); err != nil {
		t.Fatalf("UnarchivePeriod: %v", err)
	}
	pu, _ := r.GetPeriod(ctx, sc1, id)
	if pu.ArchivedAt != nil {
		t.Fatal("expected archived_at cleared")
	}

	if err := r.DeletePeriod(ctx, sc1, id); err != nil {
		t.Fatalf("DeletePeriod: %v", err)
	}
	if _, err := r.GetPeriod(ctx, sc1, id); err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestFindPeriodForDate(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := periods.NewPeriodRepository(pool)

	id, err := r.CreatePeriod(ctx, sc1, periods.PeriodInput{
		Name:      "2025 Q2",
		StartDate: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	p, err := r.FindPeriodForDate(ctx, sc1, time.Date(2025, 5, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FindPeriodForDate inside: %v", err)
	}
	if p.ID != id {
		t.Fatalf("expected period %d, got %d", id, p.ID)
	}

	_, err = r.FindPeriodForDate(ctx, sc1, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for date outside all periods")
	}
}

// TestFindPeriodForDate_ReturnsNarrowest verifies that when multiple periods
// contain the probe date (e.g. a year and a quarter nested within it),
// FindPeriodForDate returns the narrowest (shortest duration) containing
// period rather than the widest one.
func TestFindPeriodForDate_ReturnsNarrowest(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := periods.NewPeriodRepository(pool)

	yearID, err := r.CreatePeriod(ctx, sc1, periods.PeriodInput{
		Name:      "2025",
		StartDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePeriod year: %v", err)
	}

	quarterID, err := r.CreatePeriod(ctx, sc1, periods.PeriodInput{
		Name:      "2025 Q3",
		StartDate: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 9, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePeriod quarter: %v", err)
	}

	// A date inside both the year and the quarter must resolve to the
	// narrower quarter period.
	p, err := r.FindPeriodForDate(ctx, sc1, time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FindPeriodForDate inside both: %v", err)
	}
	if p.ID != quarterID {
		t.Fatalf("expected narrowest period %d (quarter), got %d", quarterID, p.ID)
	}

	// A date inside only the year must resolve to the year period.
	p2, err := r.FindPeriodForDate(ctx, sc1, time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FindPeriodForDate inside year only: %v", err)
	}
	if p2.ID != yearID {
		t.Fatalf("expected year period %d, got %d", yearID, p2.ID)
	}
}
