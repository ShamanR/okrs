// Package invitations serves its URI segment of the onboarding surface.
package invitations

import (
	"encoding/json"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/onboarding/onboardingcommon"
	onboardingsvc "okrs/internal/service/onboarding"
	"time"
)

type Handler struct {
	baseURL string
	invites onboardingcommon.InvitationStore
}

func New(invites onboardingcommon.InvitationStore, baseURL string) *Handler {
	return &Handler{invites: invites, baseURL: baseURL}
}

// POST /api/v1/admin/invitations  {role?, max_uses?, expires_at?}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		onboardingcommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	var body struct {
		Role      string     `json:"role"`
		MaxUses   *int       `json:"max_uses"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		onboardingcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.MaxUses != nil && *body.MaxUses <= 0 {
		onboardingcommon.WriteError(w, http.StatusBadRequest, "max_uses must be positive")
		return
	}
	role := domain.Role(body.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		role = domain.RoleUser
	}
	var createdBy int64
	if u := auth.UserFromContext(r.Context()); u != nil {
		createdBy = u.ID
	}

	raw, hash, err := onboardingsvc.GenerateInviteToken()
	if err != nil {
		onboardingcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.invites.Create(r.Context(), scope, role, hash, createdBy, body.MaxUses, body.ExpiresAt); err != nil {
		onboardingcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	onboardingcommon.WriteJSON(w, map[string]any{
		"token":    raw,
		"url":      onboardingcommon.InviteBaseURL(r, h.baseURL) + "/invite/" + raw,
		"role":     string(role),
		"max_uses": body.MaxUses,
	})
}

// GET /api/v1/admin/invitations
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		onboardingcommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	list, err := h.invites.ListPendingByTenant(r.Context(), scope)
	if err != nil {
		onboardingcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, inv := range list {
		out = append(out, map[string]any{
			"id": inv.ID, "role": string(inv.Role), "status": string(inv.Status),
			"max_uses": inv.MaxUses, "use_count": inv.UseCount,
			"created_at": inv.CreatedAt, "expires_at": inv.ExpiresAt,
		})
	}
	onboardingcommon.WriteJSON(w, out)
}
