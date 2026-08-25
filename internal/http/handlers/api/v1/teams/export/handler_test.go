package export

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

// Неразбираемый teamID в пути — это ошибка клиента, а не 404 и не 500.
func TestGateGetBadTeamID(t *testing.T) {
	w := handlertest.Do(New(nil).Get, http.MethodGet, "/api/v1/teams/{teamID}/export", "",
		handlertest.Tenant(1),
		handlertest.URLParam("teamID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGateGetRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Get, http.MethodGet, "/api/v1/teams/{teamID}/export", "",
		handlertest.URLParam("teamID", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}
