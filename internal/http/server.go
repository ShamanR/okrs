package http

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	apigoals "okrs/internal/http/handlers/api/v1/goals"
	apihierarhy "okrs/internal/http/handlers/api/v1/hierarhy"
	apikrs "okrs/internal/http/handlers/api/v1/krs"
	apiperiods "okrs/internal/http/handlers/api/v1/periods"
	apiteams "okrs/internal/http/handlers/api/v1/teams"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/http/handlers/web/goals"
	"okrs/internal/http/handlers/web/keyresults"
	"okrs/internal/http/handlers/web/periods"
	"okrs/internal/http/handlers/web/teams"
	"okrs/internal/http/middleware"
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
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{store: store, logger: logger, tmpl: tmpl, zone: zone, service: service.New(store)}, nil
}

func (s *Server) Routes() http.Handler {
	deps := common.Dependencies{Service: s.service, Logger: s.logger, Templates: s.tmpl, Zone: s.zone}
	r := chi.NewRouter()

	csrf := middleware.NewCSRF()
	r.Use(csrf.Handler)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))
	s.registerWebRoutes(r, deps)
	s.registerApiRoutes(r)
	return r
}

func (s *Server) registerWebRoutes(r chi.Router, deps common.Dependencies) {
	teamsHandler := teams.New(deps)
	goalsHandler := goals.New(deps)
	krHandler := keyresults.New(deps)
	periodsHandler := periods.New(deps)

	// Teams and OKR pages (SSR + form actions).
	r.Get("/teams", teamsHandler.HandleTeamManagement)
	r.Get("/teamOkrs", teamsHandler.HandleTeamOKRs)
	r.Get("/teams/new", teamsHandler.HandleNewTeam)
	r.Post("/teams", teamsHandler.HandleCreateTeam)
	r.Get("/teams/{teamID}/edit", teamsHandler.HandleEditTeam)
	r.Post("/teams/{teamID}/update", teamsHandler.HandleUpdateTeam)
	r.Post("/teams/{teamID}/delete", teamsHandler.HandleDeleteTeam)
	r.Post("/teams/{teamID}/restore", teamsHandler.HandleRestoreTeam)
	r.Post("/teams/{teamID}/hard-delete", teamsHandler.HandleHardDeleteTeam)
	r.Get("/teams/{teamID}/okr", teamsHandler.HandleTeamOKR)
	r.Post("/teams/{teamID}/okr", teamsHandler.HandleCreateGoal)

	// Period management (SSR + form actions).
	r.Get("/periods", periodsHandler.HandlePeriods)
	r.Post("/periods", periodsHandler.HandleCreatePeriod)
	r.Get("/periods/{periodID}/edit", periodsHandler.HandleEditPeriod)
	r.Post("/periods/{periodID}/update", periodsHandler.HandleUpdatePeriod)
	r.Post("/periods/{periodID}/delete", periodsHandler.HandleDeletePeriod)
	r.Post("/periods/{periodID}/move-up", periodsHandler.HandleMovePeriodUp)
	r.Post("/periods/{periodID}/move-down", periodsHandler.HandleMovePeriodDown)

	// Goal pages and goal-level actions.
	r.Get("/goals/{goalID}", goalsHandler.HandleGoalDetail)
	r.Post("/goals/{goalID}/comments", goalsHandler.HandleAddGoalComment)
	r.Post("/goals/{goalID}/key-results", goalsHandler.HandleAddKeyResult)
	r.Post("/goals/{goalID}/delete", goalsHandler.HandleDeleteGoal)
	r.Post("/goals/{goalID}/update", goalsHandler.HandleUpdateGoal)
	r.Post("/goals/{goalID}/share", goalsHandler.HandleUpdateGoalShare)

	// Key Result and stage form actions.
	r.Post("/key-results/{krID}/comments", krHandler.HandleAddKRComment)
	r.Post("/key-results/{krID}/move-up", krHandler.HandleMoveKeyResultUp)
	r.Post("/key-results/{krID}/move-down", krHandler.HandleMoveKeyResultDown)
	r.Post("/key-results/{krID}/delete", krHandler.HandleDeleteKeyResult)
	r.Post("/key-results/{krID}/update", krHandler.HandleUpdateKeyResult)
}

func (s *Server) registerApiRoutes(r chi.Router) {
	r.Get("/api/v1/hierarchy", apihierarhy.New(s.service).HandleHierarchy)
	r.Get("/api/v1/periods", apiperiods.New(s.service).HandlePeriods)

	// teamHandlers
	teamHandlers := apiteams.New(s.service)
	r.Get("/api/v1/teams/{teamID}", teamHandlers.HandleTeam)
	r.Get("/api/v1/teams/{teamID}/okrs", teamHandlers.HandleTeamOKRs)
	r.Get("/api/v1/teams/{teamID}/overview", teamHandlers.HandleTeamOverview)
	r.Post("/api/v1/teams/{teamID}/status", teamHandlers.HandleUpdateTeamPeriodStatus)

	// goals
	goalsHandler := apigoals.New(s.service)
	r.Get("/api/v1/goals/{goalID}", goalsHandler.HandleGoal)
	r.Post("/api/v1/goals/{goalID}/share", goalsHandler.HandleShareGoal)
	r.Post("/api/v1/goals/{goalID}/weight", goalsHandler.HandleUpdateGoalWeight)
	r.Post("/api/v1/goals/{goalID}/comments", goalsHandler.HandleAddGoalComment)
	r.Post("/api/v1/goals/{goalID}", goalsHandler.HandleUpdateGoal)
	r.Post("/api/v1/goals/{goalID}/move-up", goalsHandler.HandleMoveGoalUp)
	r.Post("/api/v1/goals/{goalID}/move-down", goalsHandler.HandleMoveGoalDown)

	// krs
	krsHandler := apikrs.New(s.service)
	r.Post("/api/v1/goals/{goalID}/key-results", krsHandler.HandleCreateKeyResult)
	r.Post("/api/v1/krs/{krID}/progress/percent", krsHandler.HandleUpdatePercentProgress)
	r.Post("/api/v1/krs/{krID}/progress/boolean", krsHandler.HandleUpdateBooleanProgress)
	r.Post("/api/v1/krs/{krID}/progress/project", krsHandler.HandleUpdateProjectProgress)
	r.Post("/api/v1/krs/{krID}/comments", krsHandler.HandleAddKRComment)
	r.Post("/api/v1/krs/{krID}", krsHandler.HandleUpdateKeyResult)
	r.Post("/api/v1/krs/{krID}/move-up", krsHandler.HandleMoveKeyResultUp)
	r.Post("/api/v1/krs/{krID}/move-down", krsHandler.HandleMoveKeyResultDown)

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		v1.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	})
}
