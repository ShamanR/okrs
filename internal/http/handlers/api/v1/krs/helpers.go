package krs

import (
	"fmt"
	"net/http"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/service"
	"okrs/internal/store/krs"
	"strings"
)

// parseKeyResultMeta parses meta fields for a key result based on kind.
func parseKeyResultMeta(r *http.Request, kind domain.KRKind) (service.KeyResultMetaInput, error) {
	switch kind {
	case domain.KRKindNumerical:
		return common.ParseNumericalMeta(r)
	case domain.KRKindBoolean:
		done := r.FormValue("boolean_done") == "true"
		return service.KeyResultMetaInput{BooleanDone: done}, nil
	case domain.KRKindProject:
		stages, err := parseProjectStages(r)
		if err != nil {
			return service.KeyResultMetaInput{}, err
		}
		return service.KeyResultMetaInput{ProjectStages: stages}, nil
	default:
		return service.KeyResultMetaInput{}, nil
	}
}

// parseProjectStages parses project stage fields from a multipart form.
func parseProjectStages(r *http.Request) ([]krs.ProjectStageInput, error) {
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
