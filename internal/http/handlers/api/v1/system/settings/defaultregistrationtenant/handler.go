// Package defaultregistrationtenant serves the /api/v1/system/… endpoints under its URI segment.
package defaultregistrationtenant

import (
	"encoding/json"
	"net/http"

	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	settings systemcommon.SystemSettings
}

func New(settings systemcommon.SystemSettings) *Handler { return &Handler{settings: settings} }

// PUT /api/v1/system/settings/default-registration-tenant  {tenant_id|null}
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TenantID *int64 `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.settings.SystemSet(r.Context(), "default_registration_tenant_id", body.TenantID); err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
