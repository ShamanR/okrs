package period

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/service/servicetest"
)

func TestUpdateStatusRecordsEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.Teams = []domain.Team{{ID: 10, Name: "PaaS / Infra"}}
	s := newTestUC(st, bus)
	if err := s.UpdateTeamStatus(context.Background(), domain.TenantScope{TenantID: 1}, 10, 3, domain.TeamPeriodStatusInProgress, 5); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.StatusChanged)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.TeamTitle != "PaaS / Infra" || ev.Bulk {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.After != domain.TeamPeriodStatusInProgress {
		t.Fatalf("after status wrong: %+v", ev)
	}
}
