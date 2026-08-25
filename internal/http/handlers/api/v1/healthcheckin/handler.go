package healthcheckin

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	hcsvc "okrs/internal/service/healthcheckin"
)

// Computer computes the check-in from the cached period snapshot.
// *healthcheckin.Service satisfies it.
type Computer interface {
	Get(ctx context.Context, scope domain.TenantScope, userUDID string, isAdmin bool, periodID int64, cfg hcsvc.Config) (*hcsvc.Result, error)
}

type settingsProvider interface {
	GetTenant(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error)
	SetTenantProduct(ctx context.Context, scope domain.TenantScope, key string, value any) error
}

type cacheInvalidator interface {
	InvalidateAll()
}

type Handler struct {
	svc      Computer
	settings settingsProvider
	cache    cacheInvalidator
}

func New(svc Computer, settings settingsProvider, cache cacheInvalidator) *Handler {
	return &Handler{svc: svc, settings: settings, cache: cache}
}

// HandleHealthCheckIn serves GET /api/v1/health-checkin?period_id=X
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
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

	cfg, err := hcsvc.LoadConfig(r.Context(), scope, h.settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	// Unrestricted health-check scope is granted to tenant admins (active role), matching the
	// PolicyEvaluator's tenant-scoped admin model — not any legacy global flag.
	role, _ := auth.ActiveRoleFromContext(r.Context())
	result, err := h.svc.Get(r.Context(), scope, user.UDID, role == domain.RoleAdmin, periodID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, result)
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
