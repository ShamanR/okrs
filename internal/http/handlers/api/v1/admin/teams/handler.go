// Package teams serves the /api/v1/admin/… endpoints under its URI segment.
package teams

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	"okrs/internal/http/handlers/web/common"
	teamsvc "okrs/internal/service/team"
	usersvc "okrs/internal/service/user"
	"okrs/internal/store/teams"
)

type Handler struct {
	users *usersvc.Service
	teams *teamsvc.Service
}

func New(teams *teamsvc.Service, users *usersvc.Service) *Handler {
	return &Handler{teams: teams, users: users}
}

// GET /api/v1/admin/teams
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	teams, err := h.teams.List(r.Context(), scope)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load teams", nil)
		return
	}
	deleted, err := h.teams.ListDeleted(r.Context(), scope)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load deleted teams", nil)
		return
	}
	rows := make([]admincommon.TeamRow, 0, len(teams)+len(deleted))
	for _, t := range teams {
		rows = append(rows, admincommon.MapTeamRow(t))
	}
	for _, t := range deleted {
		rows = append(rows, admincommon.MapTeamRow(t))
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"items": rows})
}

// POST /api/v1/admin/teams
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		Name        string  `json:"name"`
		Type        string  `json:"type"`
		ParentID    *int64  `json:"parent_id"`
		Lead        string  `json:"lead"`
		LeadUDID    *string `json:"lead_udid"`
		Description string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Name == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name required", nil)
		return
	}
	teamType := domain.TeamType(req.Type)
	if !common.ValidTeamType(teamType) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team type", nil)
		return
	}
	if req.LeadUDID != nil && *req.LeadUDID == "" {
		req.LeadUDID = nil
	}
	if req.LeadUDID != nil {
		missing, err := h.users.ValidateUDIDsExist(r.Context(), []string{*req.LeadUDID})
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate lead", nil)
			return
		}
		if len(missing) > 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown lead_udid", map[string]string{"lead_udid": "not found"})
			return
		}
	}
	id, err := h.teams.Create(r.Context(), scope, teams.TeamInput{
		Name: req.Name, Type: teamType, ParentID: req.ParentID,
		Lead: req.Lead, LeadUDID: req.LeadUDID, Description: req.Description,
	})
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to create team", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// PATCH /api/v1/admin/teams/{teamID}
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", nil)
		return
	}
	var req struct {
		Name        string  `json:"name"`
		Type        string  `json:"type"`
		ParentID    *int64  `json:"parent_id"`
		Lead        string  `json:"lead"`
		LeadUDID    *string `json:"lead_udid"`
		Description string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Name == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name required", nil)
		return
	}
	teamType := domain.TeamType(req.Type)
	if !common.ValidTeamType(teamType) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team type", nil)
		return
	}
	if req.LeadUDID != nil && *req.LeadUDID == "" {
		req.LeadUDID = nil
	}
	if req.LeadUDID != nil {
		missing, err := h.users.ValidateUDIDsExist(r.Context(), []string{*req.LeadUDID})
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate lead", nil)
			return
		}
		if len(missing) > 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown lead_udid", map[string]string{"lead_udid": "not found"})
			return
		}
	}
	if err := h.teams.Update(r.Context(), scope, teams.TeamInput{
		Name: req.Name, Type: teamType, ParentID: req.ParentID,
		Lead: req.Lead, LeadUDID: req.LeadUDID, Description: req.Description,
	}, teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update team", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/admin/teams/{teamID}  — soft delete
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", nil)
		return
	}
	if err := h.teams.Delete(r.Context(), scope, teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete team", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
