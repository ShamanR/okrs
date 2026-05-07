package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/store/grants"

	"github.com/go-chi/chi/v5"
)

// userAdminStore covers user operations. *store.UserRepository satisfies it.
type userAdminStore interface {
	ListUsers(ctx context.Context) ([]*domain.User, error)
	GetUser(ctx context.Context, id int64) (*domain.User, error)
	SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error
}

// settingsStore covers system settings. *store.SettingsRepository satisfies it.
type settingsStore interface {
	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
	SetSetting(ctx context.Context, key string, value any) error
}

// grantsStore covers the user_hierarchy_grants operations. *store.GrantsCache satisfies it.
type grantsStore interface {
	ListUserGrants(ctx context.Context, userID int64) ([]grants.HierarchyGrant, error)
	AddUserGrant(ctx context.Context, userID, teamID, grantedByUserID int64) error
	RemoveUserGrant(ctx context.Context, userID, teamID int64) error
}

type Handler struct {
	users    userAdminStore
	settings settingsStore
	mgr      *auth.Manager
	grants   grantsStore
}

func New(users userAdminStore, settings settingsStore, mgr *auth.Manager, grants grantsStore) *Handler {
	return &Handler{users: users, settings: settings, mgr: mgr, grants: grants}
}

// GET /api/v1/admin/users
func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, users)
}

// GET /api/v1/admin/users/{userID}
func (h *Handler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	user, err := h.users.GetUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, user)
}

// POST /api/v1/admin/users/{userID}/admin  — grant admin
func (h *Handler) HandleGrantAdmin(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.users.SetUserAdmin(r.Context(), userID, true); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/admin/users/{userID}/admin  — revoke admin
func (h *Handler) HandleRevokeAdmin(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.users.SetUserAdmin(r.Context(), userID, false); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/admin/users/{userID}/grants
func (h *Handler) HandleListGrants(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	grants, err := h.grants.ListUserGrants(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, grants)
}

// POST /api/v1/admin/users/{userID}/grants  body: {"team_id": 42}
func (h *Handler) HandleAddGrant(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		TeamID int64 `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TeamID == 0 {
		writeError(w, http.StatusBadRequest, "team_id required")
		return
	}
	grantedBy := auth.UserIDFromContext(r.Context())
	if err := h.grants.AddUserGrant(r.Context(), userID, body.TeamID, grantedBy); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/admin/users/{userID}/grants/{teamID}
func (h *Handler) HandleRemoveGrant(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	teamID, err := parseID(r, "teamID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid team id")
		return
	}
	if err := h.grants.RemoveUserGrant(r.Context(), userID, teamID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/admin/settings/access
func (h *Handler) HandleGetAccessSettings(w http.ResponseWriter, r *http.Request) {
	policy, _ := h.settings.GetSetting(r.Context(), "new_user_policy")
	nodeID, _ := h.settings.GetSetting(r.Context(), "default_hierarchy_node_id")
	writeJSON(w, map[string]any{
		"new_user_policy":           json.RawMessage(policy),
		"default_hierarchy_node_id": json.RawMessage(nodeID),
	})
}

// POST /api/v1/admin/settings/access  body: {"new_user_policy":"default_node","default_hierarchy_node_id":5}
func (h *Handler) HandleUpdateAccessSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewUserPolicy          string `json:"new_user_policy"`
		DefaultHierarchyNodeID *int64 `json:"default_hierarchy_node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.NewUserPolicy != "" {
		if err := h.settings.SetSetting(r.Context(), "new_user_policy", body.NewUserPolicy); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if body.DefaultHierarchyNodeID != nil {
		if err := h.settings.SetSetting(r.Context(), "default_hierarchy_node_id", *body.DefaultHierarchyNodeID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type meResponse struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
	Provider    string `json:"provider"`
	IsAdmin     bool   `json:"is_admin"`
}

// GET /api/v1/me
func HandleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, meResponse{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		AvatarURL:   user.AvatarURL,
		Provider:    user.Provider,
		IsAdmin:     user.IsAdmin,
	})
}

func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
