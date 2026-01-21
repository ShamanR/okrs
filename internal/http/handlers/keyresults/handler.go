package keyresults

import (
	"fmt"
	"net/http"
	"strings"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps common.Dependencies
}

func New(deps common.Dependencies) *Handler {
	return &Handler{deps: deps}
}

func (h *Handler) HandleAddStage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	weight := common.ParseIntField(r.FormValue("weight"))
	sortOrder := common.ParseIntField(r.FormValue("sort_order"))
	stages, err := h.deps.Store.ListProjectStages(ctx, krID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := common.ValidateStageWeights(stages, weight); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := h.deps.Store.AddProjectStage(ctx, store.ProjectStageInput{KeyResultID: krID, Title: common.TrimmedFormValue(r, "title"), Weight: weight, SortOrder: sortOrder}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleUpdateKeyResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	kind := r.FormValue("kind")
	weight := common.ParseIntField(r.FormValue("weight"))
	if weight < 0 || weight > 100 {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("Вес должен быть 0..100"))
		return
	}
	krKind := domain.KRKind(kind)
	if !common.ValidKRKind(krKind) {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("Неверный тип KR"))
		return
	}
	if err := h.deps.Store.UpdateKeyResult(ctx, store.KeyResultUpdateInput{
		ID:          krID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        krKind,
	}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	switch krKind {
	case domain.KRKindPercent:
		start := common.ParseFloatField(r.FormValue("percent_start"))
		target := common.ParseFloatField(r.FormValue("percent_target"))
		current := common.ParseFloatField(r.FormValue("percent_current"))
		if start == target {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Start и Target не должны быть равны"))
			return
		}
		if err := h.deps.Store.UpsertPercentMeta(ctx, store.PercentMetaInput{KeyResultID: krID, StartValue: start, TargetValue: target, CurrentValue: current}); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	case domain.KRKindLinear:
		start := common.ParseFloatField(r.FormValue("linear_start"))
		target := common.ParseFloatField(r.FormValue("linear_target"))
		current := common.ParseFloatField(r.FormValue("linear_current"))
		if start == target {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Start и Target не должны быть равны"))
			return
		}
		if err := h.deps.Store.UpsertLinearMeta(ctx, store.LinearMetaInput{KeyResultID: krID, StartValue: start, TargetValue: target, CurrentValue: current}); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	case domain.KRKindBoolean:
		done := r.FormValue("boolean_done") == "true"
		if err := h.deps.Store.UpsertBooleanMeta(ctx, krID, done); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	case domain.KRKindProject:
		stages, err := parseProjectStages(r)
		if err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		for i := range stages {
			stages[i].KeyResultID = krID
		}
		if err := h.deps.Store.ReplaceProjectStages(ctx, krID, stages); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}

	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleToggleStage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stageID, err := common.ParseID(chi.URLParam(r, "stageID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	done := r.FormValue("done") == "true"
	if err := h.deps.Store.UpdateProjectStageDone(ctx, stageID, done); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := common.FindGoalIDByStage(ctx, h.deps.Store, stageID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleMoveKeyResultUp(w http.ResponseWriter, r *http.Request) {
	h.handleMoveKeyResult(w, r, -1)
}

func (h *Handler) HandleMoveKeyResultDown(w http.ResponseWriter, r *http.Request) {
	h.handleMoveKeyResult(w, r, 1)
}

func (h *Handler) handleMoveKeyResult(w http.ResponseWriter, r *http.Request, direction int) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := h.deps.Store.MoveKeyResult(ctx, krID, direction); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleUpdatePercentCurrent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	current := common.ParseFloatField(r.FormValue("current"))
	if err := h.deps.Store.UpdatePercentCurrent(ctx, krID, current); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleUpdateLinearCurrent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	current := common.ParseFloatField(r.FormValue("current"))
	if err := h.deps.Store.UpdateLinearCurrent(ctx, krID, current); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleAddCheckpoint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	metricValue := common.ParseFloatField(r.FormValue("metric_value"))
	krPercent := common.ParseIntField(r.FormValue("kr_percent"))
	if krPercent < 0 || krPercent > 100 {
		common.RenderError(w, h.deps.Logger, errInvalidPercent())
		return
	}
	if err := h.deps.Store.AddPercentCheckpoint(ctx, store.PercentCheckpointInput{KeyResultID: krID, MetricValue: metricValue, KRPercent: krPercent}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleUpdateBoolean(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	done := r.FormValue("done") == "true"
	if err := h.deps.Store.UpsertBooleanMeta(ctx, krID, done); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleAddKRComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	text := common.TrimmedFormValue(r, "text")
	if text != "" {
		if err := h.deps.Store.AddKeyResultComment(ctx, krID, text); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleDeleteKeyResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goalID, err := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := h.deps.Store.DeleteKeyResult(ctx, krID); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func errInvalidPercent() error {
	return fmt.Errorf("Процент должен быть 0..100")
}

func parseProjectStages(r *http.Request) ([]store.ProjectStageInput, error) {
	stages := make([]store.ProjectStageInput, 0, 4)
	totalWeight := 0
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
		totalWeight += weight
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
		for i := 1; i <= 4; i++ {
			title := common.TrimmedFormValue(r, fmt.Sprintf("step_title_%d", i))
			if title == "" {
				continue
			}
			weight := common.ParseIntField(r.FormValue(fmt.Sprintf("step_weight_%d", i)))
			if weight <= 0 || weight > 100 {
				return nil, fmt.Errorf("Вес шага должен быть 1..100")
			}
			totalWeight += weight
			stages = append(stages, store.ProjectStageInput{
				Title:     title,
				Weight:    weight,
				IsDone:    r.FormValue(fmt.Sprintf("step_done_%d", i)) == "true",
				SortOrder: i,
			})
		}
	}

	if len(stages) == 0 {
		return nil, fmt.Errorf("Для Project KR требуется минимум один шаг")
	}
	if totalWeight != 100 {
		return nil, fmt.Errorf("Сумма весов шагов должна быть равна 100")
	}
	return stages, nil
}

func formatGoalRedirect(goalID int64) string {
	return fmt.Sprintf("/goals/%d", goalID)
}
