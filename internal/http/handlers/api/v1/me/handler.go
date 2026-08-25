// Package me serves GET /api/v1/me — global identity, available to any authenticated user.
package me

import (
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
)

type meResponse struct {
	ID          int64  `json:"id"`
	UDID        string `json:"udid"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
	Provider    string `json:"provider"`
}
type Handler struct {
}

func New() *Handler { return &Handler{} }

// GET /api/v1/me
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		admincommon.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	admincommon.WriteJSON(w, meResponse{
		ID:          user.ID,
		UDID:        user.UDID,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		AvatarURL:   user.AvatarURL,
		Provider:    user.Provider,
	})
}
