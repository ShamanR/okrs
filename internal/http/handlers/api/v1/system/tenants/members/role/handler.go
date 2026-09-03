// Package role serves the /api/v1/system/… endpoints under its URI segment.
package role

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/system/systemcommon"
	"okrs/internal/platform/logging"
	"okrs/internal/service/provisioning"
	"okrs/internal/store/memberships"
)

type Handler struct {
	prov systemcommon.Provisioner
}

func New(prov systemcommon.Provisioner) *Handler { return &Handler{prov: prov} }

// PUT /api/v1/system/tenants/{id}/members/{userID}/role  {role}
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	role := domain.Role(body.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		systemcommon.WriteError(w, http.StatusUnprocessableEntity, "invalid role")
		return
	}
	switch err := h.prov.SetMemberRole(r.Context(), tenantID, userID, role); {
	case errors.Is(err, memberships.ErrNotFound):
		systemcommon.WriteError(w, http.StatusNotFound, "membership not found")
	case errors.Is(err, provisioning.ErrLastAdmin):
		systemcommon.WriteError(w, http.StatusConflict, "cannot demote the last admin")
	case err != nil:
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
	default:
		logging.AccessChanged(r.Context(), "member_role_set",
			slog.Int64(logging.KeyTenantID, tenantID),
			slog.Int64("target_user_id", userID),
			slog.String("role", string(role)))
		w.WriteHeader(http.StatusNoContent)
	}
}
