package users_test

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/testutil"
	"okrs/internal/store/users"
)

func TestSystemAdminCountAndSet(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := users.NewUserRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:sa','github','sa','SA') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if n, _ := repo.CountSystemAdmins(ctx); n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
	if err := repo.SetSystemAdmin(ctx, uid, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	if n, _ := repo.CountSystemAdmins(ctx); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
	if err := repo.SetSystemAdmin(ctx, 999999, true); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUserListByTenantScopedWithStatus(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := users.NewUserRepository(pool)

	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	mk := func(key string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
			VALUES ($1,'github',$1,$1) RETURNING id`, key).Scan(&id); err != nil {
			t.Fatalf("user %s: %v", key, err)
		}
		return id
	}
	exec := func(sql string, args ...any) {
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	memberA := mk("a") // active in tenant 1
	reqB := mk("b")    // requested in tenant 1
	otherC := mk("c")  // member of tenant 2 only
	exec(`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,1,'admin','active')`, memberA)
	exec(`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,1,'user','requested')`, reqB)
	exec(`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,2,'user','active')`, otherC)

	got, err := r.ListByTenant(ctx, domain.TenantScope{TenantID: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[int64]users.TenantUser{}
	for _, tu := range got {
		byID[tu.User.ID] = tu
	}
	if _, leaked := byID[otherC]; leaked {
		t.Fatalf("tenant-2-only user must not appear under tenant 1")
	}
	if byID[memberA].Status != domain.MembershipActive || byID[memberA].Role != domain.RoleAdmin {
		t.Fatalf("memberA = %+v", byID[memberA])
	}
	if byID[reqB].Status != domain.MembershipRequested {
		t.Fatalf("reqB status = %q, want requested", byID[reqB].Status)
	}
	if byID[memberA].User.DisplayName != "a" {
		t.Fatalf("user fields not loaded: %+v", byID[memberA].User)
	}
}

func TestUpsertUserIdempotency(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := users.NewUserRepository(pool)

	in := users.UpsertUserInput{
		ProviderSubjectKey: "github|u1",
		Provider:           "github",
		Subject:            "u1",
		DisplayName:        "Alice",
		Email:              "alice@example.com",
	}

	u1, err := r.UpsertUser(ctx, in)
	if err != nil {
		t.Fatalf("UpsertUser first: %v", err)
	}
	if u1.ID == 0 {
		t.Fatal("expected non-zero user ID")
	}
	if u1.DisplayName != "Alice" {
		t.Fatalf("expected Alice, got %s", u1.DisplayName)
	}

	// Second upsert with same key but updated display name.
	in.DisplayName = "Alice Smith"
	u2, err := r.UpsertUser(ctx, in)
	if err != nil {
		t.Fatalf("UpsertUser second: %v", err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("expected same user ID on conflict, got %d != %d", u2.ID, u1.ID)
	}
	if u2.DisplayName != "Alice Smith" {
		t.Fatalf("expected updated name, got %s", u2.DisplayName)
	}
}

func TestSetSystemAdminLoadsOnRead(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := users.NewUserRepository(pool)

	u, err := r.UpsertUser(ctx, users.UpsertUserInput{
		ProviderSubjectKey: "github|sa",
		Provider:           "github",
		Subject:            "sa",
		DisplayName:        "Root",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if u.IsSystemAdmin {
		t.Fatal("new user must not be system admin")
	}
	if err := r.SetSystemAdmin(ctx, u.ID, true); err != nil {
		t.Fatalf("set system admin: %v", err)
	}
	got, err := r.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.IsSystemAdmin {
		t.Fatal("is_system_admin should be loaded as true after SetSystemAdmin")
	}
}

func TestGetUser(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := users.NewUserRepository(pool)

	u, _ := r.UpsertUser(ctx, users.UpsertUserInput{
		ProviderSubjectKey: "github|u2",
		Provider:           "github",
		Subject:            "u2",
		DisplayName:        "Bob",
	})

	got, err := r.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("expected ID %d, got %d", u.ID, got.ID)
	}
	if got.DisplayName != "Bob" {
		t.Fatalf("expected Bob, got %s", got.DisplayName)
	}
}

func TestListAndSearchUsers(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := users.NewUserRepository(pool)

	r.UpsertUser(ctx, users.UpsertUserInput{ProviderSubjectKey: "p|c1", Provider: "github", Subject: "c1", DisplayName: "Carol"})
	r.UpsertUser(ctx, users.UpsertUserInput{ProviderSubjectKey: "p|d1", Provider: "github", Subject: "d1", DisplayName: "Dave"})

	list, err := r.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 users, got %d", len(list))
	}

	// Unrestricted search by name fragment.
	results, err := r.SearchUsersUnrestricted(ctx, "car", 10)
	if err != nil {
		t.Fatalf("SearchUsersUnrestricted: %v", err)
	}
	if len(results) != 1 || results[0].DisplayName != "Carol" {
		t.Fatalf("expected Carol, got %+v", results)
	}

	// Unrestricted empty query returns all non-system users.
	all, err := r.SearchUsersUnrestricted(ctx, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersUnrestricted empty: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected at least 2 results for empty query, got %d", len(all))
	}
}

func TestSearchUsersInSet(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := users.NewUserRepository(pool)

	u1, err := r.UpsertUser(ctx, users.UpsertUserInput{
		ProviderSubjectKey: "test:set1", Provider: "test", Subject: "set1",
		DisplayName: "Set User One",
	})
	if err != nil {
		t.Fatalf("upsert u1: %v", err)
	}
	u2, err := r.UpsertUser(ctx, users.UpsertUserInput{
		ProviderSubjectKey: "test:set2", Provider: "test", Subject: "set2",
		DisplayName: "Set User Two",
	})
	if err != nil {
		t.Fatalf("upsert u2: %v", err)
	}

	// find by integer id
	results, err := r.SearchUsersInSet(ctx, []int64{u1.ID}, nil, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersInSet by id: %v", err)
	}
	if len(results) != 1 || results[0].ID != u1.ID {
		t.Errorf("expected u1, got %v", results)
	}

	// find by lead UDID
	results2, err := r.SearchUsersInSet(ctx, nil, []string{u2.UDID}, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersInSet by UDID: %v", err)
	}
	if len(results2) != 1 || results2[0].ID != u2.ID {
		t.Errorf("expected u2 by UDID, got %v", results2)
	}

	// empty inputs → nil result
	none, err := r.SearchUsersInSet(ctx, nil, nil, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersInSet empty: %v", err)
	}
	if none != nil {
		t.Errorf("expected nil for empty inputs")
	}
}
