// Package progressthresholds serves the /api/v1/admin/… endpoints under its URI segment:
// the tenant's progress evaluation thresholds shown in the admin «Настройки» section.
package progressthresholds

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	"okrs/internal/platform/logging"
	settingssvc "okrs/internal/service/settings"
)

type Handler struct {
	settings admincommon.TenantSettings
}

func New(settings admincommon.TenantSettings) *Handler { return &Handler{settings: settings} }

// GET /api/v1/admin/settings/progress-thresholds
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	admincommon.WriteJSON(w, settingssvc.LoadProgressThresholds(r.Context(), scope, h.settings))
}

// POST /api/v1/admin/settings/progress-thresholds
// body: {"stale_days":7,"behind_margin":10,"green_threshold":80,"weight_tolerance":0}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var body settingssvc.ProgressThresholds
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	// All values are validated before any write, so a rejected request changes nothing.
	if err := body.Validate(); err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	// Каждая запись фиксируется в аудите сразу после своего успеха: записи идут
	// поочерёдно и не в одной транзакции.
	set := func(key string, val int) bool {
		if err := h.settings.SetTenantProduct(r.Context(), scope, key, val); err != nil {
			admincommon.WriteError(w, http.StatusInternalServerError, err.Error())
			return false
		}
		logging.AccessChanged(r.Context(), "tenant_setting_saved", slog.String("setting", key))
		return true
	}
	if !set(settingssvc.ProgressStaleDaysKey, body.StaleDays) ||
		!set(settingssvc.ProgressBehindMarginKey, body.BehindMargin) ||
		!set(settingssvc.ProgressGreenThresholdKey, body.GreenThreshold) ||
		!set(settingssvc.ProgressWeightToleranceKey, body.WeightTolerance) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
