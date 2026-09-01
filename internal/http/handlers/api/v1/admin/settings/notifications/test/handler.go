// Package test sends one probe message through a configured channel so the admin
// learns the settings are wrong now, at the form, rather than from users who never
// received anything.
//
// The probe goes to the caller and to nobody else: sending it elsewhere would let one
// admin's form put messages into another person's messenger.
package test

import (
	"context"
	"errors"
	"net"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	adminnotifications "okrs/internal/http/handlers/api/v1/admin/settings/notifications"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	"okrs/notifychannel"

	"github.com/go-chi/chi/v5"
)

type Channels interface {
	Sender(ctx context.Context, scope domain.TenantScope, name string) (notifychannel.Sender, error)
}

type Handler struct{ svc Channels }

func New(svc Channels) *Handler { return &Handler{svc: svc} }

func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	user := auth.UserFromContext(r.Context())
	if user == nil {
		admincommon.WriteError(w, http.StatusForbidden, "no active user")
		return
	}
	if user.Email == "" {
		admincommon.WriteError(w, http.StatusUnprocessableEntity,
			"в вашем профиле нет адреса электронной почты — канал адресует получателей по нему")
		return
	}

	sender, err := h.svc.Sender(r.Context(), scope, chi.URLParam(r, "channel"))
	switch {
	case err == nil:
	case errors.Is(err, notificationchannelsvc.ErrNotConfigured):
		admincommon.WriteError(w, http.StatusConflict,
			"канал ещё не настроен — сохраните настройки и повторите проверку")
		return
	case errors.Is(err, notificationchannelsvc.ErrUnknownChannel),
		errors.Is(err, notificationchannelsvc.ErrNotAvailable):
		admincommon.WriteError(w, http.StatusNotFound, "канал недоступен")
		return
	case errors.Is(err, notificationchannelsvc.ErrNoSecretKey):
		// Same cause, same wording as the Save button on the same card: an operator
		// who removed NOTIFICATIONS_SECRET_KEY must not be told "что-то пошло не так"
		// by one button and given the reason by the other.
		admincommon.WriteError(w, http.StatusServiceUnavailable, adminnotifications.NoSecretKeyMessage)
		return
	case errors.Is(err, notificationchannelsvc.ErrInvalidConfig):
		// The channel refused the stored settings — that is an incomplete
		// configuration the admin can fix here, not a server fault. The channel's own
		// text is not echoed: it is written for a Go caller, and the fix is the same
		// either way.
		admincommon.WriteError(w, http.StatusUnprocessableEntity,
			"настройки канала неполны — проверьте поля и сохраните их заново")
		return
	default:
		admincommon.WriteError(w, http.StatusInternalServerError, "не удалось подготовить канал")
		return
	}

	msg := notifychannel.Message{
		Title: "Проверка канала уведомлений",
		Body:  "Если вы видите это сообщение, канал настроен верно.",
	}
	if err := sender.Send(r.Context(), notifychannel.Target{Email: user.Email}, msg); err != nil {
		// The channel's own message is the only thing that tells the admin whether the
		// token is revoked or the URL is wrong, so an answer from the channel's API is
		// passed through rather than replaced by a generic failure. It describes the
		// tenant's own configuration and the tenant's own external system — nothing
		// about this server (specs/040).
		//
		// A transport failure is the exception, and it is deliberately classified here
		// rather than inside the channel: what may leave the server toward a tenant
		// admin is the core's call, and it has to hold for channels written in other
		// repositories, which cannot be relied on to classify their own errors. The
		// admin supplies base_url, so echoing "dial tcp 10.0.0.5:8080: connect:
		// connection refused" turns this button into an internal-network scanner with
		// an oracle: open port, closed port and HTTP status become distinguishable for
		// any address. net.Error catches it through any channel that wraps its
		// transport failure with %w, which is what wrapping already looks like.
		if isTransportError(err) {
			admincommon.WriteError(w, http.StatusBadGateway,
				"не удалось подключиться к серверу канала — проверьте адрес и доступность сервера")
			return
		}
		admincommon.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	admincommon.WriteJSON(w, map[string]any{"ok": true})
}

// isTransportError reports whether the failure happened before the channel's API
// ever answered — refused connection, DNS failure, timeout. *url.Error,
// *net.OpError and *net.DNSError all satisfy net.Error, so one check covers the
// whole family for any channel built on net/http.
func isTransportError(err error) bool {
	var ne net.Error
	return errors.As(err, &ne)
}
