// Package joinrequest serves its URI segment of the onboarding surface.
package joinrequest

import (
	"encoding/json"
	"errors"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/onboarding/onboardingcommon"
	onboardingsvc "okrs/internal/service/onboarding"
)

type Handler struct {
	onboard onboardingcommon.OnboardService
}

func New(onboard onboardingcommon.OnboardService) *Handler { return &Handler{onboard: onboard} }

// POST /api/v1/onboarding/join-request  {slug}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		onboardingcommon.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		onboardingcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Slug == "" {
		onboardingcommon.WriteError(w, http.StatusBadRequest, "slug required")
		return
	}
	err := h.onboard.RequestAccess(r.Context(), body.Slug, user.ID)
	switch {
	case errors.Is(err, onboardingsvc.ErrTenantNotFound):
		onboardingcommon.WriteError(w, http.StatusNotFound, "tenant not found")
	case errors.Is(err, onboardingsvc.ErrAlreadyMember):
		onboardingcommon.WriteError(w, http.StatusConflict, "already a member")
	case err != nil:
		onboardingcommon.WriteError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
