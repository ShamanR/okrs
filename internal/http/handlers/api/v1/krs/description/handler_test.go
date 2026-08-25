package description

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

// Неразбираемый krID в пути — это ошибка клиента, а не 404 и не 500.
func TestGatePostBadKrID(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Post, http.MethodGet, "/api/v1/krs/{krID}/description", "",
		handlertest.Tenant(1),
		handlertest.URLParam("krID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}
