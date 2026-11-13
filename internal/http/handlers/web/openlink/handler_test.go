package openlink

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
)

type stub struct {
	memberships []domain.Membership
}

func (s *stub) ListByUser(context.Context, int64) ([]domain.Membership, error) {
	return s.memberships, nil
}

func req(target string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	ctx := auth.WithUser(r.Context(), &domain.User{ID: 10})
	ctx = auth.WithSession(ctx, &domain.AuthSession{ID: "s1"})
	return r.WithContext(ctx)
}

func do(t *testing.T, deps *stub, target string) *httptest.ResponseRecorder {
	t.Helper()
	rw := httptest.NewRecorder()
	New(deps).Get(rw, req(target))
	if rw.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rw.Code)
	}
	return rw
}

// Переход из уведомления ведёт по внутреннему пути и передаёт пространство
// события параметром ?tenant= — клиентский bootstrap принимает его во вкладку,
// иначе ссылка открыла бы данные пространства, выбранного во вкладке сейчас.
func TestCarriesTenantParamToTarget(t *testing.T) {
	deps := &stub{memberships: []domain.Membership{{UserID: 10, TenantID: 2, Status: domain.MembershipActive}}}

	rw := do(t, deps, "/open?tenant=2&to=%2Fadmin%3Fsection%3Dusers%26filter%3Drequests")

	if got := rw.Header().Get("Location"); got != "/admin?section=users&filter=requests&tenant=2" {
		t.Errorf("Location = %q", got)
	}
}

// Переход никуда не уводит за пределы продукта: внешний адрес отбрасывается.
func TestExternalTargetIsRefused(t *testing.T) {
	for _, to := range []string{
		"https%3A%2F%2Fevil.example%2Fx", // https://evil.example/x
		"%2F%2Fevil.example%2Fx",         // //evil.example/x
		"%2F%5Cevil.example",             // /\evil.example
		"admin",                          // без ведущего слэша
		"",
	} {
		deps := &stub{memberships: []domain.Membership{{UserID: 10, TenantID: 2, Status: domain.MembershipActive}}}
		rw := do(t, deps, "/open?tenant=2&to="+to)
		if got := rw.Header().Get("Location"); got != "/" {
			t.Errorf("to=%q: Location = %q, want /", to, got)
		}
	}
}

// Получатель, потерявший членство в пространстве события, туда не направляется
// и по ссылке не проходит: показывать ему чужую страницу нечего.
func TestNonMemberGetsNoTenantParam(t *testing.T) {
	deps := &stub{memberships: []domain.Membership{{UserID: 10, TenantID: 3, Status: domain.MembershipActive}}}

	rw := do(t, deps, "/open?tenant=2&to=%2Fadmin%3Fsection%3Dusers")

	if got := rw.Header().Get("Location"); got != "/" {
		t.Errorf("Location = %q, want /", got)
	}
}

// Заявка ещё не одобрена: членство есть, но не активное — это не повод
// фокусировать вкладку на пространстве.
func TestRequestedMembershipIsNotEnough(t *testing.T) {
	deps := &stub{memberships: []domain.Membership{{UserID: 10, TenantID: 2, Status: domain.MembershipRequested}}}

	rw := do(t, deps, "/open?tenant=2&to=%2Fadmin")

	if got := rw.Header().Get("Location"); got != "/" {
		t.Errorf("Location = %q, want /", got)
	}
}

// Без параметра пространства ссылка просто ведёт по пути: так открываются
// ссылки, построенные до этого изменения.
func TestWithoutTenantJustRedirects(t *testing.T) {
	deps := &stub{memberships: []domain.Membership{{UserID: 10, TenantID: 2, Status: domain.MembershipActive}}}

	rw := do(t, deps, "/open?to=%2F%3Fteam%3D3%26goal%3D7")

	if got := rw.Header().Get("Location"); got != "/?team=3&goal=7" {
		t.Errorf("Location = %q", got)
	}
}
