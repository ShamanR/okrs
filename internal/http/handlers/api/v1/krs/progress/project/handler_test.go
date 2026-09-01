package project

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.
// Ниже — сквозные тесты на реальном usecase: чек-ин одним запросом должен доносить
// прогресс, health-статус и (опционально) заметку.

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
	w := handlertest.Do(New(nil, nil, nil).Post, http.MethodGet, "/api/v1/krs/{krID}/progress/project", "",
		handlertest.Tenant(1),
		handlertest.URLParam("krID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func newTestHandler(store *servicetest.Store) *Handler {
	h, _ := newTestHandlerWithBus(store)
	return h
}

// newTestHandlerWithBus отдаёт шину наружу, чтобы тест мог в неё посмотреть. Раньше
// тесты создавали &servicetest.FakeBus{} и никогда не проверяли содержимое, поэтому
// мутация «убрать Health из CheckInInput и вернуть прямой вызов сервиса после
// CheckIn» — буквально исходный дефект, из-за которого смена статуса шла мимо шины —
// проходила весь набор зелёным.
func newTestHandlerWithBus(store *servicetest.Store) (*Handler, *servicetest.FakeBus) {
	bus := &servicetest.FakeBus{}
	uc := kruc.New(kruc.Deps{
		KRs:    keyresultsvc.New(store),
		Goals:  goalsvc.New(store),
		Events: bus,
	})
	return New(goalsvc.New(store), keyresultsvc.New(store), uc), bus
}

// Одно тело — прогресс, health-статус и заметка — обязано долетать до usecase
// одним вызовом: ровно это и есть контракт «один чек-ин — один запрос».
func TestPostAppliesProgressHealthAndNote(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, GoalID: 1, Kind: domain.KRKindProject}
	store.ProjectStages[7] = []domain.KRProjectStage{
		{ID: 100, Weight: 60, IsDone: false},
		{ID: 101, Weight: 40, IsDone: false},
	}
	h := newTestHandler(store)

	body := `{"stages":[{"id":100,"done":true}],"health_status":"at_risk","note":"первый этап готов"}`
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/krs/{krID}/progress/project", body,
		handlertest.Tenant(1), handlertest.UserID(5, "u5"), handlertest.URLParam("krID", "7"))
	handlertest.Status(t, w, http.StatusOK)

	if !store.StageUpdates[100] {
		t.Fatalf("прогресс не применился: %+v", store.StageUpdates)
	}
	if store.HealthUpdates[7] != domain.KRHealthAtRisk {
		t.Fatalf("health-статус не применился: %+v", store.HealthUpdates)
	}
	if store.Notes[7].Text != "первый этап готов" {
		t.Fatalf("заметка не применилась: %+v", store.Notes[7])
	}
}

// note отсутствует в теле — заметка не должна трогаться вообще.
func TestPostWithoutNoteLeavesNoteUntouched(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, GoalID: 1, Kind: domain.KRKindProject}
	store.ProjectStages[7] = []domain.KRProjectStage{{ID: 100, Weight: 100, IsDone: false}}
	store.Notes[7] = domain.KeyResultNote{KeyResultID: 7, Text: "прежняя заметка"}
	h := newTestHandler(store)

	body := `{"stages":[{"id":100,"done":true}]}`
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/krs/{krID}/progress/project", body,
		handlertest.Tenant(1), handlertest.UserID(5, "u5"), handlertest.URLParam("krID", "7"))
	handlertest.Status(t, w, http.StatusOK)

	if store.Notes[7].Text != "прежняя заметка" {
		t.Fatalf("заметка изменилась без note в теле: %+v", store.Notes[7])
	}
}

// note присутствует пустой строкой — это валидное значение «очистить заметку»,
// отличное от отсутствия поля.
func TestPostEmptyStringNoteClearsNote(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, GoalID: 1, Kind: domain.KRKindProject}
	store.ProjectStages[7] = []domain.KRProjectStage{{ID: 100, Weight: 100, IsDone: false}}
	store.Notes[7] = domain.KeyResultNote{KeyResultID: 7, Text: "старая"}
	h := newTestHandler(store)

	body := `{"stages":[{"id":100,"done":false}],"note":""}`
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/krs/{krID}/progress/project", body,
		handlertest.Tenant(1), handlertest.UserID(5, "u5"), handlertest.URLParam("krID", "7"))
	handlertest.Status(t, w, http.StatusOK)

	if store.Notes[7].Text != "" {
		t.Fatalf("пустая заметка не применилась: %+v", store.Notes[7])
	}
}

