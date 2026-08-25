// Package users serves the /api/v1/admin/… endpoints under its URI segment.
package users

import (
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
)

type Handler struct {
	grants admincommon.GrantsStore
	users  admincommon.UserAdminStore
}

func New(grants admincommon.GrantsStore, users admincommon.UserAdminStore) *Handler {
	return &Handler{grants: grants, users: users}
}

// GET /api/v1/admin/users
// Tenant-scoped: only the active tenant's members and access requesters, each carrying its
// membership Status/Role. Active members are augmented with their granted-node count.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	tenantUsers, err := h.users.ListByTenant(r.Context(), scope)
	if err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	allGrants, err := h.grants.AllGrants(r.Context())
	if err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Grants to soft-deleted teams stay in the table but expand to no visible
	// access (descendant expansion filters deleted teams, and on soft delete a
	// team's children are reparented away). Resolve the still-active granted
	// teams in one query so they don't count toward a user's access — otherwise
	// the "no access" filter/badge would miss a user whose only grants are dead.
	distinct := make(map[int64]struct{})
	for _, gs := range allGrants {
		for _, g := range gs {
			distinct[g.TeamID] = struct{}{}
		}
	}
	roots := make([]int64, 0, len(distinct))
	for id := range distinct {
		roots = append(roots, id)
	}
	activeIDs, err := h.grants.ListDescendantTeamIDs(r.Context(), scope, roots)
	if err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activeSet := make(map[int64]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		activeSet[id] = struct{}{}
	}

	// Augment each user with the number of hierarchy nodes granted to them, plus the membership
	// Status/Role so the admin UI can distinguish requesters (add/deny) from members.
	type userListItem struct {
		*domain.User
		GrantedNodeCount int
		Status           string
		Role             string
	}
	items := make([]userListItem, 0, len(tenantUsers))
	for _, tu := range tenantUsers {
		count := 0
		for _, g := range allGrants[tu.User.ID] {
			if _, ok := activeSet[g.TeamID]; ok {
				count++
			}
		}
		items = append(items, userListItem{
			User: tu.User, GrantedNodeCount: count,
			Status: string(tu.Status), Role: string(tu.Role),
		})
	}
	admincommon.WriteJSON(w, items)
}

// GET /api/v1/admin/users/{userID}
func (h *Handler) GetOne(w http.ResponseWriter, r *http.Request) {
	userID, err := admincommon.ParseID(r, "userID")
	if err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	user, err := h.users.GetUser(r.Context(), userID)
	if err != nil {
		admincommon.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	admincommon.WriteJSON(w, user)
}
