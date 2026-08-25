// Package accessrequests serves its URI segment of the onboarding surface.
package accessrequests

import (
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/onboarding/onboardingcommon"
)

type Handler struct {
	onboard onboardingcommon.OnboardService
}

func New(onboard onboardingcommon.OnboardService) *Handler { return &Handler{onboard: onboard} }

// GET /api/v1/admin/access-requests
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		onboardingcommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	reqs, err := h.onboard.ListAccessRequests(r.Context(), scope)
	if err != nil {
		onboardingcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(reqs))
	for _, a := range reqs {
		out = append(out, map[string]any{
			"user_id": a.UserID, "display_name": a.DisplayName, "email": a.Email,
			"role": string(a.Role), "created_at": a.CreatedAt,
		})
	}
	onboardingcommon.WriteJSON(w, out)
}
