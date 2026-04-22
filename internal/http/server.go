package http

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"okrs/internal/auth"
	apiadmin "okrs/internal/http/handlers/api/v1/admin"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	apigoals "okrs/internal/http/handlers/api/v1/goals"
	apihierarhy "okrs/internal/http/handlers/api/v1/hierarhy"
	apikrs "okrs/internal/http/handlers/api/v1/krs"
	apiperiods "okrs/internal/http/handlers/api/v1/periods"
	apiteams "okrs/internal/http/handlers/api/v1/teams"
	"okrs/internal/http/handlers/web/admin"
	"okrs/internal/http/handlers/web/authhandler"
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
	auth    *auth.Manager
	policy  *auth.PolicyEvaluator
}

func NewServer(st *store.Store, logger *slog.Logger, zone *time.Location, authMgr *auth.Manager) (*Server, error) {
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
	return &Server{
		store:   st,
		logger:  logger,
		tmpl:    tmpl,
		zone:    zone,
		service: service.New(st),
		auth:    authMgr,
		policy:  auth.NewPolicyEvaluator(st, logger),
	}, nil
}

func (s *Server) Routes() http.Handler {
	deps := common.Dependencies{Service: s.service, Logger: s.logger, Templates: s.tmpl, Zone: s.zone}
	r := chi.NewRouter()

	csrf := middleware.NewCSRF()

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))

	r.Group(func(r chi.Router) {
		r.Use(auth.AccessLogMiddleware(s.logger))

		if s.auth.Disabled() {
			r.Use(auth.AnonymousUserMiddleware)
		} else {
			r.Use(auth.SessionMiddleware(s.auth))
		}

		// Auth routes — public, no CSRF (OAuth callbacks use GET).
		authH := authhandler.New(s.auth, s.tmpl, s.logger)
		r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
			if s.auth.Disabled() {
				http.Redirect(w, r, "/teamOkrs", http.StatusFound)
				return
			}
			authH.HandleLogin(w, r)
		})
		r.Get("/auth/{provider}/start", authH.HandleProviderStart)
		r.Get("/auth/{provider}/callback", authH.HandleCallback)
		r.Post("/logout", authH.HandleLogout)

		// Legacy redirects for bookmarks.
		r.Get("/teams", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/teams", http.StatusFound)
		})
		r.Get("/periods", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/periods", http.StatusFound)
		})

		// Protected routes.
		r.Group(func(r chi.Router) {
			if !s.auth.Disabled() {
				r.Use(auth.RequireAuthMiddleware)
				r.Use(auth.ScopeMiddleware(s.policy, s.auth))
			}
			r.Use(csrf.Handler)

			s.registerWebRoutes(r, deps)
			s.registerApiRoutes(r)
			s.registerAdminRoutes(r, deps)
		})
	})

	return r
}

func (s *Server) registerWebRoutes(r chi.Router, deps common.Dependencies) {
	teamsHandler := teams.New(deps)
	goalsHandler := goals.New(deps)
	krHandler := keyresults.New(deps)

	r.Get("/teamOkrs", teamsHandler.HandleTeamOKRs)
	r.Get("/teams/{teamID}/okr", teamsHandler.HandleTeamOKR)
	r.Post("/teams/{teamID}/okr", teamsHandler.HandleCreateGoal)

	r.Post("/goals/{goalID}/comments", goalsHandler.HandleAddGoalComment)
	r.Post("/goals/{goalID}/key-results", goalsHandler.HandleAddKeyResult)
	r.Post("/goals/{goalID}/delete", goalsHandler.HandleDeleteGoal)
	r.Post("/goals/{goalID}/update", goalsHandler.HandleUpdateGoal)
	r.Post("/goals/{goalID}/share", goalsHandler.HandleUpdateGoalShare)

	r.Post("/key-results/{krID}/comments", krHandler.HandleAddKRComment)
	r.Post("/key-results/{krID}/move-up", krHandler.HandleMoveKeyResultUp)
	r.Post("/key-results/{krID}/move-down", krHandler.HandleMoveKeyResultDown)
	r.Post("/key-results/{krID}/delete", krHandler.HandleDeleteKeyResult)
	r.Post("/key-results/{krID}/update", krHandler.HandleUpdateKeyResult)
}

