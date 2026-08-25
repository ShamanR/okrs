// Package handlertest собирает HTTP-запросы для тестов обработчиков и проверяет
// ответы. Существует потому, что после раскладки «пакет на URI» почти каждый
// handler-пакет проверяет одно и то же: закрыт ли эндпоинт без tenant-scope,
// как разбирается путь и во что маппятся ошибки сервиса. Без общего хелпера эта
// обвязка копировалась бы в 80+ файлов.
//
// Пакет не тянет БД и не знает про конкретные обработчики: в него передаётся
// http.HandlerFunc, а зависимости обработчика тест подставляет сам.
package handlertest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
)

// Option настраивает запрос: контекст аутентификации, scope, параметры пути.
type Option func(*http.Request) *http.Request

// Tenant кладёт активный tenant в контекст — без него TenantScopeFromContext
// возвращает ok=false и обработчик обязан ответить 403.
func Tenant(id int64) Option {
	return func(r *http.Request) *http.Request {
		return r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: id, Status: domain.TenantActive}))
	}
}

// User кладёт аутентифицированного пользователя.
func User(udid string) Option {
	return func(r *http.Request) *http.Request {
		return r.WithContext(auth.WithUser(r.Context(), &domain.User{ID: 1, UDID: udid}))
	}
}

// UserID кладёт пользователя с заданным id (нужно там, где обработчик сравнивает id,
// а не UDID).
func UserID(id int64, udid string) Option {
	return func(r *http.Request) *http.Request {
		return r.WithContext(auth.WithUser(r.Context(), &domain.User{ID: id, UDID: udid}))
	}
}

// Role задаёт роль в активном tenant: часть эндпоинтов пускает org-scope только админу.
func Role(role domain.Role) Option {
	return func(r *http.Request) *http.Request {
		return r.WithContext(auth.WithActiveRole(r.Context(), role))
	}
}

// AllowedTeams ограничивает видимость списком команд. nil означает «без ограничений»
// (админ), пустой слайс — «доступа нет».
func AllowedTeams(ids []int64) Option {
	return func(r *http.Request) *http.Request {
		return r.WithContext(auth.WithAllowedTeamIDs(r.Context(), ids))
	}
}

// URLParam подставляет параметр пути так же, как это делает роутер chi.
func URLParam(key, value string) Option {
	return func(r *http.Request) *http.Request {
		c := chi.RouteContext(r.Context())
		if c == nil {
			c = chi.NewRouteContext()
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, c))
		}
		c.URLParams.Add(key, value)
		return r
	}
}

// Do собирает запрос, прогоняет его через обработчик и возвращает записанный ответ.
// body может быть пустым — тогда тела нет.
func Do(h http.HandlerFunc, method, target, body string, opts ...Option) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	// Пустой chi-контекст нужен всегда: обработчики зовут chi.URLParam, а он на
	// запросе без RouteContext паникует.
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	for _, o := range opts {
		r = o(r)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

// Form собирает form-urlencoded запрос — так ходят SSR-обработчики.
func Form(h http.HandlerFunc, method, target, body string, opts ...Option) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	for _, o := range opts {
		r = o(r)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

// Status проверяет код ответа и печатает тело при расхождении — без тела
// диагностировать 500 вместо 200 невозможно.
func Status(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d (тело: %s)", w.Code, want, w.Body.String())
	}
}

// ErrorCode проверяет код ответа и машинный код ошибки в теле.
func ErrorCode(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	Status(t, w, wantStatus)
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ответ не разобрался как ошибка: %v (тело: %s)", err, w.Body.String())
	}
	if resp.Error.Code != wantCode {
		t.Fatalf("код ошибки = %q, want %q", resp.Error.Code, wantCode)
	}
}

// DecodeJSON разбирает тело ответа в out.
func DecodeJSON(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("не разобрался JSON: %v (тело: %s)", err, w.Body.String())
	}
}

// Body возвращает тело ответа строкой.
func Body(w *httptest.ResponseRecorder) string {
	b, _ := io.ReadAll(w.Result().Body)
	return string(b)
}

// В API сейчас живут два конверта ошибки: основной /api/v1 отдаёт
// {"error":{"code","message","fields"}} через v1.WriteError, а планы admin/system/
// onboarding — плоский {"error":"текст"} через свои WriteError. Хелперы ниже
// принимают оба, чтобы тест проверял поведение эндпоинта, а не эту разницу.
// Сама разница — долг, помеченный TODO в спеке 040.

// errorText достаёт текст или код ошибки из любого из двух конвертов.
func errorText(body []byte) (code string, ok bool) {
	var structured struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &structured); err == nil && structured.Error.Code != "" {
		return structured.Error.Code, true
	}
	var flat struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &flat); err == nil && flat.Error != "" {
		return flat.Error, true
	}
	return "", false
}

// IsError проверяет код ответа и то, что тело — это ошибка в одном из конвертов.
func IsError(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	Status(t, w, wantStatus)
	if _, ok := errorText(w.Body.Bytes()); !ok {
		t.Fatalf("тело не похоже на ошибку: %s", w.Body.String())
	}
}

// RequiresTenantScope — общий тест для любого эндпоинта под tenant-гейтом: без
// tenant в контексте обработчик обязан ответить 403 FORBIDDEN, а не 500 и не 200.
// Проверка одинакова для всех, поэтому живёт здесь, а не копируется по пакетам.
func RequiresTenantScope(t *testing.T, h http.HandlerFunc, method, target string, opts ...Option) {
	t.Helper()
	w := Do(h, method, target, "", opts...)
	IsError(t, w, http.StatusForbidden)
}
