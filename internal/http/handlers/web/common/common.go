package common

import (
	"bytes"
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

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/service"
)

type Dependencies struct {
	Service   *service.Service
	Logger    *slog.Logger
	Templates *template.Template
	Zone      *time.Location
}

func RenderTemplate(w http.ResponseWriter, r *http.Request, tmpl *template.Template, name string, data any, logger *slog.Logger) {
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
			CurrentUser any
		}{
			PageTitle:   pageTitle,
			ContentHTML: template.HTML(content.String()),
			CurrentUser: auth.UserFromContext(r.Context()),
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
	case domain.KRKindProject, domain.KRKindPercent, domain.KRKindLinear, domain.KRKindBoolean:
		return true
	default:
		return false
	}
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
