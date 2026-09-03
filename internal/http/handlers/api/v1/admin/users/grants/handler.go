// Package grants serves the /api/v1/admin/… endpoints under its URI segment.
package grants

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	"okrs/internal/platform/logging"
)

type Handler struct {
	grants admincommon.GrantsStore
}

func New(grants admincommon.GrantsStore) *Handler { return &Handler{grants: grants} }

// GET /api/v1/admin/users/{userID}/grants
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, err := admincommon.ParseID(r, "userID")
	if err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	grants, err := h.grants.ListUserGrants(r.Context(), scope, userID)
	if err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	admincommon.WriteJSON(w, grants)
}

// POST /api/v1/admin/users/{userID}/grants  body: {"team_id": 42}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	userID, err := admincommon.ParseID(r, "userID")
	if err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		TeamID int64 `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TeamID == 0 {
		admincommon.WriteError(w, http.StatusBadRequest, "team_id required")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	grantedBy := auth.UserIDFromContext(r.Context())
	if err := h.grants.AddUserGrant(r.Context(), scope, userID, body.TeamID, grantedBy); err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.AccessChanged(r.Context(), "grant_added",
		slog.Int64("target_user_id", userID),
		slog.Int64(logging.KeyTeamID, body.TeamID))
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/admin/users/{userID}/grants/{teamID}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, err := admincommon.ParseID(r, "userID")
	if err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	teamID, err := admincommon.ParseID(r, "teamID")
	if err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid team id")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	if err := h.grants.RemoveUserGrant(r.Context(), scope, userID, teamID); err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.AccessChanged(r.Context(), "grant_removed",
		slog.Int64("target_user_id", userID),
		slog.Int64(logging.KeyTeamID, teamID))
	w.WriteHeader(http.StatusNoContent)
}
