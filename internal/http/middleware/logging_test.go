package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/httperr"
	"okrs/internal/platform/logging"
)

func decode(t *testing.T, buf *bytes.Buffer) []map[string]any {
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

func byEvent(recs []map[string]any, event string) []map[string]any {
	var out []map[string]any
	for _, r := range recs {
		if r[logging.KeyEvent] == event {
			out = append(out, r)
		}
	}
	return out
}

func serve(t *testing.T, h http.HandlerFunc, req *http.Request) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	var handler http.Handler = h
	handler = Recovery(logger)(handler)
	handler = AccessLog(logger)(handler)
	handler = RequestID(handler)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, decode(t, buf)
}

// — request_id —

func TestRequestIDIsGeneratedAndReturned(t *testing.T) {
	var seen string
	rec, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		seen, _ = logging.RequestIDFromContext(r.Context())
	}, httptest.NewRequest(http.MethodGet, "/goals", nil))

	if seen == "" {
		t.Fatal("обработчик не увидел идентификатор запроса в контексте")
	}
	if got := rec.Header().Get(RequestIDHeader); got != seen {
		t.Errorf("в ответе %s = %q, ожидался %q", RequestIDHeader, got, seen)
	}
	if recs[0][logging.KeyRequestID] != seen {
		t.Errorf("в записи request_id = %v, ожидался %q", recs[0][logging.KeyRequestID], seen)
	}
}

func TestRequestIDIsInheritedFromCaller(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/goals", nil)
	req.Header.Set(RequestIDHeader, "upstream-abc.123_XYZ")

	rec, recs := serve(t, func(http.ResponseWriter, *http.Request) {}, req)

	if got := rec.Header().Get(RequestIDHeader); got != "upstream-abc.123_XYZ" {
		t.Errorf("идентификатор не унаследован: %q", got)
	}
	if recs[0][logging.KeyRequestID] != "upstream-abc.123_XYZ" {
		t.Errorf("в записи чужой идентификатор: %v", recs[0][logging.KeyRequestID])
	}
}

// Значение извне попадает в лог-запись, поэтому перевод строки в нём разорвал бы
// JSON-строку и позволил бы клиенту подделать соседние записи.
func TestHostileRequestIDIsRejected(t *testing.T) {
	cases := map[string]string{
		"перевод строки":   "abc\ndef",
		"кавычка и скобки": `abc","level":"ERROR`,
		"пробел":           "abc def",
		"переразмерный":    strings.Repeat("a", maxRequestIDLen+1),
		"пустой":           "",
		"кириллица":        "идентификатор",
	}
	for name, hostile := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/goals", nil)
			req.Header.Set(RequestIDHeader, hostile)

			rec, recs := serve(t, func(http.ResponseWriter, *http.Request) {}, req)

			got := rec.Header().Get(RequestIDHeader)
			if got == hostile {
				t.Fatalf("недопустимое значение принято: %q", got)
			}
			if !validRequestID(got) {
				t.Fatalf("сгенерированный идентификатор сам невалиден: %q", got)
			}
			if len(recs) != 1 {
				t.Fatalf("ожидалась одна запись, получено %d — значение разорвало вывод", len(recs))
			}
		})
	}
}

// — запись о запросе —

func TestRequestRecordLevelFollowsStatus(t *testing.T) {
	cases := []struct {
		status int
		level  string
	}{
		{http.StatusOK, "INFO"},
		{http.StatusFound, "INFO"},
		{http.StatusBadRequest, "WARN"},
		{http.StatusForbidden, "WARN"},
		{http.StatusNotFound, "WARN"},
		{http.StatusInternalServerError, "ERROR"},
		{http.StatusBadGateway, "ERROR"},
	}
	for _, c := range cases {
		_, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
		}, httptest.NewRequest(http.MethodGet, "/goals", nil))

		reqRecs := byEvent(recs, logging.EventHTTPRequest)
		if len(reqRecs) != 1 {
			t.Fatalf("статус %d: ожидалась ровно одна запись о запросе, получено %d", c.status, len(reqRecs))
		}
		if reqRecs[0]["level"] != c.level {
			t.Errorf("статус %d: уровень = %v, ожидался %s", c.status, reqRecs[0]["level"], c.level)
		}
		if reqRecs[0]["status"] != float64(c.status) {
			t.Errorf("статус в записи = %v, ожидался %d", reqRecs[0]["status"], c.status)
		}
	}
}

