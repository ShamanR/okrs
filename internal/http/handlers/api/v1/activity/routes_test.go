package activity

import (
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "okrs/internal/http/handlers/api/v1"

	"github.com/go-chi/chi/v5"
)

func TestRegisterRoutes(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, New(nil))
	v1.RegisterMethodNotAllowed(r)

	// POST to a GET-only route → 405 confirms the path is registered.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /api/v1/activity, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/activity/tree-counts", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /api/v1/activity/tree-counts, got %d", w.Code)
	}
}
