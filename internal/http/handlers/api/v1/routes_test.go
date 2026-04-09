package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRegisterHierarchyRoutes(t *testing.T) {
	h := NewHandler(nil)
	r := chi.NewRouter()
	RegisterHierarchyRoutes(r, h)
	RegisterMethodNotAllowed(r)

	req := httptest.NewRequest(http.MethodPost, "/hierarchy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /hierarchy POST, got %d", w.Code)
	}
}

func TestRegisterPeriodsRoutes(t *testing.T) {
	h := NewHandler(nil)
	r := chi.NewRouter()
	RegisterPeriodsRoutes(r, h)
	RegisterMethodNotAllowed(r)

	req := httptest.NewRequest(http.MethodPost, "/periods", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /periods POST, got %d", w.Code)
	}
}

func TestRegisterTeamsRoutes(t *testing.T) {
	h := NewHandler(nil)
	r := chi.NewRouter()
	RegisterTeamsRoutes(r, h)
	RegisterMethodNotAllowed(r)

	req := httptest.NewRequest(http.MethodGet, "/teams/1/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /teams/{teamID}/status GET, got %d", w.Code)
	}
}

func TestRegisterGoalsRoutes(t *testing.T) {
	h := NewHandler(nil)
	r := chi.NewRouter()
	RegisterGoalsRoutes(r, h)
	RegisterMethodNotAllowed(r)

	req := httptest.NewRequest(http.MethodGet, "/goals/1/comments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /goals/{goalID}/comments GET, got %d", w.Code)
	}
}

func TestRegisterKeyResultsRoutes(t *testing.T) {
	h := NewHandler(nil)
	r := chi.NewRouter()
	RegisterKeyResultsRoutes(r, h)
	RegisterMethodNotAllowed(r)

	req := httptest.NewRequest(http.MethodGet, "/krs/1/comments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /krs/{krID}/comments GET, got %d", w.Code)
	}
}
