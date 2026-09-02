// Package healthcheckin serves /api/v1/admin/settings/health-checkin.
package healthcheckin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/httperr"
	"okrs/internal/platform/logging"
	hcsvc "okrs/internal/service/healthcheckin"
)

type SettingsProvider interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
	SetTenantProduct(ctx context.Context, scope domain.TenantScope, key string, value any) error
}
type CacheInvalidator interface {
	InvalidateAll()
}
type Handler struct {
	settings SettingsProvider
	cache    CacheInvalidator
}

func New(settings SettingsProvider, cache CacheInvalidator) *Handler {
	return &Handler{settings: settings, cache: cache}
}

// Get serves GET /api/v1/admin/settings/health-checkin
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "no active tenant")
		return
	}
	cfg, err := hcsvc.LoadConfig(r.Context(), scope, h.settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, cfg)
}

// Post serves POST /api/v1/admin/settings/health-checkin
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var body hcsvc.Config
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
	if body.GreenThreshold < 1 || body.GreenThreshold > 100 {
		writeError(w, http.StatusBadRequest, "green_threshold must be 1..100")
		return
	}
	if body.CommentDepth < 0 {
		writeError(w, http.StatusBadRequest, "comment_depth must be >= 0")
		return
	}
	if body.ResolvedCommentsLimit < 1 {
		writeError(w, http.StatusBadRequest, "resolved_comments_limit must be >= 1")
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
	logging.AccessChanged(r.Context(), "tenant_setting_saved",
		slog.String("setting", "health_checkin_config"))
	if h.cache != nil {
		h.cache.InvalidateAll()
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	httperr.WriteJSON(w, status, msg)
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
