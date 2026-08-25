package overview

// Тест переехал из пакета admin вместе с обработчиком GET /api/v1/admin/periods/{periodID}/overview.

import (
	"context"
	"net/http"
	"net/http/httptest"
	perioduc "okrs/internal/usecase/period"
	"testing"

	"github.com/go-chi/chi/v5"
)

// withURLParam injects a chi URL param into the request context, mimicking chi's router.
func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// Without a tenant scope in context, admin period endpoints must 403.
func TestHandlePeriodOverview_ForbiddenWithoutScope(t *testing.T) {
	h := New(perioduc.New(perioduc.Deps{}), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/periods/1/overview", nil)
	req = withURLParam(req, "periodID", "1")
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}