func TestRequestRecordOmitsQueryString(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/goals?token=s3cret&email=someone@example.com", nil)

	_, recs := serve(t, func(http.ResponseWriter, *http.Request) {}, req)

	rec := byEvent(recs, logging.EventHTTPRequest)[0]
	if rec["path"] != "/goals" {
		t.Errorf("path = %v, ожидался /goals без строки запроса", rec["path"])
	}
	for _, leak := range []string{"s3cret", "someone@example.com", "token="} {
		if strings.Contains(rec["path"].(string), leak) {
			t.Errorf("строка запроса утекла в путь: %v", rec["path"])
		}
	}
}

func TestImplicitOKIsRecorded(t *testing.T) {
	_, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}, httptest.NewRequest(http.MethodGet, "/goals", nil))

	if got := byEvent(recs, logging.EventHTTPRequest)[0]["status"]; got != float64(http.StatusOK) {
		t.Errorf("status = %v, ожидался 200", got)
	}
}

// — ошибки через общий error-writer —

func TestInternalErrorProducesExactlyOneRecordWithCodeAndCause(t *testing.T) {
	_, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		// Тот же путь, которым идут все обработчики: writer знать о логгере
		// не обязан.
		httperr.WriteJSON(w, http.StatusInternalServerError, "pq: relation \"goals\" does not exist")
	}, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))

	reqRecs := byEvent(recs, logging.EventHTTPRequest)
	if len(reqRecs) != 1 {
		t.Fatalf("ожидалась ровно одна запись об ошибке, получено %d: %v", len(reqRecs), recs)
	}
	rec := reqRecs[0]
	if rec["level"] != "ERROR" {
		t.Errorf("уровень = %v, ожидался ERROR", rec["level"])
	}
	if rec["error_code"] != "internal_error" {
		t.Errorf("error_code = %v, ожидался internal_error", rec["error_code"])
	}
	if rec["err"] != `pq: relation "goals" does not exist` {
		t.Errorf("техническая причина не попала в запись: %v", rec["err"])
	}
}

func TestInternalErrorCauseDoesNotReachTheClient(t *testing.T) {
	const cause = `pq: relation "goals" does not exist`
	rec, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		httperr.WriteJSON(w, http.StatusInternalServerError, cause)
	}, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))

	if strings.Contains(rec.Body.String(), cause) {
		t.Fatalf("техническая причина утекла в тело ответа: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), httperr.InternalErrorMessage) {
		t.Errorf("тело не содержит общий текст: %s", rec.Body.String())
	}
	if byEvent(recs, logging.EventHTTPRequest)[0]["err"] != cause {
		t.Error("причина обязана остаться в логе, раз её нет в ответе")
	}
}

// 4xx остаётся предупреждением и сохраняет текст для клиента: это его ошибка,
// а не сбой сервера.
func TestClientErrorKeepsItsMessage(t *testing.T) {
	rec, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		httperr.WriteJSON(w, http.StatusBadRequest, "team_id required")
	}, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))

	if !strings.Contains(rec.Body.String(), "team_id required") {
		t.Errorf("текст для клиента потерян: %s", rec.Body.String())
	}
	r := byEvent(recs, logging.EventHTTPRequest)[0]
	if r["level"] != "WARN" || r["error_code"] != "bad_request" {
		t.Errorf("уровень/код = %v/%v, ожидались WARN/bad_request", r["level"], r["error_code"])
	}
}

