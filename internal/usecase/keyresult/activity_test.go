package keyresult

// Тесты переехали из internal/service при выделении слоя usecase, затем — с журнала
// на шину: сценарий публикует событие, а не журнальную строку.

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/service/servicetest"
)

func TestKRProgressRecordsBeforeAfterNumbers(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	// Numerical KR: 0→100, currently at 30 (=30%). Update current to 80 (=80%).
	st.KeyResults[55] = domain.KeyResult{ID: 55, GoalID: 7, Kind: domain.KRKindNumerical, Title: "P95 latency",
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 30}}
	s := newTestUCWithBus(st, bus)
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 55, CheckInInput{Numerical: ptr(80.0)}, 5); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.KRCheckedIn)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.KRID != 55 {
		t.Fatalf("wrong event: %+v", ev)
	}
	// Regression: before/after must be the real computed percentages, not 0→0.
	if ev.ProgressBefore != 30 {
		t.Fatalf("before progress wrong (want 30): %+v", ev)
	}
	if ev.ProgressAfter != 80 {
		t.Fatalf("after progress wrong (want 80): %+v", ev)
	}
}

func TestKRNoteUpdateRecordsEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[55] = domain.KeyResult{ID: 55, GoalID: 7, Kind: domain.KRKindNumerical, Title: "P95 latency"}
	s := newTestUCWithBus(st, bus)
	// GetKeyResultNote returns nil (no prior note) → beforeText "" != "circuit breaker" → records.
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 55, CheckInInput{Note: ptr("добавили circuit breaker")}, 5); err != nil {
		t.Fatalf("note: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.KRCheckedIn)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.KRID != 55 {
		t.Fatalf("wrong note event: %+v", ev)
	}
	if ev.NoteAfter != "добавили circuit breaker" {
		t.Fatalf("note payload wrong: %+v", ev)
	}
}
