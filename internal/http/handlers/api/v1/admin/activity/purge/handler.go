// Package purge serves the /api/v1/admin/… endpoints under its URI segment.
package purge

import (
	"encoding/json"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
)

type Handler struct {
	activity admincommon.ActivityPurger
}

func New(activity admincommon.ActivityPurger) *Handler { return &Handler{activity: activity} }

// Post handles POST /api/v1/admin/activity/purge.
// Body: {"older_than":"quarter"|"year"|"all"}. Tenant-admin authority is enforced by
// RequireTenantAdminMiddleware on the route group.
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OlderThan string `json:"older_than"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	cutoff, ok := admincommon.PurgeCutoff(body.OlderThan)
	if !ok {
		admincommon.WriteError(w, http.StatusUnprocessableEntity, "invalid older_than")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	deleted, err := h.activity.Purge(r.Context(), scope, cutoff)
	if err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	admincommon.WriteJSON(w, map[string]any{"deleted": deleted})
}
