package http

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	hcsvc "okrs/internal/service/healthcheckin"
	"okrs/internal/service/notificationchannel"
	onboardingsvc "okrs/internal/service/onboarding"
	provisioningsvc "okrs/internal/service/provisioning"
	settingssvc "okrs/internal/service/settings"
	"time"

	"context"
	"okrs/internal/auth"
	v1 "okrs/internal/http/handlers/api/v1"
	apiactivity "okrs/internal/http/handlers/api/v1/activity"
	activitycat "okrs/internal/http/handlers/api/v1/activity/categorycounts"
	activitytree "okrs/internal/http/handlers/api/v1/activity/treecounts"
	adminareq "okrs/internal/http/handlers/api/v1/admin/accessrequests"
	adminapprove "okrs/internal/http/handlers/api/v1/admin/accessrequests/approve"
	admindeny "okrs/internal/http/handlers/api/v1/admin/accessrequests/deny"
	adminpurge "okrs/internal/http/handlers/api/v1/admin/activity/purge"
	admininvitations "okrs/internal/http/handlers/api/v1/admin/invitations"
	admininvrevoke "okrs/internal/http/handlers/api/v1/admin/invitations/revoke"
	adminmembers "okrs/internal/http/handlers/api/v1/admin/members"
	adminperiods "okrs/internal/http/handlers/api/v1/admin/periods"
	adminparchive "okrs/internal/http/handlers/api/v1/admin/periods/archive"
	adminpoverview "okrs/internal/http/handlers/api/v1/admin/periods/overview"
	adminpstats "okrs/internal/http/handlers/api/v1/admin/periods/stats"
	adminpactivate "okrs/internal/http/handlers/api/v1/admin/periods/teams/activate"
	adminpclose "okrs/internal/http/handlers/api/v1/admin/periods/teams/close"
	adminpunarchive "okrs/internal/http/handlers/api/v1/admin/periods/unarchive"
	adminaccess "okrs/internal/http/handlers/api/v1/admin/settings/access"
	adminfeedback "okrs/internal/http/handlers/api/v1/admin/settings/feedback"
	admingeneral "okrs/internal/http/handlers/api/v1/admin/settings/general"
	adminhc "okrs/internal/http/handlers/api/v1/admin/settings/healthcheckin"
	adminnotif "okrs/internal/http/handlers/api/v1/admin/settings/notifications"
	adminnotiftest "okrs/internal/http/handlers/api/v1/admin/settings/notifications/test"
	adminteams "okrs/internal/http/handlers/api/v1/admin/teams"
	adminhard "okrs/internal/http/handlers/api/v1/admin/teams/hard"
	adminrestore "okrs/internal/http/handlers/api/v1/admin/teams/restore"
	adminusers "okrs/internal/http/handlers/api/v1/admin/users"
	adminrole "okrs/internal/http/handlers/api/v1/admin/users/admin"
	admingrants "okrs/internal/http/handlers/api/v1/admin/users/grants"
	apiconfig "okrs/internal/http/handlers/api/v1/config"
	apigoals "okrs/internal/http/handlers/api/v1/goals"
	goalscomments "okrs/internal/http/handlers/api/v1/goals/comments"
	goalsreplies "okrs/internal/http/handlers/api/v1/goals/comments/replies"
	goalsresolve "okrs/internal/http/handlers/api/v1/goals/comments/resolve"
	goalsunresolve "okrs/internal/http/handlers/api/v1/goals/comments/unresolve"
	"okrs/internal/http/handlers/api/v1/goals/goalcommon"
	goalskeyresults "okrs/internal/http/handlers/api/v1/goals/keyresults"
	goalslinkable "okrs/internal/http/handlers/api/v1/goals/linkable"
	goalslinks "okrs/internal/http/handlers/api/v1/goals/links"
	goalsmovedown "okrs/internal/http/handlers/api/v1/goals/movedown"
	goalsmoveup "okrs/internal/http/handlers/api/v1/goals/moveup"
	goalsshare "okrs/internal/http/handlers/api/v1/goals/share"
	goalstransfer "okrs/internal/http/handlers/api/v1/goals/transfer"
	goalsweight "okrs/internal/http/handlers/api/v1/goals/weight"
	apigoaltree "okrs/internal/http/handlers/api/v1/goaltree"
	apihealthcheckin "okrs/internal/http/handlers/api/v1/healthcheckin"
	apihierarchy "okrs/internal/http/handlers/api/v1/hierarchy"
	apikrs "okrs/internal/http/handlers/api/v1/krs"
	krsdescription "okrs/internal/http/handlers/api/v1/krs/description"
	"okrs/internal/http/handlers/api/v1/krs/krscommon"
	krsmovedown "okrs/internal/http/handlers/api/v1/krs/movedown"
	krsmoveup "okrs/internal/http/handlers/api/v1/krs/moveup"
	krsnote "okrs/internal/http/handlers/api/v1/krs/note"
	krsboolean "okrs/internal/http/handlers/api/v1/krs/progress/boolean"
	krsnumerical "okrs/internal/http/handlers/api/v1/krs/progress/numerical"
	krsproject "okrs/internal/http/handlers/api/v1/krs/progress/project"
	apime "okrs/internal/http/handlers/api/v1/me"
	apinotifications "okrs/internal/http/handlers/api/v1/notifications"
	notificationsprefs "okrs/internal/http/handlers/api/v1/notifications/preferences"
	notificationsread "okrs/internal/http/handlers/api/v1/notifications/read"
	notificationsunread "okrs/internal/http/handlers/api/v1/notifications/unreadcount"
	joinrequest "okrs/internal/http/handlers/api/v1/onboarding/joinrequest"
	apiperiods "okrs/internal/http/handlers/api/v1/periods"
	periodsoverview "okrs/internal/http/handlers/api/v1/periods/overview"
	periodsactivate "okrs/internal/http/handlers/api/v1/periods/teams/activate"
	periodsclose "okrs/internal/http/handlers/api/v1/periods/teams/close"
	sessionmemberships "okrs/internal/http/handlers/api/v1/session/memberships"
	sessiontenant "okrs/internal/http/handlers/api/v1/session/tenant"
	sessiontenants "okrs/internal/http/handlers/api/v1/session/tenants"
	sysnotifchan "okrs/internal/http/handlers/api/v1/system/notificationchannels"
	syssettings "okrs/internal/http/handlers/api/v1/system/settings"
	sysdefreg "okrs/internal/http/handlers/api/v1/system/settings/defaultregistrationtenant"
	sysnoaccess "okrs/internal/http/handlers/api/v1/system/settings/noaccessmessage"
	systenants "okrs/internal/http/handlers/api/v1/system/tenants"
	syspurge "okrs/internal/http/handlers/api/v1/system/tenants/activity/purge"
	sysentitlements "okrs/internal/http/handlers/api/v1/system/tenants/entitlements"
	sysmembers "okrs/internal/http/handlers/api/v1/system/tenants/members"
	sysdeny "okrs/internal/http/handlers/api/v1/system/tenants/members/deny"
	sysrole "okrs/internal/http/handlers/api/v1/system/tenants/members/role"
	sysrestore "okrs/internal/http/handlers/api/v1/system/tenants/restore"
	syssuspend "okrs/internal/http/handlers/api/v1/system/tenants/suspend"
	sysusers "okrs/internal/http/handlers/api/v1/system/users"
	sysadmin "okrs/internal/http/handlers/api/v1/system/users/systemadmin"
	apiteams "okrs/internal/http/handlers/api/v1/teams"
	teamsexport "okrs/internal/http/handlers/api/v1/teams/export"
	teamsgoals "okrs/internal/http/handlers/api/v1/teams/goals"
	teamsokrs "okrs/internal/http/handlers/api/v1/teams/okrs"
	teamsoverview "okrs/internal/http/handlers/api/v1/teams/overview"
	teamsstatus "okrs/internal/http/handlers/api/v1/teams/status"
	apiusers "okrs/internal/http/handlers/api/v1/users"
	webauthcallback "okrs/internal/http/handlers/web/auth/callback"
	webauthstart "okrs/internal/http/handlers/web/auth/start"
	"okrs/internal/http/handlers/web/common"
	webgoalsdelete "okrs/internal/http/handlers/web/goals/delete"
	webinvite "okrs/internal/http/handlers/web/invite"
	weblogin "okrs/internal/http/handlers/web/login"
	weblogout "okrs/internal/http/handlers/web/logout"
	webnoaccess "okrs/internal/http/handlers/web/noaccess"
	"okrs/internal/http/handlers/web/shell"
	"okrs/internal/http/httpdeps"
	"okrs/internal/http/middleware"
	"okrs/internal/platform/entitlements"
	"okrs/internal/platform/eventbus"
	"okrs/internal/platform/logging"
	"okrs/internal/platform/nomembership"
	"okrs/internal/platform/secretbox"

	"okrs/internal/scheduler"
	goalsvc "okrs/internal/service/goal"
	periodsvc "okrs/internal/service/period"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
	"okrs/internal/store"
	"okrs/internal/store/grants"
	"okrs/internal/store/memberships"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"
	hcuc "okrs/internal/usecase/healthcheckin"
	"okrs/notifychannel"
	"okrs/web"

	"github.com/go-chi/chi/v5"
)

