// Package bulkstatus holds the body behind the four bulk period-status endpoints
// (admin and member-scoped, activate and close). They differ only by target status and
// by whether the change is narrowed to the caller's teams — a leaf package is the only
// place all four endpoint packages can share it without a cycle.
package bulkstatus

import (
	"context"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	perioduc "okrs/internal/usecase/period"

	"github.com/go-chi/chi/v5"
)

// TeamScopeResolver lists the teams a user leads — the "my_teams" scope.
// *grants.GrantsCache satisfies it.
type TeamScopeResolver interface {
	ListLeadTeamScope(ctx context.Context, scope domain.TenantScope, userUDID string) ([]int64, error)
}

// TeamLister enumerates a tenant's teams; the org-wide scope needs the full list.
type TeamLister interface {
	ListAll(ctx context.Context, scope domain.TenantScope) ([]domain.Team, error)
}

// Run applies a bulk status transition over the whole tenant (admin-only path).
// Run performs an admin-scope bulk status change over every team of a period. Shared by
// the activate and close endpoints: they differ only by the target status.
func Run(w http.ResponseWriter, r *http.Request, uc *perioduc.UseCase, teams TeamLister, leads TeamScopeResolver, target domain.TeamPeriodStatus) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	res, err := uc.BulkSetTeamPeriodStatus(r.Context(), scope, periodID, target, auth.UserIDFromContext(r.Context()), nil)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to apply bulk operation", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, res)
}

// RunScoped applies a bulk status transition within the requested scope:
// my_teams (teams the caller leads + descendants) or org (whole tenant, admin-only).
// RunScoped is the member-facing variant: it narrows the change to the teams the caller
// leads. Org-wide scope stays admin-gated inside the resolver.
func RunScoped(w http.ResponseWriter, r *http.Request, uc *perioduc.UseCase, teams TeamLister, leads TeamScopeResolver, target domain.TeamPeriodStatus) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "no active tenant", nil)
		return
	}
	periodID, err := common.ParseID(chi.URLParam(r, "periodID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid period id", nil)
		return
	}
	teamFilter, ok := ResolveOverviewScope(w, r, teams, leads, scope)
	if !ok {
		return
	}
	res, err := uc.BulkSetTeamPeriodStatus(r.Context(), scope, periodID, target, auth.UserIDFromContext(r.Context()), teamFilter)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to apply bulk operation", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, res)
}

// ResolveOverviewScope reads ?scope=my_teams|org and returns the team filter to apply
// (nil = whole organization). On an authorization/validation problem it writes the error
// response and returns ok=false. Shared by the scoped overview and scoped bulk handlers.
// ResolveOverviewScope decides which teams a period overview covers: "my_teams"
// (default, the caller's own) or "org" (admin only). Shared by the scoped overview and
// the member-facing bulk operations, which apply the same rule.
func ResolveOverviewScope(w http.ResponseWriter, r *http.Request, teams TeamLister, leads TeamScopeResolver, scope domain.TenantScope) (map[int64]bool, bool) {
	overviewScope := r.URL.Query().Get("scope")
	if overviewScope == "" {
		overviewScope = "my_teams"
	}
	role, _ := auth.ActiveRoleFromContext(r.Context())
	isAdmin := role == domain.RoleAdmin

	switch overviewScope {
	case "org":
		if !isAdmin {
			v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "org scope requires admin", nil)
			return nil, false
		}
		return nil, true
	case "my_teams":
		udid := ""
		if user := auth.UserFromContext(r.Context()); user != nil {
			udid = user.UDID
		}
		ids, err := leads.ListLeadTeamScope(r.Context(), scope, udid)
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to resolve scope", nil)
			return nil, false
		}
		filter := make(map[int64]bool, len(ids))
		for _, id := range ids {
			filter[id] = true
		}
		return filter, true
	default:
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid scope", nil)
		return nil, false
	}
}
