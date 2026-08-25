// Package users serves the /api/v1/system/… endpoints under its URI segment.
package users

import (
	"net/http"

	"okrs/internal/http/handlers/api/v1/system/systemcommon"
)

type Handler struct {
	users systemcommon.UserLister
}

func New(users systemcommon.UserLister) *Handler { return &Handler{users: users} }

// GET /api/v1/system/users
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	list, err := h.users.ListUsers(r.Context())
	if err != nil {
		systemcommon.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type UserDTO struct {
		ID            int64  `json:"id"`
		DisplayName   string `json:"display_name"`
		Email         string `json:"email"`
		IsSystemAdmin bool   `json:"is_system_admin"`
	}
	out := make([]UserDTO, 0, len(list))
	for _, u := range list {
		out = append(out, UserDTO{ID: u.ID, DisplayName: u.DisplayName, Email: u.Email, IsSystemAdmin: u.IsSystemAdmin})
	}
	systemcommon.WriteJSON(w, out)
}
