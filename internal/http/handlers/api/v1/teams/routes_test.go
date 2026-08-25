package teams_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/teams"
	"okrs/internal/http/handlers/api/v1/teams/status"

	"github.com/go-chi/chi/v5"
)

// GET on a POST-only route must answer 405, not 404: the distinction tells a client
// "wrong verb" apart from "no such team".
//
// Both packages are mounted because the URI spans them after the split: /api/v1/teams
// mounts the team, /api/v1/teams/…/status mounts its period status.
func TestMethodNotAllowedOnStatus(t *testing.T) {
	r := chi.NewRouter()
	teams.RegisterRoutes(r, teams.New(nil))
	status.RegisterRoutes(r, status.New(nil))
	v1.RegisterMethodNotAllowed(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/teams/1/status", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
