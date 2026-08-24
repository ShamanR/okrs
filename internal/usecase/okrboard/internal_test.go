package okrboard

// Внутренний тест: buildDirectChildrenSummary приватный, из внешнего пакета
// okrboard_test к нему не дотянуться.

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	goalsvc "okrs/internal/service/goal"
	goalsharesvc "okrs/internal/service/goalshare"
	periodsvc "okrs/internal/service/period"
	"okrs/internal/service/servicetest"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
)

func newBoardInternal(st *servicetest.Store) *UseCase {
	return New(Deps{
		Teams:    teamsvc.New(st),
		Goals:    goalsvc.New(st),
		Shares:   goalsharesvc.New(st),
		Statuses: teamstatussvc.New(st),
		Periods:  periodsvc.New(st),
	})
}

func TestBuildDirectChildrenSummaryWithoutSummaryMap(t *testing.T) {
	store := servicetest.NewStore()
	store.Teams = []domain.Team{
		{ID: 1, Name: "Parent"},
		{ID: 2, Name: "Child", ParentID: ptr(1)},
	}
	store.Statuses[[2]int64{2, 11}] = domain.TeamPeriodStatusClosed
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	store.GoalsByTeam[2] = map[int64][]domain.Goal{
		11: {{
			ID:        300,
			TeamID:    2,
			PeriodID:  11,
			Title:     "Child goal",
			UpdatedAt: now,
		}},
	}
	svc := newBoardInternal(store)
	children := []teamsvc.Node{{Team: domain.Team{ID: 2, Name: "Child", ParentID: ptr(1)}}}

	rows, err := svc.buildDirectChildrenSummary(context.Background(), domain.TenantScope{TenantID: 1}, 11, children, nil)
	if err != nil {
		t.Fatalf("build summary: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !rows[0].HasGoals {
		t.Fatalf("expected has_goals=true")
	}
	if rows[0].Status != domain.TeamPeriodStatusClosed {
		t.Fatalf("expected status closed, got %s", rows[0].Status)
	}
	if rows[0].LastUpdateAt == nil {
		t.Fatalf("expected last_updated to be set")
	}
}

func ptr(id int64) *int64 { return &id }