func parseTemplates() (*template.Template, error) {
	return template.New("").ParseFS(web.TemplatesFS, "templates/*.html")
}

type Server struct {
	store            *store.Store
	deps             httpdeps.Deps
	logger           *slog.Logger
	tmpl             *template.Template
	zone             *time.Location
	auth             *auth.Manager
	policy           *auth.PolicyEvaluator
	grantsCache      *grants.GrantsCache
	hcCache          *hcsvc.Cache
	tenantResolver   *auth.TenantResolver
	tenantCache      *tenants.TenantCache
	membershipCache  *memberships.MembershipCache
	settingsSvc      *settingssvc.Service
	provisioning     *provisioningsvc.Service
	onboarding       *onboardingsvc.Service
	entitlements     entitlements.Entitlements
	notifChannels    *notificationchannel.Service
	noMembershipName string
	assetsDev        bool
	publicRoutes     func(chi.Router)
	authedRoutes     func(chi.Router)
	tenantRoutes     func(chi.Router)
}

// shellData is the view-model every SPA shell template receives. Dev selects the
// development vs production vendored React build (see /static/vendor) and is driven
// by the WEB_ASSETS_DEV env flag; false (production) is the safe default.
type shellData struct {
	Dev bool
}

func (s *Server) shellData() shellData {
	return shellData{Dev: s.assetsDev}
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
	// NoMembershipName selects the registered nomembership.NoMembershipHandler; "" → "stub".
	NoMembershipName string
	// AssetsDev serves the development (unminified) vendored React build instead of the
	// production one; false (the OSS default) ships production React. Driven by WEB_ASSETS_DEV.
	AssetsDev bool
	// NotificationChannels are the delivery channels compiled into this build. Empty
	// in the plain OSS box: in-app needs no channel. A build assembles them next to
	// main (see app.Config), which is what lets a channel live in another module.
	NotificationChannels []notifychannel.Channel
	// NotificationSecretKey is the base64 32-byte key used to encrypt channel
	// secrets at rest. Empty means channels with secrets cannot be configured; the
	// rest of the application, in-app notifications included, works unchanged.
	NotificationSecretKey string
	// Route-mount hooks for an embedded control-plane (SaaS). Each nil → no extra routes.
	PublicRoutes func(chi.Router) // outer: session loaded, no auth gate, no CSRF (webhooks)
	AuthedRoutes func(chi.Router) // authed, not membership-gated (create-organization)
	TenantRoutes func(chi.Router) // membership-gated, tenant-scoped (billing UI)
}

