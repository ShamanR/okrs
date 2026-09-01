// Package notificationchannels exposes, to the system-admin panel, the notification
// channels this build contains. Read-only on purpose: granting a channel to a tenant
// is an ordinary entitlement write and already has an endpoint. What the panel cannot
// know on its own is which channels the binary was assembled with — a build may carry
// channels from another repository entirely.
//
// Access is gated by auth.RequireSystemAdminMiddleware on the route group, as
// everywhere else in the system plane; there is no role check here.
package notificationchannels

import (
	"net/http"

	"okrs/internal/http/handlers/api/v1/system/systemcommon"
	"okrs/notifychannel"
)

// Channels is the port: only the build list, nothing tenant-specific.
type Channels interface {
	Descriptors() []notifychannel.Descriptor
}

type Handler struct{ svc Channels }

func New(svc Channels) *Handler { return &Handler{svc: svc} }

type channelDTO struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	// EntitlementKey is the BARE key the panel must send to the entitlements
	// endpoint. provisioning.SetEntitlements prefixes it with "entitlement.", so
	// sending the full key would store "entitlement.entitlement.notifications.…".
	EntitlementKey string `json:"entitlement_key"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ds := h.svc.Descriptors()
	// Non-nil slice: the panel iterates the field, and null would need a guard in JS.
	out := make([]channelDTO, 0, len(ds))
	for _, d := range ds {
		out = append(out, channelDTO{
			Name:           d.Name,
			Title:          d.Title,
			EntitlementKey: "notifications." + d.Name,
		})
	}
	systemcommon.WriteJSON(w, map[string]any{
		"channels": out,
	})
}
