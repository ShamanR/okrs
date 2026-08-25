package auth

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
)

func TestTenantContextRoundTrip(t *testing.T) {
	tn := &domain.Tenant{ID: 7, Slug: "acme"}
	ctx := WithTenant(context.Background(), tn)
	if got := TenantFromContext(ctx); got == nil || got.ID != 7 {
		t.Fatalf("TenantFromContext = %+v, want id 7", got)
	}
	sc, ok := TenantScopeFromContext(ctx)
	if !ok || sc.TenantID != 7 {
		t.Fatalf("TenantScopeFromContext = %+v ok=%v, want {7} true", sc, ok)
	}

	ctx = WithActiveRole(ctx, domain.RoleAdmin)
	if role, ok := ActiveRoleFromContext(ctx); !ok || role != domain.RoleAdmin {
		t.Fatalf("ActiveRoleFromContext = %v ok=%v, want admin true", role, ok)
	}
}

func TestTenantFromContextNilWhenAbsent(t *testing.T) {
	if got := TenantFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil tenant, got %+v", got)
	}
	if _, ok := TenantScopeFromContext(context.Background()); ok {
		t.Fatalf("expected scope absent")
	}
}

func TestUserFromContextNilWhenNotSet(t *testing.T) {
	if got := UserFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestUserFromContextReturnsUser(t *testing.T) {
	u := &domain.User{ID: 7, DisplayName: "Alice"}
	ctx := WithUser(context.Background(), u)
	got := UserFromContext(ctx)
	if got == nil || got.ID != 7 {
		t.Fatalf("expected user ID 7, got %v", got)
	}
}

func TestUserIDFromContextWithUser(t *testing.T) {
	u := &domain.User{ID: 42}
	ctx := WithUser(context.Background(), u)
	if got := UserIDFromContext(ctx); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestUserIDFromContextWithoutUserReturnsAnonymous(t *testing.T) {
	got := UserIDFromContext(context.Background())
	if got != domain.SystemUserAnonymous {
		t.Fatalf("expected SystemUserAnonymous (%d), got %d", domain.SystemUserAnonymous, got)
	}
}

func TestSessionFromContextNilWhenNotSet(t *testing.T) {
	if got := SessionFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestSessionFromContextReturnsSession(t *testing.T) {
	sess := &domain.AuthSession{ID: "abc123"}
	ctx := withSession(context.Background(), sess)
	got := SessionFromContext(ctx)
	if got == nil || got.ID != "abc123" {
		t.Fatalf("expected session abc123, got %v", got)
	}
}
