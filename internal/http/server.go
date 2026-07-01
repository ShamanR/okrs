package http

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"okrs/internal/auth"
	"okrs/internal/domain"
	"okrs/internal/entitlements"
	v1 "okrs/internal/http/handlers/api/v1"
	apiadmin "okrs/internal/http/handlers/api/v1/admin"
	apiconfig "okrs/internal/http/handlers/api/v1/config"
	apigoals "okrs/internal/http/handlers/api/v1/goals"
	apihealthcheckin "okrs/internal/http/handlers/api/v1/healthcheckin"
	apihierarhy "okrs/internal/http/handlers/api/v1/hierarhy"
	apikrs "okrs/internal/http/handlers/api/v1/krs"
	apionboarding "okrs/internal/http/handlers/api/v1/onboarding"
	apiperiods "okrs/internal/http/handlers/api/v1/periods"
	apisystem "okrs/internal/http/handlers/api/v1/system"
	apiteams "okrs/internal/http/handlers/api/v1/teams"
	apitenants "okrs/internal/http/handlers/api/v1/tenants"
	apiusers "okrs/internal/http/handlers/api/v1/users"
	"okrs/internal/http/handlers/web/authhandler"
	"okrs/internal/http/handlers/web/common"
	"okrs/internal/http/handlers/web/goals"
	"okrs/internal/http/middleware"
	"okrs/internal/onboarding"
	"okrs/internal/service"
	"okrs/internal/store"
	"okrs/internal/store/grants"
	"okrs/internal/store/memberships"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"

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
	tenantCache     *tenants.TenantCache
	membershipCache *memberships.MembershipCache
	settingsSvc     *service.SettingsService
	provisioning    *service.ProvisioningService
	onboarding      *service.OnboardingService
	entitlements     entitlements.Entitlements
	noMembershipName string
	publicRoutes     func(chi.Router)
	authedRoutes     func(chi.Router)
	tenantRoutes     func(chi.Router)
}

// Options parameterizes the server with injectable seams. Each zero value falls back to the
// OSS default, so NewServer(..., Options{}) reproduces today's behaviour.
type Options struct {
	// Resolver, if nil, defaults to a session-only resolver built from the tenant/membership caches.
	Resolver *auth.TenantResolver
	// TenantCache / MembershipCache, if non-nil, are the caches the injected Resolver reads from.
	// The server reuses them for provisioning/onboarding so membership/tenant writes invalidate the
	// same instances the resolver serves. When nil (OSS Options{}), the server builds its own and
	// also builds the default resolver from them, so they stay consistent.
	TenantCache     *tenants.TenantCache
	MembershipCache *memberships.MembershipCache
	// Entitlements, if nil, defaults to entitlements.UnlimitedEntitlements{}.
	Entitlements entitlements.Entitlements
	// NoMembershipName selects the registered onboarding.NoMembershipHandler; "" → "stub".
	NoMembershipName string
	// Route-mount hooks for an embedded control-plane (SaaS). Each nil → no extra routes.
	PublicRoutes func(chi.Router) // outer: session loaded, no auth gate, no CSRF (webhooks)
	AuthedRoutes func(chi.Router) // authed, not membership-gated (create-organization)
	TenantRoutes func(chi.Router) // membership-gated, tenant-scoped (billing UI)
}

func NewServer(st *store.Store, grantsCache *grants.GrantsCache, logger *slog.Logger, zone *time.Location, authMgr *auth.Manager, opts Options) (*Server, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	hcLoader := func(ctx context.Context, scope domain.TenantScope, periodID int64) (*service.PeriodData, error) {
		period, err := st.Periods.GetPeriod(ctx, scope, periodID)
		if err != nil {
			return nil, err
		}
		allTeams, err := st.Teams.ListAllTeams(ctx, scope)
		if err != nil {
			return nil, err
		}
		allTeamIDs := make([]int64, len(allTeams))
		for i, t := range allTeams {
			allTeamIDs[i] = t.ID
		}
		goalsByTeam, err := st.Goals.ListGoalsByTeamsPeriod(ctx, scope, periodID, allTeamIDs)
		if err != nil {
			return nil, err
		}
		statuses, err := st.Statuses.ListTeamPeriodStatuses(ctx, scope, periodID, allTeamIDs)
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

	// Cache tenant + membership lookups on the per-request resolve hot path. These MUST be the
	// same instances the resolver reads, or membership/tenant writes (provisioning, onboarding)
	// invalidate a cache the resolver never consults and the change is invisible until the TTL.
	// app.New injects them via Options alongside the resolver built from them; Options{} (OSS)
	// falls back to building them here and the default resolver below from the same instances.
	tenantCache := opts.TenantCache
	if tenantCache == nil {
		tenantCache = tenants.NewTenantCache(st.Tenants)
	}
	membershipCache := opts.MembershipCache
	if membershipCache == nil {
		membershipCache = memberships.NewMembershipCache(st.Memberships)
	}
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(st.TenantSettings), st.TenantSettings,
		settings.NewSystemSettingsCache(st.Settings), st.Settings,
	)
	provisioning := service.NewProvisioningService(
		st.Tenants, tenantCache,
		st.Memberships, membershipCache,
		settingsSvc, grantsCache,
	)
	onboardingSvc := service.NewOnboardingService(
		st.Invitations, st.Memberships, membershipCache, st.Tenants, settingsSvc, grantsCache,
	)

	resolver := opts.Resolver
	if resolver == nil {
		resolver = auth.NewTenantResolver(auth.NewSessionStrategy(tenantCache, membershipCache))
	}
	ent := opts.Entitlements
	if ent == nil {
		ent = entitlements.UnlimitedEntitlements{}
	}
	noMembership := opts.NoMembershipName
	if noMembership == "" {
		noMembership = "stub"
	}

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
		tenantResolver:  resolver,
		tenantCache:     tenantCache,
		membershipCache: membershipCache,
		settingsSvc:     settingsSvc,
		provisioning:    provisioning,
		onboarding:      onboardingSvc,
		entitlements:     ent,
		noMembershipName: noMembership,
		publicRoutes:     opts.PublicRoutes,
		authedRoutes:     opts.AuthedRoutes,
		tenantRoutes:     opts.TenantRoutes,
	}, nil
}

