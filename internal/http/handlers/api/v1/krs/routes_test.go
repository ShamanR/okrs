package krs_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/krs"
	"okrs/internal/http/handlers/api/v1/krs/note"

	"github.com/go-chi/chi/v5"
)

// A URI that exists but does not accept the requested method must answer 405, not 404 —
// the distinction tells a client "wrong verb" apart from "no such key result".
//
// Both packages are registered because the URI spans two of them after the split:
// /api/v1/krs mounts the key result itself, /api/v1/krs/…/note mounts its note.
// Nil dependencies are fine — routing resolves before any handler body runs.
func TestMethodNotAllowedOnNote(t *testing.T) {
	r := chi.NewRouter()
	krs.RegisterRoutes(r, krs.New(nil, nil, nil))
	note.RegisterRoutes(r, note.New(nil, nil, nil))
	v1.RegisterMethodNotAllowed(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/krs/1/note", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
