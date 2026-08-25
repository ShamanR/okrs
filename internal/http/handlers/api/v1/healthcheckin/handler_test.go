package healthcheckin

// Порядок проверок: period_id → аутентификация → tenant-scope. Тесты каждого
// гейта подставляют валидными все предыдущие.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

const uri = "/api/v1/health-checkin"

// Health check-in считается всегда в рамках периода: без него запрос
// бессмысленен, поэтому period_id обязателен, а не подставляется по умолчанию.
func TestPeriodIDRequired(t *testing.T) {
	for _, q := range []string{"", "?period_id=", "?period_id=не-число", "?period_id=0", "?period_id=-1"} {
		t.Run(q, func(t *testing.T) {
			w := handlertest.Do(New(nil, nil, nil).Get, http.MethodGet, uri+q, "",
				handlertest.Tenant(1), handlertest.User("u-1"))
			handlertest.IsError(t, w, http.StatusBadRequest)
		})
	}
}

// Лента здоровья персональная (считается от UDID смотрящего), поэтому
// анонимный запрос — 401, а не пустой результат.
func TestAnonymousIs401(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Get, http.MethodGet, uri+"?period_id=1", "",
		handlertest.Tenant(1))
	handlertest.IsError(t, w, http.StatusUnauthorized)
}

func TestRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Get, http.MethodGet, uri+"?period_id=1", "",
		handlertest.User("u-1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}
