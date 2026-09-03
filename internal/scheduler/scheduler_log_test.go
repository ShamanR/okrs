package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/platform/logging"
)

func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	var out []map[string]any
	for _, line := range strings.Split(raw, "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка не является валидным JSON: %v\n%s", err, line)
		}
		out = append(out, rec)
	}
	return out
}

func find(recs []map[string]any, msg string) map[string]any {
	for _, r := range recs {
		if r["msg"] == msg {
			return r
		}
	}
	return nil
}

func TestBackgroundPassLogsStartAndSuccess(t *testing.T) {
	buf := &bytes.Buffer{}
	s := New(Deps{Logger: logging.New(logging.Config{Output: buf})})

	s.runPass(context.Background(), "notification_retention", func(context.Context) ([]any, error) {
		return []any{slog.Int64("deleted", 17)}, nil
	})

	recs := records(t, buf)
	started := find(recs, "background task started")
	if started == nil {
		t.Fatalf("нет записи о запуске задачи: %v", recs)
	}
	if started[logging.KeyEvent] != logging.EventBackgroundTask || started["task"] != "notification_retention" {
		t.Errorf("запись о запуске не несёт тип и имя задачи: %v", started)
	}

	finished := find(recs, "background task finished")
	if finished == nil {
		t.Fatalf("нет записи о завершении задачи: %v", recs)
	}
	if finished["task"] != "notification_retention" || finished["outcome"] != "ok" {
		t.Errorf("запись о завершении не несёт имя задачи и исход: %v", finished)
	}
	if finished["deleted"] != float64(17) {
		t.Errorf("запись о завершении потеряла результат прохода: %v", finished["deleted"])
	}
	if _, ok := finished["duration_ms"]; !ok {
		t.Errorf("запись о завершении не содержит длительность: %v", finished)
	}
}

func TestBackgroundPassLogsFailureWithCause(t *testing.T) {
	buf := &bytes.Buffer{}
	s := New(Deps{Logger: logging.New(logging.Config{Output: buf})})

	s.runPass(context.Background(), "progress_snapshot", func(context.Context) ([]any, error) {
		return nil, errors.New("снимок не построен")
	})

	rec := find(records(t, buf), "background task failed")
	if rec == nil {
		t.Fatalf("нет записи об ошибке фоновой задачи: %s", buf.String())
	}
	if rec["level"] != "WARN" {
		t.Errorf("уровень = %v, ожидался WARN", rec["level"])
	}
	if rec["task"] != "progress_snapshot" || rec["outcome"] != "failed" {
		t.Errorf("запись не несёт имя задачи и исход: %v", rec)
	}
	if rec["err"] != "снимок не построен" {
		t.Errorf("запись не содержит причину: %v", rec["err"])
	}
}

type failingTenants struct{ err error }

func (f failingTenants) List(context.Context) ([]domain.Tenant, error) { return nil, f.err }

// Отказ обнаружения обязан возвращаться вызывающему, а не превращаться в пустой
// список: иначе недоступная база даёт «нечего делать», и проход отчитывается
// об успехе, молча пропустив организации.
func TestDiscoveryFailureIsReturnedNotSwallowed(t *testing.T) {
	s := New(Deps{
		Zone:    time.UTC,
		Tenants: failingTenants{err: errors.New("соединение потеряно")},
	})

	due, err := s.snapshotDuePeriods(context.Background())

	if err == nil {
		t.Fatal("отказ списка организаций проглочен")
	}
	if !strings.Contains(err.Error(), "соединение потеряно") {
		t.Errorf("причина потеряна: %v", err)
	}
	if due != nil {
		t.Errorf("при отказе списка организаций делать нечего: %v", due)
	}
}

// И этот отказ должен дойти до исхода задачи в логе.
func TestDiscoveryFailureMakesThePassFail(t *testing.T) {
	buf := &bytes.Buffer{}
	s := New(Deps{
		Zone:    time.UTC,
		Tenants: failingTenants{err: errors.New("соединение потеряно")},
		Logger:  logging.New(logging.Config{Output: buf}),
	})

	s.runPass(context.Background(), "progress_snapshot", func(ctx context.Context) ([]any, error) {
		_, err := s.snapshotDuePeriods(ctx)
		return nil, err
	})

	rec := find(records(t, buf), "background task failed")
	if rec == nil {
		t.Fatalf("отказ обнаружения не сделал проход неуспешным: %s", buf.String())
	}
	if rec["outcome"] != "failed" {
		t.Errorf("outcome = %v, ожидался failed", rec["outcome"])
	}
	if !strings.Contains(rec["err"].(string), "соединение потеряно") {
		t.Errorf("причина обнаружения не дошла до записи: %v", rec["err"])
	}
}

// Отсутствующий логгер не должен ронять фоновую задачу: Deps.Logger — nil в части
// тестов, а падение планировщика из-за логирования было бы худшим из исходов.
func TestBackgroundPassSurvivesNilLogger(t *testing.T) {
	s := New(Deps{})
	s.runPass(context.Background(), "progress_snapshot", func(context.Context) ([]any, error) {
		return nil, errors.New("любая ошибка")
	})
}

// Паника прохода не должна ронять процесс: фоновая горутина без перехвата уносит
// с собой всё приложение, а рантайм печатает в stderr многострочный дамп, который
// построчный сборщик логов разобрать не может.
func TestBackgroundPassPanicIsLoggedAndContained(t *testing.T) {
	buf := &bytes.Buffer{}
	s := New(Deps{Logger: logging.New(logging.Config{Output: buf})})

	s.runPass(context.Background(), "progress_snapshot", func(context.Context) ([]any, error) {
		panic("проход сорвался")
	})

	rec := find(records(t, buf), "background task panicked")
	if rec == nil {
		t.Fatalf("паника прохода не оставила записи: %v", records(t, buf))
	}
	if rec["level"] != "ERROR" {
		t.Errorf("уровень = %v, ожидался ERROR", rec["level"])
	}
	if rec[logging.KeyEvent] != logging.EventBackgroundPanic {
		t.Errorf("event = %v, ожидался %q", rec[logging.KeyEvent], logging.EventBackgroundPanic)
	}
	if rec["task"] != "progress_snapshot" {
		t.Errorf("task = %v", rec["task"])
	}
	if stack, _ := rec["stack"].(string); stack == "" {
		t.Error("запись о панике без стека")
	}
}

// Сорвавшийся проход не должен останавливать цикл: тикер продолжает работать,
// и следующий тик обязан отработать штатно. Иначе снимки прогресса тихо
// перестали бы сниматься до перезапуска пода.
func TestLoopSurvivesAPanickingPass(t *testing.T) {
	buf := &bytes.Buffer{}
	s := New(Deps{Logger: logging.New(logging.Config{Output: buf})})
	ctx := context.Background()

	s.runPass(ctx, "progress_snapshot", func(context.Context) ([]any, error) {
		panic("первый тик сорвался")
	})
	s.runPass(ctx, "progress_snapshot", func(context.Context) ([]any, error) {
		return nil, nil
	})

	recs := records(t, buf)
	if find(recs, "background task panicked") == nil {
		t.Fatalf("нет записи о панике первого тика: %v", recs)
	}
	finished := find(recs, "background task finished")
	if finished == nil {
		t.Fatalf("второй тик не отработал после паники первого: %v", recs)
	}
	if finished["outcome"] != "ok" {
		t.Errorf("исход второго тика = %v, ожидался ok", finished["outcome"])
	}
}