// channelsWithSecret counts the channels whose configuration includes a secret at
// rest. Only those are affected by a missing NOTIFICATIONS_SECRET_KEY; a channel
// without a SecretField configures and runs fine without any key.
func channelsWithSecret(chs []notifychannel.Channel) int {
	n := 0
	for _, ch := range chs {
		if ch.Descriptor.SecretField != "" {
			n++
		}
	}
	return n
}

func NewServer(st *store.Store, grantsCache *grants.GrantsCache, logger *slog.Logger, zone *time.Location, authMgr *auth.Manager, bus *eventbus.Bus, opts Options) (*Server, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	// Загрузчик снимка периода — бизнес-логика над четырьмя сервисами сущностей,
	// поэтому живёт в слое usecase, а не здесь (спека 010, правило 1).
	hcLoader := hcuc.NewPeriodLoader(hcuc.Deps{
		Periods:  periodsvc.New(st.Periods),
		Teams:    teamsvc.New(st.Teams),
		Goals:    goalsvc.New(st.Goals),
		Statuses: teamstatussvc.New(st.Statuses),
	})

	cacheTTL := 5 * time.Minute
	hcCache := hcsvc.NewCache(hcLoader, cacheTTL, logger)

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
	settingsSvc := settingssvc.New(
		tenantsettings.NewTenantSettingsCache(st.TenantSettings), st.TenantSettings,
		settings.NewSystemSettingsCache(st.Settings), st.Settings,
	)
	onboardingSvc := onboardingsvc.New(
		st.Invitations, st.Memberships, membershipCache, st.Tenants, settingsSvc, grantsCache,
	)
	provisioning := provisioningsvc.New(
		st.Tenants, tenantCache,
		st.Memberships, membershipCache,
		settingsSvc, grantsCache, onboardingSvc, st.Users,
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

	// A missing key must not take the whole application down: a box that cannot
	// encrypt channel secrets still serves in-app notifications and everything else.
	// A key that is present but malformed is a different matter — that is an operator
	// error which has to be seen, so it fails startup.
	var secrets *secretbox.Box
	if opts.NotificationSecretKey != "" {
		secrets, err = secretbox.New(opts.NotificationSecretKey)
		if err != nil {
			return nil, fmt.Errorf("http: notification secret key: %w", err)
		}
	} else if n := channelsWithSecret(opts.NotificationChannels); n > 0 {
		// Info, not Warn, and only for channels that actually store a secret. A box
		// with no NOTIFICATIONS_SECRET_KEY is a fully supported deployment — in-app
		// notifications and everything else work, those channels just stay
		// unconfigurable. Warning on every start of a supported configuration is how
		// operators learn to ignore warnings.
		logger.Info("NOTIFICATIONS_SECRET_KEY is unset: channels that store a secret cannot be configured until it is set",
			slog.String(logging.KeyEvent, logging.EventAppStart),
			"channels", n)
	}
	notifChannels, err := notificationchannel.New(st.NotificationChannels, secrets, opts.NotificationChannels, ent, settingsSvc)
	if err != nil {
		return nil, fmt.Errorf("http: notification channels: %w", err)
	}

	return &Server{
		store:            st,
		deps:             httpdeps.Build(st, grantsCache, hcCache, bus, logger),
		logger:           logger,
		tmpl:             tmpl,
		zone:             zone,
		auth:             authMgr,
		policy:           auth.NewPolicyEvaluator(grantsCache, logger),
		grantsCache:      grantsCache,
		hcCache:          hcCache,
		tenantResolver:   resolver,
		tenantCache:      tenantCache,
		membershipCache:  membershipCache,
		settingsSvc:      settingsSvc,
		provisioning:     provisioning,
		onboarding:       onboardingSvc,
		entitlements:     ent,
		notifChannels:    notifChannels,
		noMembershipName: noMembership,
		assetsDev:        opts.AssetsDev,
		publicRoutes:     opts.PublicRoutes,
		authedRoutes:     opts.AuthedRoutes,
		tenantRoutes:     opts.TenantRoutes,
	}, nil
}

func (s *Server) Routes() http.Handler {
	// OSS no-membership page (pluggable seam): the box ships the "stub" page; a SaaS build
	// registers its own and selects it via Options.NoMembershipName.
	nomembership.Register("stub", nomembership.StubHandler{Render: func(w http.ResponseWriter, r *http.Request) {
		// Inject the customizable (markdown) no-access message; the page renders it client-side.
		var msg string
		if raw, _ := s.settingsSvc.SystemGet(r.Context(), "no_access_message"); raw != nil {
			_ = json.Unmarshal(raw, &msg)
		}
		_ = s.tmpl.ExecuteTemplate(w, "no-membership", map[string]any{"NoAccessMessage": msg, "Dev": s.assetsDev})
	}})

	deps := common.Dependencies{Logger: s.logger, Templates: s.tmpl, Zone: s.zone}
	r := chi.NewRouter()

	csrf := middleware.NewCSRF()

	// Наблюдаемость монтируется на корневой роутер, а не внутри группы ниже: chi
	// отдаёт 404 на несовпавший путь своим NotFound-обработчиком, мимо middleware
	// любой внутренней группы. При монтаже внутри группы такой 404 не оставлял
	// ни одной записи — то есть ошибочный ответ без следа в логе.
	//
	// Побочный эффект: запросы за статикой тоже попадают в лог. Это осознанный
	// выбор: полный access-log позволяет увидеть 404 на ассеты после неудачного
	// выката, а отфильтровать их по path:/static/* на стороне сборщика дешевле,
	// чем восстанавливать то, чего в логе нет.
	//
	// Порядок значим: RequestID снаружи, чтобы всё остальное логировалось с ним;
	// Recovery ВНУТРИ AccessLog, чтобы паника превращалась в 500 до того, как
	// access-log снимет статус — иначе упавший запрос не оставит записи о себе.
	r.Use(middleware.RequestID)
	r.Use(middleware.AccessLog(s.logger))
	r.Use(middleware.Recovery(s.logger))

	// Force the browser to revalidate static assets (the SPA bundles and vendored libs)
	// on every load instead of applying heuristic caching. FileServer answers with a cheap
	// 304 when the file is unchanged and fresh content once it changes, so clients never run
	// a stale bundle after a deploy. no-cache is deploy-safe across K8s instances: it needs
	// no server-side state. (Vendored files are not content-hashed, so long-lived immutable
	// caching would risk serving a stale library after an in-place version bump.)
	// Единственный инлайн-роут не-страничного вида: раздача файлов не имеет доменного
	// обработчика, монтировать её отдельным пакетом нечего.
	staticFiles := http.StripPrefix("/static/", http.FileServer(http.Dir("web/static")))
	r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		staticFiles.ServeHTTP(w, req)
	}))

	r.Group(func(r chi.Router) {
		if s.auth.Disabled() {
			r.Use(auth.AnonymousUserMiddleware)
		} else {
			r.Use(auth.SessionMiddleware(s.auth))
		}
		// Личность разрешена — переносим её в контекст логирования.
		r.Use(middleware.LogContext)

		// Auth routes — public, no CSRF (OAuth callbacks use GET).
		// Auth-эндпоинты — по пакету на URI. /login в disabled-режиме уводит на корень:
		// выбирать провайдера не из чего.
		if s.auth.Disabled() {
			r.Get("/login", func(w http.ResponseWriter, req *http.Request) {
				http.Redirect(w, req, "/", http.StatusFound)
			})
		} else {
			weblogin.RegisterRoutes(r, weblogin.New(s.auth, s.tmpl, s.logger))
		}
		webauthstart.RegisterRoutes(r, webauthstart.New(s.auth))
		webauthcallback.RegisterRoutes(r, webauthcallback.New(s.auth, s.logger, s.onboarding, s.store.Sessions))
		webinvite.RegisterRoutes(r, webinvite.New(s.onboarding, s.store.Sessions))
		weblogout.RegisterRoutes(r, weblogout.New(s.auth))

		// Legacy redirects for bookmarks.
		shell.RegisterRedirects(r, shell.PublicRedirects)

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

			webnoaccess.RegisterRoutes(r, webnoaccess.New(s.noMembershipName))

			// /api/v1/me is global identity ("who am I"), not tenant data — available to any
			// authenticated user, including one without a membership (so the no-access shell
			// can render the shared header the same way every other page does).
			apime.RegisterRoutes(r, apime.New())

			joinrequest.RegisterRoutes(r, joinrequest.New(s.onboarding))

			// Tenant switcher: authenticated but NOT membership-gated, so a user whose active
			// tenant is suspended (or where they lost membership) can still list their tenants
			// and switch to one they're active in — otherwise RequireMembership would lock them
			// out before they could recover. These handlers key off the user + explicit target,
			// not the resolved active tenant.
			sessiontenants.RegisterRoutes(r, sessiontenants.New(s.store.Memberships, s.store.Tenants))
			sessiontenant.RegisterRoutes(r, sessiontenant.New(s.store.Memberships, s.store.Tenants, s.store.Sessions))
			sessionmemberships.RegisterRoutes(r, sessionmemberships.New(s.store.Memberships, s.onboarding))

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
			// Организация разрешена только здесь — дополняем контекст tenant_id.
			r.Use(middleware.LogContext)
			r.Use(csrf.Handler)

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
	d := s.deps

	// SPA-shell'ы и legacy-редиректы — декларативные таблицы в handlers/web/shell,
	// а не по пакету на URI: в них нет логики, только «URI → шаблон» и «URI → target».
	shellH := shell.New(s.tmpl, func() shell.Data { return shell.Data{Dev: s.assetsDev} })
	shellH.RegisterShells(r, shell.Public)
	shell.RegisterRedirects(r, shell.MemberRedirects)

	// Goal delete is still used by tracker.js via the legacy form endpoint.
	webgoalsdelete.RegisterRoutes(r, webgoalsdelete.New(deps, d.GoalUC))
}

func (s *Server) registerAdminRoutes(r chi.Router, deps common.Dependencies) {
	d := s.deps

	r.Group(func(r chi.Router) {
		if !s.auth.Disabled() {
			r.Use(auth.RequireTenantAdminMiddleware)
		}

		// Admin-shell'ы и legacy deep-links — те же таблицы (см. handlers/web/shell).
		adminShellH := shell.New(s.tmpl, func() shell.Data { return shell.Data{Dev: s.assetsDev} })
		adminShellH.RegisterShells(r, shell.TenantAdmin)
		shell.RegisterRedirects(r, shell.AdminRedirects)

		// Admin user API.
		// /api/v1/admin/** — пакет на URI-сегмент.
		adminusers.RegisterRoutes(r, adminusers.New(s.grantsCache, s.store.Users))
		adminrole.RegisterRoutes(r, adminrole.New(s.onboarding))
		admingrants.RegisterRoutes(r, admingrants.New(s.grantsCache))
		adminaccess.RegisterRoutes(r, adminaccess.New(s.settingsSvc))
		admingeneral.RegisterRoutes(r, admingeneral.New(s.provisioning, s.settingsSvc))
		adminfeedback.RegisterRoutes(r, adminfeedback.New(s.settingsSvc))
		adminpurge.RegisterRoutes(r, adminpurge.New(d.Activity))
		adminperiods.RegisterRoutes(r, adminperiods.New(d.Periods))
		adminpstats.RegisterRoutes(r, adminpstats.New(d.PeriodUC, s.settingsSvc))
		adminpoverview.RegisterRoutes(r, adminpoverview.New(d.PeriodUC, s.settingsSvc))
		adminparchive.RegisterRoutes(r, adminparchive.New(d.Periods))
		adminpunarchive.RegisterRoutes(r, adminpunarchive.New(d.Periods))
		adminpactivate.RegisterRoutes(r, adminpactivate.New(d.PeriodUC, d.Teams, s.grantsCache))
		adminpclose.RegisterRoutes(r, adminpclose.New(d.PeriodUC, d.Teams, s.grantsCache))
		adminteams.RegisterRoutes(r, adminteams.New(d.Teams, d.Users))
		adminrestore.RegisterRoutes(r, adminrestore.New(d.Teams))
		adminhard.RegisterRoutes(r, adminhard.New(d.Teams))

		// Admin health check-in settings API.
		adminhc.RegisterRoutes(r, adminhc.New(s.settingsSvc, s.hcCache))

		// Admin notification-channel settings + connectivity probe.
		adminnotif.RegisterRoutes(r, adminnotif.New(s.notifChannels))
		adminnotiftest.RegisterRoutes(r, adminnotiftest.New(s.notifChannels))

		// Onboarding: tenant-admin invitations + access-request queue.
		admininvitations.RegisterRoutes(r, admininvitations.New(s.store.Invitations, s.auth.Config().BaseURL))
		admininvrevoke.RegisterRoutes(r, admininvrevoke.New(s.store.Invitations))
		adminareq.RegisterRoutes(r, adminareq.New(s.onboarding))
		adminapprove.RegisterRoutes(r, adminapprove.New(s.onboarding))
		admindeny.RegisterRoutes(r, admindeny.New(s.onboarding))
		adminmembers.RegisterRoutes(r, adminmembers.New(s.onboarding))

	})
}

func (s *Server) registerSystemRoutes(r chi.Router, csrf *middleware.CSRFMiddleware) {

	r.Group(func(r chi.Router) {
		// RequireSystemAdmin is the SOLE gate for the system plane in EVERY mode (spec 040):
		// it admits session system-admins and Bearer PROVISIONING_TOKEN machine callers, and
		// redirects an unauthenticated browser to /login. We deliberately do NOT chain
		// RequireAuth here — it would 401 a token-only machine caller (no session cookie)
		// before the gate could honor the token, breaking cross-tenant provisioning in
		// AUTH_MODE=enabled. In disabled mode the gate still rejects anonymous-local (not a
		// system-admin), so disabled-mode access requires the provisioning token.
		r.Use(auth.RequireSystemAdminMiddleware(s.auth.Config().ProvisioningToken))
		r.Use(csrf.Handler)

		// System-admin shell (React-панель поверх /api/v1/system/*).
		shell.New(s.tmpl, func() shell.Data { return shell.Data{Dev: s.assetsDev} }).RegisterShells(r, shell.System)

		// /api/v1/system/** — пакет на URI-сегмент.
		systenants.RegisterRoutes(r, systenants.New(s.provisioning, s.store.Tenants))
		sysmembers.RegisterRoutes(r, sysmembers.New(s.store.Memberships, s.provisioning))
		sysdeny.RegisterRoutes(r, sysdeny.New(s.provisioning))
		sysrole.RegisterRoutes(r, sysrole.New(s.provisioning))
		sysentitlements.RegisterRoutes(r, sysentitlements.New(s.provisioning, s.settingsSvc))
		sysnotifchan.RegisterRoutes(r, sysnotifchan.New(s.notifChannels))
		syssuspend.RegisterRoutes(r, syssuspend.New(s.provisioning))
		sysrestore.RegisterRoutes(r, sysrestore.New(s.provisioning))
		syspurge.RegisterRoutes(r, syspurge.New(s.store.Activity))
		sysusers.RegisterRoutes(r, sysusers.New(s.store.Users))
		sysadmin.RegisterRoutes(r, sysadmin.New(s.provisioning))
		syssettings.RegisterRoutes(r, syssettings.New(s.settingsSvc))
		sysdefreg.RegisterRoutes(r, sysdefreg.New(s.settingsSvc))
		sysnoaccess.RegisterRoutes(r, sysnoaccess.New(s.settingsSvc))
	})
}

// registerApiRoutes wires the /api/v1 surface. Each handler package owns its own
// route table via RegisterRoutes (the single source of truth, shared with the
// integration test router); the few single-endpoint handlers without a package
// route table are registered inline here.
func (s *Server) registerApiRoutes(r chi.Router) {
	// Каждый пакет получает ровно те сервисы и usecase, которые нужны его эндпоинтам.
	d := s.deps
	apihierarchy.RegisterRoutes(r, apihierarchy.New(d.Teams, d.Board, d.Periods, d.Users))
	// Bell feed: tenant-scoped, per-user. POST /read is state-changing and browser-invoked,
	// so it must live in this CSRF-protected group (spec 010, rule 7) — it does, registerApiRoutes
	// is only called from the membership-gated group where csrf.Handler is already mounted.
	apinotifications.RegisterRoutes(r, apinotifications.New(d.Notifications))
	notificationsunread.RegisterRoutes(r, notificationsunread.New(d.Notifications))
	notificationsread.RegisterRoutes(r, notificationsread.New(d.Notifications))
	// PUT is state-changing and browser-invoked, so it must live in this same
	// CSRF-protected, membership-gated group.
	notificationsprefs.RegisterRoutes(r, notificationsprefs.New(d.NotificationPrefs))
	// Журнал активности (лента + счётчики) — только для tenant-admin, как и очистка
	// журнала (RequireTenantAdmin). При AUTH_MODE=disabled anonymous-local — admin, доступ есть.
	r.Group(func(r chi.Router) {
		if !s.auth.Disabled() {
			r.Use(auth.RequireTenantAdminMiddleware)
		}
		apiactivity.RegisterRoutes(r, apiactivity.New(d.Activity))
		activitytree.RegisterRoutes(r, activitytree.New(d.Activity))
		activitycat.RegisterRoutes(r, activitycat.New(d.Activity))
	})
	apiperiods.RegisterRoutes(r, apiperiods.New(d.Periods))
	apiteams.RegisterRoutes(r, apiteams.New(d.Teams))
	teamsokrs.RegisterRoutes(r, teamsokrs.New(d.Board, d.Periods, d.Users))
	teamsoverview.RegisterRoutes(r, teamsoverview.New(d.Board, d.Periods, d.Users))
	teamsexport.RegisterRoutes(r, teamsexport.New(d.ExportUC))
	teamsstatus.RegisterRoutes(r, teamsstatus.New(d.PeriodUC))
	teamsgoals.RegisterRoutes(r, teamsgoals.New(d.GoalUC, d.Users))
	// /api/v1/goals/** — пакет на URI-сегмент, каждый с собственным узким конструктором.
	apigoals.RegisterRoutes(r, apigoals.New(d.Goals, d.Shares, d.Links, d.Users, d.GoalUC))
	goalslinkable.RegisterRoutes(r, goalslinkable.New(d.Links))
	goalslinks.RegisterRoutes(r, goalslinks.New(d.Goals, d.GoalUC))
	goalsshare.RegisterRoutes(r, goalsshare.New(d.Goals, d.Shares, d.GoalUC))
	goalstransfer.RegisterRoutes(r, goalstransfer.New(d.Goals, d.GoalUC))
	goalsweight.RegisterRoutes(r, goalsweight.New(d.Goals, d.Shares))
	goalscomments.RegisterRoutes(r, goalscomments.New(d.Goals, d.GoalUC, d.Shares))
	goalsreplies.RegisterRoutes(r, goalsreplies.New(d.Goals, d.GoalUC, d.Shares))
	goalsresolve.RegisterRoutes(r, goalsresolve.New(goalcommon.ResolveDeps{Goals: d.Goals, Shares: d.Shares, UC: d.GoalUC}))
	goalsunresolve.RegisterRoutes(r, goalsunresolve.New(goalcommon.ResolveDeps{Goals: d.Goals, Shares: d.Shares, UC: d.GoalUC}))
	goalsmoveup.RegisterRoutes(r, goalsmoveup.New(goalcommon.MoveDeps{Goals: d.Goals, Shares: d.Shares, Mover: d.Goals}))
	goalsmovedown.RegisterRoutes(r, goalsmovedown.New(goalcommon.MoveDeps{Goals: d.Goals, Shares: d.Shares, Mover: d.Goals}))
	goalskeyresults.RegisterRoutes(r, goalskeyresults.New(d.Goals, d.KrUC))
	apigoaltree.RegisterRoutes(r, apigoaltree.New(d.Periods, d.TreeUC))
	apikrs.RegisterRoutes(r, apikrs.New(d.Goals, d.Krs, d.KrUC))
	krsnumerical.RegisterRoutes(r, krsnumerical.New(d.Goals, d.Krs, d.KrUC))
	krsboolean.RegisterRoutes(r, krsboolean.New(d.Goals, d.Krs, d.KrUC))
	krsproject.RegisterRoutes(r, krsproject.New(d.Goals, d.Krs, d.KrUC))
	krsnote.RegisterRoutes(r, krsnote.New(d.Goals, d.Krs, d.KrUC))
	krsdescription.RegisterRoutes(r, krsdescription.New(d.Goals, d.Krs))
	krsmoveup.RegisterRoutes(r, krsmoveup.New(krscommon.MoveDeps{KRs: d.Krs, Goals: d.Goals}))
	krsmovedown.RegisterRoutes(r, krsmovedown.New(krscommon.MoveDeps{KRs: d.Krs, Goals: d.Goals}))

	apiconfig.RegisterRoutes(r, apiconfig.New(s.settingsSvc))
	apiusers.RegisterRoutes(r, apiusers.New(d.UserUC, d.Users))
	apihealthcheckin.RegisterRoutes(r, apihealthcheckin.New(d.HC, s.settingsSvc))

	// Scope-aware period overview + bulk period control available to any authenticated
	// member (my_teams — teams they lead); org scope is admin-gated inside the handler.
	periodsoverview.RegisterRoutes(r, periodsoverview.New(d.PeriodUC, s.settingsSvc, d.Teams, s.grantsCache))
	periodsactivate.RegisterRoutes(r, periodsactivate.New(d.PeriodUC, d.Teams, s.grantsCache))
	periodsclose.RegisterRoutes(r, periodsclose.New(d.PeriodUC, d.Teams, s.grantsCache))

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		v1.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	})
}

// StartBackground launches the periodic passes (health check-in cache refresh,
// progress snapshots). Deliberately separate from Routes(): building the router must
// stay a pure assembly step, so a test can construct it without spawning goroutines.
// Called by app.New once the server is assembled.
func (s *Server) StartBackground(ctx context.Context) {
	scheduler.New(scheduler.Deps{
		DB:            s.store.DB,
		HCCache:       s.hcCache,
		Snapshot:      s.deps.PeriodUC,
		Periods:       s.deps.Periods,
		Active:        s.store.Periods,
		Snaps:         s.deps.Snaps,
		Tenants:       s.store.Tenants,
		Settings:      s.settingsSvc,
		Notifications: s.deps.Notifications,
		Zone:          s.zone,
		Logger:        s.logger,
	}).Start(ctx)
}