// Невалидный health_status по-прежнему даёт 400 и не допускает никакого
// побочного изменения (ни прогресса, ни заметки).
func TestPostInvalidHealthStatusStill400(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, GoalID: 1, Kind: domain.KRKindProject}
	store.ProjectStages[7] = []domain.KRProjectStage{{ID: 100, Weight: 100, IsDone: false}}
	h := newTestHandler(store)

	body := `{"stages":[{"id":100,"done":true}],"health_status":"bogus"}`
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/krs/{krID}/progress/project", body,
		handlertest.Tenant(1), handlertest.UserID(5, "u5"), handlertest.URLParam("krID", "7"))
	handlertest.IsError(t, w, http.StatusBadRequest)

	if store.StageUpdates[100] {
		t.Fatalf("невалидный health_status не должен допускать обновление прогресса: %+v", store.StageUpdates)
	}
}

// M4 код-ревью: заметка в теле прогресс-запроса обязана нормализоваться так же,
// как в отдельном note-эндпоинте (CRLF → LF) — иначе, раз в интерфейсе заметка
// правится только через прогресс, нормализация фактически не применяется к
// единственному реально используемому пути.
func TestPostNormalizesNoteCRLF(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, GoalID: 1, Kind: domain.KRKindProject}
	store.ProjectStages[7] = []domain.KRProjectStage{{ID: 100, Weight: 100, IsDone: false}}
	h := newTestHandler(store)

	body := `{"stages":[{"id":100,"done":true}],"note":"первая строка\r\nвторая строка"}`
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/krs/{krID}/progress/project", body,
		handlertest.Tenant(1), handlertest.UserID(5, "u5"), handlertest.URLParam("krID", "7"))
	handlertest.Status(t, w, http.StatusOK)

	if store.Notes[7].Text != "первая строка\nвторая строка" {
		t.Fatalf("CRLF не нормализован: %q", store.Notes[7].Text)
	}
}

// Health-статус обязан уходить в шину внутри чек-ина, а не отдельным вызовом сервиса
// мимо неё: событие должно быть ровно одно, и HealthAfter в нём — тот, что прислали
// в теле. Проверка «store.HealthUpdates применился» этого не ловит: прямой вызов
// krs.UpdateHealthStatus её удовлетворяет, оставляя уведомление без смены статуса.
func TestPostPublishesHealthThroughBus(t *testing.T) {
	store := servicetest.NewStore()
	store.KeyResults[7] = domain.KeyResult{ID: 7, GoalID: 1, Kind: domain.KRKindProject, HealthStatus: domain.KRHealthAtRisk}
	store.ProjectStages[7] = []domain.KRProjectStage{
		{ID: 100, Weight: 60, IsDone: false},
		{ID: 101, Weight: 40, IsDone: false},
	}
	h, bus := newTestHandlerWithBus(store)

	body := `{"stages":[{"id":100,"done":true}],"health_status":"on_track"}`
	w := handlertest.Do(h.Post, http.MethodPost, "/api/v1/krs/{krID}/progress/project", body,
		handlertest.Tenant(1), handlertest.UserID(5, "u5"), handlertest.URLParam("krID", "7"))
	handlertest.Status(t, w, http.StatusOK)

	if len(bus.Events) != 1 {
		t.Fatalf("опубликовано событий: %d, want 1 (%v)", len(bus.Events), bus.KindsPublished())
	}
	ev, ok := bus.Events[0].(event.KRCheckedIn)
	if !ok {
		t.Fatalf("опубликовано %T, want event.KRCheckedIn", bus.Events[0])
	}
	if ev.HealthAfter != domain.KRHealthOnTrack {
		t.Fatalf("HealthAfter = %q, want %q — статус ушёл мимо шины", ev.HealthAfter, domain.KRHealthOnTrack)
	}
	if ev.HealthBefore != domain.KRHealthAtRisk {
		t.Errorf("HealthBefore = %q, want %q", ev.HealthBefore, domain.KRHealthAtRisk)
	}
}
