package period

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
)

func TestUpdateStatusRecordsEvent(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	st := servicetest.NewStore()
	st.Teams = []domain.Team{{ID: 10, Name: "PaaS / Infra"}}
	s := newTestUC(st, fa)
	if err := s.UpdateTeamStatus(context.Background(), domain.TenantScope{TenantID: 1}, 10, 3, domain.TeamPeriodStatusInProgress, 5); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(fa.Recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.Recorded))
	}
	ev := fa.Recorded[0]
	if ev.Category != domain.ActivityStatus || ev.Action != domain.ActionStatusChanged || ev.EntityTitle != "PaaS / Infra" {
		t.Fatalf("wrong event: %+v", ev)
	}
	if ev.Payload["after"].(map[string]any)["status"] != string(domain.TeamPeriodStatusInProgress) {
		t.Fatalf("after status wrong: %+v", ev.Payload)
	}
}
