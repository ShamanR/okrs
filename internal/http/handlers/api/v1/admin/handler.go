package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/store/grants"

	"github.com/go-chi/chi/v5"
)

// settingKeyDocumentationURL is the system_settings key holding the optional
// documentation link shown in the user menu. Empty value means "no link".
const settingKeyDocumentationURL = "documentation_url"

const (
	settingKeyFeedbackURL             = "feedback_url"
	settingKeyFeedbackPopupEnabled    = "feedback_popup_enabled"
	settingKeyFeedbackMenuLinkEnabled = "feedback_menu_link_enabled"
	settingKeyFeedbackFrequencyDays   = "feedback_frequency_days"
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
	AllGrants(ctx context.Context) (map[int64][]grants.HierarchyGrant, error)
	ListDescendantTeamIDs(ctx context.Context, rootIDs []int64) ([]int64, error)
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
	allGrants, err := h.grants.AllGrants(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	activeIDs, err := h.grants.ListDescendantTeamIDs(r.Context(), roots)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activeSet := make(map[int64]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		activeSet[id] = struct{}{}
	}

	// Augment each user with the number of hierarchy nodes granted to them, so
	// the admin UI can filter "no access" users and show a node count without
	// issuing a per-user grants request (the user list may hold thousands).
	type userListItem struct {
		*domain.User
		GrantedNodeCount int
	}
	items := make([]userListItem, 0, len(users))
	for _, u := range users {
		count := 0
		for _, g := range allGrants[u.ID] {
			if _, ok := activeSet[g.TeamID]; ok {
				count++
			}
		}
		items = append(items, userListItem{User: u, GrantedNodeCount: count})
	}
	writeJSON(w, items)
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

// GET /api/v1/admin/settings/general
func (h *Handler) HandleGetGeneralSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"documentation_url": h.documentationURL(r.Context()),
	})
}

// POST /api/v1/admin/settings/general  body: {"documentation_url":"https://..."}
func (h *Handler) HandleUpdateGeneralSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DocumentationURL string `json:"documentation_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	link := strings.TrimSpace(body.DocumentationURL)
	if link != "" && !isValidHTTPURL(link) {
		writeError(w, http.StatusBadRequest, "documentation_url must be a valid http(s) URL")
		return
	}
	if err := h.settings.SetSetting(r.Context(), settingKeyDocumentationURL, link); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// documentationURL reads the stored documentation link; empty string if unset.
func (h *Handler) documentationURL(ctx context.Context) string {
	return h.settingString(ctx, settingKeyDocumentationURL)
}

// GET /api/v1/admin/settings/feedback
func (h *Handler) HandleGetFeedbackSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, map[string]any{
		"feedback_url":               h.settingString(ctx, settingKeyFeedbackURL),
		"feedback_popup_enabled":     h.settingBool(ctx, settingKeyFeedbackPopupEnabled),
		"feedback_menu_link_enabled": h.settingBool(ctx, settingKeyFeedbackMenuLinkEnabled),
		"feedback_frequency_days":    h.settingInt(ctx, settingKeyFeedbackFrequencyDays, 30),
	})
}

// POST /api/v1/admin/settings/feedback
// body: {"feedback_url":"https://...","feedback_popup_enabled":true,"feedback_menu_link_enabled":true,"feedback_frequency_days":30}
func (h *Handler) HandleUpdateFeedbackSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FeedbackURL             string `json:"feedback_url"`
		FeedbackPopupEnabled    bool   `json:"feedback_popup_enabled"`
		FeedbackMenuLinkEnabled bool   `json:"feedback_menu_link_enabled"`
		FeedbackFrequencyDays   int    `json:"feedback_frequency_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	link := strings.TrimSpace(body.FeedbackURL)
	// No strict http(s) requirement here (unlike documentation_url) — any link is
	// accepted. The value is rendered into an href, so only block schemes that
	// could execute script (XSS), per the no-raw-HTML architecture rule.
	if link != "" && hasUnsafeURLScheme(link) {
		writeError(w, http.StatusBadRequest, "feedback_url must not use a javascript:, data:, or vbscript: scheme")
		return
	}
	if body.FeedbackFrequencyDays < 1 {
		writeError(w, http.StatusBadRequest, "feedback_frequency_days must be >= 1")
		return
	}
	set := func(key string, val any) bool {
		if err := h.settings.SetSetting(r.Context(), key, val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return false
		}
		return true
	}
	if !set(settingKeyFeedbackURL, link) ||
		!set(settingKeyFeedbackPopupEnabled, body.FeedbackPopupEnabled) ||
		!set(settingKeyFeedbackMenuLinkEnabled, body.FeedbackMenuLinkEnabled) ||
		!set(settingKeyFeedbackFrequencyDays, body.FeedbackFrequencyDays) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// settingString reads a string setting; empty when unset or malformed.
func (h *Handler) settingString(ctx context.Context, key string) string {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// settingBool reads a bool setting; false when unset or malformed.
func (h *Handler) settingBool(ctx context.Context, key string) bool {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// settingInt reads an int setting; returns def when unset, malformed, or < 1.
func (h *Handler) settingInt(ctx context.Context, key string, def int) int {
	raw, err := h.settings.GetSetting(ctx, key)
	if err != nil || raw == nil {
		return def
	}
	var n int
	if json.Unmarshal(raw, &n) != nil || n < 1 {
		return def
	}
	return n
}

// hasUnsafeURLScheme reports whether s uses a scheme that can execute script
// when placed in an href (javascript:, data:, vbscript:). Used where any link
// shape is allowed but rendered-href XSS must still be prevented.
func hasUnsafeURLScheme(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	for _, scheme := range []string{"javascript:", "data:", "vbscript:"} {
		if strings.HasPrefix(lower, scheme) {
			return true
		}
	}
	return false
}

// isValidHTTPURL reports whether s is an absolute http(s) URL. Restricting the
// scheme keeps unsafe values (e.g. javascript:) out of a rendered href.
func isValidHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

type meResponse struct {
	ID          int64  `json:"id"`
	UDID        string `json:"udid"`
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
		UDID:        user.UDID,
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
