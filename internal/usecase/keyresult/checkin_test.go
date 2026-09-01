package keyresult

// Тесты на CheckIn — единственную операцию чек-ина, заменяющую по отдельности
// публиковавшие события методы прогресса и UpsertNote.

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/service/servicetest"
)

// Чек-ин, меняющий прогресс, health-статус и заметку одновременно, публикует РОВНО
// одно событие KRCheckedIn с корректными before/after по всем трём величинам —
// это и есть цель всего рефакторинга («один запрос — одно событие»).
func TestCheckInPublishesOneEventWithAllBeforeAfter(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[55] = domain.KeyResult{
		ID: 55, GoalID: 7, Kind: domain.KRKindNumerical, Title: "P95 latency",
		HealthStatus: domain.KRHealthAtRisk,
		Numerical:    &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 30},
	}
	// Названия KR и цели В ФИКСТУРЕ ОБЯЗАНЫ РАЗЛИЧАТЬСЯ: одинаковые (или пустые)
	// прошли бы и при подмене одного другим — ровно эту мутацию и ловят проверки ниже.
	st.Goals[7] = domain.Goal{ID: 7, TeamID: 3, PeriodID: 4, Title: "Снизить отток"}
	s := newTestUCWithBus(st, bus)

	current := 80.0
	health := domain.KRHealthOnTrack
	note := "ждём поставку"
	err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 55,
		CheckInInput{Numerical: &current, Health: &health, Note: &note}, 5)
	if err != nil {
		t.Fatalf("checkin: %v", err)
	}

	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d: %+v", len(bus.Events), bus.Events)
	}
	ev, ok := bus.Events[0].(event.KRCheckedIn)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.ProgressBefore != 30 || ev.ProgressAfter != 80 {
		t.Errorf("progress before/after: got %d/%d, want 30/80", ev.ProgressBefore, ev.ProgressAfter)
	}
	if ev.HealthBefore != domain.KRHealthAtRisk || ev.HealthAfter != domain.KRHealthOnTrack {
		t.Errorf("health before/after: got %q/%q, want at_risk/on_track", ev.HealthBefore, ev.HealthAfter)
	}
	if ev.NoteBefore != "" || ev.NoteAfter != "ждём поставку" {
		t.Errorf("note before/after: got %q/%q", ev.NoteBefore, ev.NoteAfter)
	}
	// Название KR — единственный источник и для строки «предмет» в карточке
	// уведомления (через anchorOf → EntityTitle → Subject), и для строк журнала
	// прогресса и обсуждения (service/activity/journal.go). Проверять его после
	// публикации бесполезно: и mapping_test, и subject_test начинаются с уже
	// готового события. Мутация KRTitle: kr.Title → g.Title прошла весь набор
	// зелёным именно потому, что этой проверки не было.
	if ev.KRTitle != "P95 latency" {
		t.Errorf("KRTitle = %q, want %q (название KR, а не цели)", ev.KRTitle, "P95 latency")
	}
	if ev.GoalTitle != "Снизить отток" {
		t.Errorf("GoalTitle = %q, want %q", ev.GoalTitle, "Снизить отток")
	}
	if ev.GoalID != 7 || ev.KRID != 55 {
		t.Errorf("GoalID/KRID = %d/%d, want 7/55", ev.GoalID, ev.KRID)
	}
}

// Смена ТОЛЬКО health-статуса (без прогресса и без заметки) обязана публиковать
// событие: до фикса health-статус вообще не порождал событий, и это часть
// чинимого дефекта.
func TestCheckInHealthOnlyStillPublishes(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[7] = domain.KeyResult{ID: 7, GoalID: 1, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthNotStarted}
	s := newTestUCWithBus(st, bus)

	health := domain.KRHealthOnTrack
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 7, CheckInInput{Health: &health}, 1); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event for a health-only check-in, got %d", len(bus.Events))
	}
	ev := bus.Events[0].(event.KRCheckedIn)
	if ev.ProgressBefore != ev.ProgressAfter {
		t.Errorf("progress must be unchanged (before==after), got %d/%d", ev.ProgressBefore, ev.ProgressAfter)
	}
	if ev.HealthBefore != domain.KRHealthNotStarted || ev.HealthAfter != domain.KRHealthOnTrack {
		t.Errorf("health before/after: got %q/%q", ev.HealthBefore, ev.HealthAfter)
	}
}

