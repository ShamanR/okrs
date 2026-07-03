package tenants

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/service"
	"okrs/internal/store/memberships"

	"github.com/go-chi/chi/v5"
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

type Handler struct {
	members  MembershipLookup
	tenants  TenantLookup
	sessions SessionWriter
	leaver   MembershipLeaver
}

func New(m MembershipLookup, t TenantLookup, s SessionWriter, l MembershipLeaver) *Handler {
	return &Handler{members: m, tenants: t, sessions: s, leaver: l}
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

type membershipDTO struct {
	TenantID int64  `json:"tenant_id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

// ListMyMemberships returns the caller's memberships (all statuses) for /settings.
func (h *Handler) ListMyMemberships(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	list, err := h.members.ListByUserWithTenant(r.Context(), user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	out := make([]membershipDTO, 0, len(list))
	for _, m := range list {
		out = append(out, membershipDTO{TenantID: m.TenantID, Slug: m.Slug, Name: m.Name, Role: string(m.Role), Status: string(m.Status)})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// LeaveTenant lets the caller leave a tenant / cancel their pending request.
func (h *Handler) LeaveTenant(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	tenantID, err := strconv.ParseInt(chi.URLParam(r, "tenantID"), 10, 64)
	if err != nil || tenantID <= 0 {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	switch err := h.leaver.LeaveTenant(r.Context(), tenantID, user.ID); {
	case errors.Is(err, service.ErrLastAdmin):
		http.Error(w, `{"error":"last admin cannot leave"}`, http.StatusConflict)
	case err != nil:
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
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
