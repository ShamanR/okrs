package authhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"

	"github.com/go-chi/chi/v5"
)

type fakeOnboarder struct {
	claimTenant   int64
	claimErr      error
	ensureCreated bool
	claimCalls    int
	ensureCalls   int
	lastClaimUser int64
}

func (f *fakeOnboarder) ClaimInvitation(_ context.Context, _ string, userID int64) (*domain.Membership, error) {
	f.claimCalls++
	f.lastClaimUser = userID
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return &domain.Membership{UserID: userID, TenantID: f.claimTenant, Status: domain.MembershipActive}, nil
}

func (f *fakeOnboarder) EnsureRegistration(_ context.Context, _ int64) (bool, error) {
	f.ensureCalls++
	return f.ensureCreated, nil
}

type fakeSessions struct {
	activeTenant int64
	sessionID    string
}

func (f *fakeSessions) SetActiveTenant(_ context.Context, sessionID string, tenantID int64) error {
	f.sessionID = sessionID
	f.activeTenant = tenantID
	return nil
}

func TestOnboardAfterLoginClaimsInvite(t *testing.T) {
	ob := &fakeOnboarder{claimTenant: 7}
	sess := &fakeSessions{}
	h := New(nil, nil, nil, ob, sess)

	req := httptest.NewRequest(http.MethodGet, "/auth/x/callback", nil)
	req.AddCookie(&http.Cookie{Name: "okrs_invite", Value: "tok"})
	rw := httptest.NewRecorder()
	h.onboardAfterLogin(rw, req, 42, "sess-1")

	if ob.claimCalls != 1 || ob.lastClaimUser != 42 {
		t.Fatalf("claim not invoked for the user: %+v", ob)
	}
	if ob.ensureCalls != 0 {
		t.Fatalf("ensure must be skipped after a successful claim")
	}
	if sess.activeTenant != 7 || sess.sessionID != "sess-1" {
		t.Fatalf("active tenant not set to claimed tenant: %+v", sess)
	}
}

func TestOnboardAfterLoginRegistersWhenNoInvite(t *testing.T) {
	ob := &fakeOnboarder{ensureCreated: true}
	h := New(nil, nil, nil, ob, &fakeSessions{})

	req := httptest.NewRequest(http.MethodGet, "/auth/x/callback", nil)
	rw := httptest.NewRecorder()
	h.onboardAfterLogin(rw, req, 9, "sess-2")

	if ob.claimCalls != 0 {
		t.Fatalf("no invite cookie → claim must not run")
	}
	if ob.ensureCalls != 1 {
		t.Fatalf("ensure registration must run")
	}
}

func TestHandleInviteSetsCookieAndRedirects(t *testing.T) {
	h := New(nil, nil, nil, nil, nil)
	r := chi.NewRouter()
	r.Get("/invite/{token}", h.HandleInvite)

	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/invite/tok-abc", nil))

	if rw.Code != http.StatusFound || rw.Header().Get("Location") != "/login?next=/" {
		t.Fatalf("want 302 → /login?next=/, got %d → %q", rw.Code, rw.Header().Get("Location"))
	}
	var found bool
	for _, c := range rw.Result().Cookies() {
		if c.Name == "okrs_invite" && c.Value == "tok-abc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("okrs_invite cookie not set; cookies=%v", rw.Result().Cookies())
	}
}

func TestHandleInviteAuthenticatedClaimsImmediately(t *testing.T) {
	ob := &fakeOnboarder{claimTenant: 7}
	sw := &fakeSessions{}
	h := New(nil, nil, nil, ob, sw)
	r := chi.NewRouter()
	r.Get("/invite/{token}", h.HandleInvite)

	ctx := auth.WithSession(auth.WithUser(context.Background(), &domain.User{ID: 3}), &domain.AuthSession{ID: "sess-1"})
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/invite/tok", nil).WithContext(ctx))

	if rw.Code != http.StatusFound || rw.Header().Get("Location") != "/" {
		t.Fatalf("want 302 → /, got %d → %q", rw.Code, rw.Header().Get("Location"))
	}
	if ob.claimCalls != 1 || ob.lastClaimUser != 3 {
		t.Fatalf("claim not invoked for the user: %+v", ob)
	}
	if sw.activeTenant != 7 || sw.sessionID != "sess-1" {
		t.Fatalf("active tenant not focused on claimed tenant: %+v", sw)
	}
	for _, c := range rw.Result().Cookies() {
		if c.Name == "okrs_invite" && c.Value != "" && c.MaxAge >= 0 {
			t.Fatalf("authenticated claim must not stash okrs_invite cookie")
		}
	}
}
