package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/service"
)

type Dependencies struct {
	Service   *service.Service
	Logger    *slog.Logger
	Templates *template.Template
	Zone      *time.Location
}

func RenderError(w http.ResponseWriter, logger *slog.Logger, err error) {
	logger.Error("request failed", slog.String("error", err.Error()))
	http.Error(w, "Произошла ошибка", http.StatusInternalServerError)
}

func RenderJSONError(w http.ResponseWriter, logger *slog.Logger, err error) {
	logger.Error("api failed", slog.String("error", err.Error()))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func WriteJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func ParsePeriodID(r *http.Request) (int64, error) {
	value := r.URL.Query().Get("period_id")
	if value == "" {
		value = r.URL.Query().Get("period")
	}
	if value == "" {
		value = r.FormValue("period_id")
	}
	if value == "" {
		value = r.FormValue("period")
	}
	if value == "" {
		return 0, nil
	}
	return ParseID(value)
}

func ValidateGoalInput(priority domain.Priority, workType domain.WorkType, focusType domain.FocusType, weight int) string {
	if weight < 0 || weight > 100 {
		return "Вес должен быть 0..100"
	}
	if !ValidPriority(priority) {
		return "Неверный приоритет"
	}
	if !ValidWorkType(workType) {
		return "Неверный тип работы"
	}
	if !ValidFocusType(focusType) {
		return "Неверный тип фокуса"
	}
	return ""
}

func ValidPriority(p domain.Priority) bool {
	switch p {
	case domain.PriorityP0, domain.PriorityP1, domain.PriorityP2, domain.PriorityP3:
		return true
	default:
		return false
	}
}

func ValidWorkType(t domain.WorkType) bool {
	switch t {
	case domain.WorkTypeDiscovery, domain.WorkTypeDelivery:
		return true
	default:
		return false
	}
}

func ValidFocusType(t domain.FocusType) bool {
	switch t {
	case domain.FocusProfitability, domain.FocusStability, domain.FocusSpeedEfficiency, domain.FocusTechIndependence:
		return true
	default:
		return false
	}
}

func ValidTeamType(t domain.TeamType) bool {
	switch t {
	case domain.TeamTypeDepartment, domain.TeamTypeCluster, domain.TeamTypeUnit, domain.TeamTypeGroup, domain.TeamTypeTeam, domain.TeamTypeSquad, domain.TeamTypeEmployee:
		return true
	default:
		return false
	}
}

func TeamTypeLabel(t domain.TeamType) string {
	switch t {
	case domain.TeamTypeDepartment:
		return "Департамент"
	case domain.TeamTypeCluster:
		return "Кластер"
	case domain.TeamTypeUnit:
		return "Юнит"
	case domain.TeamTypeGroup:
		return "Группа"
	case domain.TeamTypeTeam:
		return "Команда"
	case domain.TeamTypeSquad:
		return "Сквад"
	case domain.TeamTypeEmployee:
		return "Сотрудник"
	default:
		return "Команда"
	}
}

func ValidTeamPeriodStatus(status domain.TeamPeriodStatus) bool {
	switch status {
	case domain.TeamPeriodStatusNoGoals, domain.TeamPeriodStatusForming, domain.TeamPeriodStatusReady, domain.TeamPeriodStatusInProgress, domain.TeamPeriodStatusClosed:
		return true
	default:
		return false
	}
}

func TeamPeriodStatusLabel(status domain.TeamPeriodStatus) string {
	switch status {
	case domain.TeamPeriodStatusNoGoals:
		return "Нет целей"
	case domain.TeamPeriodStatusForming:
		return "Черновик"
	case domain.TeamPeriodStatusReady:
		return "К валидации"
	case domain.TeamPeriodStatusInProgress:
		return "В работе"
	case domain.TeamPeriodStatusClosed:
		return "Закрыты"
	default:
		return "Нет целей"
	}
}

func ValidKRKind(k domain.KRKind) bool {
	switch k {
	case domain.KRKindProject, domain.KRKindNumerical, domain.KRKindBoolean:
		return true
	default:
		return false
	}
}

// KRKindLabel returns the Russian UI alias for a KR kind.
func KRKindLabel(k domain.KRKind) string {
	switch k {
	case domain.KRKindBoolean:
		return "Бинарный"
	case domain.KRKindProject:
		return "Проектный"
	case domain.KRKindNumerical:
		return "Числовой"
	default:
		return string(k)
	}
}

// ParseNumericalMeta parses NUMERICAL meta fields (unit, values, optional checkpoints)
// from a form. Shared by the API and web KR handlers.
func ParseNumericalMeta(r *http.Request) (service.KeyResultMetaInput, error) {
	unit := strings.TrimSpace(r.FormValue("numerical_unit"))
	if !domain.IsValidKRUnit(unit) {
		return service.KeyResultMetaInput{}, fmt.Errorf("Недопустимая единица измерения")
	}
	checkpoints, err := parseNumericalCheckpoints(r)
	if err != nil {
		return service.KeyResultMetaInput{}, err
	}
	return service.KeyResultMetaInput{
		NumericalStart:       ParseFloatField(r.FormValue("numerical_start")),
		NumericalTarget:      ParseFloatField(r.FormValue("numerical_target")),
		NumericalCurrent:     ParseFloatField(r.FormValue("numerical_current")),
		NumericalUnit:        unit,
		NumericalCheckpoints: checkpoints,
	}, nil
}

// parseNumericalCheckpoints parses optional NUMERICAL checkpoints from parallel form arrays.
func parseNumericalCheckpoints(r *http.Request) ([]domain.KRNumericalCheckpoint, error) {
	values := r.Form["checkpoint_value[]"]
	percents := r.Form["checkpoint_percent[]"]
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]domain.KRNumericalCheckpoint, 0, len(values))
	seen := make(map[float64]struct{}, len(values))
	for i, raw := range values {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value := ParseFloatField(raw)
		percentStr := ""
		if i < len(percents) {
			percentStr = percents[i]
		}
		percent := ParseIntField(percentStr)
		if percent < 0 || percent > 100 {
			return nil, fmt.Errorf("Процент промежуточного значения должен быть 0..100")
		}
		if _, dup := seen[value]; dup {
			return nil, fmt.Errorf("Промежуточные значения не должны дублироваться")
		}
		seen[value] = struct{}{}
		out = append(out, domain.KRNumericalCheckpoint{Value: value, ProgressPercent: percent})
	}
	return out, nil
}

func ParseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func ValidateStageWeights(existing []domain.KRProjectStage, newWeight int) error {
	if newWeight <= 0 || newWeight > 100 {
		return errors.New("Вес этапа должен быть 1..100")
	}
	return nil
}

func ParseFloatField(value string) float64 {
	result, _ := strconv.ParseFloat(value, 64)
	return result
}

func ParseIntField(value string) int {
	result, _ := strconv.Atoi(value)
	return result
}

func TrimmedFormValue(r *http.Request, key string) string {
	return strings.TrimSpace(r.FormValue(key))
}
