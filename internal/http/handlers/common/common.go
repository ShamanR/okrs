package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/okr"
	"okrs/internal/store"
)

type Dependencies struct {
	Store     *store.Store
	Logger    *slog.Logger
	Templates *template.Template
	Zone      *time.Location
}

func RenderTemplate(w http.ResponseWriter, tmpl *template.Template, name string, data any, logger *slog.Logger) {
	if name == "base" {
		pageTitle, contentTemplate, err := extractLayoutFields(data)
		if err != nil {
			RenderError(w, logger, err)
			return
		}
		var content bytes.Buffer
		if err := tmpl.ExecuteTemplate(&content, contentTemplate, data); err != nil {
			RenderError(w, logger, err)
			return
		}
		layout := struct {
			PageTitle   string
			ContentHTML template.HTML
		}{
			PageTitle:   pageTitle,
			ContentHTML: template.HTML(content.String()),
		}
		if err := tmpl.ExecuteTemplate(w, name, layout); err != nil {
			RenderError(w, logger, err)
		}
		return
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		RenderError(w, logger, err)
		return
	}
	_, _ = w.Write(buf.Bytes())
}

func extractLayoutFields(data any) (string, string, error) {
	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return "", "", fmt.Errorf("layout data must be a struct")
	}
	pageTitle := value.FieldByName("PageTitle")
	contentTemplate := value.FieldByName("ContentTemplate")
	if !pageTitle.IsValid() || !contentTemplate.IsValid() {
		return "", "", fmt.Errorf("layout fields missing")
	}
	return pageTitle.String(), contentTemplate.String(), nil
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

func ParseQuarter(r *http.Request, zone *time.Location) (int, int) {
	q := r.URL.Query().Get("quarter")
	if q != "" {
		parts := strings.Split(q, "-")
		if len(parts) == 2 {
			year, _ := strconv.Atoi(parts[0])
			quarter, _ := strconv.Atoi(parts[1])
			if year > 0 && quarter >= 1 && quarter <= 4 {
				return year, quarter
			}
		}
	}

	year, _ := strconv.Atoi(r.FormValue("year"))
	quarter, _ := strconv.Atoi(r.FormValue("quarter"))
	if year > 0 && quarter >= 1 && quarter <= 4 {
		return year, quarter
	}

	current := store.CurrentQuarter(time.Now().In(zone))
	return current.Year, current.Quarter
}

type QuarterOption struct {
	Year     int
	Quarter  int
	Selected bool
}

func BuildQuarterOptions(selectedYear, selectedQuarter int, zone *time.Location) []QuarterOption {
	current := store.CurrentQuarter(time.Now().In(zone))
	options := make([]QuarterOption, 0, 9)
	start := -4
	end := 4
	for i := start; i <= end; i++ {
		q := addQuarters(current, i)
		options = append(options, QuarterOption{Year: q.Year, Quarter: q.Quarter, Selected: q.Year == selectedYear && q.Quarter == selectedQuarter})
	}
	return options
}

func addQuarters(q domain.Quarter, delta int) domain.Quarter {
	total := (q.Year*4 + (q.Quarter - 1)) + delta
	if total < 0 {
		total = 0
	}
	newYear := total / 4
	newQuarter := total%4 + 1
	return domain.Quarter{Year: newYear, Quarter: newQuarter}
}

func CalculateGoalProgress(goal domain.Goal) int {
	for i := range goal.KeyResults {
		goal.KeyResults[i].Progress = CalculateKRProgress(goal.KeyResults[i])
	}
	return okr.GoalProgress(goal.KeyResults)
}

func CalculateKRProgress(kr domain.KeyResult) int {
	switch kr.Kind {
	case domain.KRKindProject:
		if kr.Project == nil {
			return 0
		}
		return okr.ProjectProgress(kr.Project.Stages)
	case domain.KRKindPercent:
		if kr.Percent == nil {
			return 0
		}
		return okr.PercentProgress(kr.Percent.StartValue, kr.Percent.TargetValue, kr.Percent.CurrentValue, kr.Percent.Checkpoints)
	case domain.KRKindLinear:
		if kr.Linear == nil {
			return 0
		}
		return okr.LinearProgress(kr.Linear.StartValue, kr.Linear.TargetValue, kr.Linear.CurrentValue)
	case domain.KRKindBoolean:
		if kr.Boolean == nil {
			return 0
		}
		return okr.BooleanProgress(kr.Boolean.IsDone)
	default:
		return 0
	}
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
	case domain.TeamTypeCluster, domain.TeamTypeUnit, domain.TeamTypeTeam:
		return true
	default:
		return false
	}
}

func TeamTypeLabel(t domain.TeamType) string {
	switch t {
	case domain.TeamTypeCluster:
		return "Кластер"
	case domain.TeamTypeUnit:
		return "Юнит"
	case domain.TeamTypeTeam:
		return "Команда"
	default:
		return "Команда"
	}
}

func ValidTeamQuarterStatus(status domain.TeamQuarterStatus) bool {
	switch status {
	case domain.TeamQuarterStatusNoGoals, domain.TeamQuarterStatusForming, domain.TeamQuarterStatusInProgress, domain.TeamQuarterStatusValidated, domain.TeamQuarterStatusClosed:
		return true
	default:
		return false
	}
}

func TeamQuarterStatusLabel(status domain.TeamQuarterStatus) string {
	switch status {
	case domain.TeamQuarterStatusNoGoals:
		return "Нет целей"
	case domain.TeamQuarterStatusForming:
		return "Черновик целей"
	case domain.TeamQuarterStatusInProgress:
		return "Готовы к валидации"
	case domain.TeamQuarterStatusValidated:
		return "Провалидировано"
	case domain.TeamQuarterStatusClosed:
		return "Цели закрыты"
	default:
		return "Нет целей"
	}
}

func ValidKRKind(k domain.KRKind) bool {
	switch k {
	case domain.KRKindProject, domain.KRKindPercent, domain.KRKindLinear, domain.KRKindBoolean:
		return true
	default:
		return false
	}
}

func FindGoalIDByKR(ctx context.Context, store *store.Store, krID int64) (int64, error) {
	var goalID int64
	err := store.DB.QueryRow(ctx, `SELECT goal_id FROM key_results WHERE id=$1`, krID).Scan(&goalID)
	return goalID, err
}

func FindGoalIDByStage(ctx context.Context, store *store.Store, stageID int64) (int64, error) {
	var goalID int64
	err := store.DB.QueryRow(ctx, `
		SELECT kr.goal_id
		FROM kr_project_stages s
		JOIN key_results kr ON kr.id = s.key_result_id
		WHERE s.id=$1`, stageID).Scan(&goalID)
	return goalID, err
}

func ParseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func ValidateStageWeights(existing []domain.KRProjectStage, newWeight int) error {
	var total int
	for _, stage := range existing {
		total += stage.Weight
	}
	if newWeight <= 0 || newWeight > 100 || total+newWeight != 100 {
		return errors.New("Сумма весов этапов должна быть равна 100")
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
