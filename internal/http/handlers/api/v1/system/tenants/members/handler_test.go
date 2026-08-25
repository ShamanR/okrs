package members

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

// Неразбираемый userID в пути — это ошибка клиента, а не 404 и не 500.
func TestGateDeleteBadUserID(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Delete, http.MethodGet, "/api/v1/system/tenants/{id}/members/{userID}", "",
		handlertest.Tenant(1),
		handlertest.URLParam("userID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}
