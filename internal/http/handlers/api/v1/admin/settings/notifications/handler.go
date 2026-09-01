// Package notifications serves the tenant admin's notification-channel screen: which
// channels this tenant may use, what is configured, and how to change it.
//
// The screen only ever shows channels the tenant was granted (design spec §13.4) —
// no locked cards, no upsell. That filtering lives in the service; this package must
// not add a channel the service did not return.
//
// Admin rights are enforced by auth.RequireTenantAdminMiddleware on the route group,
// as in every neighbouring admin handler; there is no role check here.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	"okrs/notifychannel"

	"github.com/go-chi/chi/v5"
)

// NoSecretKeyMessage explains a deployment with no NOTIFICATIONS_SECRET_KEY. It is
// exported and shared with the probe handler in the child package on purpose: "Save"
// and "Test" sit on one card, and one cause answered with two different wordings
// reads to the admin as two different failures.
const NoSecretKeyMessage = "на сервере не настроен ключ шифрования NOTIFICATIONS_SECRET_KEY — канал с секретом настроить нельзя"

// Channels is the port, declared consumer-side per specs/010.
type Channels interface {
	List(ctx context.Context, scope domain.TenantScope) ([]notificationchannelsvc.ChannelState, error)
	Save(ctx context.Context, scope domain.TenantScope, in notificationchannelsvc.SaveInput, byUserID int64) error
}

type Handler struct{ svc Channels }

func New(svc Channels) *Handler { return &Handler{svc: svc} }

// GET /api/v1/admin/settings/notifications
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	states, err := h.svc.List(r.Context(), scope)
	if err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, "не удалось получить каналы")
		return
	}

	out := make([]dto.NotificationChannelDTO, 0, len(states))
	for _, st := range states {
		fields := make([]dto.NotificationChannelField, 0, len(st.Descriptor.Fields))
		for _, f := range st.Descriptor.Fields {
			fields = append(fields, dto.NotificationChannelField{
				Key: f.Key, Label: f.Label, Kind: string(f.Kind), Required: f.Required, Hint: f.Hint,
			})
		}
		out = append(out, dto.NotificationChannelDTO{
			Name: st.Descriptor.Name, Title: st.Descriptor.Title,
			Enabled: st.Enabled, Configured: st.Configured, SecretHint: st.SecretHint,
			Values: publicValues(st),
			Fields: fields,
		})
	}
	admincommon.WriteJSON(w, map[string]any{"channels": out})
}

// publicValues drops every secret-kind field a second time. The service already
// strips Descriptor.SecretField; this is the last hop before the wire, and it
// keys off Field.Kind rather than that same name on purpose — a guard that reuses
// the layer below's own recognition of "what is secret" only catches a bug where
// that layer forgets to filter, not a bug in the definition of secret itself. A
// descriptor with more than one FieldSecret (there is exactly one SecretField
// today, but nothing in the contract promises that stays true) is covered either
// way. Costs one map copy per channel.
func publicValues(st notificationchannelsvc.ChannelState) map[string]any {
	secret := make(map[string]bool, len(st.Descriptor.Fields))
	for _, f := range st.Descriptor.Fields {
		if f.Kind == notifychannel.FieldSecret {
			secret[f.Key] = true
		}
	}
	out := make(map[string]any, len(st.Values))
	for k, v := range st.Values {
		if secret[k] {
			continue
		}
		out[k] = v
	}
	return out
}

// fieldRequiredMessage names the empty field in the admin's own words — the label
// the form showed — falling back to the key if the descriptor gave no label.
func fieldRequiredMessage(err error) string {
	var fe *notificationchannelsvc.FieldRequiredError
	if !errors.As(err, &fe) {
		return "чтобы включить канал, заполните все обязательные поля"
	}
	name := fe.Label
	if name == "" {
		name = fe.Key
	}
	return "чтобы включить канал, заполните поле «" + name + "»"
}

type saveRequest struct {
	Enabled bool           `json:"enabled"`
	Values  map[string]any `json:"values"`
	// Secret empty means "keep the stored one": the form shows a mask, and an admin
	// editing only the server URL must not silently drop the token.
	Secret string `json:"secret"`
}

// PUT /api/v1/admin/settings/notifications/{channel}
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	var req saveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	in := notificationchannelsvc.SaveInput{
		Channel: chi.URLParam(r, "channel"),
		Enabled: req.Enabled,
		Values:  req.Values,
		Secret:  req.Secret,
	}
	err := h.svc.Save(r.Context(), scope, in, auth.UserIDFromContext(r.Context()))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, notificationchannelsvc.ErrUnknownChannel),
		errors.Is(err, notificationchannelsvc.ErrNotAvailable):
		// One status for both: a channel this tenant does not have must be
		// indistinguishable from one that does not exist. Answering 403 to the former
		// would confirm which channels the product has — an admin is not supposed to
		// enumerate a catalogue they were not granted.
		admincommon.WriteError(w, http.StatusNotFound, "канал недоступен")
	case errors.Is(err, notificationchannelsvc.ErrNoSecretKey):
		admincommon.WriteError(w, http.StatusServiceUnavailable, NoSecretKeyMessage)
	case errors.Is(err, notificationchannelsvc.ErrSecretRequired):
		admincommon.WriteError(w, http.StatusUnprocessableEntity,
			"чтобы включить канал, нужен секрет — заполните поле или сначала сохраните его отдельно")
	case errors.Is(err, notificationchannelsvc.ErrFieldRequired):
		// The field is named, not just "something is missing": the form is generated
		// from the descriptor, so only the server knows which input the admin left
		// empty by the time the request arrives.
		admincommon.WriteError(w, http.StatusUnprocessableEntity, fieldRequiredMessage(err))
	default:
		admincommon.WriteError(w, http.StatusInternalServerError, "не удалось сохранить канал")
	}
}
