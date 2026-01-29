package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/service"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service.Service
}

const maxMultipartMemory = 32 << 20

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
	r.Post("/goals/{goalID}", h.handleUpdateGoal)
	r.Post("/goals/{goalID}/key-results", h.handleCreateKeyResult)
	r.Post("/goals/{goalID}/move-up", h.handleMoveGoalUp)
	r.Post("/goals/{goalID}/move-down", h.handleMoveGoalDown)

	r.Post("/krs/{krID}/progress/percent", h.handleUpdatePercentProgress)
	r.Post("/krs/{krID}/progress/boolean", h.handleUpdateBooleanProgress)
	r.Post("/krs/{krID}/progress/project", h.handleUpdateProjectProgress)
	r.Post("/krs/{krID}/comments", h.handleAddKRComment)
	r.Post("/krs/{krID}", h.handleUpdateKeyResult)
	r.Post("/krs/{krID}/move-up", h.handleMoveKeyResultUp)
	r.Post("/krs/{krID}/move-down", h.handleMoveKeyResultDown)
	r.Post("/teams/{teamID}/status", h.handleUpdateTeamQuarterStatus)

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

func (h *Handler) handleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	priority := domain.Priority(r.FormValue("priority"))
	workType := domain.WorkType(r.FormValue("work_type"))
	focusType := domain.FocusType(r.FormValue("focus_type"))
	teamID, err := parseOptionalID(r.FormValue("team_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team_id", map[string]string{"team_id": "invalid"})
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	if validationErr := common.ValidateGoalInput(priority, workType, focusType, weight); validationErr != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationErr, nil)
		return
	}
	goalWeight := weight
	if teamID != nil {
		goal, err := h.service.GetGoal(r.Context(), goalID)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
			return
		}
		goalWeight = goal.Weight
	}
	if err := h.service.UpdateGoal(r.Context(), store.GoalUpdateInput{
		ID:          goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Priority:    priority,
		Weight:      goalWeight,
		WorkType:    workType,
		FocusType:   focusType,
		OwnerText:   common.TrimmedFormValue(r, "owner_text"),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update goal", nil)
		return
	}
	if teamID != nil {
		if err := h.service.UpdateGoalWeight(r.Context(), goalID, *teamID, weight); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update weight", nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleCreateKeyResult(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	kind := domain.KRKind(r.FormValue("kind"))
	if !common.ValidKRKind(kind) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr kind", map[string]string{"kind": "invalid"})
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	if weight < 0 || weight > 100 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
		return
	}
	meta, err := parseKeyResultMeta(r, kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	krID, err := h.service.CreateKeyResultWithMeta(r.Context(), store.KeyResultInput{
		GoalID:      goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        kind,
	}, meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create key result", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": krID})
}

func (h *Handler) handleUpdateKeyResult(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	kind := domain.KRKind(r.FormValue("kind"))
	if !common.ValidKRKind(kind) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr kind", map[string]string{"kind": "invalid"})
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	if weight < 0 || weight > 100 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
		return
	}
	meta, err := parseKeyResultMeta(r, kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.UpdateKeyResultWithMeta(r.Context(), store.KeyResultUpdateInput{
		ID:          krID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        kind,
	}, meta); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update key result", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleUpdateTeamQuarterStatus(w http.ResponseWriter, r *http.Request) {
	teamID, err := common.ParseID(chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	year, quarter := common.ParseQuarter(r, time.UTC)
	status := domain.TeamQuarterStatus(r.FormValue("status"))
	if !common.ValidTeamQuarterStatus(status) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid status", map[string]string{"status": "invalid"})
		return
	}
	if err := h.service.UpdateTeamQuarterStatus(r.Context(), teamID, year, quarter, status); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update status", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleMoveGoalUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, -1)
}

func (h *Handler) handleMoveGoalDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveGoal(w, r, 1)
}

func (h *Handler) handleMoveGoal(w http.ResponseWriter, r *http.Request, direction int) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.MoveGoal(r.Context(), goalID, direction); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to move goal", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleMoveKeyResultUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveKeyResult(w, r, -1)
}

func (h *Handler) handleMoveKeyResultDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveKeyResult(w, r, 1)
}

func (h *Handler) handleMoveKeyResult(w http.ResponseWriter, r *http.Request, direction int) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.MoveKeyResult(r.Context(), krID, direction); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to move key result", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func parseKeyResultMeta(r *http.Request, kind domain.KRKind) (service.KeyResultMetaInput, error) {
	switch kind {
	case domain.KRKindPercent:
		start := common.ParseFloatField(r.FormValue("percent_start"))
		target := common.ParseFloatField(r.FormValue("percent_target"))
		if start == target {
			return service.KeyResultMetaInput{}, fmt.Errorf("Start и Target не должны быть равны")
		}
		return service.KeyResultMetaInput{
			PercentStart:   start,
			PercentTarget:  target,
			PercentCurrent: common.ParseFloatField(r.FormValue("percent_current")),
		}, nil
	case domain.KRKindLinear:
		start := common.ParseFloatField(r.FormValue("linear_start"))
		target := common.ParseFloatField(r.FormValue("linear_target"))
		if start == target {
			return service.KeyResultMetaInput{}, fmt.Errorf("Start и Target не должны быть равны")
		}
		return service.KeyResultMetaInput{
			LinearStart:   start,
			LinearTarget:  target,
			LinearCurrent: common.ParseFloatField(r.FormValue("linear_current")),
		}, nil
	case domain.KRKindBoolean:
		done := r.FormValue("boolean_done") == "true"
		return service.KeyResultMetaInput{BooleanDone: done}, nil
	case domain.KRKindProject:
		stages, err := parseProjectStages(r)
		if err != nil {
			return service.KeyResultMetaInput{}, err
		}
		return service.KeyResultMetaInput{ProjectStages: stages}, nil
	default:
		return service.KeyResultMetaInput{}, nil
	}
}

func parseProjectStages(r *http.Request) ([]store.ProjectStageInput, error) {
	stages := make([]store.ProjectStageInput, 0, 4)
	titles := r.Form["step_title[]"]
	weights := r.Form["step_weight[]"]
	dones := r.Form["step_done[]"]
	sortOrder := 1

	for i, title := range titles {
		trimmed := strings.TrimSpace(title)
		if trimmed == "" {
			continue
		}
		weightValue := ""
		if i < len(weights) {
			weightValue = weights[i]
		}
		weight := common.ParseIntField(weightValue)
		if weight <= 0 || weight > 100 {
			return nil, fmt.Errorf("Вес шага должен быть 1..100")
		}
		isDone := false
		if i < len(dones) {
			isDone = dones[i] == "true"
		}
		stages = append(stages, store.ProjectStageInput{
			Title:     trimmed,
			Weight:    weight,
			IsDone:    isDone,
			SortOrder: sortOrder,
		})
		sortOrder++
	}

	if len(stages) == 0 {
		return nil, fmt.Errorf("Для Project KR требуется минимум один шаг")
	}
	return stages, nil
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
