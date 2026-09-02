package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// decodeLines разбирает вывод как последовательность JSON-записей, по одной
// на строку, и заодно проверяет само это свойство: запись, размазанная по
// нескольким строкам, не разберётся системой сбора логов.
func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	out := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка %d не является валидным JSON-объектом: %v\n%s", i, err, line)
		}
		out = append(out, rec)
	}
	return out
}

func newTestLogger(t *testing.T, cfg Config) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	cfg.Output = buf
	return New(cfg), buf
}

func TestNewWritesSingleLineJSONByDefault(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})

	logger.Info("привет", slog.String(KeyEvent, EventAppStart))

	recs := decodeLines(t, buf)
	if len(recs) != 1 {
		t.Fatalf("ожидалась одна запись, получено %d", len(recs))
	}
	if recs[0]["msg"] != "привет" {
		t.Errorf("msg = %v, ожидалось \"привет\"", recs[0]["msg"])
	}
}

func TestNewTextFormatKeepsSameFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(Config{Format: "text", Service: "okrs", Env: "test", Output: buf})

	logger.Info("привет", slog.String(KeyEvent, EventAppStart))

	out := buf.String()
	if json.Valid([]byte(strings.TrimSpace(out))) {
		t.Errorf("текстовый формат не должен давать JSON: %s", out)
	}
	for _, want := range []string{KeyEvent + "=" + EventAppStart, KeyService + "=okrs", KeyEnv + "=test"} {
		if !strings.Contains(out, want) {
			t.Errorf("в текстовом выводе нет %q: %s", want, out)
		}
	}
}

func TestRequiredFieldsPresentOnEveryRecord(t *testing.T) {
	logger, buf := newTestLogger(t, Config{Service: "okrs", Env: "test"})

	logger.Info("любая запись", slog.String(KeyEvent, EventAppReady))

	rec := decodeLines(t, buf)[0]
	for _, key := range []string{"time", "level", "msg", KeyEvent, KeyService, KeyEnv} {
		if _, ok := rec[key]; !ok {
			t.Errorf("в записи нет обязательного поля %q: %v", key, rec)
		}
	}
}

func TestRecordWithoutEventGetsUnspecifiedMarker(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})

	logger.Info("автор забыл тип записи")

	if got := decodeLines(t, buf)[0][KeyEvent]; got != EventUnspecified {
		t.Errorf("%s = %v, ожидалось %q", KeyEvent, got, EventUnspecified)
	}
}

func TestEventFromWithIsNotDuplicated(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})

	logger.With(slog.String(KeyEvent, EventBackgroundTask)).Info("задача")

	rec := decodeLines(t, buf)[0]
	if got := rec[KeyEvent]; got != EventBackgroundTask {
		t.Errorf("%s = %v, ожидалось %q", KeyEvent, got, EventBackgroundTask)
	}
	if n := strings.Count(buf.String(), `"`+KeyEvent+`"`); n != 1 {
		t.Errorf("поле %s встречается %d раз, ожидался 1: %s", KeyEvent, n, buf.String())
	}
}

func TestMultilineValueStaysOnOneLine(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})
	stack := "goroutine 1 [running]:\nmain.main()\n\t/app/main.go:1 +0x20"

	logger.Error("паника", slog.String(KeyEvent, EventHTTPPanic), slog.String("stack", stack))

	recs := decodeLines(t, buf)
	if len(recs) != 1 {
		t.Fatalf("многострочное значение разорвало запись на %d строк", len(recs))
	}
	if recs[0]["stack"] != stack {
		t.Errorf("стек исказился при экранировании: %v", recs[0]["stack"])
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{"", DefaultLevel, false},
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"INFO", slog.LevelInfo, false},
		{" warn ", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"verbose", DefaultLevel, true},
		{"7", DefaultLevel, true},
	}
	for _, c := range cases {
		got, err := ParseLevel(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseLevel(%q) err = %v, ожидалась ошибка = %v", c.in, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("ParseLevel(%q) = %v, ожидалось %v", c.in, got, c.want)
		}
	}
}

func TestDefaultLevelHidesDebug(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})

	logger.Debug("отладка")
	logger.Info("информация")

	recs := decodeLines(t, buf)
	if len(recs) != 1 || recs[0]["msg"] != "информация" {
		t.Fatalf("на уровне по умолчанию должна остаться только info-запись: %v", recs)
	}
}

func TestDebugLevelShowsEveryLevel(t *testing.T) {
	logger, buf := newTestLogger(t, Config{Level: "debug"})

	logger.Debug("отладка")
	logger.Info("информация")
	logger.Warn("предупреждение")
	logger.Error("ошибка")

	if recs := decodeLines(t, buf); len(recs) != 4 {
		t.Fatalf("ожидалось 4 записи, получено %d", len(recs))
	}
}

