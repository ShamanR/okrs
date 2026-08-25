package keyresult

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
)

func TestKRProgressRecordsBeforeAfterNumbers(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	st := servicetest.NewStore()
	// Numerical KR: 0→100, currently at 30 (=30%). Update current to 80 (=80%).
	st.KeyResults[55] = domain.KeyResult{ID: 55, GoalID: 7, Kind: domain.KRKindNumerical, Title: "P95 latency",
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 30}}
	s := newTestUCWithActivity(st, fa)
	if err := s.UpdateProgressNumerical(context.Background(), domain.TenantScope{TenantID: 1}, 55, 80, 5); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(fa.Recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.Recorded))
	}
	ev := fa.Recorded[0]
	if ev.Category != domain.ActivityProgress || ev.Action != domain.ActionKRProgress || *ev.KRID != 55 {
		t.Fatalf("wrong event: %+v", ev)
	}
	// Regression: before/after must be the real computed percentages, not 0→0.
	if ev.Payload["before"].(map[string]any)["progress"] != 30 {
		t.Fatalf("before progress wrong (want 30): %+v", ev.Payload)
	}
	if ev.Payload["after"].(map[string]any)["progress"] != 80 {
		t.Fatalf("after progress wrong (want 80): %+v", ev.Payload)
	}
}

func TestKRNoteUpdateRecordsEvent(t *testing.T) {
	fa := &servicetest.ActivityRepo{}
	st := servicetest.NewStore()
	st.KeyResults[55] = domain.KeyResult{ID: 55, GoalID: 7, Kind: domain.KRKindNumerical, Title: "P95 latency"}
	s := newTestUCWithActivity(st, fa)
	// GetKeyResultNote returns nil (no prior note) → beforeText "" != "circuit breaker" → records.
	if err := s.UpsertNote(context.Background(), domain.TenantScope{TenantID: 1}, 55, "добавили circuit breaker", 5); err != nil {
		t.Fatalf("note: %v", err)
	}
	if len(fa.Recorded) != 1 {
		t.Fatalf("want 1 event, got %d", len(fa.Recorded))
	}
	ev := fa.Recorded[0]
	if ev.Category != domain.ActivityDiscussion || ev.Action != domain.ActionKRNoteUpdated || *ev.KRID != 55 {
		t.Fatalf("wrong note event: %+v", ev)
	}
	if ev.Payload["after"].(map[string]any)["note"] != "добавили circuit breaker" {
		t.Fatalf("note payload wrong: %+v", ev.Payload)
	}
}
