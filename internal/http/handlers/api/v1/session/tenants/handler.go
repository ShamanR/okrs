// Package tenants serves /api/v1/session/… under its URI segment.
package tenants

import (
	"context"
	"encoding/json"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/httperr"
)

// MembershipLookup lists the caller's memberships so the switcher can name the tenants.
type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}

// TenantLookup resolves a membership's tenant id to the tenant itself.
type TenantLookup interface {
	GetByID(ctx context.Context, id int64) (*domain.Tenant, error)
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

// Get returns the current user's active-membership tenants for the switcher.
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
		// Причина уходит в итоговую запись о запросе, а не в тело ответа.
		_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError), err)
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
