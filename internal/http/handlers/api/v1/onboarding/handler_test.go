package onboarding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
	apionboarding "okrs/internal/http/handlers/api/v1/onboarding"
	"okrs/internal/service"
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
	t.Helper()
	pool, cleanup := testutil.SetupDB(t)
	t.Cleanup(cleanup)

	invRepo := invitations.NewInvitationRepository(pool)
	memRepo := memberships.NewMembershipRepository(pool)
	tnRepo := tenants.NewTenantRepository(pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(pool)
	sysRepo := settings.NewSettingsRepository(pool)
	settingsSvc := service.NewSettingsService(
		tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
		settings.NewSystemSettingsCache(sysRepo), sysRepo,
	)
	granter := grants.NewGrantsCache(grants.NewGrantRepository(pool))
	onboardSvc := service.NewOnboardingService(invRepo, memRepo, memberships.NewMembershipCache(memRepo), tnRepo, settingsSvc, granter)
	h := apionboarding.New(invRepo, onboardSvc, "https://okrs.example")

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
	r.Post("/api/v1/admin/invitations", h.HandleCreateInvitation)
	r.Post("/api/v1/admin/invitations/{id}/revoke", h.HandleRevokeInvitation)
	r.Get("/api/v1/admin/invitations", h.HandleListInvitations)
	r.Get("/api/v1/admin/access-requests", h.HandleListAccessRequests)
	r.Post("/api/v1/admin/access-requests/{userID}/approve", h.HandleApproveAccessRequest)
	r.Post("/api/v1/onboarding/join-request", h.HandleJoinRequest)
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
