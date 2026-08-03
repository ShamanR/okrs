package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store/periods"
	"okrs/internal/store/teams"

	"github.com/go-chi/chi/v5"
)

// settingsReader loads per-tenant settings; *service.SettingsService satisfies it.
type settingsReader interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
}

type ServiceHandler struct {
	service  *service.Service
	settings settingsReader
}

func NewServiceHandler(svc *service.Service, settings settingsReader) *ServiceHandler {
	return &ServiceHandler{service: svc, settings: settings}
}

// ── PERIODS ───────────────────────────────────────────────────────────────────

// GET /api/v1/admin/periods
// Unlike the public periods endpoint, this includes archived periods so admins can manage them.
func (h *ServiceHandler) HandleListPeriods(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	views, err := h.service.ListPeriodViews(r.Context(), scope, true)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load periods", nil)
		return
	}
	items := make([]dto.PeriodInfo, 0, len(views))
	for _, v := range views {
		items = append(items, v1.MapPeriodView(v))
	}
	v1.WriteJSON(w, http.StatusOK, dto.PeriodsResponse{Items: items})
}

// POST /api/v1/admin/periods
func (h *ServiceHandler) HandleCreatePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
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
	id, err := h.service.CreatePeriod(r.Context(), scope, periods.PeriodInput{Name: req.Name, StartDate: start, EndDate: end})
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to create period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// PATCH /api/v1/admin/periods/{periodID}
func (h *ServiceHandler) HandleUpdatePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
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
	if err := h.service.UpdatePeriod(r.Context(), scope, periodID, periods.PeriodInput{Name: req.Name, StartDate: start, EndDate: end}); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/admin/periods/{periodID}
func (h *ServiceHandler) HandleDeletePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	if err := h.service.DeletePeriod(r.Context(), scope, periodID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete period", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/admin/periods/{periodID}/archive
func (h *ServiceHandler) HandleArchivePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	if err := h.service.ArchivePeriod(r.Context(), scope, periodID); err != nil {
		if errors.Is(err, service.ErrPeriodNotClosed) {
			v1.WriteError(w, http.StatusConflict, "CONFLICT", "only a closed period can be archived", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to archive period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/v1/admin/periods/{periodID}/unarchive
func (h *ServiceHandler) HandleUnarchivePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	if err := h.service.UnarchivePeriod(r.Context(), scope, periodID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to unarchive period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// weightTolerance loads the tenant's health-checkin weight tolerance (defaults to 0).
func (h *ServiceHandler) weightTolerance(r *http.Request, scope domain.TenantScope) int {
	if h.settings == nil {
		return 0
	}
	cfg, err := service.LoadHealthCheckInConfig(r.Context(), scope, h.settings)
	if err != nil {
		return 0
	}
	return cfg.WeightTolerance
}

// GET /api/v1/admin/periods/stats
func (h *ServiceHandler) HandlePeriodStats(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	items, err := h.service.PeriodStats(r.Context(), scope, h.weightTolerance(r, scope))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load period stats", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /api/v1/admin/periods/{periodID}/overview
func (h *ServiceHandler) HandlePeriodOverview(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	ov, err := h.service.PeriodOverview(r.Context(), scope, periodID, h.weightTolerance(r, scope))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load overview", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, ov)
}

func (h *ServiceHandler) handleBulk(w http.ResponseWriter, r *http.Request, target domain.TeamPeriodStatus) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	res, err := h.service.BulkSetTeamPeriodStatus(r.Context(), scope, periodID, target, auth.UserIDFromContext(r.Context()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to apply bulk operation", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, res)
}

// POST /api/v1/admin/periods/{periodID}/teams/activate
func (h *ServiceHandler) HandleActivatePeriodTeams(w http.ResponseWriter, r *http.Request) {
	h.handleBulk(w, r, domain.TeamPeriodStatusInProgress)
}

// POST /api/v1/admin/periods/{periodID}/teams/close
func (h *ServiceHandler) HandleClosePeriodTeams(w http.ResponseWriter, r *http.Request) {
	h.handleBulk(w, r, domain.TeamPeriodStatusClosed)
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
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	teams, err := h.service.ListTeams(r.Context(), scope)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load teams", nil)
		return
	}
	deleted, err := h.service.ListDeletedTeams(r.Context(), scope)
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
	id, err := h.service.CreateTeam(r.Context(), scope, teams.TeamInput{
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
	if err := h.service.UpdateTeam(r.Context(), scope, teams.TeamInput{
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
	if err := h.service.DeleteTeam(r.Context(), scope, teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete team", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/admin/teams/{teamID}/restore
func (h *ServiceHandler) HandleRestoreTeam(w http.ResponseWriter, r *http.Request) {
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
	if err := h.service.RestoreTeam(r.Context(), scope, teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to restore team", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/admin/teams/{teamID}/hard
func (h *ServiceHandler) HandleHardDeleteTeam(w http.ResponseWriter, r *http.Request) {
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
	if err := h.service.HardDeleteTeam(r.Context(), scope, teamID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
