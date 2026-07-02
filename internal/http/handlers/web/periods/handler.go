package periods

import (
	"fmt"
	"net/http"
	"time"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/store/periods"

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
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	periods, err := h.deps.Service.ListPeriods(r.Context(), scope)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	page := periodsPage{
		Periods:         periods,
		PageTitle:       "Периоды",
		ContentTemplate: "periods-content",
	}
	common.RenderTemplate(w, r, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleEditPeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	period, err := h.deps.Service.GetPeriod(r.Context(), scope, periodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	page := struct {
		Period          domain.Period
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{
		Period:          period,
		PageTitle:       fmt.Sprintf("Редактировать период %s", period.Name),
		ContentTemplate: "period-edit-content",
	}
	common.RenderTemplate(w, r, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) HandleCreatePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	name := common.TrimmedFormValue(r, "name")
	startDateRaw := common.TrimmedFormValue(r, "start_date")
	endDateRaw := common.TrimmedFormValue(r, "end_date")
	if name == "" || startDateRaw == "" || endDateRaw == "" {
		h.renderPeriodsWithError(w, r, scope, "Все поля обязательны")
		return
	}
	startDate, err := time.Parse("2006-01-02", startDateRaw)
	if err != nil {
		h.renderPeriodsWithError(w, r, scope, "Некорректная дата начала")
		return
	}
	endDate, err := time.Parse("2006-01-02", endDateRaw)
	if err != nil {
		h.renderPeriodsWithError(w, r, scope, "Некорректная дата окончания")
		return
	}
	if endDate.Before(startDate) {
		h.renderPeriodsWithError(w, r, scope, "Дата окончания должна быть позже даты начала")
		return
	}
	if _, err := h.deps.Service.CreatePeriod(r.Context(), scope, periods.PeriodInput{
		Name:      name,
		StartDate: startDate,
		EndDate:   endDate,
	}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/periods", http.StatusSeeOther)
}

func (h *Handler) HandleUpdatePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	name := common.TrimmedFormValue(r, "name")
	startDateRaw := common.TrimmedFormValue(r, "start_date")
	endDateRaw := common.TrimmedFormValue(r, "end_date")
	if name == "" || startDateRaw == "" || endDateRaw == "" {
		h.renderPeriodEditWithError(w, r, scope, periodID, "Все поля обязательны")
		return
	}
	startDate, err := time.Parse("2006-01-02", startDateRaw)
	if err != nil {
		h.renderPeriodEditWithError(w, r, scope, periodID, "Некорректная дата начала")
		return
	}
	endDate, err := time.Parse("2006-01-02", endDateRaw)
	if err != nil {
		h.renderPeriodEditWithError(w, r, scope, periodID, "Некорректная дата окончания")
		return
	}
	if endDate.Before(startDate) {
		h.renderPeriodEditWithError(w, r, scope, periodID, "Дата окончания должна быть позже даты начала")
		return
	}
	if err := h.deps.Service.UpdatePeriod(r.Context(), scope, periodID, periods.PeriodInput{
		Name:      name,
		StartDate: startDate,
		EndDate:   endDate,
	}); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/periods", http.StatusSeeOther)
}

func (h *Handler) HandleDeletePeriod(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := h.deps.Service.DeletePeriod(r.Context(), scope, periodID); err != nil {
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
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, fmt.Errorf("invalid period id"))
		return
	}
	if err := h.deps.Service.MovePeriod(r.Context(), scope, periodID, direction); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	http.Redirect(w, r, "/periods", http.StatusSeeOther)
}

func (h *Handler) renderPeriodsWithError(w http.ResponseWriter, r *http.Request, scope domain.TenantScope, message string) {
	periods, err := h.deps.Service.ListPeriods(r.Context(), scope)
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
	common.RenderTemplate(w, r, h.deps.Templates, "base", page, h.deps.Logger)
}

func (h *Handler) renderPeriodEditWithError(w http.ResponseWriter, r *http.Request, scope domain.TenantScope, periodID int64, message string) {
	period, err := h.deps.Service.GetPeriod(r.Context(), scope, periodID)
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	page := struct {
		Period          domain.Period
		FormError       string
		PageTitle       string
		ContentTemplate string
	}{
		Period:          period,
		FormError:       message,
		PageTitle:       fmt.Sprintf("Редактировать период %s", period.Name),
		ContentTemplate: "period-edit-content",
	}
	common.RenderTemplate(w, r, h.deps.Templates, "base", page, h.deps.Logger)
}
