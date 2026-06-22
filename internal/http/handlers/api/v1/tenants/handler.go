package tenants

import (
	"context"
	"encoding/json"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/domain"
)

// MembershipLookup lists a user's active memberships.
type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}

// TenantLookup loads tenants by slug or id.
type TenantLookup interface {
	GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	GetByID(ctx context.Context, id int64) (*domain.Tenant, error)
}

// SessionWriter persists the session's active tenant.
type SessionWriter interface {
	SetActiveTenant(ctx context.Context, sessionID string, tenantID int64) error
}

type Handler struct {
	members  MembershipLookup
	tenants  TenantLookup
	sessions SessionWriter
}

func New(m MembershipLookup, t TenantLookup, s SessionWriter) *Handler {
	return &Handler{members: m, tenants: t, sessions: s}
}

type switchRequest struct {
	Slug     string `json:"slug"`
	TenantID int64  `json:"tenant_id"`
}

type tenantDTO struct {
	ID     int64  `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// SwitchTenant sets the session's active tenant after verifying active membership.
func (h *Handler) SwitchTenant(w http.ResponseWriter, r *http.Request) {
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

// ListMyTenants returns the current user's active-membership tenants for the switcher.
func (h *Handler) ListMyTenants(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var activeID int64
	if sess := auth.SessionFromContext(r.Context()); sess != nil && sess.ActiveTenantID != nil {
		activeID = *sess.ActiveTenantID
	}

	ms, err := h.members.ListByUser(r.Context(), user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	out := make([]tenantDTO, 0, len(ms))
	for _, m := range ms {
		tn, err := h.tenants.GetByID(r.Context(), m.TenantID)
		if err != nil {
			continue
		}
		out = append(out, tenantDTO{ID: tn.ID, Slug: tn.Slug, Name: tn.Name, Active: tn.ID == activeID})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
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
