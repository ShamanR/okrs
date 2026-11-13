package invite

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/core/domain"

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

func TestHandleInviteAuthenticatedClaimsImmediately(t *testing.T) {
	ob := &fakeOnboarder{claimTenant: 7}
	h := New(ob)
	r := chi.NewRouter()
	r.Get("/invite/{token}", h.Get)

	ctx := auth.WithSession(auth.WithUser(context.Background(), &domain.User{ID: 3}), &domain.AuthSession{ID: "sess-1"})
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/invite/tok", nil).WithContext(ctx))

	if rw.Code != http.StatusFound || rw.Header().Get("Location") != "/?tenant=7" {
		t.Fatalf("want 302 → /?tenant=7, got %d → %q", rw.Code, rw.Header().Get("Location"))
	}
	if ob.claimCalls != 1 || ob.lastClaimUser != 3 {
		t.Fatalf("claim not invoked for the user: %+v", ob)
	}
	for _, c := range rw.Result().Cookies() {
		if c.Name == "okrs_invite" && c.Value != "" && c.MaxAge >= 0 {
			t.Fatalf("authenticated claim must not stash okrs_invite cookie")
		}
	}
}

func TestHandleInviteSetsCookieAndRedirects(t *testing.T) {
	h := New(nil)
	r := chi.NewRouter()
	r.Get("/invite/{token}", h.Get)

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
