// Package onboarding holds the pluggable no-membership seam. The box defines the
// interface + registry and ships an OSS stub; a SaaS module registers its own
// "create organization / join" page. This package stays free of internal/store imports.
package onboarding

import (
	"net/http"
	"sync"
)

// NoMembershipHandler renders the page shown to an authenticated user who has no active
// membership in any tenant.
type NoMembershipHandler interface {
	ServeNoMembership(w http.ResponseWriter, r *http.Request)
}

// StubHandler is the OSS default: it delegates rendering to an injected func so the seam
// itself never imports the HTTP template layer.
type StubHandler struct {
	Render func(w http.ResponseWriter, r *http.Request)
}

func (h StubHandler) ServeNoMembership(w http.ResponseWriter, r *http.Request) {
	h.Render(w, r)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]NoMembershipHandler)
)

// Register makes a no-membership handler available by name.
func Register(name string, h NoMembershipHandler) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = h
}

// Get looks a registered handler up by name.
func Get(name string) (NoMembershipHandler, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	h, ok := registry[name]
	return h, ok
}
