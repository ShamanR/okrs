package callback

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/core/domain"
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

func TestOnboardAfterLoginClaimsInvite(t *testing.T) {
	ob := &fakeOnboarder{claimTenant: 7}
	h := New(nil, nil, ob)

	req := httptest.NewRequest(http.MethodGet, "/auth/x/callback", nil)
	req.AddCookie(&http.Cookie{Name: "okrs_invite", Value: "tok"})
	rw := httptest.NewRecorder()
	claimed := h.onboardAfterLogin(rw, req, 42)

	if ob.claimCalls != 1 || ob.lastClaimUser != 42 {
		t.Fatalf("claim not invoked for the user: %+v", ob)
	}
	if ob.ensureCalls != 0 {
		t.Fatalf("ensure must be skipped after a successful claim")
	}
	if claimed != 7 {
		t.Fatalf("claimed tenant = %d, want 7 (goes to the client as ?tenant=)", claimed)
	}
}

func TestOnboardAfterLoginRegistersWhenNoInvite(t *testing.T) {
	ob := &fakeOnboarder{ensureCreated: true}
	h := New(nil, nil, ob)

	req := httptest.NewRequest(http.MethodGet, "/auth/x/callback", nil)
	rw := httptest.NewRecorder()
	claimed := h.onboardAfterLogin(rw, req, 9)

	if ob.claimCalls != 0 {
		t.Fatalf("no invite cookie → claim must not run")
	}
	if ob.ensureCalls != 1 {
		t.Fatalf("ensure registration must run")
	}
	if claimed != 0 {
		t.Fatalf("claimed tenant = %d, want 0", claimed)
	}
}
