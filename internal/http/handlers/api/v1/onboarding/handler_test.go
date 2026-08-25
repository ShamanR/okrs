package onboarding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	onboardingsvc "okrs/internal/service/onboarding"
	settingssvc "okrs/internal/service/settings"
	"strconv"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	adminareq "okrs/internal/http/handlers/api/v1/admin/accessrequests"
	adminapprove "okrs/internal/http/handlers/api/v1/admin/accessrequests/approve"
	admindeny "okrs/internal/http/handlers/api/v1/admin/accessrequests/deny"
	admininvitations "okrs/internal/http/handlers/api/v1/admin/invitations"
	admininvrevoke "okrs/internal/http/handlers/api/v1/admin/invitations/revoke"
	adminmembers "okrs/internal/http/handlers/api/v1/admin/members"
	joinrequest "okrs/internal/http/handlers/api/v1/onboarding/joinrequest"
	"okrs/internal/store/grants"
	"okrs/internal/store/invitations"
	"okrs/internal/store/memberships"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenants"
	"okrs/internal/store/tenantsettings"
	"okrs/internal/store/testutil"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// buildRouter wires onboarding routes, injecting tenant #1 + the given user into context.
func buildRouter(t *testing.T, user *domain.User) (*chi.Mux, *pgxpool.Pool) {
	return buildRouterBase(t, user, "https://okrs.example")
}

