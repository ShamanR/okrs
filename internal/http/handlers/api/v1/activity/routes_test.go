package activity_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/activity"
	"okrs/internal/http/handlers/api/v1/activity/categorycounts"
	"okrs/internal/http/handlers/api/v1/activity/treecounts"

	"github.com/go-chi/chi/v5"
)

// POST to a GET-only route must answer 405, which also proves the path is registered:
// an unregistered path would answer 404 and the assertion would fail for the wrong reason.
//
// All three packages are mounted because the /api/v1/activity URI space spans them after
// the split. Nil dependencies are fine — routing resolves before any handler body runs.
func TestMethodNotAllowed(t *testing.T) {
	r := chi.NewRouter()
	activity.RegisterRoutes(r, activity.New(nil))
	treecounts.RegisterRoutes(r, treecounts.New(nil))
	categorycounts.RegisterRoutes(r, categorycounts.New(nil))
	v1.RegisterMethodNotAllowed(r)

	for _, uri := range []string{
		"/api/v1/activity",
		"/api/v1/activity/tree-counts",
		"/api/v1/activity/category-counts",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, uri, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s: expected 405, got %d", uri, w.Code)
		}
	}
}