// Ядро исправляемого дефекта: если ни прогресс, ни health, ни заметка не
// изменились фактически, CheckIn не публикует ничего. Сегодня прогресс
// публикуется безусловно, поэтому смена только health-статуса даёт побочное
// уведомление «обновил прогресс 50% → 50%» — этот тест фиксирует, что так больше
// не происходит даже когда прогресс ЯВНО отправлен, но не изменился.
func TestCheckInPublishesNothingWhenNothingChanged(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[7] = domain.KeyResult{
		ID: 7, GoalID: 1, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthOnTrack,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 50},
	}
	s := newTestUCWithBus(st, bus)

	// Same numerical value, same health, no note — a resubmission of the check-in
	// form with nothing actually edited (e.g. a checkpoint value re-sent as-is).
	current := 50.0
	health := domain.KRHealthOnTrack
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 7,
		CheckInInput{Numerical: &current, Health: &health}, 1); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if len(bus.Events) != 0 {
		t.Fatalf("want 0 events when nothing changed, got %d: %+v", len(bus.Events), bus.Events)
	}
}

// Явный health в том же запросе не должен быть молча переписан авто-завершением
// (AutoCompleteHealth срабатывает на достижение 100%, опираясь на health ДО
// вызова) — иначе чек-ин «прогресс 100% + статус=on_track» тайно превращался бы
// в «done», противореча явному выбору пользователя в том же запросе.
func TestCheckInExplicitHealthNotOverriddenByAutoComplete(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[7] = domain.KeyResult{
		ID: 7, GoalID: 1, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthAtRisk,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 90},
	}
	s := newTestUCWithBus(st, bus)

	current := 100.0
	health := domain.KRHealthOnTrack
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 7,
		CheckInInput{Numerical: &current, Health: &health}, 1); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if got := st.HealthUpdates[7]; got != domain.KRHealthOnTrack {
		t.Fatalf("explicit health status was overridden: store has %q, want on_track", got)
	}
}

// Регресс из код-ревью: когда прогресс пересекает 100% БЕЗ явного health в этом
// же чек-ине, AutoCompleteHealth пишет в стор "done", и опубликованное событие
// обязано нести этот РЕЗУЛЬТИРУЮЩИЙ статус в HealthAfter, а не устаревший
// beforeHealth. Иначе уведомление рисует правило 1 («●On Track 90% → 100%»)
// вместо правила 2 («●On Track → ✓Closed 90% → 100%») ровно в тот момент, когда
// переход статуса — главная новость.
func TestCheckInAutoCompleteReflectsInEvent(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[7] = domain.KeyResult{
		ID: 7, GoalID: 1, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthOnTrack,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 90},
	}
	s := newTestUCWithBus(st, bus)

	current := 100.0
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 7, CheckInInput{Numerical: &current}, 1); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev := bus.Events[0].(event.KRCheckedIn)
	if ev.HealthBefore != domain.KRHealthOnTrack {
		t.Fatalf("HealthBefore = %q, want on_track", ev.HealthBefore)
	}
	if ev.HealthAfter != domain.KRHealthDone {
		t.Fatalf("HealthAfter = %q, want done (auto-complete result), got stale beforeHealth", ev.HealthAfter)
	}
}

// Регресс из код-ревью: заметка обязана читаться ДО записи, а не после. Раньше
// фейковый стор не связывал UpsertKeyResultNote с GetKeyResultNote (первый был
// no-op, второй всегда возвращал nil), поэтому beforeNote был тождественно
// пустой строкой во всех тестах, и порядок чтение/запись внутри CheckIn был
// ненаблюдаем — мутация, переставляющая их местами, проходила весь go test
// ./... незамеченной. Этот тест предзаполняет заметку через ту же карту, что
// теперь читает и пишет фейк, так что NoteBefore обязан содержать прежний текст.
func TestCheckInNoteBeforeReflectsPriorText(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[55] = domain.KeyResult{ID: 55, GoalID: 7, Kind: domain.KRKindNumerical, Title: "P95 latency"}
	st.Notes[55] = domain.KeyResultNote{KeyResultID: 55, Text: "старая заметка"}
	s := newTestUCWithBus(st, bus)

	note := "новая заметка"
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 55, CheckInInput{Note: &note}, 5); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev, ok := bus.Events[0].(event.KRCheckedIn)
	if !ok {
		t.Fatalf("wrong event type: %+v", bus.Events[0])
	}
	if ev.NoteBefore != "старая заметка" {
		t.Fatalf("NoteBefore = %q, want %q (заметка прочитана ПОСЛЕ записи, а не до)", ev.NoteBefore, "старая заметка")
	}
	if ev.NoteAfter != "новая заметка" {
		t.Fatalf("NoteAfter = %q, want %q", ev.NoteAfter, "новая заметка")
	}
}

