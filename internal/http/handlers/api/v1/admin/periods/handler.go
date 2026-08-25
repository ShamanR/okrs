// Package periods serves the /api/v1/admin/… endpoints under its URI segment.
package periods

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"okrs/internal/auth"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	periodsvc "okrs/internal/service/period"
	"okrs/internal/store/periods"
)

type Handler struct {
	periods *periodsvc.Service
}

func New(periods *periodsvc.Service) *Handler { return &Handler{periods: periods} }

// GET /api/v1/admin/periods
// Unlike the public periods endpoint, this includes archived periods so admins can manage them.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	views, err := h.periods.ListViews(r.Context(), scope, true)
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
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
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
	id, err := h.periods.Create(r.Context(), scope, periods.PeriodInput{Name: req.Name, StartDate: start, EndDate: end})
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to create period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// PATCH /api/v1/admin/periods/{periodID}
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
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
	if err := h.periods.Update(r.Context(), scope, periodID, periods.PeriodInput{Name: req.Name, StartDate: start, EndDate: end}); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update period", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/v1/admin/periods/{periodID}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
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
	if err := h.periods.Delete(r.Context(), scope, periodID); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to delete period", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
