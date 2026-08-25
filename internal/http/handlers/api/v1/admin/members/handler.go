// Package members serves its URI segment of the onboarding surface.
package members

import (
	"net/http"
	"okrs/internal/http/handlers/api/v1/onboarding/onboardingcommon"
)

type Handler struct {
	onboard onboardingcommon.OnboardService
}

func New(onboard onboardingcommon.OnboardService) *Handler { return &Handler{onboard: onboard} }

// DELETE /api/v1/admin/members/{userID} — unlink a user from the active tenant.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	onboardingcommon.AccessRequestAction(w, r, h.onboard.RemoveMember)
}