func TestInvalidLevelFallsBackToInfoAndWarns(t *testing.T) {
	logger, buf := newTestLogger(t, Config{Level: "лишь бы что"})

	recs := decodeLines(t, buf)
	if len(recs) != 1 {
		t.Fatalf("ожидалось предупреждение о некорректной настройке, получено %d записей", len(recs))
	}
	if recs[0]["level"] != "WARN" || recs[0][KeyEvent] != EventConfigInvalid {
		t.Errorf("ожидалось WARN/%s, получено %v/%v", EventConfigInvalid, recs[0]["level"], recs[0][KeyEvent])
	}

	buf.Reset()
	logger.Debug("отладка")
	logger.Info("информация")
	if recs := decodeLines(t, buf); len(recs) != 1 || recs[0]["msg"] != "информация" {
		t.Errorf("после отката должен действовать уровень info: %v", recs)
	}
}

func TestInvalidFormatFallsBackToJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(Config{Format: "xml", Output: buf})
	logger.Info("проверка", slog.String(KeyEvent, EventAppStart))

	recs := decodeLines(t, buf)
	if len(recs) != 2 {
		t.Fatalf("ожидались предупреждение и запись, получено %d", len(recs))
	}
	if recs[0][KeyEvent] != EventConfigInvalid {
		t.Errorf("первой записью ожидалось предупреждение о формате: %v", recs[0])
	}
}

// Диагностика о некорректной настройке обязана быть видна при ЛЮБОМ допустимом
// уровне. Иначе она бесполезна ровно тогда, когда нужна: при LOG_LEVEL=error
// предупреждение о некорректном LOG_FORMAT отфильтровывалось бы тем самым
// уровнем, о соседе которого сообщает.
func TestConfigDiagnosticIsNotFilteredByTheConfiguredLevel(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			buf := &bytes.Buffer{}
			New(Config{Level: level, Format: "xml", Output: buf})

			recs := decodeLines(t, buf)
			if len(recs) != 1 {
				t.Fatalf("ожидалось предупреждение о некорректном формате, получено %d записей: %s",
					len(recs), buf.String())
			}
			if recs[0][KeyEvent] != EventConfigInvalid || recs[0]["level"] != "WARN" {
				t.Errorf("event/level = %v/%v, ожидались %s/WARN", recs[0][KeyEvent], recs[0]["level"], EventConfigInvalid)
			}
		})
	}
}

// Обход порога распространяется только на диагностику конфигурации: обычные
// записи по-прежнему подчиняются настроенному уровню.
func TestConfiguredLevelStillFiltersOrdinaryRecords(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(Config{Level: "error", Format: "xml", Output: buf})
	buf.Reset()

	logger.Warn("обычное предупреждение")
	logger.Info("обычная информация")
	logger.Error("настоящая ошибка")

	recs := decodeLines(t, buf)
	if len(recs) != 1 || recs[0]["msg"] != "настоящая ошибка" {
		t.Fatalf("на уровне error должна остаться только error-запись: %v", recs)
	}
}

func TestAllEventsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, e := range AllEvents() {
		if e == "" {
			t.Error("пустой идентификатор типа записи")
		}
		if seen[e] {
			t.Errorf("дублирующийся идентификатор типа записи %q", e)
		}
		seen[e] = true
	}
}

func TestIsDeniedKey(t *testing.T) {
	denied := []string{
		"password", "Password", "user_password", "passwd",
		"token", "access_token", "refresh-token", "Token",
		"secret", "secret_key", "client.secret", "NotificationSecret",
		"authorization", "Authorization", "cookie", "Set-Cookie",
		"api_key", "apiKey", "credentials", "private_key", "session_id",
		"email", "user_email", "OwnerEmail", "webhook_url",
	}
	for _, k := range denied {
		if !IsDeniedKey(k) {
			t.Errorf("ключ %q должен подлежать редакции", k)
		}
	}

	allowed := []string{
		"time", "level", "msg", KeyEvent, KeyService, KeyEnv,
		KeyRequestID, KeyTenantID, KeyActorID, KeyTeamID, KeyPeriodID,
		"status", "method", "path", "duration_ms", "goal_id", "channel", "err", "",
	}
	for _, k := range allowed {
		if IsDeniedKey(k) {
			t.Errorf("ключ %q не должен подлежать редакции", k)
		}
	}
}

func TestRedactionHidesOriginalValue(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})
	const secret = "ыыы-очень-секретное-значение"

	logger.Info("настройка изменена",
		slog.String(KeyEvent, EventAccessChanged),
		slog.String("token", secret),
		slog.String("user_email", "someone@example.com"),
		slog.String("goal_id", "42"),
	)

	out := buf.String()
	if strings.Contains(out, secret) || strings.Contains(out, "someone@example.com") {
		t.Fatalf("секрет или PII попали в вывод: %s", out)
	}
	rec := decodeLines(t, buf)[0]
	if rec["token"] != Mask || rec["user_email"] != Mask {
		t.Errorf("ожидалась маска %q, получено token=%v user_email=%v", Mask, rec["token"], rec["user_email"])
	}
	if rec["goal_id"] != "42" {
		t.Errorf("несекретное значение не должно маскироваться: %v", rec["goal_id"])
	}
}

