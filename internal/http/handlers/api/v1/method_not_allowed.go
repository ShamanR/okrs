package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func RegisterMethodNotAllowed(r chi.Router) {
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	})
}
