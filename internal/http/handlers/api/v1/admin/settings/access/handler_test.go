package access

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
	w := handlertest.Do(New(nil).Get, http.MethodGet, "/api/v1/admin/settings/access", "")
	handlertest.IsError(t, w, http.StatusForbidden)
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403.
// Тело передаётся валидное: здесь оно разбирается до гейта, и с мусором
// в теле ответ был бы 400 и гейт остался бы непроверенным.
func TestGatePostRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Post, http.MethodGet, "/api/v1/admin/settings/access", `{}`)
	handlertest.IsError(t, w, http.StatusForbidden)
}
