package krscommon

// Тесты на два хелпера, вынесенные по код-ревью после Task 2 kr-checkin: до этого
// разбор health_status был скопирован дословно в три хендлера прогресса (мутация
// ревьюера, вырезающая валидацию в одном хендлере, ловилась только тестом этого
// одного пакета), а нормализация CRLF в заметке жила только в отдельном
// note-хендлере и не применялась к тому же полю в хендлерах прогресса.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/core/domain"
)

func TestNormalizeNoteTextConvertsCRLFToLF(t *testing.T) {
	got := NormalizeNoteText("первая строка\r\nвторая строка\r\nтретья")
	want := "первая строка\nвторая строка\nтретья"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeNoteTextLeavesLFAlone(t *testing.T) {
	got := NormalizeNoteText("уже LF\nбез изменений")
	if got != "уже LF\nбез изменений" {
		t.Fatalf("текст без CRLF не должен меняться: %q", got)
	}
}

// s == nil означает «health_status не часть этого запроса» — хелпер не должен
// ничего писать в ответ и должен вернуть written == false, чтобы вызывающий
// продолжил обработку.
func TestParseHealthStatusNilFieldIsNotAnError(t *testing.T) {
	w := httptest.NewRecorder()
	status, written := ParseHealthStatus(w, nil)
	if written {
		t.Fatalf("nil health_status не должен писать ответ")
	}
	if status != nil {
		t.Fatalf("nil health_status не должен парситься: %+v", status)
	}
	if w.Code != 200 {
		// httptest.ResponseRecorder по умолчанию 200, пока никто явно не написал код —
		// проверяем, что хелпер действительно ничего не написал.
		t.Fatalf("код ответа не должен меняться при отсутствии поля, got %d", w.Code)
	}
}

func TestParseHealthStatusValidValueParses(t *testing.T) {
	w := httptest.NewRecorder()
	s := "on_track"
	status, written := ParseHealthStatus(w, &s)
	if written {
		t.Fatalf("валидный health_status не должен писать ответ об ошибке")
	}
	if status == nil || *status != domain.KRHealthOnTrack {
		t.Fatalf("got %+v, want on_track", status)
	}
}

// Невалидное значение обязано писать 400 САМ хелпер (единая логика на все три
// хендлера прогресса) и сигнализировать written == true — вызывающий обязан
// вернуться немедленно, ничего больше не делая.
func TestParseHealthStatusInvalidValueWrites400(t *testing.T) {
	w := httptest.NewRecorder()
	s := "bogus"
	status, written := ParseHealthStatus(w, &s)
	if !written {
		t.Fatalf("невалидный health_status обязан писать ответ")
	}
	if status != nil {
		t.Fatalf("невалидный health_status не должен возвращать распарсенный статус: %+v", status)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("код ответа = %d, want 400", w.Code)
	}
}
