// Package shell serves the SPA shells and the legacy redirects.
//
// These are deliberately NOT split one package per URI like the rest of the handlers:
// a shell route carries no logic, it is a row in a "URI → template" table, and a
// legacy route is a row in a "URI → target" table. Nineteen packages holding one
// ExecuteTemplate call each would be noise with no navigational gain. The exception
// is written down in specs/070-code-structure.md so it does not read as an oversight.
//
// /no-access is NOT here: it resolves through the nomembership registry and injects a
// configurable message, so it is live logic and gets its own package.
package shell

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Data is the view-model every shell template receives. Dev selects the development
// vs production vendored React build and is driven by the WEB_ASSETS_DEV env flag.
type Data struct {
	Dev bool
}

// Route binds a URI to the shell template it renders.
type Route struct {
	URI      string
	Template string
}

// Redirect binds a legacy URI to its canonical target.
type Redirect struct {
	From string
	To   string
	// KeepQuery carries the original query string over to the target. Only /teamOkrs
	// needs it: bookmarks there encode the selected team and period.
	KeepQuery bool
}

// Public are the shells reachable by any authenticated member.
var Public = []Route{
	{"/", "tracker-shell"},
	{"/teams/{teamID}/okr", "tracker-shell"},
	{"/settings", "settings-shell"},
	{"/period-overview", "period-overview-shell"},
	{"/goal-tree", "goal-tree-shell"},
}

// TenantAdmin are the shells behind the tenant-admin gate.
var TenantAdmin = []Route{
	{"/admin", "admin-shell"},
	{"/admin/access", "admin-shell"},
	{"/admin/teams", "admin-shell"},
	{"/admin/periods", "admin-shell"},
	{"/admin/health-checkin", "admin-shell"},
	{"/activity-log", "activity-shell"},
}

// System is the system-admin shell.
var System = []Route{
	{"/system", "system-shell"},
}

// PublicRedirects are legacy bookmarks outside the membership gate.
var PublicRedirects = []Redirect{
	{From: "/teams", To: "/admin/teams"},
	{From: "/periods", To: "/admin/periods"},
}

// MemberRedirects are legacy deep-links for authenticated members.
var MemberRedirects = []Redirect{
	{From: "/teamOkrs", To: "/", KeepQuery: true},
}

// AdminRedirects are legacy admin deep-links; the SPA routes them client-side now.
var AdminRedirects = []Redirect{
	{From: "/admin/teams/new", To: "/admin"},
	{From: "/admin/teams/{teamID}/edit", To: "/admin"},
	{From: "/admin/periods/{periodID}/edit", To: "/admin"},
	{From: "/admin/users/{userID}", To: "/admin"},
}

// Handler renders shells from the parsed template set.
type Handler struct {
	tmpl *template.Template
	data func() Data
}

func New(tmpl *template.Template, data func() Data) *Handler {
	return &Handler{tmpl: tmpl, data: data}
}

// RegisterShells mounts a table of shell routes on r.
func (h *Handler) RegisterShells(r chi.Router, routes []Route) {
	for _, rt := range routes {
		name := rt.Template
		r.Get(rt.URI, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = h.tmpl.ExecuteTemplate(w, name, h.data())
		})
	}
}

// RegisterRedirects mounts a table of legacy redirects on r.
func RegisterRedirects(r chi.Router, rds []Redirect) {
	for _, rd := range rds {
		to, keep := rd.To, rd.KeepQuery
		r.Get(rd.From, func(w http.ResponseWriter, req *http.Request) {
			target := to
			if keep {
				if qs := req.URL.RawQuery; qs != "" {
					target += "?" + qs
				}
			}
			http.Redirect(w, req, target, http.StatusFound)
		})
	}
}