// buildRouterBase is buildRouter with a configurable invite baseURL ("" → request-derived).
func buildRouterBase(t *testing.T, user *domain.User, baseURL string) (*chi.Mux, *pgxpool.Pool) {
	t.Helper()
	pool, cleanup := testutil.SetupDB(t)
	t.Cleanup(cleanup)

	invRepo := invitations.NewInvitationRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	tnRepo := tenants.NewTenantRepository(pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := settingssvc.New(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	granter := grants.NewGrantsCache(grants.NewGrantRepository(pool))
	onboardSvc := onboardingsvc.New(invRepo, memRepo, memberships.NewMembershipCache(memRepo), tnRepo, settingsSvc, granter)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := auth.WithTenant(req.Context(), &domain.Tenant{ID: 1, Status: domain.TenantActive})
			if user != nil {
				ctx = auth.WithUser(ctx, user)
			}
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	// Онбординг разложен по пакету на URI; тест монтирует их так же, как server.go,
	// и ходит по настоящим путям — это проверка сценария целиком, а не обработчика.
	admininvitations.RegisterRoutes(r, admininvitations.New(invRepo, baseURL))
	admininvrevoke.RegisterRoutes(r, admininvrevoke.New(invRepo))
	adminareq.RegisterRoutes(r, adminareq.New(onboardSvc))
	adminapprove.RegisterRoutes(r, adminapprove.New(onboardSvc))
	admindeny.RegisterRoutes(r, admindeny.New(onboardSvc))
	adminmembers.RegisterRoutes(r, adminmembers.New(onboardSvc))
	joinrequest.RegisterRoutes(r, joinrequest.New(onboardSvc))
	return r, pool
}

func TestCreateInvitationLink(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, _ := buildRouter(t, admin)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations",
		strings.NewReader(`{"role":"admin","max_uses":5}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: code %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		Token   string `json:"token"`
		URL     string `json:"url"`
		Role    string `json:"role"`
		MaxUses *int   `json:"max_uses"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.Token == "" || !strings.HasSuffix(created.URL, "/invite/"+created.Token) {
		t.Fatalf("bad create body: %+v", created)
	}
	if created.Role != "admin" || created.MaxUses == nil || *created.MaxUses != 5 {
		t.Fatalf("bad create body: %+v", created)
	}

	// List returns the link with counts and no email.
	lw := httptest.NewRecorder()
	r.ServeHTTP(lw, httptest.NewRequest(http.MethodGet, "/api/v1/admin/invitations", nil))
	if lw.Code != http.StatusOK {
		t.Fatalf("list: code %d", lw.Code)
	}
	var list []map[string]any
	_ = json.NewDecoder(lw.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("want 1 link, got %d", len(list))
	}
	if _, hasEmail := list[0]["email"]; hasEmail {
		t.Fatalf("list must not expose email: %+v", list[0])
	}
	if list[0]["use_count"] == nil || list[0]["max_uses"] == nil {
		t.Fatalf("list must include counts: %+v", list[0])
	}
}

func TestCreateInvitationDerivesURLFromRequest(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, _ := buildRouterBase(t, admin, "") // no configured baseURL → derive from request

	// Simulate an ingress that terminates TLS and forwards the public host.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations", strings.NewReader(`{"role":"user"}`))
	req.Host = "okrs.acme.internal"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "okrs.acme.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: code %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	want := "https://okrs.acme.com/invite/" + created.Token
	if created.URL != want {
		t.Fatalf("url = %q, want %q", created.URL, want)
	}
}

func TestCreateInvitationRejectsBadMaxUses(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, _ := buildRouter(t, admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations",
		strings.NewReader(`{"role":"user","max_uses":0}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("max_uses=0 must be 400, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestRevokeInvitation(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, pool := buildRouter(t, admin)
	_ = pool

	cw := httptest.NewRecorder()
	r.ServeHTTP(cw, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations",
		strings.NewReader(`{"role":"user"}`)))
	if cw.Code != http.StatusCreated {
		t.Fatalf("create: %d", cw.Code)
	}
	// One link exists → id is 1 in a fresh DB.
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations/1/revoke", nil))
	if rw.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d (%s)", rw.Code, rw.Body.String())
	}
	// Revoking again is idempotent 204.
	rw2 := httptest.NewRecorder()
	r.ServeHTTP(rw2, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations/1/revoke", nil))
	if rw2.Code != http.StatusNoContent {
		t.Fatalf("re-revoke: %d", rw2.Code)
	}
	// Now not listed.
	lw := httptest.NewRecorder()
	r.ServeHTTP(lw, httptest.NewRequest(http.MethodGet, "/api/v1/admin/invitations", nil))
	var list []map[string]any
	_ = json.NewDecoder(lw.Body).Decode(&list)
	if len(list) != 0 {
		t.Fatalf("revoked link must not be listed, got %d", len(list))
	}
}

func TestAccessRequestQueueApprove(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, pool := buildRouter(t, admin)
	ctx := context.Background()

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:q','github','q','Q') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	memRepo := memberships.NewMembershipRepository(pool)
	if _, err := memRepo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested}); err != nil {
		t.Fatalf("seed request: %v", err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/access-requests", nil))
	var reqs []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&reqs)
	if len(reqs) != 1 {
		t.Fatalf("expected 1 access request, got %d", len(reqs))
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/access-requests/"+strconv.FormatInt(uid, 10)+"/approve", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("approve: code %d (%s)", w.Code, w.Body.String())
	}
	m, _ := memRepo.Get(ctx, uid, 1)
	if m.Status != domain.MembershipActive {
		t.Fatalf("status = %q, want active", m.Status)
	}
}

func TestRemoveMemberEndpoint(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, pool := buildRouter(t, admin)
	ctx := context.Background()

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:rm','github','rm','RM') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	memRepo := memberships.NewMembershipRepository(pool)
	if _, err := memRepo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/members/"+strconv.FormatInt(uid, 10), nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("remove: code %d (%s)", w.Code, w.Body.String())
	}
	if _, err := memRepo.Get(ctx, uid, 1); err != memberships.ErrNotFound {
		t.Fatalf("membership must be removed, got %v", err)
	}
}

func TestJoinRequestEndpoint(t *testing.T) {
	// A fresh user with no membership; the router middleware reads the pointer at request
	// time, so we can fill the id after creating the user row.
	user := &domain.User{DisplayName: "U"}
	r, pool := buildRouter(t, user)
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:join','github','join','U') RETURNING id`).Scan(&user.ID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/onboarding/join-request",
		strings.NewReader(`{"slug":"default"}`)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("join: code %d (%s)", w.Code, w.Body.String())
	}
	m, err := memberships.NewMembershipRepository(pool).Get(context.Background(), user.ID, 1)
	if err != nil || m.Status != domain.MembershipRequested {
		t.Fatalf("expected requested membership, got %+v err=%v", m, err)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/onboarding/join-request",
		strings.NewReader(`{"slug":"nope"}`)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown slug: want 404, got %d", w.Code)
	}
}
