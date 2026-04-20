package keyresults

import (
	"fmt"
	"net/http"
	"strings"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps common.Dependencies
}

const maxMultipartMemory = 32 << 20

func New(deps common.Dependencies) *Handler {
	return &Handler{deps: deps}
}

func (h *Handler) HandleUpdateKeyResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
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

	meta := service.KeyResultMetaInput{}
	switch krKind {
	case domain.KRKindPercent:
		start := common.ParseFloatField(r.FormValue("percent_start"))
		target := common.ParseFloatField(r.FormValue("percent_target"))
		if start == target {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Start и Target не должны быть равны"))
			return
		}
		meta.PercentStart = start
		meta.PercentTarget = target
		meta.PercentCurrent = common.ParseFloatField(r.FormValue("percent_current"))
	case domain.KRKindLinear:
		start := common.ParseFloatField(r.FormValue("linear_start"))
		target := common.ParseFloatField(r.FormValue("linear_target"))
		if start == target {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Start и Target не должны быть равны"))
			return
		}
		meta.LinearStart = start
		meta.LinearTarget = target
		meta.LinearCurrent = common.ParseFloatField(r.FormValue("linear_current"))
	case domain.KRKindBoolean:
		meta.BooleanDone = r.FormValue("boolean_done") == "true"
	case domain.KRKindProject:
		stages, err := parseProjectStages(r)
		if err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
		meta.ProjectStages = stages
	}

	if err := h.deps.Service.UpdateKeyResultWithMeta(ctx, store.KeyResultUpdateInput{
		ID:          krID,
		Title:       common.TrimmedFormValue(r, "title"),
		Description: common.TrimmedFormValue(r, "description"),
		Weight:      weight,
		Kind:        krKind,
	}, meta); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}

	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := h.deps.Service.FindGoalIDByKR(ctx, krID)
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
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := h.deps.Service.MoveKeyResult(ctx, krID, direction); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := h.deps.Service.FindGoalIDByKR(ctx, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleAddKRComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	text := common.TrimmedFormValue(r, "text")
	if text != "" {
		if err := h.deps.Service.AddKeyResultComment(ctx, krID, text); err != nil {
			common.RenderError(w, h.deps.Logger, err)
			return
		}
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	goalID, _ := h.deps.Service.FindGoalIDByKR(ctx, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func (h *Handler) HandleDeleteKeyResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	goalID, err := h.deps.Service.FindGoalIDByKR(ctx, krID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := h.deps.Service.DeleteKeyResult(ctx, krID); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if returnURL := r.FormValue("return"); returnURL != "" {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
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
		for i := 1; i <= 4; i++ {
			title := common.TrimmedFormValue(r, fmt.Sprintf("step_title_%d", i))
			if title == "" {
				continue
			}
			weight := common.ParseIntField(r.FormValue(fmt.Sprintf("step_weight_%d", i)))
			if weight <= 0 || weight > 100 {
				return nil, fmt.Errorf("Вес шага должен быть 1..100")
			}
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
	return stages, nil
}

func formatGoalRedirect(_ int64) string {
	return "/teamOkrs"
}
