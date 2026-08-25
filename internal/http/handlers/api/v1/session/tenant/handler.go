// Package tenant serves /api/v1/session/… under its URI segment.
package tenant

import (
	"context"
	"encoding/json"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
)

// MembershipLookup lists the caller's memberships so the switch target can be verified.
type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}

// TenantLookup resolves the switch target when it arrives as a slug.
type TenantLookup interface {
	GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
}

// SessionWriter persists the session's active tenant.
type SessionWriter interface {
	SetActiveTenant(ctx context.Context, sessionID string, tenantID int64) error
}

type switchRequest struct {
	Slug     string `json:"slug"`
	TenantID int64  `json:"tenant_id"`
}

type Handler struct {
	members  MembershipLookup
	tenants  TenantLookup
	sessions SessionWriter
}

func New(members MembershipLookup, tenants TenantLookup, sessions SessionWriter) *Handler {
	return &Handler{members: members, tenants: tenants, sessions: sessions}
}

// Post sets the session's active tenant after verifying active membership.
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	sess := auth.SessionFromContext(r.Context())
	if user == nil || sess == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req switchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	targetID := req.TenantID
	if req.Slug != "" {
		tn, err := h.tenants.GetBySlug(r.Context(), req.Slug)
		if err != nil {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		targetID = tn.ID
	}
	if targetID == 0 {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	if !h.isActiveMember(r.Context(), user.ID, targetID) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	if err := h.sessions.SetActiveTenant(r.Context(), sess.ID, targetID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) isActiveMember(ctx context.Context, userID, tenantID int64) bool {
	ms, err := h.members.ListByUser(ctx, userID)
	if err != nil {
		return false
	}
	for _, m := range ms {
		if m.TenantID == tenantID {
			return true
		}
	}
	return false
}
