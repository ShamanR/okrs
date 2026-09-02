package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"okrs/internal/platform/logging"
)

func decodeRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	var out []map[string]any
	for i, line := range strings.Split(raw, "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка %d не является валидным JSON: %v\n%s", i, err, line)
		}
		out = append(out, rec)
	}
	return out
}

// findRecord ищет первую запись с заданными типом и сообщением.
func findRecord(recs []map[string]any, event, msg string) map[string]any {
	for _, r := range recs {
		if r[logging.KeyEvent] == event && r["msg"] == msg {
			return r
		}
	}
	return nil
}

// Отказ на этапе запуска обязан оставить запись уровня error с причиной — иначе
// упавший под в Kubernetes молча уходит в CrashLoopBackOff без объяснения.
func TestStartupFailureIsLoggedBeforeExit(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		msg  string
	}{
		{
			name: "некорректная таймзона",
			env:  map[string]string{"TZ": "Нигде/Ничего"},
			msg:  "invalid timezone",
		},
		{
			name: "нечитаемый адрес базы данных",
			env:  map[string]string{"DATABASE_URL": "это не url"},
			msg:  "failed to connect db",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			buf := &bytes.Buffer{}

			if code := runWith(buf, false); code != 1 {
				t.Fatalf("код возврата = %d, ожидался 1", code)
			}

			recs := decodeRecords(t, buf)
			if findRecord(recs, logging.EventAppStart, "starting") == nil {
				t.Errorf("нет записи о старте приложения: %v", recs)
			}
			rec := findRecord(recs, logging.EventAppStart, c.msg)
			if rec == nil {
				t.Fatalf("нет записи об отказе %q: %v", c.msg, recs)
			}
			if rec["level"] != "ERROR" {
				t.Errorf("уровень = %v, ожидался ERROR", rec["level"])
			}
			if rec["err"] == nil && rec["tz"] == nil {
				t.Errorf("запись об отказе не содержит причину: %v", rec)
			}
		})
	}
}

func TestShutdownSequenceLogsEveryStepOnCleanStop(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	shutdownSequence(logger,
		func(context.Context) error { return nil },
		func(time.Duration) error { return nil },
	)

	recs := decodeRecords(t, buf)
	for _, msg := range []string{"shutdown signal received", "http server stopped serving", "event bus drained"} {
		rec := findRecord(recs, logging.EventAppShutdown, msg)
		if rec == nil {
			t.Errorf("нет записи %q: %v", msg, recs)
			continue
		}
		if rec["level"] != "INFO" {
			t.Errorf("запись %q имеет уровень %v, ожидался INFO", msg, rec["level"])
		}
	}
}

func TestShutdownSequenceWarnsWhenStepDoesNotComplete(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	shutdownSequence(logger,
		func(context.Context) error { return errors.New("контекст истёк") },
		func(time.Duration) error { return errors.New("дренаж не уложился в таймаут") },
	)

	recs := decodeRecords(t, buf)
	cases := map[string]string{
		"http shutdown did not complete cleanly": "контекст истёк",
		"event bus did not drain cleanly":        "дренаж не уложился в таймаут",
	}
	for msg, cause := range cases {
		rec := findRecord(recs, logging.EventAppShutdown, msg)
		if rec == nil {
			t.Errorf("нет записи %q: %v", msg, recs)
			continue
		}
		if rec["level"] != "WARN" {
			t.Errorf("запись %q имеет уровень %v, ожидался WARN", msg, rec["level"])
		}
		if rec["err"] != cause {
			t.Errorf("запись %q не содержит причину %q: %v", msg, cause, rec["err"])
		}
	}
}

func TestShutdownSequencePassesTimeoutsToSteps(t *testing.T) {
	buf := &bytes.Buffer{}
	var gotDrainTimeout time.Duration
	var hadDeadline bool

	shutdownSequence(logging.New(logging.Config{Output: buf}),
		func(ctx context.Context) error {
			_, hadDeadline = ctx.Deadline()
			return nil
		},
		func(d time.Duration) error {
			gotDrainTimeout = d
			return nil
		},
	)

	if !hadDeadline {
		t.Error("остановка приёма запросов должна получать контекст с таймаутом")
	}
	if gotDrainTimeout != busDrainTimeout {
		t.Errorf("таймаут дренажа = %v, ожидался %v", gotDrainTimeout, busDrainTimeout)
	}
}
