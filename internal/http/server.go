package http

import (
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	apiv1 "okrs/internal/api/v1"
	"okrs/internal/domain"
	"okrs/internal/http/handlers/api"
	"okrs/internal/http/handlers/common"
	"okrs/internal/http/handlers/goals"
	"okrs/internal/http/handlers/keyresults"
	"okrs/internal/http/handlers/teams"
	"okrs/internal/service"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	store   *store.Store
	logger  *slog.Logger
	tmpl    *template.Template
	zone    *time.Location
	service *service.Service
}

func NewServer(store *store.Store, logger *slog.Logger, zone *time.Location) (*Server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"sumKRWeights": func(keyResults []domain.KeyResult) int {
			total := 0
			for _, kr := range keyResults {
				total += kr.Weight
			}
			return total
		},
		"sumStageWeights": func(stages []domain.KRProjectStage) int {
			total := 0
			for _, stage := range stages {
				total += stage.Weight
			}
			return total
		},
		"stageAt": func(stages []domain.KRProjectStage, index int) *domain.KRProjectStage {
			if index < 0 || index >= len(stages) {
				return nil
			}
			stage := stages[index]
			return &stage
		},
		"priorityBadgeClass": func(priority domain.Priority) string {
			switch priority {
			case domain.PriorityP0:
				return "text-bg-danger"
			case domain.PriorityP1, domain.PriorityP2:
				return "text-bg-success"
			case domain.PriorityP3:
				return "text-bg-secondary"
			default:
				return "text-bg-secondary"
			}
		},
		"objectiveStatus": func(weight, progress int) int {
			return int(math.Round(float64(weight*progress) / 100.0))
		},
		"relativeTime": func(t time.Time) string {
			if t.IsZero() {
				return "нет данных"
			}
			now := time.Now()
			if now.Before(t) {
				return "только что"
			}
			diff := now.Sub(t)
			if diff < time.Minute {
				return "только что"
			}
			if diff < time.Hour {
				minutes := int(diff.Minutes())
				return fmt.Sprintf("%d %s назад", minutes, pluralizeRu(minutes, "минуту", "минуты", "минут"))
			}
			if diff < 24*time.Hour {
				hours := int(diff.Hours())
				return fmt.Sprintf("%d %s назад", hours, pluralizeRu(hours, "час", "часа", "часов"))
			}
			days := int(diff.Hours() / 24)
			return fmt.Sprintf("%d %s назад", days, pluralizeRu(days, "день", "дня", "дней"))
		},
		"absoluteTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("02.01.2006 15:04")
		},
		"goalStatusLabel": func(goal domain.Goal, year, quarter int) string {
			if len(goal.KeyResults) == 0 {
				return "Нет данных"
			}
			totalWeight := 0
			latestUpdate := goal.UpdatedAt
			for _, kr := range goal.KeyResults {
				totalWeight += kr.Weight
				if kr.UpdatedAt.After(latestUpdate) {
					latestUpdate = kr.UpdatedAt
				}
			}
			if totalWeight == 0 {
				return "Нет данных"
			}
			start, end := quarterBounds(year, quarter, zone)
			planned := plannedProgress(nowInZone(zone), start, end)
			status := progressToStatus(goal.Progress, planned)
			if time.Since(latestUpdate) > 21*24*time.Hour && status == "В норме" {
				return "Риск"
			}
			return status
		},
		"goalStatusClass": func(label string) string {
			switch strings.ToLower(label) {
			case strings.ToLower("В норме"):
				return "text-bg-success"
			case strings.ToLower("Риск"):
				return "text-bg-warning"
			case strings.ToLower("Отставание"):
				return "text-bg-danger"
			default:
				return "text-bg-secondary"
			}
		},
		"plannedProgress": func(year, quarter int) int {
			start, end := quarterBounds(year, quarter, zone)
			return plannedProgress(nowInZone(zone), start, end)
		},
		"krContribution": func(weight, progress, totalWeight int) float64 {
			if totalWeight == 0 {
				return 0
			}
			return float64(weight*progress) / float64(totalWeight)
		},
		"krMetricSummary": func(kr domain.KeyResult) string {
			switch kr.Kind {
			case domain.KRKindPercent:
				if kr.Percent != nil {
					return fmt.Sprintf("Число: Start %.2f → Target %.2f (текущее %.2f)", kr.Percent.StartValue, kr.Percent.TargetValue, kr.Percent.CurrentValue)
				}
				return "Число: Start → Target"
			case domain.KRKindLinear:
				if kr.Linear != nil {
					return fmt.Sprintf("Число: Start %.2f → Target %.2f (текущее %.2f)", kr.Linear.StartValue, kr.Linear.TargetValue, kr.Linear.CurrentValue)
				}
				return "Число: Start → Target"
			case domain.KRKindBoolean:
				if kr.Boolean != nil {
					if kr.Boolean.IsDone {
						return "Бинарный: Выполнено — Да"
					}
					return "Бинарный: Выполнено — Нет"
				}
				return "Бинарный: Выполнено — Нет"
			case domain.KRKindProject:
				if kr.Project != nil {
					return fmt.Sprintf("Проект: этапов %d", len(kr.Project.Stages))
				}
				return "Проект: этапы не заданы"
			default:
				return ""
			}
		},
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{store: store, logger: logger, tmpl: tmpl, zone: zone, service: service.New(store)}, nil
}

