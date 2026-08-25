package periods

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGateGetRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Get, http.MethodGet, "/api/v1/admin/periods", "")
	handlertest.IsError(t, w, http.StatusForbidden)
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGatePostRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Post, http.MethodGet, "/api/v1/admin/periods", "")
	handlertest.IsError(t, w, http.StatusForbidden)
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGatePatchRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Patch, http.MethodGet, "/api/v1/admin/periods/{periodID}", "",
		handlertest.URLParam("periodID", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGateDeleteRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Delete, http.MethodGet, "/api/v1/admin/periods/{periodID}", "",
		handlertest.URLParam("periodID", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}