// Адрес почты попадает в лог не отдельным атрибутом, а внутри чужого текста:
// внешний канал адресует получателя путём /api/v4/users/email/<escaped email>
// и оборачивает этот путь в текст ошибки. Редакция по имени ключа такое не видит.
func TestEmailInsideAnotherValueIsMasked(t *testing.T) {
	cases := map[string]string{
		"в тексте ошибки внешнего канала": `mattermost: /api/v4/users/email/admin%40example.com: status 404`,
		"в открытом виде":                 "не найден получатель admin@example.com",
		"в пути запроса":                  "/api/v1/users/a.b+c@sub.example.co.uk",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			logger, buf := newTestLogger(t, Config{})

			logger.Error("внешний вызов не удался",
				slog.String(KeyEvent, EventExternalCall),
				slog.String("err", value))

			out := buf.String()
			for _, leak := range []string{"admin@example.com", "admin%40example.com", "a.b+c@sub.example.co.uk"} {
				if strings.Contains(out, leak) {
					t.Fatalf("адрес попал в лог (%q): %s", leak, out)
				}
			}
			if !strings.Contains(out, Mask) {
				t.Errorf("адрес не заменён маской: %s", out)
			}
		})
	}
}

// Маскирование по содержимому не должно трогать обычные значения: оно узкое,
// только про адреса почты.
func TestNonEmailValuesAreUntouched(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})

	logger.Info("запрос",
		slog.String(KeyEvent, EventHTTPRequest),
		slog.String("path", "/api/v1/goals/42"),
		slog.String("err", "pq: relation \"goals\" does not exist"),
		slog.String("host", "mm.internal:8065"))

	rec := decodeLines(t, buf)[0]
	if rec["path"] != "/api/v1/goals/42" {
		t.Errorf("path изменён: %v", rec["path"])
	}
	if rec["err"] != `pq: relation "goals" does not exist` {
		t.Errorf("текст ошибки изменён: %v", rec["err"])
	}
	if rec["host"] != "mm.internal:8065" {
		t.Errorf("host изменён: %v", rec["host"])
	}
}

func TestContextFieldsAddedOnContextualCall(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})
	team := int64(7)
	period := int64(9)
	ctx := WithScope(WithRequestID(context.Background(), "req-1"), Scope{
		TenantID: 1, ActorID: 2, TeamID: &team, PeriodID: &period,
	})

	logger.InfoContext(ctx, "действие", slog.String(KeyEvent, EventDomainEvent))

	rec := decodeLines(t, buf)[0]
	want := map[string]any{
		KeyRequestID: "req-1",
		KeyTenantID:  float64(1),
		KeyActorID:   float64(2),
		KeyTeamID:    float64(7),
		KeyPeriodID:  float64(9),
	}
	for k, v := range want {
		if rec[k] != v {
			t.Errorf("%s = %v, ожидалось %v", k, rec[k], v)
		}
	}
}

func TestMissingContextIsOmittedNotZeroed(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})
	ctx := WithScope(context.Background(), Scope{TenantID: 5})

	logger.InfoContext(ctx, "действие вне команды и периода", slog.String(KeyEvent, EventDomainEvent))

	rec := decodeLines(t, buf)[0]
	if rec[KeyTenantID] != float64(5) {
		t.Errorf("%s = %v, ожидалось 5", KeyTenantID, rec[KeyTenantID])
	}
	for _, key := range []string{KeyRequestID, KeyActorID, KeyTeamID, KeyPeriodID} {
		if _, ok := rec[key]; ok {
			t.Errorf("отсутствующее поле %q не должно подставляться: %v", key, rec[key])
		}
	}
}

func TestContextFieldsSurviveWithAndGroupless(t *testing.T) {
	logger, buf := newTestLogger(t, Config{})
	ctx := WithRequestID(context.Background(), "req-2")

	logger.With(slog.String("component", "handler")).InfoContext(ctx, "запись")

	rec := decodeLines(t, buf)[0]
	if rec[KeyRequestID] != "req-2" {
		t.Errorf("%s = %v после With, ожидалось req-2", KeyRequestID, rec[KeyRequestID])
	}
}

func TestFromContextFallsBackToDefault(t *testing.T) {
	if FromContext(context.Background()) != slog.Default() {
		t.Error("без логгера в контексте ожидался slog.Default()")
	}
	custom := Discard()
	if FromContext(WithLogger(context.Background(), custom)) != custom {
		t.Error("логгер из контекста не вернулся")
	}
}
