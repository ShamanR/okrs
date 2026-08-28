package period

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/service/servicetest"
)

func TestComputeBulkAffected_SkipsNoGoalsAndAlreadyTarget(t *testing.T) {
	deleted := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	teams := []domain.Team{
		{ID: 1, Name: "HasGoals-Ready"},
		{ID: 2, Name: "HasGoals-AlreadyInProgress"},
		{ID: 3, Name: "NoGoals"},
		{ID: 4, Name: "Deleted", DeletedAt: &deleted},
	}
	goalsByTeam := map[int64][]domain.Goal{
		1: {{ID: 10, TeamID: 1, Weight: 100}},
		2: {{ID: 20, TeamID: 2, Weight: 100}},
		4: {{ID: 40, TeamID: 4, Weight: 100}},
	}
	statuses := map[int64]domain.TeamPeriodStatus{
		1: domain.TeamPeriodStatusReady,
		2: domain.TeamPeriodStatusInProgress,
	}
	affected, skipped := computeBulkAffected(teams, goalsByTeam, statuses, domain.TeamPeriodStatusInProgress)

	if len(affected) != 1 || affected[0] != 1 {
		t.Fatalf("affected: want [1], got %v", affected)
	}
	if skipped != 1 { // only team 3 (no goals); team 2 already target, team 4 deleted
		t.Fatalf("skipped: want 1, got %d", skipped)
	}
}

func TestBulkSetTeamPeriodStatus_ActivatesAndLogsPerTeam(t *testing.T) {
	store := servicetest.NewStore()
	store.Teams = []domain.Team{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}
	store.GoalsByTeam[1] = map[int64][]domain.Goal{9: {{ID: 10}}} // ready -> affected
	store.GoalsByTeam[2] = map[int64][]domain.Goal{9: {{ID: 20}}} // already in_progress -> skip (no change)
	// team 3: no goals -> skipped
	store.Statuses[[2]int64{1, 9}] = domain.TeamPeriodStatusReady
	store.Statuses[[2]int64{2, 9}] = domain.TeamPeriodStatusInProgress
	bus := &servicetest.FakeBus{}
	svc := newTestUC(store, bus)

	res, err := svc.BulkSetTeamPeriodStatus(context.Background(), domain.TenantScope{TenantID: 1}, 9, domain.TeamPeriodStatusInProgress, 42, nil)
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if res.Affected != 1 || res.Skipped != 1 {
		t.Fatalf("result: want affected=1 skipped=1, got %+v", res)
	}
	if len(store.BulkSetTeamIDs) != 1 || store.BulkSetTeamIDs[0] != 1 || store.BulkSetStatus != domain.TeamPeriodStatusInProgress {
		t.Fatalf("set call wrong: ids=%v status=%s", store.BulkSetTeamIDs, store.BulkSetStatus)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("expected one event per affected team (one PublishBatch call), got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.StatusChanged)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.TeamTitle != "A" || !ev.Bulk {
		t.Fatalf("event wrong: want team A with bulk=true, got %+v", ev)
	}
}

func TestBulkSetTeamPeriodStatus_TeamFilterRestrictsToScope(t *testing.T) {
	store := servicetest.NewStore()
	store.Teams = []domain.Team{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	store.GoalsByTeam[1] = map[int64][]domain.Goal{9: {{ID: 10}}}
	store.GoalsByTeam[2] = map[int64][]domain.Goal{9: {{ID: 20}}}
	store.Statuses[[2]int64{1, 9}] = domain.TeamPeriodStatusReady
	store.Statuses[[2]int64{2, 9}] = domain.TeamPeriodStatusReady
	bus := &servicetest.FakeBus{}
	svc := newTestUC(store, bus)

	// Only team 1 is in scope: team 2 must be untouched even though it would qualify.
	res, err := svc.BulkSetTeamPeriodStatus(context.Background(), domain.TenantScope{TenantID: 1}, 9, domain.TeamPeriodStatusInProgress, 42, map[int64]bool{1: true})
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if res.Affected != 1 {
		t.Fatalf("want affected=1 (scoped to team 1), got %+v", res)
	}
	if len(store.BulkSetTeamIDs) != 1 || store.BulkSetTeamIDs[0] != 1 {
		t.Fatalf("out-of-scope team must not be changed: ids=%v", store.BulkSetTeamIDs)
	}
}
