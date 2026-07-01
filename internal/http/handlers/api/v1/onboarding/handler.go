// Package onboarding implements the HTTP surface for the onboarding primitives: tenant-admin
// invitations + access-request queue, and the user self-service join-request.
package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store/memberships"

	"github.com/go-chi/chi/v5"
)

// invitationStore covers tenant invitation persistence. *store.InvitationRepository satisfies it.
type invitationStore interface {
	Create(ctx context.Context, scope domain.TenantScope, role domain.Role, tokenHash string, createdBy int64, maxUses *int, expiresAt *time.Time) (*domain.Invitation, error)
	ListPendingByTenant(ctx context.Context, scope domain.TenantScope) ([]domain.Invitation, error)
	Revoke(ctx context.Context, scope domain.TenantScope, id int64) error
}

// onboardService covers the join-request + access-request flows. *service.OnboardingService satisfies it.
type onboardService interface {
	RequestAccess(ctx context.Context, slug string, userID int64) error
	ListAccessRequests(ctx context.Context, scope domain.TenantScope) ([]memberships.AccessRequest, error)
	ApproveRequest(ctx context.Context, scope domain.TenantScope, userID int64) error
	DenyRequest(ctx context.Context, scope domain.TenantScope, userID int64) error
}

type Handler struct {
	invites invitationStore
	onboard onboardService
	baseURL string
}

func New(invites invitationStore, onboard onboardService, baseURL string) *Handler {
	return &Handler{invites: invites, onboard: onboard, baseURL: baseURL}
}

// POST /api/v1/admin/invitations  {role?, max_uses?, expires_at?}
func (h *Handler) HandleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	var body struct {
		Role      string     `json:"role"`
		MaxUses   *int       `json:"max_uses"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.MaxUses != nil && *body.MaxUses <= 0 {
		writeError(w, http.StatusBadRequest, "max_uses must be positive")
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

	raw, hash, err := service.GenerateInviteToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.invites.Create(r.Context(), scope, role, hash, createdBy, body.MaxUses, body.ExpiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"token":    raw,
		"url":      h.baseURL + "/invite/" + raw,
		"role":     string(role),
		"max_uses": body.MaxUses,
	})
}

// POST /api/v1/admin/invitations/{id}/revoke
func (h *Handler) HandleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.invites.Revoke(r.Context(), scope, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/admin/invitations
func (h *Handler) HandleListInvitations(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	list, err := h.invites.ListPendingByTenant(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	writeJSON(w, out)
}

// GET /api/v1/admin/access-requests
func (h *Handler) HandleListAccessRequests(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	reqs, err := h.onboard.ListAccessRequests(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(reqs))
	for _, a := range reqs {
		out = append(out, map[string]any{
			"user_id": a.UserID, "display_name": a.DisplayName, "email": a.Email,
			"role": string(a.Role), "created_at": a.CreatedAt,
		})
	}
	writeJSON(w, out)
}

// POST /api/v1/admin/access-requests/{userID}/approve
func (h *Handler) HandleApproveAccessRequest(w http.ResponseWriter, r *http.Request) {
	h.accessRequestAction(w, r, h.onboard.ApproveRequest)
}

// POST /api/v1/admin/access-requests/{userID}/deny
func (h *Handler) HandleDenyAccessRequest(w http.ResponseWriter, r *http.Request) {
	h.accessRequestAction(w, r, h.onboard.DenyRequest)
}

func (h *Handler) accessRequestAction(w http.ResponseWriter, r *http.Request, fn func(context.Context, domain.TenantScope, int64) error) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := fn(r.Context(), scope, userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/onboarding/join-request  {slug}
func (h *Handler) HandleJoinRequest(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Slug == "" {
		writeError(w, http.StatusBadRequest, "slug required")
		return
	}
	err := h.onboard.RequestAccess(r.Context(), body.Slug, user.ID)
	switch {
	case errors.Is(err, service.ErrTenantNotFound):
		writeError(w, http.StatusNotFound, "tenant not found")
	case errors.Is(err, service.ErrAlreadyMember):
		writeError(w, http.StatusConflict, "already a member")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
