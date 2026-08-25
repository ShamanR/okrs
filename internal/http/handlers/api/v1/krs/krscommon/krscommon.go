// Package krscommon holds what the key-result endpoints share: tenant-scope extraction
// and parsing of the per-kind metadata a KR carries.
//
// A leaf package for the same reason as goals/goalcommon: the parent krs package mounts
// the sub-packages, so importing it back for helpers would be an import cycle.
package krscommon

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/handlers/web/common"
	keyresultsvc "okrs/internal/service/keyresult"
	"okrs/internal/store/krs"

	"github.com/go-chi/chi/v5"
)

// TenantScope extracts TenantScope from the request context, writing 403 and
// returning false if it is missing.
// TenantScope pulls the active tenant out of the request context, answering 403 when
// it is absent. Shared by every /api/v1/krs/** endpoint and by goals/key-results.
func TenantScope(w http.ResponseWriter, r *http.Request) (domain.TenantScope, bool) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
	}
	return scope, ok
}

// ParseMeta parses meta fields for a key result based on kind.
func ParseMeta(r *http.Request, kind domain.KRKind) (keyresultsvc.MetaInput, error) {
	switch kind {
	case domain.KRKindNumerical:
		return common.ParseNumericalMeta(r)
	case domain.KRKindBoolean:
		done := r.FormValue("boolean_done") == "true"
		return keyresultsvc.MetaInput{BooleanDone: done}, nil
	case domain.KRKindProject:
		stages, err := ParseProjectStages(r)
		if err != nil {
			return keyresultsvc.MetaInput{}, err
		}
		return keyresultsvc.MetaInput{ProjectStages: stages}, nil
	default:
		return keyresultsvc.MetaInput{}, nil
	}
}

// ParseProjectStages parses project stage fields from a multipart form.
func ParseProjectStages(r *http.Request) ([]krs.ProjectStageInput, error) {
	stages := make([]krs.ProjectStageInput, 0, 4)
	titles := r.Form["step_title[]"]
	weights := r.Form["step_weight[]"]
	dones := r.Form["step_done[]"]
	sortOrder := 1

	for i, title := range titles {
		trimmed := strings.TrimSpace(title)
		if trimmed == "" {
			continue
		}
		weightValue := ""
		if i < len(weights) {
			weightValue = weights[i]
		}
		weight := common.ParseIntField(weightValue)
		if weight <= 0 || weight > 100 {
			return nil, fmt.Errorf("Вес шага должен быть 1..100")
		}
		isDone := false
		if i < len(dones) {
			isDone = dones[i] == "true"
		}
		stages = append(stages, krs.ProjectStageInput{
			Title:     trimmed,
			Weight:    weight,
			IsDone:    isDone,
			SortOrder: sortOrder,
		})
		sortOrder++
	}

	if len(stages) == 0 {
		return nil, fmt.Errorf("Для Project KR требуется минимум один шаг")
	}
	return stages, nil
}

// KRGetter reads one key result. *keyresult.Service satisfies it.
type KRGetter interface {
	Get(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error)
}

// GoalGetter reads one goal. *goal.Service satisfies it.
type GoalGetter interface {
	Get(ctx context.Context, scope domain.TenantScope, id int64) (domain.Goal, error)
}

// KRMover reorders a key result within its goal. *keyresult.Service satisfies it.
type KRMover interface {
	Get(ctx context.Context, scope domain.TenantScope, id int64) (domain.KeyResult, error)
	Move(ctx context.Context, scope domain.TenantScope, krID int64, direction int) error
}

// MoveDeps are the collaborators the move-up/move-down endpoints need.
type MoveDeps struct {
	KRs   KRMover
	Goals GoalGetter
}

// GoalForKR resolves the parent goal of a KR and returns it.
// Returns an error if the KR or its goal cannot be found.
// GoalForKR resolves the goal a key result belongs to. Every /api/v1/krs/** endpoint
// needs it: authorization is by the goal's team, not by the KR itself.
func GoalForKR(ctx context.Context, krs KRGetter, goals GoalGetter, scope domain.TenantScope, krID int64) (domain.Goal, error) {
	kr, err := krs.Get(ctx, scope, krID)
	if err != nil {
		return domain.Goal{}, err
	}
	return goals.Get(ctx, scope, kr.GoalID)
}

// MoveKeyResult is the body behind both …/move-up and …/move-down: the two URIs differ
// only by the direction. Lives here so both endpoint packages can share it without a cycle.
func MoveKeyResult(w http.ResponseWriter, r *http.Request, d MoveDeps, direction int) {
	krID, err := common.ParseID(chi.URLParam(r, "krID"))
	if err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid kr id", map[string]string{"kr_id": "invalid"})
		return
	}
	scope, ok := TenantScope(w, r)
	if !ok {
		return
	}
	goal, err := GoalForKR(r.Context(), d.KRs, d.Goals, scope, krID)
	if err != nil || !auth.CanAccessTeamFromCtx(r.Context(), goal.TeamID) {
		v1.WriteError(w, http.StatusNotFound, "NOT_FOUND", "key result not found", nil)
		return
	}
	if err := d.KRs.Move(r.Context(), scope, krID, direction); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to move key result", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
