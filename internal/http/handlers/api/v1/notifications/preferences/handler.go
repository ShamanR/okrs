// Package preferences serves GET/PUT /api/v1/notifications/preferences.
package preferences

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	notificationprefsvc "okrs/internal/service/notificationpref"
	"okrs/internal/store/notificationprefs"
)

// PrefService is the port this handler needs. *notificationpref.Service satisfies it.
type PrefService interface {
	GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error)
	// SetAll writes the whole matrix, validating every row before writing any — a
	// per-row Set would let a rejected payload leave its earlier rows applied.
	SetAll(ctx context.Context, scope domain.TenantScope, userID int64, ps []notificationprefs.Preference) error
}

type Handler struct{ svc PrefService }

func New(svc PrefService) *Handler { return &Handler{svc: svc} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	prefs, err := h.svc.GetAll(r.Context(), scope, auth.UserIDFromContext(r.Context()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load preferences", nil)
		return
	}
	out := dto.NotificationPreferences{
		Items: make([]dto.NotificationPreference, 0, len(prefs)),
		// Same list notificationpref.Service.Set validates a caller's channels
		// against — one source of truth for what this build can deliver to.
		Channels: notificationprefsvc.AvailableChannels,
	}
	for _, p := range prefs {
		out.Items = append(out.Items, dto.NotificationPreference{
			Type: p.Type, Enabled: p.Enabled, Scope: p.Scope, Channels: p.Channels,
			Addressed: notificationprefs.IsAddressed(p.Type),
		})
	}
	v1.WriteJSON(w, http.StatusOK, out)
}

// Put upserts the preferences listed in the request; types the caller omits are left
// untouched, not reset to defaults. The settings screen always sends the whole
// matrix, so in practice this behaves like a replace, but a partial payload does not
// erase preferences for the types it leaves out.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	var req struct {
		Items []dto.NotificationPreference `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	// The type set is closed (notificationprefs.AllTypes), but req.Items is a client-
	// controlled slice: nothing before this point stops a caller from sending far more
	// entries than there are types, or the same type many times over. Reject both, so
	// the sequential, non-transactional loop below can never run more than
	// len(AllTypes) times regardless of what the request body claims.
	if len(req.Items) > len(notificationprefs.AllTypes) {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "too many items", nil)
		return
	}
	seen := make(map[string]bool, len(req.Items))
	for _, it := range req.Items {
		if seen[it.Type] {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "duplicate type",
				map[string]string{"type": "duplicate"})
			return
		}
		seen[it.Type] = true
	}

	userID := auth.UserIDFromContext(r.Context())
	prefs := make([]notificationprefs.Preference, 0, len(req.Items))
	for _, it := range req.Items {
		prefs = append(prefs, notificationprefs.Preference{
			Type: it.Type, Enabled: it.Enabled, Scope: it.Scope, Channels: it.Channels,
		})
	}
	// SetAll, not a Set per item: it validates the whole matrix before writing any of
	// it. Writing row by row meant a bad type in the third item left the first two
	// applied while the response said the matrix was rejected — the user saw settings
	// they never asked for. At most len(notificationprefs.AllTypes) rows reach here,
	// rejected above for length and duplicates, so this is not an N+1 waiting to grow.
	err := h.svc.SetAll(r.Context(), scope, userID, prefs)
	switch {
	case errors.Is(err, notificationprefsvc.ErrInvalidType):
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown notification type",
			map[string]string{"type": "invalid"})
		return
	case errors.Is(err, notificationprefsvc.ErrInvalidScope):
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown scope",
			map[string]string{"scope": "invalid"})
		return
	case errors.Is(err, notificationprefsvc.ErrInvalidChannel):
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown channel",
			map[string]string{"channels": "invalid"})
		return
	case err != nil:
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to save preferences", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
