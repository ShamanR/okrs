package periods

import (
	"fmt"
	"net/http"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
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

type periodsPage struct {
	Periods         []domain.Period
	FormError       string
	PageTitle       string
	ContentTemplate string
}

func (h *Handler) HandlePeriods(w http.ResponseWriter, r *http.Request) {
	periods, err := h.deps.Store.ListPeriods(r.Context())
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	page := periodsPage{
		Periods:         periods,
		PageTitle:       "Периоды",
		ContentTemplate: "periods-content",
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleCreatePeriod(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	name := common.TrimmedFormValue(r, "name")
	startDateRaw := common.TrimmedFormValue(r, "start_date")
	endDateRaw := common.TrimmedFormValue(r, "end_date")
	if name == "" || startDateRaw == "" || endDateRaw == "" {
		h.renderPeriodsWithError(w, r, "Все поля обязательны")
		return
	}
	startDate, err := time.Parse("2006-01-02", startDateRaw)
	if err != nil {
		h.renderPeriodsWithError(w, r, "Некорректная дата начала")
		return
	}
	endDate, err := time.Parse("2006-01-02", endDateRaw)
	if err != nil {
		h.renderPeriodsWithError(w, r, "Некорректная дата окончания")
		return
	}
	if endDate.Before(startDate) {
		h.renderPeriodsWithError(w, r, "Дата окончания должна быть позже даты начала")
		return
	}
	if _, err := h.deps.Store.CreatePeriod(r.Context(), store.PeriodInput{
		Name:      name,
		StartDate: startDate,
		EndDate:   endDate,
	}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/periods", http.StatusSeeOther)
}

func (h *Handler) HandleMovePeriodUp(w http.ResponseWriter, r *http.Request) {
	h.handleMove(w, r, -1)
}

func (h *Handler) HandleMovePeriodDown(w http.ResponseWriter, r *http.Request) {
	h.handleMove(w, r, 1)
}

func (h *Handler) handleMove(w http.ResponseWriter, r *http.Request, direction int) {
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("invalid period id"))
		return
	}
	if err := h.deps.Store.MovePeriod(r.Context(), periodID, direction); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/periods", http.StatusSeeOther)
}

func (h *Handler) renderPeriodsWithError(w http.ResponseWriter, r *http.Request, message string) {
	periods, err := h.deps.Store.ListPeriods(r.Context())
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	page := periodsPage{
		Periods:         periods,
		FormError:       message,
		PageTitle:       "Периоды",
		ContentTemplate: "periods-content",
	}
	common.RenderTemplate(w, h.deps.Templates, "base", page, h.deps.Logger)
}
