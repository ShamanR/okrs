// Package delete serves POST /goals/{goalID}/delete — the legacy form endpoint the tracker still uses.
package delete

import (
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/web/common"
	goaluc "okrs/internal/usecase/goal"
)

type Handler struct {
	deps  common.Dependencies
	goals *goaluc.UseCase
}

func New(deps common.Dependencies, goals *goaluc.UseCase) *Handler {
	return &Handler{deps: deps, goals: goals}
}

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	scope, ok := auth.TenantScopeFromContext(ctx)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var requestingTeamID int64
	if s := r.FormValue("team_id"); s != "" {
		requestingTeamID, _ = common.ParseID(s)
	}
	teamID, periodID, err := h.goals.Delete(ctx, scope, goalID, requestingTeamID, auth.UserIDFromContext(ctx))
	if err != nil {
		if errors.Is(err, domain.ErrPeriodClosed) {
			common.RenderError(w, h.deps.Logger, fmt.Errorf("Период закрыт, изменения недоступны"))
			return
		}
		common.RenderError(w, h.deps.Logger, err)
		return
	}
	redirectToTeam(w, r, teamID, periodID)
}

const maxMultipartMemory = 32 << 20

func redirectToTeam(w http.ResponseWriter, r *http.Request, teamID, periodID int64) {
	http.Redirect(w, r, fmt.Sprintf("/teams/%d/okr?period_id=%d", teamID, periodID), http.StatusSeeOther)
}
