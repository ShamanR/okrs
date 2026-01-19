package keyresults

import (
	"fmt"
	"net/http"

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
	goalID, _ := common.FindGoalIDByStage(ctx, h.deps.Store, stageID)
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
	goalID, _ := common.FindGoalIDByKR(ctx, h.deps.Store, krID)
	http.Redirect(w, r, formatGoalRedirect(goalID), http.StatusSeeOther)
}

func errInvalidPercent() error {
	return fmt.Errorf("Процент должен быть 0..100")
}

func formatGoalRedirect(goalID int64) string {
	return fmt.Sprintf("/goals/%d", goalID)
}
