package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store/periods"
	"okrs/internal/store/teams"

	"github.com/go-chi/chi/v5"
)

type ServiceHandler struct {
	service *service.Service
}

func NewServiceHandler(svc *service.Service) *ServiceHandler {
	return &ServiceHandler{service: svc}
}

// ── PERIODS ───────────────────────────────────────────────────────────────────

// POST /api/v1/admin/periods
func (h *ServiceHandler) HandleCreatePeriod(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Name == "" || req.StartDate == "" || req.EndDate == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name, start_date, end_date required", nil)
		return
	}
	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid start_date", nil)
		return
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid end_date", nil)
		return
	}
	if end.Before(start) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "end_date must be after start_date", nil)
		return
	}
	id, err := h.service.CreatePeriod(r.Context(), periods.PeriodInput{Name: req.Name, StartDate: start, EndDate: end})
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to create period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// PATCH /api/v1/admin/periods/{periodID}
func (h *ServiceHandler) HandleUpdatePeriod(w http.ResponseWriter, r *http.Request) {
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	var req struct {
		Name      string `json:"name"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid start_date", nil)
		return
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid end_date", nil)
		return
	}
	if end.Before(start) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "end_date must be after start_date", nil)
		return
	}
	if err := h.service.UpdatePeriod(r.Context(), periodID, periods.PeriodInput{Name: req.Name, StartDate: start, EndDate: end}); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/admin/periods/{periodID}
func (h *ServiceHandler) HandleDeletePeriod(w http.ResponseWriter, r *http.Request) {
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	if err := h.service.DeletePeriod(r.Context(), periodID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete period", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/admin/periods/{periodID}/move-up
func (h *ServiceHandler) HandleMovePeriodUp(w http.ResponseWriter, r *http.Request) {
	h.handleMovePeriod(w, r, -1)
}

// POST /api/v1/admin/periods/{periodID}/move-down
func (h *ServiceHandler) HandleMovePeriodDown(w http.ResponseWriter, r *http.Request) {
	h.handleMovePeriod(w, r, 1)
}

func (h *ServiceHandler) handleMovePeriod(w http.ResponseWriter, r *http.Request, dir int) {
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	if err := h.service.MovePeriod(r.Context(), periodID, dir); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to move period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── TEAMS ─────────────────────────────────────────────────────────────────────

type teamRow struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	TypeLabel   string  `json:"type_label"`
	ParentID    *int64  `json:"parent_id"`
	Lead        string  `json:"lead"`
	LeadUDID    *string `json:"lead_udid,omitempty"`
	Description string  `json:"description"`
	DeletedAt   *string `json:"deleted_at,omitempty"`
}

func mapTeamRow(t domain.Team) teamRow {
	var deletedAt *string
	if t.DeletedAt != nil {
		s := t.DeletedAt.Format("2006-01-02")
		deletedAt = &s
	}
	return teamRow{
		ID:          t.ID,
		Name:        t.Name,
		Type:        string(t.Type),
		TypeLabel:   common.TeamTypeLabel(t.Type),
		ParentID:    t.ParentID,
		Lead:        t.Lead,
		LeadUDID:    t.LeadUDID,
		Description: t.Description,
		DeletedAt:   deletedAt,
	}
}

// GET /api/v1/admin/teams
func (h *ServiceHandler) HandleListTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.service.ListTeams(r.Context())
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load teams", nil)
		return
	}
	deleted, err := h.service.ListDeletedTeams(r.Context())
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load deleted teams", nil)
		return
	}
	rows := make([]teamRow, 0, len(teams)+len(deleted))
	for _, t := range teams {
		rows = append(rows, mapTeamRow(t))
	}
	for _, t := range deleted {
		rows = append(rows, mapTeamRow(t))
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"items": rows})
}

// POST /api/v1/admin/teams
func (h *ServiceHandler) HandleCreateTeam(w http.ResponseWriter, r *http.Request) {
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
		missing, err := h.service.ValidateUserUDIDsExist(r.Context(), []string{*req.LeadUDID})
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate lead", nil)
			return
		}
		if len(missing) > 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown lead_udid", map[string]string{"lead_udid": "not found"})
			return
		}
	}
	id, err := h.service.CreateTeam(r.Context(), teams.TeamInput{
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
func (h *ServiceHandler) HandleUpdateTeam(w http.ResponseWriter, r *http.Request) {
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
		missing, err := h.service.ValidateUserUDIDsExist(r.Context(), []string{*req.LeadUDID})
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to validate lead", nil)
			return
		}
		if len(missing) > 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown lead_udid", map[string]string{"lead_udid": "not found"})
			return
		}
	}
	if err := h.service.UpdateTeam(r.Context(), teams.TeamInput{
		Name: req.Name, Type: teamType, ParentID: req.ParentID,
		Lead: req.Lead, LeadUDID: req.LeadUDID, Description: req.Description,
	}, teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update team", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/admin/teams/{teamID}  — soft delete
func (h *ServiceHandler) HandleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", nil)
		return
	}
	if err := h.service.DeleteTeam(r.Context(), teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete team", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/admin/teams/{teamID}/restore
func (h *ServiceHandler) HandleRestoreTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", nil)
		return
	}
	if err := h.service.RestoreTeam(r.Context(), teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to restore team", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/admin/teams/{teamID}/hard
func (h *ServiceHandler) HandleHardDeleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", nil)
		return
	}
	if err := h.service.HardDeleteTeam(r.Context(), teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
