// Package settings serves the /api/v1/system/… endpoints under its URI segment.
package settings

import (
	"encoding/json"
	"net/http"

	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	settings systemcommon.SystemSettings
}

func New(settings systemcommon.SystemSettings) *Handler { return &Handler{settings: settings} }

// GET /api/v1/system/settings
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	raw, err := h.settings.SystemGet(r.Context(), "default_registration_tenant_id")
	if err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var tenantID *int64
	if raw != nil {
		_ = json.Unmarshal(raw, &tenantID)
	}
	msgRaw, err := h.settings.SystemGet(r.Context(), "no_access_message")
	if err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var msg string
	if msgRaw != nil {
		_ = json.Unmarshal(msgRaw, &msg)
	}
	systemcommon.WriteJSON(w, map[string]any{
		"default_registration_tenant_id": tenantID,
		"no_access_message":              msg,
	})
}
