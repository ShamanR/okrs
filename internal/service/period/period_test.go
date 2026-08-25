package period_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/service/period"
	"okrs/internal/service/servicetest"
)

func TestListPeriodViews_ExcludesArchivedForPublic(t *testing.T) {
	now := time.Now()
	arch := now.AddDate(0, 0, -2)
	store := servicetest.NewStore()
	store.Periods = []domain.Period{
		{ID: 1, StartDate: now.AddDate(0, 0, -30), EndDate: now.AddDate(0, 0, -1)},                    // closed
		{ID: 2, StartDate: now.AddDate(0, 0, -30), EndDate: now.AddDate(0, 0, -1), ArchivedAt: &arch}, // archived
	}
	svc := period.New(store)

	pub, err := svc.ListViews(context.Background(), domain.TenantScope{TenantID: 1}, false)
	if err != nil {
		t.Fatalf("list period views (public): %v", err)
	}
	if len(pub) != 1 || pub[0].ID != 1 {
		t.Fatalf("public must exclude archived, got %+v", pub)
	}

	all, err := svc.ListViews(context.Background(), domain.TenantScope{TenantID: 1}, true)
	if err != nil {
		t.Fatalf("list period views (admin): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("admin must include archived, got %d", len(all))
	}
}

func TestArchivePeriod_RejectsNonClosed(t *testing.T) {
	now := time.Now()
	store := servicetest.NewStore()
	store.Periods = []domain.Period{{
		ID:        1,
		StartDate: now.AddDate(0, 0, -1),
		EndDate:   now.AddDate(0, 0, 10), // active
	}}
	svc := period.New(store)

	err := svc.Archive(context.Background(), domain.TenantScope{TenantID: 1}, 1)
	if !errors.Is(err, domain.ErrPeriodNotClosed) {
		t.Fatalf("expected domain.ErrPeriodNotClosed, got %v", err)
	}
}

func TestArchivePeriod_AllowsClosed(t *testing.T) {
	now := time.Now()
	store := servicetest.NewStore()
	store.Periods = []domain.Period{{
		ID:        1,
		StartDate: now.AddDate(0, 0, -30),
		EndDate:   now.AddDate(0, 0, -1), // closed
	}}
	svc := period.New(store)

	if err := svc.Archive(context.Background(), domain.TenantScope{TenantID: 1}, 1); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
