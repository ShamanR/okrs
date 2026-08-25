// Package noaccessmessage serves the /api/v1/system/… endpoints under its URI segment.
package noaccessmessage

import (
	"encoding/json"
	"net/http"

	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	settings systemcommon.SystemSettings
}

func New(settings systemcommon.SystemSettings) *Handler { return &Handler{settings: settings} }

// PUT /api/v1/system/settings/no-access-message  {message}
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.settings.SystemSet(r.Context(), "no_access_message", body.Message); err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
