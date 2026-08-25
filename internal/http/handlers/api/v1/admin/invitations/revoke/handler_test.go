package revoke

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGatePostRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Post, http.MethodGet, "/api/v1/admin/invitations/{id}/revoke", "",
		handlertest.URLParam("id", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}