func (s *Server) registerAdminRoutes(r chi.Router, deps common.Dependencies) {
	teamsHandler := teams.New(deps)
	periodsHandler := periods.New(deps)
	adminHandler := admin.New(s.store, s.tmpl, s.policy, s.logger)
	adminAPI := apiadmin.New(s.store, s.auth)

	r.Group(func(r chi.Router) {
		if !s.auth.Disabled() {
			r.Use(auth.RequireAdminMiddleware)
		}

		r.Get("/admin", adminHandler.HandleIndex)
		r.Get("/admin/access", adminHandler.HandleAccess)
		r.Get("/admin/users/{userID}", adminHandler.HandleUserDetail)

		r.Get("/admin/teams", teamsHandler.HandleTeamManagement)
		r.Get("/admin/teams/new", teamsHandler.HandleNewTeam)
		r.Post("/admin/teams", teamsHandler.HandleCreateTeam)
		r.Get("/admin/teams/{teamID}/edit", teamsHandler.HandleEditTeam)
		r.Post("/admin/teams/{teamID}/update", teamsHandler.HandleUpdateTeam)
		r.Post("/admin/teams/{teamID}/delete", teamsHandler.HandleDeleteTeam)
		r.Post("/admin/teams/{teamID}/restore", teamsHandler.HandleRestoreTeam)
		r.Post("/admin/teams/{teamID}/hard-delete", teamsHandler.HandleHardDeleteTeam)

		r.Get("/admin/periods", periodsHandler.HandlePeriods)
		r.Post("/admin/periods", periodsHandler.HandleCreatePeriod)
		r.Get("/admin/periods/{periodID}/edit", periodsHandler.HandleEditPeriod)
		r.Post("/admin/periods/{periodID}/update", periodsHandler.HandleUpdatePeriod)
		r.Post("/admin/periods/{periodID}/delete", periodsHandler.HandleDeletePeriod)
		r.Post("/admin/periods/{periodID}/move-up", periodsHandler.HandleMovePeriodUp)
		r.Post("/admin/periods/{periodID}/move-down", periodsHandler.HandleMovePeriodDown)

		r.Get("/api/v1/admin/users", adminAPI.HandleListUsers)
		r.Get("/api/v1/admin/users/{userID}", adminAPI.HandleGetUser)
		r.Post("/api/v1/admin/users/{userID}/admin", adminAPI.HandleGrantAdmin)
		r.Delete("/api/v1/admin/users/{userID}/admin", adminAPI.HandleRevokeAdmin)
		r.Get("/api/v1/admin/users/{userID}/grants", adminAPI.HandleListGrants)
		r.Post("/api/v1/admin/users/{userID}/grants", adminAPI.HandleAddGrant)
		r.Delete("/api/v1/admin/users/{userID}/grants/{teamID}", adminAPI.HandleRemoveGrant)
		r.Get("/api/v1/admin/settings/access", adminAPI.HandleGetAccessSettings)
		r.Post("/api/v1/admin/settings/access", adminAPI.HandleUpdateAccessSettings)
	})
}

func (s *Server) registerApiRoutes(r chi.Router) {
	r.Get("/api/v1/hierarchy", apihierarhy.New(s.service).HandleHierarchy)
	r.Get("/api/v1/periods", apiperiods.New(s.service).HandlePeriods)
	r.Get("/api/v1/me", apiadmin.HandleMe)

	teamHandlers := apiteams.New(s.service)
	r.Get("/api/v1/teams/{teamID}", teamHandlers.HandleTeam)
	r.Get("/api/v1/teams/{teamID}/okrs", teamHandlers.HandleTeamOKRs)
	r.Get("/api/v1/teams/{teamID}/overview", teamHandlers.HandleTeamOverview)
	r.Post("/api/v1/teams/{teamID}/status", teamHandlers.HandleUpdateTeamPeriodStatus)

	goalsHandler := apigoals.New(s.service)
	r.Get("/api/v1/goals/{goalID}", goalsHandler.HandleGoal)
	r.Post("/api/v1/goals/{goalID}/share", goalsHandler.HandleShareGoal)
	r.Post("/api/v1/goals/{goalID}/weight", goalsHandler.HandleUpdateGoalWeight)
	r.Post("/api/v1/goals/{goalID}/comments", goalsHandler.HandleAddGoalComment)
	r.Post("/api/v1/goals/{goalID}", goalsHandler.HandleUpdateGoal)
	r.Post("/api/v1/goals/{goalID}/move-up", goalsHandler.HandleMoveGoalUp)
	r.Post("/api/v1/goals/{goalID}/move-down", goalsHandler.HandleMoveGoalDown)

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
