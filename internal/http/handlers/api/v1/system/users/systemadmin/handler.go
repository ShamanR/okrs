// Package systemadmin serves the /api/v1/system/… endpoints under its URI segment.
package systemadmin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/system/systemcommon"
	"okrs/internal/platform/logging"
	"okrs/internal/service/provisioning"
	"okrs/internal/store/users"
)

type Handler struct {
	prov systemcommon.Provisioner
}

func New(prov systemcommon.Provisioner) *Handler { return &Handler{prov: prov} }

// PUT /api/v1/system/users/{userID}/system-admin  {is_system_admin}
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		IsSystemAdmin bool `json:"is_system_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		systemcommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var callerID int64
	if u := auth.UserFromContext(r.Context()); u != nil {
		callerID = u.ID
	}
	switch err := h.prov.SetSystemAdmin(r.Context(), callerID, userID, body.IsSystemAdmin); {
	case errors.Is(err, users.ErrNotFound):
		systemcommon.WriteError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, provisioning.ErrLastSystemAdmin):
		systemcommon.WriteError(w, http.StatusConflict, "cannot revoke the last system admin")
	case errors.Is(err, provisioning.ErrSelfLockout):
		systemcommon.WriteError(w, http.StatusConflict, "cannot revoke your own system-admin")
	case err != nil:
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
	default:
		logging.AccessChanged(r.Context(), "system_admin_set",
			slog.Int64("target_user_id", userID),
			slog.Bool("is_system_admin", body.IsSystemAdmin))
		w.WriteHeader(http.StatusNoContent)
	}
}
