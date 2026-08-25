// Package members serves the /api/v1/system/… endpoints under its URI segment.
package members

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	members systemcommon.MemberLister
	prov    systemcommon.Provisioner
}

func New(members systemcommon.MemberLister, prov systemcommon.Provisioner) *Handler {
	return &Handler{members: members, prov: prov}
}

// POST /api/v1/system/tenants/{id}/members  {user_id, role}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	var body struct {
		UserID int64  `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.UserID == 0 {
		systemcommon.WriteError(w, http.StatusBadRequest, "user_id required")
		return
	}
	role := domain.Role(body.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		role = domain.RoleUser
	}
	m, err := h.prov.AttachMember(r.Context(), tenantID, body.UserID, role)
	if err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	systemcommon.WriteJSON(w, map[string]any{
		"user_id": m.UserID, "tenant_id": m.TenantID, "role": string(m.Role), "status": string(m.Status),
	})
}

// GET /api/v1/system/tenants/{id}/members
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	list, err := h.members.ListByTenant(r.Context(), domain.TenantScope{TenantID: id})
	if err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, map[string]any{
			"user_id": m.UserID, "display_name": m.DisplayName, "email": m.Email,
			"role": string(m.Role), "status": string(m.Status),
		})
	}
	systemcommon.WriteJSON(w, out)
}

// DELETE /api/v1/system/tenants/{id}/members/{userID}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.prov.RemoveMember(r.Context(), id, userID); err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
