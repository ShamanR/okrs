// Package tenants serves the /api/v1/system/… endpoints under its URI segment.
package tenants

import (
	"encoding/json"
	"errors"
	"net/http"

	"okrs/internal/http/handlers/api/v1/system/systemcommon"
	"okrs/internal/store/tenants"
)

type Handler struct {
	prov    systemcommon.Provisioner
	tenants systemcommon.TenantLister
}

func New(prov systemcommon.Provisioner, tenants systemcommon.TenantLister) *Handler {
	return &Handler{prov: prov, tenants: tenants}
}

// POST /api/v1/system/tenants  {name, slug, entitlements?}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string         `json:"name"`
		Slug         string         `json:"slug"`
		Entitlements map[string]any `json:"entitlements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	tn, err := h.prov.CreateTenant(r.Context(), body.Name, body.Slug)
	if err != nil {
		switch {
		case errors.Is(err, tenants.ErrInvalidSlug):
			systemcommon.WriteError(w, http.StatusUnprocessableEntity, "invalid slug")
		case errors.Is(err, tenants.ErrSlugTaken):
			systemcommon.WriteError(w, http.StatusConflict, "slug already taken")
		default:
			systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if len(body.Entitlements) > 0 {
		if err := h.prov.SetEntitlements(r.Context(), tn.ID, body.Entitlements); err != nil {
			systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusCreated)
	systemcommon.WriteJSON(w, systemcommon.ToTenantDTO(tn))
}

// GET /api/v1/system/tenants
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	list, err := h.tenants.List(r.Context())
	if err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]systemcommon.TenantDTO, 0, len(list))
	for i := range list {
		out = append(out, systemcommon.ToTenantDTO(&list[i]))
	}
	systemcommon.WriteJSON(w, out)
}

// PATCH /api/v1/system/tenants/{id}  {name, slug}
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	tn, err := h.prov.UpdateTenant(r.Context(), tenantID, body.Name, body.Slug)
	if err != nil {
		switch {
		case errors.Is(err, tenants.ErrNotFound):
			systemcommon.WriteError(w, http.StatusNotFound, "tenant not found")
		case errors.Is(err, tenants.ErrSlugTaken):
			systemcommon.WriteError(w, http.StatusConflict, "slug already taken")
		case errors.Is(err, tenants.ErrInvalidSlug):
			systemcommon.WriteError(w, http.StatusUnprocessableEntity, "invalid slug")
		case errors.Is(err, tenants.ErrInvalidName):
			systemcommon.WriteError(w, http.StatusUnprocessableEntity, "invalid name")
		default:
			systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	systemcommon.WriteJSON(w, systemcommon.ToTenantDTO(tn))
}
