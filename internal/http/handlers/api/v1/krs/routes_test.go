package krs

import (
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "okrs/internal/http/handlers/api/v1"

	"github.com/go-chi/chi/v5"
)

func TestRegisterRoutes(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, v1.NewHandler(nil))
	v1.RegisterMethodNotAllowed(r)

	req := httptest.NewRequest(http.MethodGet, "/krs/1/comments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
