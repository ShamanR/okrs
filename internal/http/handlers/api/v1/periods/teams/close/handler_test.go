package close

// Обёртка над общим телом bulkstatus: проверяются его гейты. Целевой статус
// (closed) — единственное, чем этот пакет отличается от соседнего; он уходит
// в usecase, который здесь не подставить (конкретный тип), поэтому статус
// покрыт интеграционно, а тут — разбор пути и tenant-гейт.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

const uri = "/api/v1/periods/1/teams/close"

func TestRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Post, http.MethodPost, uri, "",
		handlertest.URLParam("periodID", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}

func TestBadPeriodIDIs400(t *testing.T) {
	w := handlertest.Do(New(nil, nil, nil).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("periodID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}
