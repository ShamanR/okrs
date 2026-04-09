package hierarhy

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

	req := httptest.NewRequest(http.MethodPost, "/hierarchy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
