package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/service"
)

// Without a tenant scope in context, admin period endpoints must 403.
func TestHandlePeriodOverview_ForbiddenWithoutScope(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/periods/1/overview", nil)
	req = withURLParam(req, "periodID", "1")
	w := httptest.NewRecorder()
	h.HandlePeriodOverview(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestHandlePeriodStats_ForbiddenWithoutScope(t *testing.T) {
	h := NewServiceHandler(service.New(service.Deps{}), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/periods/stats", nil)
	w := httptest.NewRecorder()
	h.HandlePeriodStats(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}
