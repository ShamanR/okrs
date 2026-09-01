// Package read serves POST /api/v1/notifications/read — marking notifications read.
package read

import (
	"context"
	"encoding/json"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
)

// ReadMarker is the port this handler needs. *notification.Service satisfies it.
type ReadMarker interface {
	MarkRead(ctx context.Context, scope domain.TenantScope, userID int64, ids []int64, all bool) error
}

type Handler struct{ svc ReadMarker }

func New(svc ReadMarker) *Handler { return &Handler{svc: svc} }

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if !req.All && len(req.IDs) == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "ids or all required",
			map[string]string{"ids": "required"})
		return
	}
	// The service scopes the update by user_id, so one user can never mark another's.
	if err := h.svc.MarkRead(r.Context(), scope, auth.UserIDFromContext(r.Context()), req.IDs, req.All); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to mark read", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
