package stats

// Тест переехал из пакета admin вместе с обработчиком GET /api/v1/admin/periods/stats.

import (
	"net/http"
	"net/http/httptest"
	perioduc "okrs/internal/usecase/period"
	"testing"
)

func TestHandlePeriodStats_ForbiddenWithoutScope(t *testing.T) {
	h := New(perioduc.New(perioduc.Deps{}), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/periods/stats", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}
