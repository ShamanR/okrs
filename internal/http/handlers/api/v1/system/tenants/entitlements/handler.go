// Package entitlements serves the /api/v1/system/… endpoints under its URI segment.
package entitlements

import (
	"encoding/json"
	"net/http"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	prov     systemcommon.Provisioner
	settings systemcommon.SystemSettings
}

func New(prov systemcommon.Provisioner, settings systemcommon.SystemSettings) *Handler {
	return &Handler{prov: prov, settings: settings}
}

// PUT /api/v1/system/tenants/{id}/entitlements  { "sso": true, "max_users": 50 }
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.prov.SetEntitlements(r.Context(), tenantID, body); err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/system/tenants/{id}/entitlements
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	ent, err := h.settings.TenantEntitlements(r.Context(), domain.TenantScope{TenantID: id})
	if err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ent == nil {
		ent = map[string]json.RawMessage{}
	}
	systemcommon.WriteJSON(w, ent)
}
