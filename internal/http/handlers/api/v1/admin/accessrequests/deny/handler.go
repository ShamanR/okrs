// Package deny serves its URI segment of the onboarding surface.
package deny

import (
	"net/http"
	"okrs/internal/http/handlers/api/v1/onboarding/onboardingcommon"
)

type Handler struct {
	onboard onboardingcommon.OnboardService
}

func New(onboard onboardingcommon.OnboardService) *Handler { return &Handler{onboard: onboard} }

// POST /api/v1/admin/access-requests/{userID}/deny
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	onboardingcommon.AccessRequestAction(w, r, h.onboard.DenyRequest)
}
