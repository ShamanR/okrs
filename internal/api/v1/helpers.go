package v1

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/common"
	"okrs/internal/service"
	"okrs/internal/store"
)

// parseKeyResultMeta parses meta fields for a key result based on kind.
func parseKeyResultMeta(r *http.Request, kind domain.KRKind) (service.KeyResultMetaInput, error) {
	switch kind {
	case domain.KRKindPercent:
		start := common.ParseFloatField(r.FormValue("percent_start"))
		target := common.ParseFloatField(r.FormValue("percent_target"))
		if start == target {
			return service.KeyResultMetaInput{}, fmt.Errorf("Start и Target не должны быть равны")
		}
		return service.KeyResultMetaInput{
			PercentStart:   start,
			PercentTarget:  target,
			PercentCurrent: common.ParseFloatField(r.FormValue("percent_current")),
		}, nil
	case domain.KRKindLinear:
		start := common.ParseFloatField(r.FormValue("linear_start"))
		target := common.ParseFloatField(r.FormValue("linear_target"))
		if start == target {
			return service.KeyResultMetaInput{}, fmt.Errorf("Start и Target не должны быть равны")
		}
		return service.KeyResultMetaInput{
			LinearStart:   start,
			LinearTarget:  target,
			LinearCurrent: common.ParseFloatField(r.FormValue("linear_current")),
		}, nil
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
func parseProjectStages(r *http.Request) ([]store.ProjectStageInput, error) {
	stages := make([]store.ProjectStageInput, 0, 4)
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
		stages = append(stages, store.ProjectStageInput{
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

// parseOptionalID returns an optional int64 pointer from a string.
func parseOptionalID(value string) (*int64, error) {
	if value == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
