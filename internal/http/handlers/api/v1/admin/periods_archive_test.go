package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/service"
	"okrs/internal/store/periods"

	"github.com/go-chi/chi/v5"
)

// fakePeriodRepo is a minimal service.PeriodRepo for handler tests. Only GetPeriod
// is exercised by ArchivePeriod's closed-status check; the other methods exist
// solely to satisfy the interface.
type fakePeriodRepo struct {
	period domain.Period
}

func (f *fakePeriodRepo) ListPeriods(context.Context, domain.TenantScope) ([]domain.Period, error) {
	return []domain.Period{f.period}, nil
}
func (f *fakePeriodRepo) GetPeriod(_ context.Context, _ domain.TenantScope, _ int64) (domain.Period, error) {
	return f.period, nil
}
func (f *fakePeriodRepo) FindPeriodForDate(context.Context, domain.TenantScope, time.Time) (domain.Period, error) {
	return f.period, nil
}
func (f *fakePeriodRepo) CreatePeriod(context.Context, domain.TenantScope, periods.PeriodInput) (int64, error) {
	return 0, nil
}
func (f *fakePeriodRepo) UpdatePeriod(context.Context, domain.TenantScope, int64, periods.PeriodInput) error {
	return nil
}
func (f *fakePeriodRepo) DeletePeriod(context.Context, domain.TenantScope, int64) error { return nil }
func (f *fakePeriodRepo) ArchivePeriod(context.Context, domain.TenantScope, int64) error {
	return nil
}
func (f *fakePeriodRepo) UnarchivePeriod(context.Context, domain.TenantScope, int64) error {
	return nil
}

// newAdminHandlerWithPeriod builds a ServiceHandler wired to a service.Service whose
// only functioning repo is Periods, returning p for any GetPeriod call.
func newAdminHandlerWithPeriod(_ *testing.T, p domain.Period) *ServiceHandler {
	svc := service.New(service.Deps{Periods: &fakePeriodRepo{period: p}})
	return NewServiceHandler(svc, nil, nil)
}

// withTenantScope attaches the given tenant scope so TenantScopeFromContext resolves it.
func withTenantScope(r *http.Request, scope domain.TenantScope) *http.Request {
	return r.WithContext(auth.WithTenant(r.Context(), &domain.Tenant{ID: scope.TenantID, Status: domain.TenantActive}))
}

// withURLParam injects a chi URL param into the request context, mimicking chi's router.
func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// Archiving an active (not-yet-closed) period must be rejected with 409 — only closed
// periods can be archived, per service.ArchivePeriod / ErrPeriodNotClosed.
func TestHandleArchivePeriod_Conflict(t *testing.T) {
	now := time.Now()
	h := newAdminHandlerWithPeriod(t, domain.Period{ID: 1, StartDate: now.AddDate(0, 0, -1), EndDate: now.AddDate(0, 0, 5)})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/periods/1/archive", nil)
	req = withTenantScope(req, domain.TenantScope{TenantID: 1})
	req = withURLParam(req, "periodID", "1")
	w := httptest.NewRecorder()
	h.HandleArchivePeriod(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d (%s)", w.Code, w.Body.String())
	}
}

// A closed period archives successfully.
func TestHandleArchivePeriod_AllowsClosed(t *testing.T) {
	now := time.Now()
	h := newAdminHandlerWithPeriod(t, domain.Period{ID: 1, StartDate: now.AddDate(0, 0, -10), EndDate: now.AddDate(0, 0, -1)})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/periods/1/archive", nil)
	req = withTenantScope(req, domain.TenantScope{TenantID: 1})
	req = withURLParam(req, "periodID", "1")
	w := httptest.NewRecorder()
	h.HandleArchivePeriod(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
}
