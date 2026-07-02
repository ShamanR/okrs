package krs

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store/krs"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

// goalForKR resolves the parent goal of a KR and returns it.
// Returns an error if the KR or its goal cannot be found.
func (h *Handler) goalForKR(ctx context.Context, scope domain.TenantScope, krID int64) (domain.Goal, error) {
	kr, err := h.service.GetKeyResult(ctx, scope, krID)
	if err != nil {
		return domain.Goal{}, err
	}
	return h.service.GetGoal(ctx, scope, kr.GoalID)
}

// tenantScope extracts TenantScope from the request context, writing 403 and
// returning false if it is missing.
func (h *Handler) tenantScope(w http.ResponseWriter, r *http.Request) (domain.TenantScope, bool) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
	}
	return scope, ok
}

func (h *Handler) HandleCreateKeyResult(w http.ResponseWriter, r *http.Request) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	goal, err := h.service.GetGoal(r.Context(), scope, goalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	kind := domain.KRKind(r.FormValue("kind"))
	if !common.ValidKRKind(kind) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr kind", map[string]string{"kind": "invalid"})
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	if weight < 0 || weight > 100 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
		return
	}
	meta, err := parseKeyResultMeta(r, kind)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	krID, err := h.service.CreateKeyResultWithMeta(r.Context(), scope, krs.KeyResultInput{
		GoalID:      goalID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        kind,
	}, meta)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to create key result", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]int64{"id": krID})
}

func (h *Handler) HandleUpdateKeyResult(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	goal, err := h.goalForKR(r.Context(), scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	kind := domain.KRKind(r.FormValue("kind"))
	if !common.ValidKRKind(kind) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr kind", map[string]string{"kind": "invalid"})
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	if weight < 0 || weight > 100 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid weight", map[string]string{"weight": "0..100"})
		return
	}
	meta, err := parseKeyResultMeta(r, kind)
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.UpdateKeyResultWithMeta(r.Context(), scope, krs.KeyResultUpdateInput{
		ID:          krID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        kind,
	}, meta); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update key result", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleUpdateNumericalProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	goal, err := h.goalForKR(r.Context(), scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	var req struct {
		CurrentValue float64 `json:"current_value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.UpdateKRProgressNumerical(r.Context(), scope, krID, req.CurrentValue); err != nil {
		v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleUpdateBooleanProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	goal, err := h.goalForKR(r.Context(), scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	var req struct {
		Done bool `json:"done"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if err := h.service.UpdateKRProgressBoolean(r.Context(), scope, krID, req.Done); err != nil {
		v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleUpdateProjectProgress(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	goal, err := h.goalForKR(r.Context(), scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	var req struct {
		Stages []struct {
			ID   int64 `json:"id"`
			Done bool  `json:"done"`
		} `json:"stages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if len(req.Stages) == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "stages required", map[string]string{"stages": "required"})
		return
	}
	updates := make([]service.ProjectStageUpdate, 0, len(req.Stages))
	for _, stage := range req.Stages {
		if stage.ID == 0 {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "stage id required", map[string]string{"stage_id": "required"})
			return
		}
		updates = append(updates, service.ProjectStageUpdate{ID: stage.ID, IsDone: stage.Done})
	}
	if err := h.service.UpdateKRProgressProject(r.Context(), scope, krID, updates); err != nil {
		v1.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleUpsertKRNote(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	goal, err := h.goalForKR(r.Context(), scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	req.Text = strings.ReplaceAll(req.Text, "\r\n", "\n")
	if strings.TrimSpace(req.Text) == "" {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "text required", map[string]string{"text": "required"})
		return
	}
	if err := h.service.UpsertKeyResultNote(r.Context(), scope, krID, req.Text, auth.UserIDFromContext(r.Context())); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to upsert note", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleUpdateKRDescription updates only the description of a KR. Allowed in the
// same situations as notes (access check only), so a description can be added
// from the progress-update modal when full editing is otherwise locked.
func (h *Handler) HandleUpdateKRDescription(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	goal, err := h.goalForKR(r.Context(), scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	var req struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	req.Description = strings.ReplaceAll(req.Description, "\r\n", "\n")
	if err := h.service.UpdateKeyResultDescription(r.Context(), scope, krID, req.Description); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update description", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandleMoveKeyResultUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveKeyResult(w, r, -1)
}

func (h *Handler) HandleMoveKeyResultDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveKeyResult(w, r, 1)
}

func (h *Handler) HandleDeleteKeyResult(w http.ResponseWriter, r *http.Request) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	kr, err := h.service.GetKeyResult(r.Context(), scope, krID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	goal, err := h.service.GetGoal(r.Context(), scope, kr.GoalID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if err := h.service.DeleteKeyResult(r.Context(), scope, krID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete key result", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleMoveKeyResult(w http.ResponseWriter, r *http.Request, direction int) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := h.tenantScope(w, r)
	if !ok {
		return
	}
	goal, err := h.goalForKR(r.Context(), scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	if err := h.service.MoveKeyResult(r.Context(), scope, krID, direction); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to move key result", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
