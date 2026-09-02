package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

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

// Отсутствующий логгер не должен ронять фоновую задачу: Deps.Logger — nil в части
// тестов, а падение планировщика из-за логирования было бы худшим из исходов.
func TestBackgroundPassSurvivesNilLogger(t *testing.T) {
	s := New(Deps{})
	s.runPass(context.Background(), "progress_snapshot", func(context.Context) ([]any, error) {
		return nil, errors.New("любая ошибка")
	})
}
