package memberships

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/testutil"
)

func TestListByUserWithTenant(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMembershipRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:m','github','m','M') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	list, err := repo.ListByUserWithTenant(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].TenantID != 1 || list[0].Slug != "default" || list[0].Status != domain.MembershipRequested {
		t.Fatalf("list = %+v, want one requested membership in tenant default", list)
	}
}

func TestCountActiveAdmins(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMembershipRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	seat := func(sub string, role domain.Role, status domain.MembershipStatus) {
		var uid int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
			VALUES ($1,'github',$2,$2) RETURNING id`, "github:"+sub, sub).Scan(&uid); err != nil {
			t.Fatalf("seed user %s: %v", sub, err)
		}
		if _, err := repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: role, Status: status}); err != nil {
			t.Fatalf("seat %s: %v", sub, err)
		}
	}
	seat("a1", domain.RoleAdmin, domain.MembershipActive)
	seat("a2", domain.RoleAdmin, domain.MembershipActive)
	seat("u1", domain.RoleUser, domain.MembershipActive)
	seat("r1", domain.RoleAdmin, domain.MembershipRequested) // requested admin doesn't count

	n, err := repo.CountActiveAdmins(ctx, scope)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
}

func TestMembershipDeleteAnyStatusScoped(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMembershipRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:d','github','d','D') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed t1: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 2, Role: domain.RoleUser, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("seed t2: %v", err)
	}

	if err := repo.Delete(ctx, domain.TenantScope{TenantID: 1}, uid); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, uid, 1); err != ErrNotFound {
		t.Fatalf("t1 membership should be gone, got %v", err)
	}
	if _, err := repo.Get(ctx, uid, 2); err != nil {
		t.Fatalf("t2 membership must survive: %v", err)
	}
	if err := repo.Delete(ctx, domain.TenantScope{TenantID: 1}, uid); err != nil {
		t.Fatalf("second delete should be a no-op: %v", err)
	}
}

func TestListByTenantReturnsAllStatuses(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMembershipRepository(pool)

	var active, pending int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:a','github','a','Active') RETURNING id`).Scan(&active); err != nil {
		t.Fatalf("seed active: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:p','github','p','Pending') RETURNING id`).Scan(&pending); err != nil {
		t.Fatalf("seed pending: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: active, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive}); err != nil {
		t.Fatalf("upsert active: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: pending, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested}); err != nil {
		t.Fatalf("upsert pending: %v", err)
	}

	got, err := repo.ListByTenant(ctx, domain.TenantScope{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byUser := map[int64]AccessRequest{}
	for _, a := range got {
		byUser[a.UserID] = a
	}
	if byUser[active].Status != domain.MembershipActive || byUser[active].Role != domain.RoleAdmin {
		t.Fatalf("active = %+v", byUser[active])
	}
	if byUser[pending].Status != domain.MembershipRequested {
		t.Fatalf("pending = %+v", byUser[pending])
	}
}

func TestListAccessRequests(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMembershipRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('github:r','github','r','Req','r@example.com') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := repo.Upsert(ctx, domain.Membership{UserID: uid, TenantID: 1, Role: domain.RoleUser, Status: domain.MembershipRequested}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	reqs, err := repo.ListAccessRequests(ctx, domain.TenantScope{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reqs) != 1 || reqs[0].UserID != uid || reqs[0].DisplayName != "Req" {
		t.Fatalf("got %+v", reqs)
	}
}

func TestMembershipUpsertAndList(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('google:m', 'google', 'm', 'M') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := NewMembershipRepository(pool)
	if _, err := repo.Upsert(ctx, domain.Membership{
		UserID: userID, TenantID: 1, Role: domain.RoleAdmin, Status: domain.MembershipActive,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Role != domain.RoleAdmin {
		t.Fatalf("unexpected memberships: %+v", got)
	}

	// requested membership is excluded from the active list.
	if err := repo.SetStatus(ctx, userID, 1, domain.MembershipRequested); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, _ = repo.ListByUser(ctx, userID)
	if len(got) != 0 {
		t.Fatalf("requested membership should be excluded, got %+v", got)
	}
}
