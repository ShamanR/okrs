package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func recovered(t *testing.T, body func()) map[string]any {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := New(Config{Output: buf})

	func() {
		defer RecoverBackground(context.Background(), logger, "ночной_проход")
		body()
	}()

	out := strings.TrimRight(buf.String(), "\n")
	if out == "" {
		return nil
	}
	if strings.Contains(out, "\n") {
		t.Fatalf("ожидалась одна запись, получено:\n%s", out)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("строка не является валидным JSON: %v\n%s", err, out)
	}
	return rec
}

// Паника фоновой задачи обязана оставлять разбираемую запись: рантайм печатает
// в stderr многострочный дамп, который построчный сборщик логов не разберёт,
// и под перезапускается без единого следа причины.
func TestBackgroundPanicIsRecoveredAndLogged(t *testing.T) {
	rec := recovered(t, func() { panic("проход сорвался") })

	if rec == nil {
		t.Fatal("паника не оставила записи")
	}
	if rec["level"] != "ERROR" {
		t.Errorf("уровень = %v, ожидался ERROR", rec["level"])
	}
	if rec[KeyEvent] != EventBackgroundPanic {
		t.Errorf("event = %v, ожидался %q", rec[KeyEvent], EventBackgroundPanic)
	}
	if rec["task"] != "ночной_проход" {
		t.Errorf("task = %v", rec["task"])
	}
	if rec["outcome"] != "panicked" {
		t.Errorf("outcome = %v, ожидался panicked", rec["outcome"])
	}
	if cause, _ := rec["panic"].(string); !strings.Contains(cause, "проход сорвался") {
		t.Errorf("причина = %v", rec["panic"])
	}
	if stack, _ := rec["stack"].(string); stack == "" {
		t.Error("запись без стека: разбирать нечего")
	}
}

// Управление обязано вернуться вызывающему: именно на этом держится живучесть
// цикла — тикер продолжает работать, и следующий тик отрабатывает.
func TestRecoveredPanicReturnsControlToTheCaller(t *testing.T) {
	reached := false
	func() {
		defer func() { reached = true }()
		defer RecoverBackground(context.Background(), New(Config{Output: &bytes.Buffer{}}), "проход")
		panic("сорвалось")
	}()
	if !reached {
		t.Fatal("паника прошла сквозь перехват")
	}
}

// Без паники помощник обязан молчать: иначе каждый штатный тик писал бы запись
// об ошибке, и выборка «что падает в фоне» стала бы бесполезной.
func TestNoPanicLeavesNoRecord(t *testing.T) {
	if rec := recovered(t, func() {}); rec != nil {
		t.Errorf("штатный проход оставил запись: %v", rec)
	}
}