// Часть обработчиков отвечает напрямую через http.Error, мимо общих error-writer'ов.
// Такой ответ обязан всё равно нести машиночитаемый код: иначе выборка по error_code
// молча теряет часть отказов.
func TestDirectHTTPErrorStillGetsErrorCode(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not_found"},
		{http.StatusInternalServerError, "internal_error"},
	}
	for _, c := range cases {
		_, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"internal"}`, c.status)
		}, httptest.NewRequest(http.MethodGet, "/api/v1/session/tenants", nil))

		rec := byEvent(recs, logging.EventHTTPRequest)[0]
		if rec["error_code"] != c.code {
			t.Errorf("статус %d: error_code = %v, ожидался %q", c.status, rec["error_code"], c.code)
		}
	}
}

// Успешный ответ кода ошибки не получает: иначе он засоряет выборку отказов.
func TestSuccessfulResponseHasNoErrorCode(t *testing.T) {
	_, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))

	if got, ok := byEvent(recs, logging.EventHTTPRequest)[0]["error_code"]; ok {
		t.Errorf("успешный ответ не должен нести error_code: %v", got)
	}
}

// Обработчик, отвечающий через http.Error, может передать причину в обёртку ответа:
// она попадает в лог и не попадает в тело.
func TestCauseRecordedAlongsideDirectHTTPError(t *testing.T) {
	const cause = `pq: connection refused`
	rec, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError), errors.New(cause))
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	}, httptest.NewRequest(http.MethodGet, "/api/v1/session/tenants", nil))

	if strings.Contains(rec.Body.String(), cause) {
		t.Fatalf("причина утекла в тело ответа: %s", rec.Body.String())
	}
	r := byEvent(recs, logging.EventHTTPRequest)[0]
	if r["err"] != cause {
		t.Errorf("причина не попала в лог: %v", r["err"])
	}
	if r["error_code"] != "internal_error" || r["level"] != "ERROR" {
		t.Errorf("код/уровень = %v/%v", r["error_code"], r["level"])
	}
}

// — паника —

func TestPanicIsLoggedWithStackAndAnswered500(t *testing.T) {
	rec, recs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		panic("сломалось на ровном месте")
	}, httptest.NewRequest(http.MethodGet, "/goals", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("код ответа = %d, ожидался 500", rec.Code)
	}

	panics := byEvent(recs, logging.EventHTTPPanic)
	if len(panics) != 1 {
		t.Fatalf("ожидалась одна запись о панике, получено %d: %v", len(panics), recs)
	}
	if panics[0]["level"] != "ERROR" {
		t.Errorf("уровень = %v, ожидался ERROR", panics[0]["level"])
	}
	if !strings.Contains(panics[0]["stack"].(string), "logging_test.go") {
		t.Errorf("в записи нет стека вызовов: %v", panics[0]["stack"])
	}
	if panics[0][logging.KeyRequestID] == nil {
		t.Error("запись о панике не связана с запросом")
	}

	// Паника даёт две записи разных типов: сама паника со стеком и обычное
	// завершение запроса со статусом 500. Это разные события, а не дубль.
	reqRecs := byEvent(recs, logging.EventHTTPRequest)
	if len(reqRecs) != 1 || reqRecs[0]["status"] != float64(http.StatusInternalServerError) {
		t.Fatalf("паника не оставила записи о запросе со статусом 500: %v", recs)
	}
	if reqRecs[0]["level"] != "ERROR" {
		t.Errorf("уровень записи о запросе = %v, ожидался ERROR", reqRecs[0]["level"])
	}
}

// Recovery стоит снаружи и видит запрос до разрешения сессии и организации,
// поэтому берёт накопленный scope из обёртки ответа. Без этого запись
// о панике в защищённом обработчике не сказала бы, чью организацию затронуло.
func TestPanicRecordKeepsResolvedTenantAndActor(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	user := &domain.User{ID: 42, Provider: "github"}
	tenant := &domain.Tenant{ID: 7, Status: domain.TenantActive}

	var handler http.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("сломалось внутри защищённой группы")
	})
	handler = LogContext(handler)
	// Подменяет сессию и резолв организации: важно, что они кладут
	// значения в НОВЫЙ request, который внешний recovery не видит.
	handler = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithUser(r.Context(), user)
			ctx = auth.WithTenant(ctx, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}(handler)
	handler = Recovery(logger)(handler)
	handler = AccessLog(logger)(handler)
	handler = RequestID(handler)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))

	panics := byEvent(decode(t, buf), logging.EventHTTPPanic)
	if len(panics) != 1 {
		t.Fatalf("ожидалась одна запись о панике: %s", buf.String())
	}
	if panics[0][logging.KeyTenantID] != float64(7) {
		t.Errorf("tenant_id = %v, ожидалось 7", panics[0][logging.KeyTenantID])
	}
	if panics[0][logging.KeyActorID] != float64(42) {
		t.Errorf("actor_id = %v, ожидалось 42", panics[0][logging.KeyActorID])
	}
	if panics[0][logging.KeyRequestID] == nil {
		t.Error("запись о панике потеряла связь с запросом")
	}
}

func TestPanicDoesNotAffectTheNextRequest(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			panic("bang")
		}
		_, _ = w.Write([]byte("ok"))
	})
	handler = Recovery(logger)(handler)
	handler = AccessLog(logger)(handler)
	handler = RequestID(handler)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	after := httptest.NewRecorder()
	handler.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/goals", nil))
	if after.Code != http.StatusOK || after.Body.String() != "ok" {
		t.Fatalf("следующий запрос не обслужен: %d %q", after.Code, after.Body.String())
	}
}

// http.ErrAbortHandler — договорённый способ оборвать соединение молча; его
// перехват превратил бы намеренный разрыв в ложную ошибку сервера.
func TestAbortHandlerPanicIsNotSwallowed(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := Recovery(logging.New(logging.Config{Output: buf}))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))

	defer func() {
		if rv := recover(); rv != http.ErrAbortHandler {
			t.Fatalf("ErrAbortHandler не пропущен наружу: %v", rv)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

// — личность в записи —

func TestAuthenticatedUserAppearsInRequestRecord(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	user := &domain.User{ID: 42, Provider: "github", DisplayName: "Кто-то", IsSystemAdmin: true}
	tenant := &domain.Tenant{ID: 7, Status: domain.TenantActive}

	var handler http.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	handler = LogContext(handler)
	// Подменяет SessionMiddleware/TenantResolveMiddleware: важно, что они кладут
	// значения в НОВЫЙ request и передают его вниз.
	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithUser(r.Context(), user)
			ctx = auth.WithTenant(ctx, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	handler = inject(handler)
	handler = AccessLog(logger)(handler)
	handler = RequestID(handler)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/goals", nil))

	rec := byEvent(decode(t, buf), logging.EventHTTPRequest)[0]
	if rec["authenticated"] != true {
		t.Errorf("authenticated = %v, ожидалось true", rec["authenticated"])
	}
	if rec[logging.KeyActorID] != float64(42) {
		t.Errorf("actor_id = %v, ожидалось 42", rec[logging.KeyActorID])
	}
	if rec[logging.KeyTenantID] != float64(7) {
		t.Errorf("tenant_id = %v, ожидалось 7", rec[logging.KeyTenantID])
	}
	if rec["provider"] != "github" || rec["is_system_admin"] != true {
		t.Errorf("провайдер/системный админ = %v/%v", rec["provider"], rec["is_system_admin"])
	}
	if strings.Contains(buf.String(), "Кто-то") {
		t.Errorf("отображаемое имя попало в лог: %s", buf.String())
	}
}

func TestAnonymousRequestIsMarkedUnauthenticated(t *testing.T) {
	_, recs := serve(t, func(http.ResponseWriter, *http.Request) {}, httptest.NewRequest(http.MethodGet, "/goals", nil))

	rec := byEvent(recs, logging.EventHTTPRequest)[0]
	if rec["authenticated"] != false {
		t.Errorf("authenticated = %v, ожидалось false", rec["authenticated"])
	}
	if _, ok := rec[logging.KeyActorID]; ok {
		t.Errorf("неизвестный пользователь не должен получать actor_id: %v", rec[logging.KeyActorID])
	}
}

// — отказ в доступе —

func TestAccessDeniedIsLoggedAsItsOwnRecord(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	var handler http.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("обработчик не должен был выполниться")
	})
	handler = auth.RequireAuthMiddleware(handler)
	handler = LogContext(handler)
	handler = AccessLog(logger)(handler)
	handler = RequestID(handler)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("код ответа = %d, ожидался 401", rec.Code)
	}

	recs := decode(t, buf)
	denied := byEvent(recs, logging.EventAuthzDenied)
	if len(denied) != 1 {
		t.Fatalf("ожидалась одна запись об отказе, получено %d: %v", len(denied), recs)
	}
	if denied[0]["level"] != "WARN" {
		t.Errorf("уровень = %v, ожидался WARN", denied[0]["level"])
	}
	if denied[0]["reason"] != "no authenticated user" {
		t.Errorf("причина отказа = %v", denied[0]["reason"])
	}
	if denied[0]["path"] != "/api/v1/goals" {
		t.Errorf("запрошенный ресурс = %v", denied[0]["path"])
	}
	if denied[0][logging.KeyRequestID] == nil {
		t.Error("отказ не связан с запросом")
	}
}

func TestDeniedRecordCarriesUserIDWhenKnown(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	user := &domain.User{ID: 99, Provider: "github", DisplayName: "Админ"}

	var handler http.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("обработчик не должен был выполниться")
	})
	handler = auth.RequireTenantAdminMiddleware(handler)
	handler = LogContext(handler)
	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
		})
	}
	handler = inject(handler)
	handler = AccessLog(logger)(handler)
	handler = RequestID(handler)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/admin/teams", nil))

	denied := byEvent(decode(t, buf), logging.EventAuthzDenied)
	if len(denied) != 1 {
		t.Fatalf("ожидалась одна запись об отказе: %s", buf.String())
	}
	if denied[0][logging.KeyActorID] != float64(99) {
		t.Errorf("actor_id = %v, ожидалось 99", denied[0][logging.KeyActorID])
	}
	if strings.Contains(buf.String(), "Админ") {
		t.Errorf("отображаемое имя попало в лог: %s", buf.String())
	}
}

// — обёртка ответа —

func TestRecorderKeepsFirstErrorAndFirstStatus(t *testing.T) {
	rec := &Recorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}

	rec.RecordError("internal_error", errors.New("первая"))
	rec.RecordError("bad_request", errors.New("вторая"))
	rec.WriteHeader(http.StatusInternalServerError)
	rec.WriteHeader(http.StatusOK)

	if rec.errCode != "internal_error" || rec.errCause.Error() != "первая" {
		t.Errorf("побеждать должен первый вызов: %q/%v", rec.errCode, rec.errCause)
	}
	if rec.Status() != http.StatusInternalServerError {
		t.Errorf("status = %d, ожидался 500", rec.Status())
	}
}

// Обёртка не должна отбирать у обработчика Flush: без Unwrap
// http.ResponseController перестал бы находить исходный writer.
func TestRecorderUnwrapsToTheOriginalWriter(t *testing.T) {
	inner := httptest.NewRecorder()
	rec := &Recorder{ResponseWriter: inner}

	if rec.Unwrap() != http.ResponseWriter(inner) {
		t.Error("Unwrap не вернул исходный writer")
	}
	if err := http.NewResponseController(rec).Flush(); err != nil {
		t.Errorf("Flush через обёртку не работает: %v", err)
	}
}

// Записи, которые обработчик пишет сам, обязаны нести идентификатор запроса:
// без этого их нельзя связать с завершением запроса и между собой.
func TestHandlerRecordsInheritTheRequestID(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{Output: buf})

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Контекстный вариант — то, на что переведены вызовы в слоях http и usecase.
		logging.FromContext(r.Context()).InfoContext(r.Context(), "работа обработчика",
			slog.String(logging.KeyEvent, logging.EventDomainEvent))
	})
	handler = LogContext(handler)
	handler = AccessLog(logger)(handler)
	handler = RequestID(handler)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/goals", nil))

	id := rec.Header().Get(RequestIDHeader)
	recs := decode(t, buf)
	if len(recs) != 2 {
		t.Fatalf("ожидались запись обработчика и запись о запросе, получено %d", len(recs))
	}
	for _, r := range recs {
		if r[logging.KeyRequestID] != id {
			t.Errorf("запись %q не связана с запросом: %v", r["msg"], r[logging.KeyRequestID])
		}
	}
}
