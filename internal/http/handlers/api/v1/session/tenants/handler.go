// Package tenants serves /api/v1/session/… under its URI segment.
package tenants

import (
	"context"
	"encoding/json"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/store/memberships"
)

// MembershipLookup lists a user's memberships.
type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
	ListByUserWithTenant(ctx context.Context, userID int64) ([]memberships.MembershipWithTenant, error)
}

// MembershipLeaver removes the caller's own membership. *service.OnboardingService satisfies it.
type MembershipLeaver interface {
	LeaveTenant(ctx context.Context, tenantID, userID int64) error
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
type tenantDTO struct {
	ID     int64  `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type Handler struct {
	members MembershipLookup
	tenants TenantLookup
}

func New(members MembershipLookup, tenants TenantLookup) *Handler {
	return &Handler{members: members, tenants: tenants}
}

// ListMyTenants returns the current user's active-membership tenants for the switcher.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
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
