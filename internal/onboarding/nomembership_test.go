package onboarding_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/onboarding"
)

func TestNoMembershipRegistryAndStub(t *testing.T) {
	called := false
	onboarding.Register("stub", onboarding.StubHandler{Render: func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}})

	h, ok := onboarding.Get("stub")
	if !ok {
		t.Fatal("registered handler not found")
	}
	rw := httptest.NewRecorder()
	h.ServeNoMembership(rw, httptest.NewRequest(http.MethodGet, "/no-access", nil))
	if !called || rw.Code != http.StatusOK {
		t.Fatalf("stub Render not invoked; called=%v code=%d", called, rw.Code)
	}
}
