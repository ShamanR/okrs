package note

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.
// Ниже — сквозной тест на реальном usecase: правка заметки вне чек-ина обязана
// идти через тот же CheckIn и порождать то же единственное событие KRCheckedIn.

import (
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/http/handlers/handlertest"
	goalsvc "okrs/internal/service/goal"
	keyresultsvc "okrs/internal/service/keyresult"
	"okrs/internal/service/servicetest"
	kruc "okrs/internal/usecase/keyresult"
)

// Неразбираемый krID в пути — это ошибка клиента, а не 404 и не 500.
func TestGatePostBadKrID(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Post, http.MethodGet, "/api/v1/krs/{krID}/note", "",
		handlertest.Tenant(1),
		handlertest.URLParam("krID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Правка заметки вне чек-ина обязана дойти до CheckIn и опубликовать РОВНО одно
// событие KRCheckedIn с заполненными NoteBefore/NoteAfter — тем же путём, что и
// заметка, отправленная вместе с прогрессом.
func TestPostGoesThroughCheckIn(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, GoalID: 1, Kind: domain.KRKindNumerical}
	store.Notes[7] = domain.KeyResultNote{KeyResultID: 7, Text: "старая заметка"}
	bus := &servicetest.FakeBus{}
	uc := kruc.New(kruc.Deps{
		KRs:    keyresultsvc.New(store),
		Goals:  goalsvc.New(store),
		Events: bus,
	})
	h := New(goalsvc.New(store), keyresultsvc.New(store), uc)

	body := `{"text":"новая заметка"}`
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/krs/{krID}/note", body,
		handlertest.Tenant(1), handlertest.UserID(5, "u5"), handlertest.URLParam("krID", "7"))
	handlertest.Status(t, w, http.StatusOK)

	if store.Notes[7].Text != "новая заметка" {
		t.Fatalf("заметка не применилась: %+v", store.Notes[7])
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d: %+v", len(bus.Events), bus.Events)
	}
	ev, ok := bus.Events[0].(event.KRCheckedIn)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.NoteBefore != "старая заметка" || ev.NoteAfter != "новая заметка" {
		t.Fatalf("note before/after: got %q/%q", ev.NoteBefore, ev.NoteAfter)
	}
}
