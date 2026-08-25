package goals_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/goals"
	"okrs/internal/http/handlers/api/v1/goals/comments"

	"github.com/go-chi/chi/v5"
)

// A URI that exists but does not accept the requested method must answer 405, not 404.
// The distinction matters to clients: 404 says "no such goal", 405 says "wrong verb".
//
// Both packages are registered because the URI now spans two of them: /api/v1/goals
// mounts the goal itself, /api/v1/goals/…/comments mounts the comment collection.
// Nil dependencies are fine — routing is resolved before any handler runs.
func TestMethodNotAllowedOnComments(t *testing.T) {
	r := chi.NewRouter()
	goals.RegisterRoutes(r, goals.New(nil, nil, nil, nil, nil))
	comments.RegisterRoutes(r, comments.New(nil, nil, nil))
	v1.RegisterMethodNotAllowed(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/goals/1/comments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