func (s *Server) Routes() http.Handler {
	deps := common.Dependencies{Store: s.store, Service: s.service, Logger: s.logger, Templates: s.tmpl, Zone: s.zone}
	teamsHandler := teams.New(deps)
	goalsHandler := goals.New(deps)
	krHandler := keyresults.New(deps)
	apiHandler := api.New(deps)
	apiV1Handler := apiv1.NewHandler(s.service)

	r := chi.NewRouter()

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))

	r.Get("/teams", teamsHandler.HandleTeams)
	r.Get("/teams/new", teamsHandler.HandleNewTeam)
	r.Post("/teams", teamsHandler.HandleCreateTeam)
	r.Get("/teams/{teamID}/edit", teamsHandler.HandleEditTeam)
	r.Post("/teams/{teamID}/update", teamsHandler.HandleUpdateTeam)
	r.Post("/teams/{teamID}/delete", teamsHandler.HandleDeleteTeam)
	r.Get("/teams/{teamID}/okr", teamsHandler.HandleTeamOKR)
	r.Post("/teams/{teamID}/okr", teamsHandler.HandleCreateGoal)
	r.Post("/teams/{teamID}/okr/status", teamsHandler.HandleUpdateTeamQuarterStatus)

	r.Get("/goals/{goalID}", goalsHandler.HandleGoalDetail)
	r.Post("/goals/{goalID}/comments", goalsHandler.HandleAddGoalComment)
	r.Post("/goals/{goalID}/key-results", goalsHandler.HandleAddKeyResult)
	r.Post("/goals/{goalID}/key-results/weights", goalsHandler.HandleUpdateKeyResultWeights)
	r.Post("/goals/{goalID}/move-up", goalsHandler.HandleMoveGoalUp)
	r.Post("/goals/{goalID}/move-down", goalsHandler.HandleMoveGoalDown)
	r.Post("/goals/{goalID}/delete", goalsHandler.HandleDeleteGoal)
	r.Post("/goals/{goalID}/update", goalsHandler.HandleUpdateGoal)
	r.Post("/goals/{goalID}/share", goalsHandler.HandleUpdateGoalShare)
	r.Get("/goals/year", goalsHandler.HandleYearGoals)

	r.Post("/key-results/{krID}/stages", krHandler.HandleAddStage)
	r.Post("/stages/{stageID}/toggle", krHandler.HandleToggleStage)
	r.Post("/key-results/{krID}/percent", krHandler.HandleUpdatePercentCurrent)
	r.Post("/key-results/{krID}/linear", krHandler.HandleUpdateLinearCurrent)
	r.Post("/key-results/{krID}/checkpoints", krHandler.HandleAddCheckpoint)
	r.Post("/key-results/{krID}/boolean", krHandler.HandleUpdateBoolean)
	r.Post("/key-results/{krID}/project-stages", krHandler.HandleUpdateProjectStages)
	r.Post("/key-results/{krID}/comments", krHandler.HandleAddKRComment)
	r.Post("/key-results/{krID}/move-up", krHandler.HandleMoveKeyResultUp)
	r.Post("/key-results/{krID}/move-down", krHandler.HandleMoveKeyResultDown)
	r.Post("/key-results/{krID}/delete", krHandler.HandleDeleteKeyResult)
	r.Post("/key-results/{krID}/update", krHandler.HandleUpdateKeyResult)

	r.Route("/api", func(r chi.Router) {
		r.Get("/teams", apiHandler.HandleAPITeams)
		r.Get("/teams/{teamID}/goals", apiHandler.HandleAPITeamGoals)
		r.Get("/goals/{goalID}", apiHandler.HandleAPIGoal)
	})

	r.Mount("/api/v1", apiV1Handler.Routes())

	return r
}

func pluralizeRu(value int, singular, few, many string) string {
	if value%100 >= 11 && value%100 <= 14 {
		return many
	}
	switch value % 10 {
	case 1:
		return singular
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

func quarterBounds(year, quarter int, zone *time.Location) (time.Time, time.Time) {
	if quarter < 1 || quarter > 4 {
		quarter = 1
	}
	startMonth := time.Month((quarter-1)*3 + 1)
	start := time.Date(year, startMonth, 1, 0, 0, 0, 0, zone)
	end := start.AddDate(0, 3, 0)
	return start, end
}

func nowInZone(zone *time.Location) time.Time {
	if zone != nil {
		return time.Now().In(zone)
	}
	return time.Now()
}

func plannedProgress(now, start, end time.Time) int {
	if end.Before(start) {
		return 0
	}
	if now.Before(start) {
		return 0
	}
	if now.After(end) {
		return 100
	}
	total := end.Sub(start).Seconds()
	elapsed := now.Sub(start).Seconds()
	if total <= 0 {
		return 0
	}
	return int(math.Round((elapsed / total) * 100))
}

func progressToStatus(progress, planned int) string {
	switch {
	case planned == 0 && progress == 0:
		return "Нет данных"
	case progress >= planned-10:
		return "В норме"
	case progress >= planned-25:
		return "Риск"
	default:
		return "Отставание"
	}
}
