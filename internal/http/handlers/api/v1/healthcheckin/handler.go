package healthcheckin

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/service"
)

type serviceProvider interface {
	GetHealthCheckIn(ctx context.Context, scope domain.TenantScope, userUDID string, isAdmin bool, periodID int64, cfg service.HealthCheckInConfig) (*service.HealthCheckInResult, error)
}

type settingsProvider interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
	SetTenantProduct(ctx context.Context, scope domain.TenantScope, key string, value any) error
}

type cacheInvalidator interface {
	InvalidateAll()
}

type Handler struct {
	svc      serviceProvider
	settings settingsProvider
	cache    cacheInvalidator
}

func New(svc serviceProvider, settings settingsProvider, cache cacheInvalidator) *Handler {
	return &Handler{svc: svc, settings: settings, cache: cache}
}

// HandleHealthCheckIn serves GET /api/v1/health-checkin?period_id=X
func (h *Handler) HandleHealthCheckIn(w http.ResponseWriter, r *http.Request) {
	periodIDStr := r.URL.Query().Get("period_id")
	periodID, err := strconv.ParseInt(periodIDStr, 10, 64)
	if err != nil || periodID <= 0 {
		writeError(w, http.StatusBadRequest, "period_id required")
		return
	}

	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}

	cfg, err := service.LoadHealthCheckInConfig(r.Context(), scope, h.settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	result, err := h.svc.GetHealthCheckIn(r.Context(), scope, user.UDID, user.IsAdmin, periodID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, result)
}

// HandleGetHealthCheckInSettings serves GET /api/v1/admin/settings/health-checkin
func (h *Handler) HandleGetHealthCheckInSettings(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	cfg, err := service.LoadHealthCheckInConfig(r.Context(), scope, h.settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, cfg)
}

// HandleUpdateHealthCheckInSettings serves POST /api/v1/admin/settings/health-checkin
func (h *Handler) HandleUpdateHealthCheckInSettings(w http.ResponseWriter, r *http.Request) {
	var body service.HealthCheckInConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.StaleDays <= 0 {
		writeError(w, http.StatusBadRequest, "stale_days must be > 0")
		return
	}
	if body.CacheTTLMinutes <= 0 {
		writeError(w, http.StatusBadRequest, "cache_ttl_minutes must be > 0")
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	if err := h.settings.SetTenantProduct(r.Context(), scope, "health_checkin_config", body); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.cache != nil {
		h.cache.InvalidateAll()
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
