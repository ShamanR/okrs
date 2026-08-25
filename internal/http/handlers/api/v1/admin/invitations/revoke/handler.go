// Package revoke serves its URI segment of the onboarding surface.
package revoke

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/onboarding/onboardingcommon"
	"strconv"
)

type Handler struct {
	invites onboardingcommon.InvitationStore
}

func New(invites onboardingcommon.InvitationStore) *Handler { return &Handler{invites: invites} }

// POST /api/v1/admin/invitations/{id}/revoke
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		onboardingcommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		onboardingcommon.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.invites.Revoke(r.Context(), scope, id); err != nil {
		onboardingcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
