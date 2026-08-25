// Package purge serves the /api/v1/system/… endpoints under its URI segment.
package purge

import (
	"encoding/json"
	"net/http"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	activity systemcommon.ActivityPurger
}

func New(activity systemcommon.ActivityPurger) *Handler { return &Handler{activity: activity} }

// HandlePurgeActivity handles POST /api/v1/system/tenants/{id}/activity/purge.
// Body: {"older_than":"quarter"|"year"|"all"}. System-admin authority is enforced by
// RequireSystemAdminMiddleware on the route group; the tenant id comes from the path.
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := systemcommon.PathID(w, r)
	if !ok {
		return
	}
	var body struct {
		OlderThan string `json:"older_than"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	cutoff, ok := systemcommon.PurgeCutoff(body.OlderThan)
	if !ok {
		systemcommon.WriteError(w, http.StatusUnprocessableEntity, "invalid older_than")
		return
	}
	deleted, err := h.activity.Purge(r.Context(), domain.TenantScope{TenantID: tenantID}, cutoff)
	if err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	systemcommon.WriteJSON(w, map[string]any{"deleted": deleted})
}
