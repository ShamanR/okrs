// Package notifications serves GET /api/v1/notifications — the bell feed.
package notifications

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/render/notify"
	notificationsvc "okrs/internal/service/notification"
	storenotif "okrs/internal/store/notifications"

	"github.com/go-chi/chi/v5"
)

// NotificationReader is the port this handler needs. *notification.Service satisfies
// it. The cursor crosses this boundary as an opaque string in both directions — its
// encoding is service/notification's business, per specs/010 (§66); this handler
// never sees a *storenotif.Cursor.
type NotificationReader interface {
	List(ctx context.Context, scope domain.TenantScope, userID int64, f storenotif.ListFilter, cursor string) ([]storenotif.Notification, string, error)
	// Delete reports whether a row was removed. False covers both "no such
	// notification" and "belongs to someone else" — the handler must not tell them
	// apart, or the endpoint becomes a probe for other people's notification ids.
	Delete(ctx context.Context, scope domain.TenantScope, userID, id int64) (bool, error)
}

type Handler struct{ svc NotificationReader }

func New(svc NotificationReader) *Handler { return &Handler{svc: svc} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	userID := auth.UserIDFromContext(r.Context())

	f := storenotif.ListFilter{
		UnreadOnly: r.URL.Query().Get("unread") == "1",
		Limit:      20,
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			f.Limit = n
		}
	}
	items, next, err := h.svc.List(r.Context(), scope, userID, f, r.URL.Query().Get("cursor"))
	if err != nil {
		if errors.Is(err, notificationsvc.ErrInvalidCursor) {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid cursor", map[string]string{"cursor": "invalid"})
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load notifications", nil)
		return
	}

	out := dto.NotificationList{Items: make([]dto.Notification, 0, len(items)), NextCursor: next}
	for _, n := range items {
		out.Items = append(out.Items, toDTO(n))
	}
	v1.WriteJSON(w, http.StatusOK, out)
}

// Delete removes one of the caller's own notifications.
// DELETE /api/v1/notifications/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid id", map[string]string{"id": "invalid"})
		return
	}
	removed, err := h.svc.Delete(r.Context(), scope, auth.UserIDFromContext(r.Context()), id)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "internal error", nil)
		return
	}
	if !removed {
		// Same answer for "never existed" and "not yours": a different one would let
		// a member of the same tenant confirm another user's notification ids.
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "notification not found", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toDTO(n storenotif.Notification) dto.Notification {
	actor := n.ActorDisplayName
	if n.ActorRemoved || actor == "" {
		// Former member: neutral placeholder, no name and no avatar.
		actor = "Бывший участник"
	}
	text := notify.Render(notify.Input{
		Kind:        event.Kind(n.Kind),
		ActorName:   actor,
		EntityTitle: n.EntityTitle,
		Count:       n.CoalesceCount,
		Payload:     n.Payload,
	})
	d := dto.Notification{
		ID: n.ID, Type: n.Type, Kind: n.Kind,
		Title:     text.Title,
		Body:      text.Body,
		Subject:   text.Subject,
		Count:     n.CoalesceCount,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
		Read:      n.ReadAt != nil,
		ActorName: actor,
		URL:       targetURL(n),
	}
	if !n.ActorRemoved {
		d.ActorAvatar = n.ActorAvatarURL
	}
	// The whole object is omitted when both halves are empty, so the client can test
	// one field instead of two and never renders an empty context line.
	if n.TeamPath != "" || n.GoalTitle != "" {
		d.Context = &dto.NotificationContext{Team: n.TeamPath, Goal: n.GoalTitle}
	}
	return d
}

// targetURL builds the link the bell entry navigates to. Empty when there is no goal
// to open — the notification still renders, it just is not clickable.
//
// Parameter names and order (team, period, goal, kr, comment) must match
// buildTargetURL in web/static/ui.js, the canonical deep-link builder: tracker.js's
// readURLNav reads exactly those names and nothing else. A previous version of this
// function used goal_id/team_id/period_id, which the tracker never reads at all —
// every bell click landed on the tracker with no navigation whatsoever.
func targetURL(n storenotif.Notification) string {
	if n.GoalID == nil {
		return ""
	}
	// Values are formatted int64s only (no user input reaches this string), so a
	// hand-built query string is safe and keeps the param order — team, period,
	// goal, kr, comment — matching buildTargetURL exactly, unlike url.Values.Encode
	// which would alphabetize them.
	var b strings.Builder
	b.WriteString("/?")
	first := true
	write := func(key string, v int64) {
		if !first {
			b.WriteByte('&')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.FormatInt(v, 10))
		first = false
	}
	if n.TeamID != nil {
		write("team", *n.TeamID)
	}
	if n.PeriodID != nil {
		write("period", *n.PeriodID)
	}
	write("goal", *n.GoalID)
	if n.KRID != nil {
		write("kr", *n.KRID)
	}
	if n.CommentID != nil {
		write("comment", *n.CommentID)
	}
	return b.String()
}
