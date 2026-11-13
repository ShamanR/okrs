// Package openlink serves GET /open — the hand-off a notification link outside
// the product goes through.
//
// A message in a messenger is read by someone whose current tab may be focused on
// another space: pages render whatever space the CLIENT stored for the tab, so a
// plain deep link would quietly show the wrong space's data. This hand-off verifies
// the recipient is an active member of the event's space and forwards the space as
// the ?tenant= query parameter — the client bootstrap adopts it into the tab's
// storage on arrival.
//
// It stays outside the membership gate on purpose: the recipient may currently be
// focused on a space they lost or that was suspended, and this is exactly the route
// that gets them back to the one the notification is about.
package openlink

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
)

// MembershipLookup lists the caller's memberships so the target space can be
// verified before it is handed to the client.
type MembershipLookup interface {
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
}

type Handler struct {
	members MembershipLookup
}

func New(members MembershipLookup) *Handler {
	return &Handler{members: members}
}

// Get verifies ?tenant= (the caller must be an active member of it) and redirects
// to ?to= with the tenant parameter carried over. Anything it cannot honour lands
// on the product's root rather than on a page showing another space's data.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("to")
	target := safePath(raw)
	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant"), 10, 64)

	if tenantID > 0 {
		switch {
		case !h.isActiveMember(r.Context(), tenantID):
			// The recipient is no longer in that space: the deep link would read
			// from whatever space they are in now, which is not what the message
			// was about. Send them home instead.
			target = "/"
		case target != "/" || raw == "/":
			// The tenant focus parameter rides along only with a target that
			// survived sanitation — a refused address stays a plain root.
			target = withTenantParam(target, tenantID)
		}
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// withTenantParam appends the tenant focus parameter to an in-product path.
func withTenantParam(path string, tenantID int64) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "tenant=" + strconv.FormatInt(tenantID, 10)
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
