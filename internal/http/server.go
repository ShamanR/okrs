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
	"okrs/internal/http/handlers/web/authhandler"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/http/handlers/web/goals"
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
	goalsHandler := goals.New(deps)

	// Tracker SPA — serves the React shell for the main OKR tracker.
	trackerShell := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = s.tmpl.ExecuteTemplate(w, "tracker-shell", nil)
	}
	r.Get("/teamOkrs", trackerShell)
	r.Get("/teams/{teamID}/okr", trackerShell)

	// Goal delete is still used by tracker.js via the legacy form endpoint.
	r.Post("/goals/{goalID}/delete", goalsHandler.HandleDeleteGoal)
}

func (s *Server) registerAdminRoutes(r chi.Router, deps common.Dependencies) {
	adminAPI := apiadmin.New(s.store, s.auth)
	serviceH := apiadmin.NewServiceHandler(s.service)

	r.Group(func(r chi.Router) {
		if !s.auth.Disabled() {
			r.Use(auth.RequireAdminMiddleware)
		}

		// Admin SPA — all web admin pages serve the React shell.
		adminShell := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = s.tmpl.ExecuteTemplate(w, "admin-shell", nil)
		}
		r.Get("/admin", adminShell)
		r.Get("/admin/access", adminShell)
		r.Get("/admin/teams", adminShell)
		r.Get("/admin/periods", adminShell)
		// Legacy deep-links → root SPA.
		redirect := func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin", http.StatusFound) }
		r.Get("/admin/teams/new", redirect)
		r.Get("/admin/teams/{teamID}/edit", redirect)
		r.Get("/admin/periods/{periodID}/edit", redirect)
		r.Get("/admin/users/{userID}", redirect)

		// Admin user API.
		r.Get("/api/v1/admin/users", adminAPI.HandleListUsers)
		r.Get("/api/v1/admin/users/{userID}", adminAPI.HandleGetUser)
		r.Post("/api/v1/admin/users/{userID}/admin", adminAPI.HandleGrantAdmin)
		r.Delete("/api/v1/admin/users/{userID}/admin", adminAPI.HandleRevokeAdmin)
		r.Get("/api/v1/admin/users/{userID}/grants", adminAPI.HandleListGrants)
		r.Post("/api/v1/admin/users/{userID}/grants", adminAPI.HandleAddGrant)
		r.Delete("/api/v1/admin/users/{userID}/grants/{teamID}", adminAPI.HandleRemoveGrant)
		r.Get("/api/v1/admin/settings/access", adminAPI.HandleGetAccessSettings)
		r.Post("/api/v1/admin/settings/access", adminAPI.HandleUpdateAccessSettings)

		// Admin periods API.
		r.Post("/api/v1/admin/periods", serviceH.HandleCreatePeriod)
		r.Patch("/api/v1/admin/periods/{periodID}", serviceH.HandleUpdatePeriod)
		r.Delete("/api/v1/admin/periods/{periodID}", serviceH.HandleDeletePeriod)
		r.Post("/api/v1/admin/periods/{periodID}/move-up", serviceH.HandleMovePeriodUp)
		r.Post("/api/v1/admin/periods/{periodID}/move-down", serviceH.HandleMovePeriodDown)

		// Admin teams API.
		r.Get("/api/v1/admin/teams", serviceH.HandleListTeams)
		r.Post("/api/v1/admin/teams", serviceH.HandleCreateTeam)
		r.Patch("/api/v1/admin/teams/{teamID}", serviceH.HandleUpdateTeam)
		r.Delete("/api/v1/admin/teams/{teamID}", serviceH.HandleDeleteTeam)
		r.Post("/api/v1/admin/teams/{teamID}/restore", serviceH.HandleRestoreTeam)
		r.Delete("/api/v1/admin/teams/{teamID}/hard", serviceH.HandleHardDeleteTeam)
	})
}

func (s *Server) registerApiRoutes(r chi.Router) {
	r.Get("/api/v1/hierarchy", apihierarhy.New(s.service).HandleHierarchy)
	r.Get("/api/v1/periods", apiperiods.New(s.service).HandlePeriods)
	r.Get("/api/v1/me", apiadmin.HandleMe)
	r.Get("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		users, err := s.store.ListUsers(r.Context())
		if err != nil {
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load users", nil)
			return
		}
		type userItem struct {
			ID          int64  `json:"id"`
			DisplayName string `json:"display_name"`
			AvatarURL   string `json:"avatar_url"`
			Provider    string `json:"provider"`
			Email       string `json:"email"`
		}
		items := make([]userItem, 0, len(users))
		for _, u := range users {
			items = append(items, userItem{
				ID:          u.ID,
				DisplayName: u.DisplayName,
				AvatarURL:   u.AvatarURL,
				Provider:    u.Provider,
				Email:       u.Email,
			})
		}
		v1.WriteJSON(w, http.StatusOK, items)
	})

	teamHandlers := apiteams.New(s.service)
	r.Get("/api/v1/teams/{teamID}", teamHandlers.HandleTeam)
	r.Get("/api/v1/teams/{teamID}/okrs", teamHandlers.HandleTeamOKRs)
	r.Get("/api/v1/teams/{teamID}/overview", teamHandlers.HandleTeamOverview)
	r.Post("/api/v1/teams/{teamID}/status", teamHandlers.HandleUpdateTeamPeriodStatus)
	r.Post("/api/v1/teams/{teamID}/goals", teamHandlers.HandleCreateGoal)

	goalsHandler := apigoals.New(s.service)
	r.Get("/api/v1/goals/{goalID}", goalsHandler.HandleGoal)
	r.Post("/api/v1/goals/{goalID}/share", goalsHandler.HandleShareGoal)
	r.Post("/api/v1/goals/{goalID}/weight", goalsHandler.HandleUpdateGoalWeight)
	r.Post("/api/v1/goals/{goalID}/comments", goalsHandler.HandleAddGoalComment)
	r.Post("/api/v1/goals/{goalID}", goalsHandler.HandleUpdateGoal)
	r.Post("/api/v1/goals/{goalID}/move-up", goalsHandler.HandleMoveGoalUp)
	r.Post("/api/v1/goals/{goalID}/move-down", goalsHandler.HandleMoveGoalDown)
	r.Delete("/api/v1/goals/{goalID}", goalsHandler.HandleDeleteGoal)

	krsHandler := apikrs.New(s.service)
	r.Post("/api/v1/goals/{goalID}/key-results", krsHandler.HandleCreateKeyResult)
	r.Post("/api/v1/krs/{krID}/progress/percent", krsHandler.HandleUpdatePercentProgress)
	r.Post("/api/v1/krs/{krID}/progress/boolean", krsHandler.HandleUpdateBooleanProgress)
	r.Post("/api/v1/krs/{krID}/progress/project", krsHandler.HandleUpdateProjectProgress)
	r.Post("/api/v1/krs/{krID}/comments", krsHandler.HandleAddKRComment)
	r.Post("/api/v1/krs/{krID}", krsHandler.HandleUpdateKeyResult)
	r.Post("/api/v1/krs/{krID}/move-up", krsHandler.HandleMoveKeyResultUp)
	r.Post("/api/v1/krs/{krID}/move-down", krsHandler.HandleMoveKeyResultDown)
	r.Delete("/api/v1/krs/{krID}", krsHandler.HandleDeleteKeyResult)

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		v1.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	})
}