// Находка из код-ревью, переигранная после фикса «фактический прогресс вместо
// нулей» (см. TestCheckInProgressNotInSubmissionCarriesActualProgress ниже):
// чек-ин только с заметкой на project-KR ТЕПЕРЬ читает ListProjectStages — иначе
// неоткуда взять фактический прогресс для payload'а события. Но ошибка этого
// чтения по-прежнему не должна блокировать сохранение заметки: она никак не
// зависит от списка стадий, а before==after сохраняет требуемое равенство даже
// когда взять реальное значение не удалось.
func TestCheckInNoteOnlySavesEvenWhenProjectStagesFail(t *testing.T) {
	st := servicetest.NewStore()
	st.KeyResults[9] = domain.KeyResult{ID: 9, GoalID: 1, Kind: domain.KRKindProject}
	st.ListProjectStagesErr = errors.New("boom")
	bus := &servicetest.FakeBus{}
	s := newTestUCWithBus(st, bus)

	note := "заметка без прогресса"
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 9, CheckInInput{Note: &note}, 1); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if st.Notes[9].Text != note {
		t.Fatalf("заметка не сохранилась при ошибке чтения стадий: %+v", st.Notes[9])
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev := bus.Events[0].(event.KRCheckedIn)
	if ev.ProgressBefore != ev.ProgressAfter {
		t.Errorf("progress must stay before==after on a read failure, got %d/%d", ev.ProgressBefore, ev.ProgressAfter)
	}
}

// Находка из код-ревью: чек-ин, не затрагивающий прогресс (например, только
// health-статус или только заметка), обязан нести в событии ФАКТИЧЕСКИЙ текущий
// прогресс KR в обоих полях (before и after), а не нули. Ноли безвредны для
// журнала и рендерера (им важно только равенство), но payload события уходит
// дословно в сохраняемый notifications.payload_json каждого получателя — это
// запись о том, где KR действительно стоял в момент чек-ина, и ноль в ней
// фиксирует то, чего не было. Читающего абсолютное значение потребителя сегодня
// нет (каналы уведомлений payload не читают вовсе) — требование именно к
// правдивости записи.
func TestCheckInProgressNotInSubmissionCarriesActualProgress(t *testing.T) {
	t.Run("boolean", func(t *testing.T) {
		bus := &servicetest.FakeBus{}
		st := servicetest.NewStore()
		st.KeyResults[7] = domain.KeyResult{
			ID: 7, GoalID: 1, Kind: domain.KRKindBoolean, HealthStatus: domain.KRHealthAtRisk,
			Boolean: &domain.KRBoolean{IsDone: true},
		}
		s := newTestUCWithBus(st, bus)

		health := domain.KRHealthOnTrack
		if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 7, CheckInInput{Health: &health}, 1); err != nil {
			t.Fatalf("checkin: %v", err)
		}
		ev := bus.Events[0].(event.KRCheckedIn)
		if ev.ProgressBefore != 100 || ev.ProgressAfter != 100 {
			t.Fatalf("progress before/after: got %d/%d, want 100/100 (фактический прогресс, а не нули)", ev.ProgressBefore, ev.ProgressAfter)
		}
	})

	t.Run("project", func(t *testing.T) {
		bus := &servicetest.FakeBus{}
		st := servicetest.NewStore()
		st.KeyResults[8] = domain.KeyResult{ID: 8, GoalID: 1, Kind: domain.KRKindProject, HealthStatus: domain.KRHealthOnTrack}
		st.ProjectStages[8] = []domain.KRProjectStage{
			{ID: 1, Weight: 60, IsDone: true},
			{ID: 2, Weight: 40, IsDone: false},
		}
		s := newTestUCWithBus(st, bus)

		note := "почти готово"
		if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 8, CheckInInput{Note: &note}, 1); err != nil {
			t.Fatalf("checkin: %v", err)
		}
		ev := bus.Events[0].(event.KRCheckedIn)
		if ev.ProgressBefore != 60 || ev.ProgressAfter != 60 {
			t.Fatalf("progress before/after: got %d/%d, want 60/60 (фактический прогресс, а не нули)", ev.ProgressBefore, ev.ProgressAfter)
		}
	})
}

