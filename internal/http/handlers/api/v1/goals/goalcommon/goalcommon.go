// Package goalcommon holds what every /api/v1/goals/** endpoint needs: the shared
// access check, request-parameter parsing and the goal→DTO mapping.
//
// It is a leaf package on purpose. The obvious home would be the parent goals package,
// but that one mounts the sub-packages via RegisterRoutes — importing it back for
// helpers would be an import cycle.
package goalcommon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/store/goals"
	"okrs/internal/store/shares"

	"github.com/go-chi/chi/v5"
)

// ShareLister is the narrow port CanAccess needs: *goalshare.Service satisfies it.
type ShareLister interface {
	List(ctx context.Context, scope domain.TenantScope, goalID int64) ([]shares.GoalShare, error)
}

// canAccessGoal reports whether the current request may act on a goal that is
// visible to the user: it must be owned by an accessible team or shared into one.
// A shared goal appears on the cards of every team it is shared into, so comment /
// resolve actions must accept users who can reach any of those teams — not only the
// owner team. Mirrors the shared-goal visibility used by the OKR list and move/leave-share.
// CanAccess reports whether the caller may see this goal: it owns the goal's team, or
// the goal is shared with a team the caller may see. Declared here rather than on each
// handler because seven endpoints under /api/v1/goals need the same check.
func CanAccess(ctx context.Context, shares ShareLister, scope domain.TenantScope, goal domain.Goal) bool {
	if auth.CanAccessTeamFromCtx(ctx, goal.TeamID) {
		return true
	}
	shareList, err := shares.List(ctx, scope, goal.ID)
	if err != nil {
		return false
	}
	for _, sh := range shareList {
		if auth.CanAccessTeamFromCtx(ctx, sh.TeamID) {
			return true
		}
	}
	return false
}

// parseTeamID extracts team_id from either a JSON body (tracker SPA via apiPost)
// or a form field (legacy multipart page). Returns 0 when absent/invalid.
func TeamID(r *http.Request) int64 {
	if raw := strings.TrimSpace(r.FormValue("team_id")); raw != "" {
		if id, err := common.ParseID(raw); err == nil {
			return id
		}
		return 0
	}
	var req struct {
		TeamID int64 `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return 0
	}
	return req.TeamID
}

// allowedTeams returns the caller's allowed team IDs and whether they are unrestricted
// (admin: nil allowed set). Mirrors how board/list endpoints read scope from context.
func AllowedTeams(r *http.Request) (allowed []int64, adminAll bool) {
	allowed, _ = auth.AllowedTeamIDsFromCtx(r.Context())
	return allowed, allowed == nil
}
func OptionalID(value string) (*int64, error) {
	if value == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
func GoalResponse(goal domain.Goal, userRefs map[string]*dto.UserRef) dto.GoalResponse {
	comments := make([]dto.GoalComment, 0, len(goal.Comments))
	for _, comment := range goal.Comments {
		comments = append(comments, v1.MapGoalComment(comment))
	}
	krList := make([]dto.KeyResult, 0, len(goal.KeyResults))
	for _, kr := range goal.KeyResults {
		krList = append(krList, v1.MapKeyResult(kr))
	}
	goalDetail := dto.GoalDetails{
		ID:          goal.ID,
		TeamID:      goal.TeamID,
		PeriodID:    goal.PeriodID,
		Title:       goal.Title,
		Description: goal.Description,
		Priority:    string(goal.Priority),
		Weight:      goal.Weight,
		WorkType:    string(goal.WorkType),
		FocusType:   string(goal.FocusType),
		Owners:      v1.ResolveOwnersByUDIDs(goal.OwnerUDIDs, goal.OwnerText, userRefs),
		Progress:    goal.Progress,
		KeyResults:  krList,
		Parents:     v1.MapGoalRefs(goal.Parents),
		Children:    v1.MapGoalRefs(goal.Children),
		CreatedAt:   goal.CreatedAt,
		UpdatedAt:   goal.UpdatedAt,
	}
	return dto.GoalResponse{Goal: goalDetail, Comments: comments}
}

// GoalGetter reads a single goal. *goal.Service satisfies it.
type GoalGetter interface {
	Get(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error)
}

// ShareGetter reads one goal↔team share row. *goalshare.Service satisfies it.
type ShareGetter interface {
	Get(ctx context.Context, scope domain.TenantScope, goalID, teamID int64) (shares.GoalShare, error)
}

// CommentSetter flips a comment's resolved flag. *goal.UseCase satisfies it.
type CommentSetter interface {
	SetCommentResolved(ctx context.Context, scope domain.TenantScope, goalID, commentID int64, resolved bool, userID int64) error
}

// Mover reorders a goal within one team's board. *goal.Service satisfies it.
type Mover interface {
	Move(ctx context.Context, scope domain.TenantScope, teamID, goalID int64, direction int) error
}

// ResolveDeps are the collaborators the resolve/unresolve endpoints need. A struct
// rather than one fat interface: the three methods come from three different services,
// so no single type could implement a combined port.
type ResolveDeps struct {
	Goals  GoalGetter
	Shares ShareLister
	UC     CommentSetter
}

// MoveDeps are the collaborators the move-up/move-down endpoints need.
type MoveDeps struct {
	Goals  GoalGetter
	Shares ShareGetter
	Mover  Mover
}

// setGoalCommentResolved marks a comment resolved or clears the resolution.
// Access is gated by access to the parent goal — any user in the goal's scope
// may resolve/reopen, matching who may comment.
// SetCommentResolved is the body behind both …/comments/{id}/resolve and …/unresolve:
// the two URIs differ only by the boolean. Lives in this leaf package because it is the
// only place both endpoint packages can import without an import cycle.
func SetCommentResolved(w http.ResponseWriter, r *http.Request, d ResolveDeps, resolved bool) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	commentID, err := common.ParseID(chi.URLParam(r, "commentID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid comment id", map[string]string{"comment_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	goal, err := d.Goals.Get(r.Context(), scope, goalID)
	if err != nil || !CanAccess(r.Context(), d.Shares, scope, goal) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if err := d.UC.SetCommentResolved(r.Context(), scope, goalID, commentID, resolved, auth.UserIDFromContext(r.Context())); err != nil {
		if errors.Is(err, goals.ErrNotFound) {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "comment not found", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to update comment", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MoveGoal is the body behind both …/move-up and …/move-down: the two URIs differ only
// by the direction. Same reason as SetCommentResolved for living here.
func MoveGoal(w http.ResponseWriter, r *http.Request, d MoveDeps, direction int) {
	goalID, err := common.ParseID(chi.URLParam(r, "goalID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid goal id", map[string]string{"goal_id": "invalid"})
		return
	}
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	// The move is scoped to the team whose period is being viewed: a shared goal
	// has an independent order per team, so reordering must target that team's list.
	// The tracker SPA sends team_id as a JSON body; the legacy page as a form field.
	teamID := TeamID(r)
	if teamID == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid team id", map[string]string{"team_id": "invalid"})
		return
	}
	if !auth.CanAccessTeamFromCtx(r.Context(), teamID) {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "access denied", nil)
		return
	}
	// The goal must actually belong to teamID's view — either owned by it or shared into it.
	goal, err := d.Goals.Get(r.Context(), scope, goalID)
	if err != nil {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
		return
	}
	if goal.TeamID != teamID {
		if _, err := d.Shares.Get(r.Context(), scope, goalID, teamID); err != nil {
			v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "goal not found", nil)
			return
		}
	}
	if err := d.Mover.Move(r.Context(), scope, teamID, goalID, direction); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to move goal", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
