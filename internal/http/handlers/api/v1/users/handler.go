package users

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"okrs/internal/auth"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
)

type serviceIface interface {
	SearchUsersInScope(ctx context.Context, scopeTeamIDs []int64, q string, limit int) ([]*domain.User, error)
	GetUsersByUDIDs(ctx context.Context, udids []string) ([]*domain.User, error)
	ListUserLeadTeams(ctx context.Context) (map[string]string, error)
}

type Handler struct {
	svc serviceIface
}

func New(svc serviceIface) *Handler {
	return &Handler{svc: svc}
}

type userResponse struct {
	UDID        string `json:"udid"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Provider    string `json:"provider"`
	Email       string `json:"email,omitempty"`
	LedTeam     string `json:"led_team,omitempty"`
}

// GET /api/v1/users
// At least one of ?ids[]=<udid> or ?q=<string> must be present.
// ids[] mode: return up to 100 users by UDID (no scope filtering — used for loading known references).
// q mode:    return up to 20 users matching q within the caller's hierarchy scope.
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query()["ids[]"]
	_, hasQ := r.URL.Query()["q"]
	q := r.URL.Query().Get("q")

	if len(ids) == 0 && !hasQ {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "at least one of ids[] or q is required", nil)
		return
	}

	ctx := r.Context()
	var users []*domain.User
	var err error

	if len(ids) > 0 {
		users, err = h.svc.GetUsersByUDIDs(ctx, ids)
	} else {
		var scopeTeamIDs []int64
		if rawID := r.URL.Query().Get("scope_team_id"); rawID != "" {
			teamID, parseErr := strconv.ParseInt(rawID, 10, 64)
			if parseErr != nil || !auth.CanAccessTeamFromCtx(ctx, teamID) {
				v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no access to team", nil)
				return
			}
			scopeTeamIDs = []int64{teamID}
		} else {
			scopeTeamIDs, _ = auth.AllowedTeamIDsFromCtx(ctx)
		}
		users, err = h.svc.SearchUsersInScope(ctx, scopeTeamIDs, q, 20)
	}
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to list users", nil)
		return
	}

	leadTeams, err := h.svc.ListUserLeadTeams(ctx)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to list lead teams", nil)
		return
	}

	resp := make([]userResponse, 0, len(users))
	for _, u := range users {
		item := userResponse{
			UDID:        u.UDID,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarURL,
			Provider:    u.Provider,
			Email:       u.Email,
		}
		if team, ok := leadTeams[u.DisplayName]; ok {
			item.LedTeam = team
		}
		resp = append(resp, item)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
