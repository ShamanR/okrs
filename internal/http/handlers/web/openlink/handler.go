// Package openlink serves GET /open — the hand-off a notification link outside
// the product goes through.
//
// A message in a messenger is read by someone whose session may be active in
// another space: /admin and the tracker both render whatever the SESSION says is
// active, so a plain deep link would quietly show the wrong space's data. This
// hand-off makes the event's space active first and only then walks to the page.
//
// It stays outside the membership gate on purpose: the recipient may currently be
// active in a space they lost or that was suspended, and this is exactly the route
// that gets them back to the one the notification is about.
package openlink

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/httperr"
)

// MembershipLookup lists the caller's memberships so the target space can be
// verified before the session is moved to it.
type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}

// SessionWriter persists the session's active tenant.
type SessionWriter interface {
	SetActiveTenant(ctx context.Context, sessionID string, tenantID int64) error
}

type Handler struct {
	members  MembershipLookup
	sessions SessionWriter
}

func New(members MembershipLookup, sessions SessionWriter) *Handler {
	return &Handler{members: members, sessions: sessions}
}

// Get switches the session to ?tenant= (when the caller is an active member of it)
// and redirects to ?to=. Anything it cannot honour lands on the product's root
// rather than on a page showing another space's data.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	target := safePath(r.URL.Query().Get("to"))
	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant"), 10, 64)

	if tenantID > 0 {
		switch {
		case !h.isActiveMember(r.Context(), tenantID):
			// The recipient is no longer in that space: the deep link would read
			// from whatever space they are in now, which is not what the message
			// was about. Send them home instead.
			target = "/"
		default:
			if err := h.switchTo(r.Context(), tenantID); err != nil {
				// The reason goes to the request record, not to the reader. They
				// still reach the product, just in their current space.
				_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError), err)
				target = "/"
			}
		}
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// switchTo moves the session. A session-less request (AUTH_MODE=disabled has no
// session) has nothing to move, and there is only one identity there anyway.
func (h *Handler) switchTo(ctx context.Context, tenantID int64) error {
	sess := auth.SessionFromContext(ctx)
	if sess == nil {
		return nil
	}
	return h.sessions.SetActiveTenant(ctx, sess.ID, tenantID)
}

func (h *Handler) isActiveMember(ctx context.Context, tenantID int64) bool {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return false
	}
	ms, err := h.members.ListByUser(ctx, user.ID)
	if err != nil {
		return false
	}
	for _, m := range ms {
		if m.TenantID == tenantID && m.Status == domain.MembershipActive {
			return true
		}
	}
	return false
}

// safePath keeps the redirect inside this product. The parameter travels through
// a messenger, so it is attacker-influenced in the same way any query parameter
// is: anything but a plain absolute path — an absolute URL, a protocol-relative
// "//host", a backslash the browser normalises into one — becomes the root.
func safePath(to string) string {
	if !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") || strings.HasPrefix(to, "/\\") {
		return "/"
	}
	return to
}
