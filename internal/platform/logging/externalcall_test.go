package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func callRecord(t *testing.T, err error, extra ...any) (map[string]any, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	ctx := WithLogger(context.Background(), New(Config{Output: buf}))

	ExternalCall(ctx, "notification_channel", 120*time.Millisecond, err, extra...)

	var rec map[string]any
	if e := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); e != nil {
		t.Fatalf("запись не является валидным JSON: %v\n%s", e, buf.String())
	}
	return rec, buf.String()
}

func TestSuccessfulExternalCallIsLogged(t *testing.T) {
	rec, _ := callRecord(t, nil, slog.String("channel", "mattermost"))

	if rec[KeyEvent] != EventExternalCall {
		t.Errorf("event = %v, ожидался %q", rec[KeyEvent], EventExternalCall)
	}
	if rec["level"] != "INFO" || rec["outcome"] != "ok" {
		t.Errorf("уровень/исход = %v/%v, ожидались INFO/ok", rec["level"], rec["outcome"])
	}
	if rec["target"] != "notification_channel" || rec["channel"] != "mattermost" {
		t.Errorf("цель обращения не названа: %v", rec)
	}
	if rec["duration_ms"] != float64(120) {
		t.Errorf("duration_ms = %v, ожидалось 120", rec["duration_ms"])
	}
}

func TestFailedExternalCallIsLoggedWithCause(t *testing.T) {
	rec, _ := callRecord(t, errors.New("mattermost: status 401"), slog.String("channel", "mattermost"))

	if rec["level"] != "ERROR" || rec["outcome"] != "failed" {
		t.Errorf("уровень/исход = %v/%v, ожидались ERROR/failed", rec["level"], rec["outcome"])
	}
	if rec["err"] != "mattermost: status 401" {
		t.Errorf("причина отказа не записана: %v", rec["err"])
	}
}

func TestRetryAttemptIsRecordedWhenTheCallerRetries(t *testing.T) {
	rec, _ := callRecord(t, errors.New("временный сбой"), slog.Int("attempt", 2))

	if rec["attempt"] != float64(2) {
		t.Errorf("attempt = %v, ожидалось 2", rec["attempt"])
	}
}

// Учётные данные и адрес подключения с ними не должны доходить до лога: даже если
// вызывающий код передаст их атрибутом, редакция по имени ключа их скроет.
func TestExternalCallRecordHidesCredentials(t *testing.T) {
	const secret = "xoxb-очень-секретный-токен"
	_, out := callRecord(t, nil,
		slog.String("token", secret),
		slog.String("webhook_url", "https://user:"+secret+"@mm.internal/hooks/abc"),
		slog.String("channel", "mattermost"),
	)

	for _, leak := range []string{secret, "user:", "hooks/abc"} {
		if strings.Contains(out, leak) {
			t.Errorf("учётные данные попали в лог (%q): %s", leak, out)
		}
	}
	if !strings.Contains(out, Mask) {
		t.Errorf("значения не заменены маской: %s", out)
	}
}
