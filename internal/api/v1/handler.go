package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"okrs/internal/http/handlers/common"
	"okrs/internal/service"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service.Service
}

func NewHandler(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/hierarchy", h.handleHierarchy)
	r.Get("/teams", h.handleTeams)
	r.Get("/teams/{teamID}", h.handleTeam)
	r.Get("/teams/{teamID}/okrs", h.handleTeamOKRs)
	r.Get("/goals/{goalID}", h.handleGoal)

	r.Post("/goals/{goalID}/share", h.handleShareGoal)
	r.Post("/goals/{goalID}/weight", h.handleUpdateGoalWeight)
	r.Post("/goals/{goalID}/comments", h.handleAddGoalComment)

	r.Post("/krs/{krID}/progress/percent", h.handleUpdatePercentProgress)
	r.Post("/krs/{krID}/progress/boolean", h.handleUpdateBooleanProgress)
	r.Post("/krs/{krID}/progress/project", h.handleUpdateProjectProgress)
	r.Post("/krs/{krID}/comments", h.handleAddKRComment)

	return r
}

func (h *Handler) handleHierarchy(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.service.GetHierarchy(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load hierarchy", nil)
		return
	}
	writeJSON(w, http.StatusOK, hierarchyResponse{Items: mapHierarchy(nodes)})
}

func (h *Handler) handleTeams(w http.ResponseWriter, r *http.Request) {
	year, quarter := common.ParseQuarter(r, time.UTC)
	orgID, err := parseOptionalID(r.URL.Query().Get("org_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid org_id", map[string]string{"org_id": "invalid"})
		return
	}
	teams, err := h.service.GetTeamsWithQuarterSummary(r.Context(), year, quarter, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load teams", nil)
		return
	}
	writeJSON(w, http.StatusOK, mapTeamsResponse(year, quarter, teams))
}

func (h *Handler) handleTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	team, err := h.service.GetTeam(r.Context(), teamID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "team not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, teamInfo{
		ID:        team.ID,
		Name:      team.Name,
		Type:      string(team.Type),
		TypeLabel: common.TeamTypeLabel(team.Type),
		ParentID:  team.ParentID,
	})
}

func (h *Handler) handleTeamOKRs(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	year, quarter := common.ParseQuarter(r, time.UTC)
	okr, err := h.service.GetTeamOKR(r.Context(), teamID, year, quarter)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "team okr not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, mapTeamOKRResponse(okr))
}

func (h *Handler) handleGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	goal, err := h.service.GetGoal(r.Context(), goalID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, mapGoalResponse(goal))
}

type shareGoalRequest struct {
	Targets []shareTargetRequest `json:"targets"`
}

type shareTargetRequest struct {
	TeamID int64 `json:"team_id"`
	Weight int   `json:"weight"`
}

func (h *Handler) handleShareGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	var req shareGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if len(req.Targets) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "targets required", map[string]string{"targets": "required"})
		return
	}
	targets := make([]service.ShareTarget, 0, len(req.Targets))
	for _, target := range req.Targets {
		if target.TeamID == 0 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "required"})
			return
		}
		if target.Weight < 0 || target.Weight > 100 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
			return
		}
		targets = append(targets, service.ShareTarget{TeamID: target.TeamID, Weight: target.Weight})
	}
	if err := h.service.ShareGoal(r.Context(), goalID, targets); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to share goal", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type updateGoalWeightRequest struct {
	TeamID int64 `json:"team_id"`
	Weight int   `json:"weight"`
}

func (h *Handler) handleUpdateGoalWeight(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	var req updateGoalWeightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.TeamID == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "team_id required", map[string]string{"team_id": "required"})
		return
	}
	if req.Weight < 0 || req.Weight > 100 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
		return
	}
	if err := h.service.UpdateGoalWeight(r.Context(), goalID, req.TeamID, req.Weight); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update weight", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type addCommentRequest struct {
	Text string `json:"text"`
}

func (h *Handler) handleAddGoalComment(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	var req addCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.service.AddGoalComment(r.Context(), goalID, req.Text); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to add comment", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleAddKRComment(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	var req addCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.service.AddKeyResultComment(r.Context(), krID, req.Text); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to add comment", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type updatePercentRequest struct {
	CurrentValue float64 `json:"current_value"`
}

func (h *Handler) handleUpdatePercentProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	var req updatePercentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.UpdateKRProgressPercent(r.Context(), krID, req.CurrentValue); err != nil {
		writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type updateBooleanRequest struct {
	Done bool `json:"done"`
}

func (h *Handler) handleUpdateBooleanProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	var req updateBooleanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.UpdateKRProgressBoolean(r.Context(), krID, req.Done); err != nil {
		writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type updateProjectRequest struct {
	Stages []updateProjectStage `json:"stages"`
}

type updateProjectStage struct {
	ID   int64 `json:"id"`
	Done bool  `json:"done"`
}

func (h *Handler) handleUpdateProjectProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if len(req.Stages) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "stages required", map[string]string{"stages": "required"})
		return
	}
	updates := make([]service.ProjectStageUpdate, 0, len(req.Stages))
	for _, stage := range req.Stages {
		if stage.ID == 0 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "stage id required", map[string]string{"stage_id": "required"})
			return
		}
		updates = append(updates, service.ProjectStageUpdate{ID: stage.ID, IsDone: stage.Done})
	}
	if err := h.service.UpdateKRProgressProject(r.Context(), krID, updates); err != nil {
		writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func parseOptionalID(value string) (*int64, error) {
	if value == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
