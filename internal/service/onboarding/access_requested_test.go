package onboarding_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/service/onboarding"
	"okrs/internal/service/servicetest"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

func seedRequester(t *testing.T, pool *pgxpool.Pool, key string) int64 {
	t.Helper()
	var uid int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ($1,'github',$1,$1) RETURNING id`, key).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return uid
}

func accessRequests(bus *servicetest.FakeBus) []event.AccessRequested {
	var out []event.AccessRequested
	for _, ev := range bus.Events {
		if e, ok := ev.(event.AccessRequested); ok {
			out = append(out, e)
		}
	}
	return out
}

// Новая заявка публикует ровно одно событие о пространстве заявки, от имени
// заявителя и с названием пространства.
func TestRequestAccessPublishesForNewRequest(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	bus := &servicetest.FakeBus{}
	svc := newOnboardingWithBus(t, pool, bus)
	uid := seedRequester(t, pool, "github:new")

	if err := svc.RequestAccess(context.Background(), "default", uid); err != nil {
		t.Fatalf("request: %v", err)
	}

	got := accessRequests(bus)
	if len(got) != 1 {
		t.Fatalf("опубликовано %d событий AccessRequested, want 1", len(got))
	}
	ev := got[0]
	if ev.Scope != (domain.TenantScope{TenantID: 1}) || ev.ActorID != uid {
		t.Errorf("scope/actor = %v/%d, want tenant 1 / %d", ev.Scope, ev.ActorID, uid)
	}
	if ev.TenantTitle == "" {
		t.Error("событие обязано нести название пространства")
	}
	if ev.TeamID != nil || ev.PeriodID != nil {
		t.Error("у заявки нет команды и периода")
	}
	if ev.OccurredAt.IsZero() {
		t.Error("OccurredAt обязан быть проставлен: от него зависит схлопывание")
	}
}

// Повторная отправка ожидающей заявки ничего не публикует; заявка после
// отклонения (членства уже нет) — новая и публикуется.
func TestRequestAccessRepeatAndAfterDeny(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	bus := &servicetest.FakeBus{}
	svc := newOnboardingWithBus(t, pool, bus)
	uid := seedRequester(t, pool, "github:repeat")

	if err := svc.RequestAccess(ctx, "default", uid); err != nil {
		t.Fatalf("первая заявка: %v", err)
	}
	if err := svc.RequestAccess(ctx, "default", uid); err != nil {
		t.Fatalf("повторная заявка: %v", err)
	}
	if n := len(accessRequests(bus)); n != 1 {
		t.Fatalf("после повторной отправки событий %d, want 1", n)
	}

	if err := svc.DenyRequest(ctx, domain.TenantScope{TenantID: 1}, uid); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if err := svc.RequestAccess(ctx, "default", uid); err != nil {
		t.Fatalf("заявка после отклонения: %v", err)
	}
	if n := len(accessRequests(bus)); n != 2 {
		t.Fatalf("заявка после отклонения должна публиковаться: событий %d, want 2", n)
	}
}

// Отказы — неизвестный slug и действующее членство — ничего не публикуют.
func TestRequestAccessErrorsPublishNothing(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	bus := &servicetest.FakeBus{}
	svc := newOnboardingWithBus(t, pool, bus)
	uid := seedRequester(t, pool, "github:member")

	if err := svc.RequestAccess(ctx, "nope", uid); err != onboarding.ErrTenantNotFound {
		t.Fatalf("unknown slug: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,1,'user','active')`, uid); err != nil {
		t.Fatalf("membership: %v", err)
	}
	if err := svc.RequestAccess(ctx, "default", uid); err != onboarding.ErrAlreadyMember {
		t.Fatalf("active member: %v", err)
	}
	if len(bus.Events) != 0 {
		t.Fatalf("отказы не должны публиковать событий, got %v", bus.KindsPublished())
	}
}

// Обработка заявки — одобрение, отклонение, отмена самим заявителем — событий не
// публикует. Заявка, поданная снова после отмены, — новая и публикуется.
func TestProcessingARequestPublishesNothing(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	bus := &servicetest.FakeBus{}
	svc := newOnboardingWithBus(t, pool, bus)

	approved := seedRequester(t, pool, "github:approved")
	denied := seedRequester(t, pool, "github:denied")
	cancelled := seedRequester(t, pool, "github:cancelled")
	for _, uid := range []int64{approved, denied, cancelled} {
		if err := svc.RequestAccess(ctx, "default", uid); err != nil {
			t.Fatalf("request %d: %v", uid, err)
		}
	}
	if n := len(bus.Events); n != 3 {
		t.Fatalf("три заявки — три события, got %d", n)
	}

	if err := svc.ApproveRequest(ctx, scope, approved); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.DenyRequest(ctx, scope, denied); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if err := svc.LeaveTenant(ctx, 1, cancelled); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if n := len(bus.Events); n != 3 {
		t.Fatalf("обработка заявок не публикует событий, got %v", bus.KindsPublished())
	}

	if err := svc.RequestAccess(ctx, "default", cancelled); err != nil {
		t.Fatalf("request after cancel: %v", err)
	}
	if n := len(accessRequests(bus)); n != 4 {
		t.Fatalf("заявка после отмены — новая: событий %d, want 4", n)
	}
}

// Сервис без шины (nil) работает как прежде: заявка создаётся, паники нет.
func TestRequestAccessWithoutPublisher(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	svc := newOnboardingForTest(t, pool)
	uid := seedRequester(t, pool, "github:nobus")
	if err := svc.RequestAccess(context.Background(), "default", uid); err != nil {
		t.Fatalf("request: %v", err)
	}
}
