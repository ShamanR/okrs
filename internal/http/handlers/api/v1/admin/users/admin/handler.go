// Package adminrole serves the /api/v1/admin/… endpoints under its URI segment.
package adminrole

import (
	"net/http"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
)

type Handler struct {
	roles admincommon.MemberRoleSetter
}

func New(roles admincommon.MemberRoleSetter) *Handler { return &Handler{roles: roles} }

// POST /api/v1/admin/users/{userID}/admin  — grant admin in the active tenant
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	admincommon.SetMemberRole(w, r, h.roles, domain.RoleAdmin)
}

// DELETE /api/v1/admin/users/{userID}/admin  — revoke admin in the active tenant
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	admincommon.SetMemberRole(w, r, h.roles, domain.RoleUser)
}
