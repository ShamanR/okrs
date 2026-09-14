// Package preferences serves GET/PUT /api/v1/notifications/preferences.
package preferences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	notificationprefsvc "okrs/internal/service/notificationpref"
	"okrs/internal/store/notificationprefs"
)

// inAppTitle labels the bell column. The bell is a reserved pseudo-channel with
// no descriptor of its own, so its label lives here; every other column is
// labelled by the channel itself.
const inAppTitle = "В приложении"

// loadFailed — то, что видит клиент при любом отказе чтения: какой именно шаг
// не удался, ему не поможет. Причина и шаг уходят в запись о запросе.
const loadFailed = "failed to load preferences"

// PrefService is the port this handler needs. *notificationpref.Service satisfies it.
type PrefService interface {
	GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error)
	// SetAll writes the whole matrix, validating every row before writing any — a
	// per-row Set would let a rejected payload leave its earlier rows applied.
	SetAll(ctx context.Context, scope domain.TenantScope, userID int64, ps []notificationprefs.Preference) error
	// DeliveryDefaults is what the matrix falls back to per channel for a user who
	// never chose. The screen shows effective state, so rendering it needs this.
	DeliveryDefaults(ctx context.Context, scope domain.TenantScope) (map[string]bool, error)
}

// Channels supplies the matrix columns. Declared consumer-side;
// *notificationchannel.Service satisfies it. nil in a build with no channels —
// the bell alone is a complete matrix.
type Channels interface {
	List(ctx context.Context, scope domain.TenantScope) ([]notificationchannelsvc.ChannelState, error)
}

type Handler struct {
	svc      PrefService
	channels Channels
}

func New(svc PrefService, channels Channels) *Handler {
	return &Handler{svc: svc, channels: channels}
}

// columns builds the matrix header: the bell first, then every external channel
// the tenant has switched on, in build order.
//
// A channel the administrator has not enabled is left out entirely rather than
// shown disabled: nothing can be delivered through it, so a column for it would
// only invite people to configure something that does nothing.
func (h *Handler) columns(ctx context.Context, scope domain.TenantScope) ([]dto.NotificationChannelOption, error) {
	out := []dto.NotificationChannelOption{
		{Name: notificationprefs.ChannelInApp, Title: inAppTitle, DefaultOn: true},
	}
	if h.channels == nil {
		return out, nil
	}
	states, err := h.channels.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	for _, st := range states {
		if !st.Enabled || st.Descriptor.Name == notificationprefs.ChannelInApp {
			continue
		}
		out = append(out, dto.NotificationChannelOption{
			Name:      st.Descriptor.Name,
			Title:     st.Descriptor.Title,
			DefaultOn: st.DefaultOn,
		})
	}
	return out, nil
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	// Три разных отказа отвечают клиенту одинаково — ему всё равно, какой из них
	// случился, — но в запись о запросе каждый уходит со своей причиной и своим
	// шагом. Без этого «failed to load preferences» одинаково выглядит и при
	// отвалившейся базе, и при недоступном канале, и расследовать его приходится
	// подключением к базе.
	prefs, err := h.svc.GetAll(r.Context(), scope, auth.UserIDFromContext(r.Context()))
	if err != nil {
		v1.WriteInternalError(w, "INTERNAL", loadFailed, fmt.Errorf("preferences: %w", err))
		return
	}
	defaults, err := h.svc.DeliveryDefaults(r.Context(), scope)
	if err != nil {
		v1.WriteInternalError(w, "INTERNAL", loadFailed, fmt.Errorf("channel defaults: %w", err))
		return
	}
	cols, err := h.columns(r.Context(), scope)
	if err != nil {
		v1.WriteInternalError(w, "INTERNAL", loadFailed, fmt.Errorf("channel columns: %w", err))
		return
	}

	out := dto.NotificationPreferences{
		Items:    make([]dto.NotificationPreference, 0, len(prefs)),
		Channels: cols,
	}
	for _, p := range prefs {
		// Effective state, not stored state: a cell the user never touched shows
		// the administrator's current default, which is exactly what will happen
		// to their notifications.
		state := make(map[string]bool, len(cols))
		for _, c := range cols {
			on := defaults[c.Name]
			if choice, chose := p.ChannelOverrides[c.Name]; chose {
				on = choice
			}
			state[c.Name] = on
		}
		out.Items = append(out.Items, dto.NotificationPreference{
			Type: p.Type, Enabled: p.Enabled, Scope: p.Scope, Channels: state,
			Addressed: notificationprefs.IsAddressed(p.Type),
		})
	}
	v1.WriteJSON(w, http.StatusOK, out)
}

// Put upserts the preferences listed in the request; types the caller omits are left
// untouched, not reset to defaults. The settings screen always sends the whole
// matrix, so in practice this behaves like a replace, but a partial payload does not
// erase preferences for the types it leaves out.
//
// The payload carries effective state — the checkboxes as the user left them.
// Turning that into stored deviations is the service's job: a cell that agrees
// with the tenant default is recorded as "no opinion", so a later change of that
// default still reaches this user.
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
			Type: it.Type, Enabled: it.Enabled, Scope: it.Scope, ChannelOverrides: it.Channels,
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
		v1.WriteInternalError(w, "INTERNAL", "failed to save preferences", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
