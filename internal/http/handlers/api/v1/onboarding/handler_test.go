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
	r.Get("/api/v1/admin/invitations", h.HandleListInvitations)
	r.Get("/api/v1/admin/access-requests", h.HandleListAccessRequests)
	r.Post("/api/v1/admin/access-requests/{userID}/approve", h.HandleApproveAccessRequest)
	r.Post("/api/v1/onboarding/join-request", h.HandleJoinRequest)
	return r, pool
}

func TestCreateAndListInvitation(t *testing.T) {
	admin := &domain.User{ID: 1, DisplayName: "Admin"}
	r, _ := buildRouter(t, admin)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/invitations",
		strings.NewReader(`{"email":"x@example.com","role":"admin"}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: code %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.Token == "" || !strings.HasSuffix(created.URL, created.Token) {
		t.Fatalf("bad token/url: %+v", created)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/invitations", nil))
	var list []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 pending invite, got %d", len(list))
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
