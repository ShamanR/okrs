package http

import (
	"context"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"okrs/internal/auth"
	"okrs/internal/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	apiadmin "okrs/internal/http/handlers/api/v1/admin"
	apiconfig "okrs/internal/http/handlers/api/v1/config"
	apigoals "okrs/internal/http/handlers/api/v1/goals"
	apihealthcheckin "okrs/internal/http/handlers/api/v1/healthcheckin"
	apihierarhy "okrs/internal/http/handlers/api/v1/hierarhy"
	apikrs "okrs/internal/http/handlers/api/v1/krs"
	apiperiods "okrs/internal/http/handlers/api/v1/periods"
	apiteams "okrs/internal/http/handlers/api/v1/teams"
	apitenants "okrs/internal/http/handlers/api/v1/tenants"
	apiusers "okrs/internal/http/handlers/api/v1/users"
	"okrs/internal/http/handlers/web/authhandler"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/http/handlers/web/goals"
	"okrs/internal/http/middleware"
	"okrs/internal/service"
	"okrs/internal/store"
	"okrs/internal/store/grants"

	"github.com/go-chi/chi/v5"
)

//go:embed templates/*.html
var templatesFS embed.FS

func parseTemplates() (*template.Template, error) {
	return template.New("").Funcs(template.FuncMap{
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
}

type Server struct {
	store       *store.Store
	logger      *slog.Logger
	tmpl        *template.Template
	zone        *time.Location
	service     *service.Service
	auth        *auth.Manager
	policy      *auth.PolicyEvaluator
	grantsCache *grants.GrantsCache
	hcCache     *service.HealthCheckInCache
	tenantResolver *auth.TenantResolver
}

func NewServer(st *store.Store, grantsCache *grants.GrantsCache, logger *slog.Logger, zone *time.Location, authMgr *auth.Manager) (*Server, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	hcLoader := func(ctx context.Context, periodID int64) (*service.PeriodData, error) {
		// TODO(tenancy): per-tenant health cache
		period, err := st.Periods.GetPeriod(ctx, domain.TenantScope{TenantID: 1}, periodID)
		if err != nil {
			return nil, err
		}
		allTeams, err := st.Teams.ListAllTeams(ctx, domain.TenantScope{TenantID: 1})
		if err != nil {
			return nil, err
		}
		allTeamIDs := make([]int64, len(allTeams))
		for i, t := range allTeams {
			allTeamIDs[i] = t.ID
		}
		goalsByTeam, err := st.Goals.ListGoalsByTeamsPeriod(ctx, domain.TenantScope{TenantID: 1}, periodID, allTeamIDs) // TODO(tenancy)
		if err != nil {
			return nil, err
		}
		statuses, err := st.Statuses.ListTeamPeriodStatuses(ctx, periodID, allTeamIDs)
		if err != nil {
			return nil, err
		}
		return &service.PeriodData{
			PeriodID:    periodID,
			Period:      period,
			Teams:       allTeams,
			GoalsByTeam: goalsByTeam,
			Statuses:    statuses,
			CachedAt:    time.Now(),
		}, nil
	}

	cacheTTL := 5 * time.Minute
	hcCache := service.NewHealthCheckInCache(hcLoader, cacheTTL, logger)

	return &Server{
		store:       st,
		logger:      logger,
		tmpl:        tmpl,
		zone:        zone,
		service:     service.NewFromStore(st, grantsCache, hcCache),
		auth:        authMgr,
		policy:      auth.NewPolicyEvaluator(grantsCache, logger),
		grantsCache: grantsCache,
		hcCache:     hcCache,
		tenantResolver: auth.NewTenantResolver(st.Tenants, st.Memberships),
	}, nil
}

func (s *Server) Routes() http.Handler {
	ctx := context.Background()
	s.hcCache.StartRefreshLoop(ctx, 5*time.Minute, func(ctx context.Context) int64 {
		// TODO(tenancy): per-tenant health cache
		p, err := s.service.FindPeriodForDate(ctx, domain.TenantScope{TenantID: 1}, time.Now().In(s.zone))
		if err != nil {
			return 0
		}
		return p.ID
	})

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
				http.Redirect(w, r, "/", http.StatusFound)
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

		// "No access" page for authenticated users without an active membership.
		// Lives OUTSIDE the membership-gated group so RequireMembership can redirect here
		// without a loop. Plan 4 replaces this stub with the pluggable NoMembershipHandler.
		r.Get("/no-access", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><html lang="ru"><head><meta charset="utf-8">` +
				`<title>Нет доступа</title></head><body style="font-family:sans-serif;padding:2rem">` +
				`<h1>Нет доступа</h1><p>У вашей учётной записи нет доступа ни к одной организации. ` +
				`Обратитесь к администратору.</p></body></html>`))
		})

		// Protected routes.
		r.Group(func(r chi.Router) {
			if !s.auth.Disabled() {
				r.Use(auth.RequireAuthMiddleware)
				r.Use(auth.TenantResolveMiddleware(s.tenantResolver))
				r.Use(auth.RequireMembershipMiddleware)
				r.Use(auth.ScopeMiddleware(s.policy, s.auth))
			}
			r.Use(csrf.Handler)

			// Tenant session: list memberships and switch active tenant.
			tenantH := apitenants.New(s.store.Memberships, s.store.Tenants, s.store.Sessions)
			r.Get("/api/v1/session/tenants", tenantH.ListMyTenants)
			r.Post("/api/v1/session/tenant", tenantH.SwitchTenant)

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
	r.Get("/", trackerShell)
	r.Get("/teams/{teamID}/okr", trackerShell)

	// Personal settings SPA — team descriptions (for leads) and sidebar node picker.
	// Available to any authenticated user (not admin-only); not part of the admin panel.
	r.Get("/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = s.tmpl.ExecuteTemplate(w, "settings-shell", nil)
	})

	// Страницы-заглушки разделов навигации (гамбургер-меню). Доступны любому
	// авторизованному пользователю, как /settings.
	stubShell := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = s.tmpl.ExecuteTemplate(w, "stub-shell", nil)
	}
	r.Get("/activity-log", stubShell)
	r.Get("/goal-tree", stubShell)

	// Legacy redirect for bookmarks — the tracker now lives at the root.
	r.Get("/teamOkrs", func(w http.ResponseWriter, r *http.Request) {
		target := "/"
		if qs := r.URL.RawQuery; qs != "" {
			target += "?" + qs
		}
		http.Redirect(w, r, target, http.StatusFound)
	})

	// Goal delete is still used by tracker.js via the legacy form endpoint.
	r.Post("/goals/{goalID}/delete", goalsHandler.HandleDeleteGoal)
}

func (s *Server) registerAdminRoutes(r chi.Router, deps common.Dependencies) {
	adminAPI := apiadmin.New(s.store.Users, s.store.Settings, s.auth, s.grantsCache)
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
		r.Get("/api/v1/admin/settings/general", adminAPI.HandleGetGeneralSettings)
		r.Post("/api/v1/admin/settings/general", adminAPI.HandleUpdateGeneralSettings)
		r.Get("/api/v1/admin/settings/feedback", adminAPI.HandleGetFeedbackSettings)
		r.Post("/api/v1/admin/settings/feedback", adminAPI.HandleUpdateFeedbackSettings)

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

		// Admin health check-in settings API.
		hcHandler := apihealthcheckin.New(s.service, s.store.Settings, s.hcCache)
		r.Get("/api/v1/admin/settings/health-checkin", hcHandler.HandleGetHealthCheckInSettings)
		r.Post("/api/v1/admin/settings/health-checkin", hcHandler.HandleUpdateHealthCheckInSettings)

		r.Get("/admin/health-checkin", adminShell)
	})
}

func (s *Server) registerApiRoutes(r chi.Router) {
	r.Get("/api/v1/hierarchy", apihierarhy.New(s.service).HandleHierarchy)
	r.Get("/api/v1/periods", apiperiods.New(s.service).HandlePeriods)
	r.Get("/api/v1/me", apiadmin.HandleMe)
	r.Get("/api/v1/config", apiconfig.New(s.store.Settings).HandleConfig)
	r.Get("/api/v1/users", apiusers.New(s.service).Handle)

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
	r.Delete("/api/v1/goals/{goalID}/share/{teamID}", goalsHandler.HandleLeaveGoalShare)
	r.Delete("/api/v1/goals/{goalID}", goalsHandler.HandleDeleteGoal)

	krsHandler := apikrs.New(s.service)
	r.Post("/api/v1/goals/{goalID}/key-results", krsHandler.HandleCreateKeyResult)
	r.Post("/api/v1/krs/{krID}/progress/numerical", krsHandler.HandleUpdateNumericalProgress)
	r.Post("/api/v1/krs/{krID}/progress/boolean", krsHandler.HandleUpdateBooleanProgress)
	r.Post("/api/v1/krs/{krID}/progress/project", krsHandler.HandleUpdateProjectProgress)
	r.Post("/api/v1/krs/{krID}/note", krsHandler.HandleUpsertKRNote)
	r.Post("/api/v1/krs/{krID}/description", krsHandler.HandleUpdateKRDescription)
	r.Post("/api/v1/krs/{krID}", krsHandler.HandleUpdateKeyResult)
	r.Post("/api/v1/krs/{krID}/move-up", krsHandler.HandleMoveKeyResultUp)
	r.Post("/api/v1/krs/{krID}/move-down", krsHandler.HandleMoveKeyResultDown)
	r.Delete("/api/v1/krs/{krID}", krsHandler.HandleDeleteKeyResult)

	hcHandler := apihealthcheckin.New(s.service, s.store.Settings, s.hcCache)
	r.Get("/api/v1/health-checkin", hcHandler.HandleHealthCheckIn)

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		v1.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	})
}