// Прогресс, отправленный на KR другого вида, отклоняется до какой-либо записи —
// как и раньше у отдельных методов UpdateProgressNumerical/Boolean/Project.
func TestCheckInRejectsMismatchedKind(t *testing.T) {
	st := servicetest.NewStore()
	st.KeyResults[1] = domain.KeyResult{ID: 1, Kind: domain.KRKindBoolean}
	s := newTestUC(st)

	current := 50.0
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 1, CheckInInput{Numerical: &current}, 1); err == nil {
		t.Fatal("expected error for boolean KR with numerical update")
	}
}

// Находка код-ревью M1: симметрично прогрессу (пункт 5 задания), заметка тоже
// обязана нести ФАКТИЧЕСКОЕ текущее значение в обоих полях события, когда она не
// входит в чек-ин, — а не пустую строку, даже если у KR реально есть заметка.
// Чек-ин только с прогрессом на KR с уже существующей заметкой — до фикса
// NoteBefore/NoteAfter были "" (заметка не читалась вовсе), после — фактический
// текст в обоих полях. Равенство before==after обязано сохраниться (проверяется
// ниже отдельно), иначе журнал начал бы писать строку обсуждения там, где раньше
// не писал.
func TestCheckInProgressOnlyCarriesActualNote(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[7] = domain.KeyResult{
		ID: 7, GoalID: 1, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthOnTrack,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 30},
	}
	st.Notes[7] = domain.KeyResultNote{KeyResultID: 7, Text: "уже была заметка"}
	s := newTestUCWithBus(st, bus)

	current := 80.0
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 7, CheckInInput{Numerical: &current}, 1); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if len(bus.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.Events))
	}
	ev := bus.Events[0].(event.KRCheckedIn)
	if ev.NoteBefore != "уже была заметка" || ev.NoteAfter != "уже была заметка" {
		t.Fatalf("note before/after: got %q/%q, want фактический текст в обоих полях (не нули/пустые строки)", ev.NoteBefore, ev.NoteAfter)
	}
}

// То же исправление не должно давать ложных срабатываний журнала: чек-ин, не
// трогающий заметку, обязан сохранять NoteBefore == NoteAfter даже теперь, когда
// заметка читается безусловно, — иначе toRows начал бы писать строку обсуждения
// там, где раньше не писал (см. правило "before/after различаются → строка").
func TestCheckInUnconditionalNoteReadKeepsEquality(t *testing.T) {
	bus := &servicetest.FakeBus{}
	st := servicetest.NewStore()
	st.KeyResults[7] = domain.KeyResult{
		ID: 7, GoalID: 1, Kind: domain.KRKindNumerical, HealthStatus: domain.KRHealthOnTrack,
		Numerical: &domain.KRNumerical{StartValue: 0, TargetValue: 100, CurrentValue: 30},
	}
	st.Notes[7] = domain.KeyResultNote{KeyResultID: 7, Text: "неизменная заметка"}
	s := newTestUCWithBus(st, bus)

	current := 80.0
	if err := s.CheckIn(context.Background(), domain.TenantScope{TenantID: 1}, 7, CheckInInput{Numerical: &current}, 1); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	ev := bus.Events[0].(event.KRCheckedIn)
	if ev.NoteBefore != ev.NoteAfter {
		t.Fatalf("note must stay before==after when note is not part of the submission, got %q/%q", ev.NoteBefore, ev.NoteAfter)
	}
}
