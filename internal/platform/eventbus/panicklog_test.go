package eventbus

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"okrs/internal/core/event"
	"okrs/internal/platform/logging"
)

// allRecords разбирает весь вывод: тестам про панику важны не только записи
// о ней самой, но и то, чем она НЕ притворяется.
func allRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка не является валидным JSON: %v\n%s", err, line)
		}
		out = append(out, rec)
	}
	return out
}

func recordsOf(recs []map[string]any, evt string) []map[string]any {
	var out []map[string]any
	for _, rec := range recs {
		if rec[logging.KeyEvent] == evt {
			out = append(out, rec)
		}
	}
	return out
}

// panicked публикует событие в шину с сорвавшимся обработчиком и возвращает
// разобранный вывод логгера.
func panicked(t *testing.T, ctx context.Context) []map[string]any {
	t.Helper()
	buf := &bytes.Buffer{}
	bus := New(logging.New(logging.Config{Output: buf}))
	Subscribe(bus, "panicky", func(context.Context, []event.GoalCreated) error {
		panic("обработчик сорвался")
	})
	bus.Start(context.Background())

	bus.Publish(ctx, event.GoalCreated{GoalID: 1})
	if err := bus.Close(2 * time.Second); err != nil {
		t.Fatalf("шина не закрылась: %v", err)
	}
	return allRecords(t, buf)
}

// Паника в фоновом обработчике обязана оставлять запись уровня error: изоляция
// паники без записи означает, что следствие мутации молча не наступило, и в
// логе от этого не остаётся ничего.
func TestBackgroundPanicIsLoggedAsError(t *testing.T) {
	recs := panicked(t, context.Background())

	panics := recordsOf(recs, logging.EventBackgroundPanic)
	if len(panics) != 1 {
		t.Fatalf("паника оставила %d записей, ожидалась 1: %v", len(panics), recs)
	}
	rec := panics[0]
	if rec["level"] != "ERROR" {
		t.Errorf("уровень = %v, ожидался ERROR", rec["level"])
	}
	if rec["subscriber"] != "panicky" {
		t.Errorf("подписчик = %v, ожидался \"panicky\"", rec["subscriber"])
	}
	if cause, _ := rec["panic"].(string); !strings.Contains(cause, "обработчик сорвался") {
		t.Errorf("причина паники = %v", rec["panic"])
	}
	if stack, _ := rec["stack"].(string); stack == "" {
		t.Error("запись о панике без стека: разбирать нечего")
	}
}

// Паника не должна прятаться под типом штатной записи о доменном событии:
// иначе «показать все паники» одним фильтром не собирается, а алерт на срыв
// фонового обработчика не на чем построить.
func TestBackgroundPanicIsDistinguishableFromOrdinaryEventRecords(t *testing.T) {
	recs := panicked(t, context.Background())

	for _, rec := range recordsOf(recs, logging.EventDomainEvent) {
		if _, ok := rec["stack"]; ok {
			t.Errorf("паника записана под типом %q: %v", logging.EventDomainEvent, rec)
		}
	}
}
