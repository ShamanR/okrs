// Package memberships serves /api/v1/session/… under its URI segment.
package memberships

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/http/httperr"
	"okrs/internal/service/provisioning"
	"okrs/internal/store/memberships"
	"strconv"
)

// MembershipLookup lists the caller's memberships together with their tenants.
type MembershipLookup interface {
	ListByUserWithTenant(ctx context.Context, userID int64) ([]memberships.MembershipWithTenant, error)
}

// MembershipLeaver removes the caller's own membership. *provisioning.Service satisfies it.
type MembershipLeaver interface {
	LeaveTenant(ctx context.Context, tenantID, userID int64) error
}

type membershipDTO struct {
	TenantID int64  `json:"tenant_id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type Handler struct {
	members MembershipLookup
	leaver  MembershipLeaver
}

func New(members MembershipLookup, leaver MembershipLeaver) *Handler {
	return &Handler{members: members, leaver: leaver}
}

// Get returns the caller's memberships (all statuses) for /settings.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	list, err := h.members.ListByUserWithTenant(r.Context(), user.ID)
	if err != nil {
		// Причина уходит в итоговую запись о запросе, а не в тело ответа.
		_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError), err)
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

// Delete lets the caller leave a tenant / cancel their pending request.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
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
	case errors.Is(err, provisioning.ErrLastAdmin):
		http.Error(w, `{"error":"last admin cannot leave"}`, http.StatusConflict)
	case err != nil:
		_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError), err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
