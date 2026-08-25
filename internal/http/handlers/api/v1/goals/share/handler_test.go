package share

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

// Неразбираемый goalID в пути — это ошибка клиента, а не 404 и не 500.
func TestGatePostBadGoalID(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Post, http.MethodGet, "/api/v1/goals/{goalID}/share", "",
		handlertest.Tenant(1),
		handlertest.URLParam("goalID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGatePostRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Post, http.MethodGet, "/api/v1/goals/{goalID}/share", "",
		handlertest.URLParam("goalID", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}

// Неразбираемый goalID в пути — это ошибка клиента, а не 404 и не 500.
func TestGateDeleteBadGoalID(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Delete, http.MethodGet, "/api/v1/goals/{goalID}/share/{teamID}", "",
		handlertest.Tenant(1),
		handlertest.URLParam("goalID", "не-число"),
		handlertest.URLParam("teamID", "1"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGateDeleteRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Delete, http.MethodGet, "/api/v1/goals/{goalID}/share/{teamID}", "",
		handlertest.URLParam("goalID", "1"),
		handlertest.URLParam("teamID", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}