func (s *Server) Routes() http.Handler {
	// OSS no-membership page (pluggable seam): the box ships the "stub" page; a SaaS build
	// registers its own and selects it via Options.NoMembershipName.
	onboarding.Register("stub", onboarding.StubHandler{Render: func(w http.ResponseWriter, r *http.Request) {
		// Inject the customizable (markdown) no-access message; the page renders it client-side.
		var msg string
		if raw, _ := s.settingsSvc.SystemGet(r.Context(), "no_access_message"); raw != nil {
			_ = json.Unmarshal(raw, &msg)
		}
		_ = s.tmpl.ExecuteTemplate(w, "no-membership", map[string]any{"NoAccessMessage": msg})
	}})

	ctx := context.Background()
	s.hcCache.StartRefreshLoop(ctx, 5*time.Minute, func(ctx context.Context) []service.HCActive {
		now := time.Now().In(s.zone)
		tenants, err := s.store.Tenants.List(ctx)
		if err != nil {
			return nil
		}
		var active []service.HCActive
		for _, tn := range tenants {
			scope := domain.TenantScope{TenantID: tn.ID}
			p, err := s.service.FindPeriodForDate(ctx, scope, now)
			if err != nil {
				continue
			}
			active = append(active, service.HCActive{Scope: scope, PeriodID: p.ID})
		}
		return active
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
		authH := authhandler.New(s.auth, s.tmpl, s.logger, s.onboarding, s.store.Sessions)
		r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
			if s.auth.Disabled() {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			authH.HandleLogin(w, r)
		})
		r.Get("/auth/{provider}/start", authH.HandleProviderStart)
		r.Get("/auth/{provider}/callback", authH.HandleCallback)
		r.Get("/invite/{token}", authH.HandleInvite)
		r.Post("/logout", authH.HandleLogout)

		// Legacy redirects for bookmarks.
		r.Get("/teams", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/teams", http.StatusFound)
		})
		r.Get("/periods", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/periods", http.StatusFound)
		})

		// Public control-plane mounts (SaaS): outer tier — session loaded, no auth gate, no CSRF
		// (e.g. billing webhooks with their own signature verification). nil in OSS.
		if s.publicRoutes != nil {
			s.publicRoutes(r)
		}

		// No-membership page + self-service join-request: authenticated but NOT
		// membership-gated (the caller has no membership yet), so they live OUTSIDE the
		// membership group — RequireMembership redirects here without a loop. csrf.Handler is
		// on this group so serving GET /no-access sets the okr_csrf_token cookie that the
		// join-request POST then validates (double-submit).
		r.Group(func(r chi.Router) {
			if !s.auth.Disabled() {
				r.Use(auth.RequireAuthMiddleware)
			}
			r.Use(csrf.Handler)

			r.Get("/no-access", func(w http.ResponseWriter, r *http.Request) {
				h, ok := onboarding.Get(s.noMembershipName)
				if !ok {
					http.Error(w, "no-membership handler not registered", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				h.ServeNoMembership(w, r)
			})

			// /api/v1/me is global identity ("who am I"), not tenant data — available to any
			// authenticated user, including one without a membership (so the no-access shell
			// can render the shared header the same way every other page does).
			r.Get("/api/v1/me", apiadmin.HandleMe)

			onboardH := apionboarding.New(s.store.Invitations, s.onboarding, s.auth.Config().BaseURL)
			r.Post("/api/v1/onboarding/join-request", onboardH.HandleJoinRequest)

			// Authed control-plane mounts (SaaS): authed but not membership-gated
			// (e.g. self-service "create organization"). nil in OSS.
			if s.authedRoutes != nil {
				s.authedRoutes(r)
			}
		})

		// System-admin plane — tenant-less, so it lives OUTSIDE the membership-gated
		// group (a system admin need not be a member of any tenant). Gated by
		// RequireSystemAdmin (session is_system_admin OR provisioning token).
		s.registerSystemRoutes(r, csrf)

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

			// Tenant-scoped control-plane mounts (SaaS): membership-gated (e.g. billing UI). nil in OSS.
			if s.tenantRoutes != nil {
				s.tenantRoutes(r)
			}
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
	adminAPI := apiadmin.New(s.store.Users, s.settingsSvc, s.auth, s.grantsCache)
	serviceH := apiadmin.NewServiceHandler(s.service)

	r.Group(func(r chi.Router) {
		if !s.auth.Disabled() {
			r.Use(auth.RequireTenantAdminMiddleware)
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
		hcHandler := apihealthcheckin.New(s.service, s.settingsSvc, s.hcCache)
		r.Get("/api/v1/admin/settings/health-checkin", hcHandler.HandleGetHealthCheckInSettings)
		r.Post("/api/v1/admin/settings/health-checkin", hcHandler.HandleUpdateHealthCheckInSettings)

		// Onboarding: tenant-admin invitations + access-request queue.
		onboardH := apionboarding.New(s.store.Invitations, s.onboarding, s.auth.Config().BaseURL)
		r.Post("/api/v1/admin/invitations", onboardH.HandleCreateInvitation)
		r.Post("/api/v1/admin/invitations/{id}/revoke", onboardH.HandleRevokeInvitation)
		r.Get("/api/v1/admin/invitations", onboardH.HandleListInvitations)
		r.Get("/api/v1/admin/access-requests", onboardH.HandleListAccessRequests)
		r.Post("/api/v1/admin/access-requests/{userID}/approve", onboardH.HandleApproveAccessRequest)
		r.Post("/api/v1/admin/access-requests/{userID}/deny", onboardH.HandleDenyAccessRequest)
		r.Delete("/api/v1/admin/members/{userID}", onboardH.HandleRemoveMember)

		r.Get("/admin/health-checkin", adminShell)
	})
}

func (s *Server) registerSystemRoutes(r chi.Router, csrf *middleware.CSRFMiddleware) {
	sysH := apisystem.New(s.provisioning, s.settingsSvc, s.store.Users, s.store.Tenants, s.store.Memberships)

	r.Group(func(r chi.Router) {
		// Authorization is mandatory for the whole system plane in EVERY mode: the
		// RequireSystemAdmin gate always applies (no AUTH_MODE=disabled bypass). RequireAuth is
		// only about loading/redirecting the session and is skipped in disabled mode; the gate
		// still rejects anonymous-local (not a system-admin), so disabled-mode access needs a
		// provisioning token.
		if !s.auth.Disabled() {
			r.Use(auth.RequireAuthMiddleware)
		}
		r.Use(auth.RequireSystemAdminMiddleware(s.auth.Config().ProvisioningToken))
		r.Use(csrf.Handler)

		// System-admin shell (React panel; powered by the /api/v1/system/* endpoints below).
		r.Get("/system", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = s.tmpl.ExecuteTemplate(w, "system-shell", nil)
		})

		r.Post("/api/v1/system/tenants", sysH.HandleCreateTenant)
		r.Get("/api/v1/system/tenants", sysH.HandleListTenants)
		r.Post("/api/v1/system/tenants/{id}/members", sysH.HandleAttachMember)
		r.Get("/api/v1/system/tenants/{id}/members", sysH.HandleListMembers)
		r.Post("/api/v1/system/tenants/{id}/members/{userID}/deny", sysH.HandleDenyMember)
		r.Delete("/api/v1/system/tenants/{id}/members/{userID}", sysH.HandleRemoveMember)
		r.Put("/api/v1/system/tenants/{id}/entitlements", sysH.HandleSetEntitlements)
		r.Get("/api/v1/system/tenants/{id}/entitlements", sysH.HandleGetEntitlements)
		r.Post("/api/v1/system/tenants/{id}/suspend", sysH.HandleSuspend)
		r.Post("/api/v1/system/tenants/{id}/restore", sysH.HandleRestore)
		r.Get("/api/v1/system/users", sysH.HandleListUsers)
		r.Get("/api/v1/system/settings", sysH.HandleGetSettings)
		r.Put("/api/v1/system/settings/default-registration-tenant", sysH.HandleSetDefaultRegistrationTenant)
		r.Put("/api/v1/system/settings/no-access-message", sysH.HandleSetNoAccessMessage)
	})
}

func (s *Server) registerApiRoutes(r chi.Router) {
	r.Get("/api/v1/hierarchy", apihierarhy.New(s.service).HandleHierarchy)
	r.Get("/api/v1/periods", apiperiods.New(s.service).HandlePeriods)
	r.Get("/api/v1/config", apiconfig.New(s.settingsSvc).HandleConfig)
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

	hcHandler := apihealthcheckin.New(s.service, s.settingsSvc, s.hcCache)
	r.Get("/api/v1/health-checkin", hcHandler.HandleHealthCheckIn)

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		v1.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	})
}
