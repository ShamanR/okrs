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
)

// NotificationReader is the port this handler needs. *notification.Service satisfies
// it. The cursor crosses this boundary as an opaque string in both directions — its
// encoding is service/notification's business, per specs/010 (§66); this handler
// never sees a *storenotif.Cursor.
type NotificationReader interface {
	List(ctx context.Context, scope domain.TenantScope, userID int64, f storenotif.ListFilter, cursor string) ([]storenotif.Notification, string, error)
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
		Count:     n.CoalesceCount,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
		Read:      n.ReadAt != nil,
		ActorName: actor,
		URL:       targetURL(n),
	}
	if !n.ActorRemoved {
		d.ActorAvatar = n.ActorAvatarURL
	}
	return d
}

// targetURL builds the link the bell entry navigates to. Empty when there is no goal
// to open — the notification still renders, it just is not clickable.
func targetURL(n storenotif.Notification) string {
	if n.GoalID == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("/?goal_id=")
	b.WriteString(strconv.FormatInt(*n.GoalID, 10))
	if n.TeamID != nil {
		b.WriteString("&team_id=")
		b.WriteString(strconv.FormatInt(*n.TeamID, 10))
	}
	if n.PeriodID != nil {
		b.WriteString("&period_id=")
		b.WriteString(strconv.FormatInt(*n.PeriodID, 10))
	}
	return b.String()
}
