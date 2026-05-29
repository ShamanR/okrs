package users_test

import (
	"context"
	"testing"

	"okrs/internal/store/testutil"
	"okrs/internal/store/users"
)

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

	u1, _ := r.UpsertUser(ctx, users.UpsertUserInput{ProviderSubjectKey: "p|e1", Provider: "github", Subject: "e1", DisplayName: "Eve"})
	r.UpsertUser(ctx, users.UpsertUserInput{ProviderSubjectKey: "p|f1", Provider: "github", Subject: "f1", DisplayName: "Frank"})

	// Search restricted to Eve's ID only.
	results, err := r.SearchUsersInSet(ctx, []int64{u1.ID}, nil, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersInSet: %v", err)
	}
	if len(results) != 1 || results[0].DisplayName != "Eve" {
		t.Fatalf("expected Eve only, got %+v", results)
	}

	// Empty set returns nil.
	none, err := r.SearchUsersInSet(ctx, nil, nil, "", 10)
	if err != nil {
		t.Fatalf("SearchUsersInSet empty: %v", err)
	}
	if none != nil {
		t.Fatalf("expected nil for empty set, got %+v", none)
	}
}

func TestSetUserAdmin(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := users.NewUserRepository(pool)

	u, _ := r.UpsertUser(ctx, users.UpsertUserInput{ProviderSubjectKey: "p|g1", Provider: "github", Subject: "g1", DisplayName: "Grace"})
	if u.IsAdmin {
		t.Fatal("new user should not be admin")
	}

	if err := r.SetUserAdmin(ctx, u.ID, true); err != nil {
		t.Fatalf("SetUserAdmin true: %v", err)
	}
	got, _ := r.GetUser(ctx, u.ID)
	if !got.IsAdmin {
		t.Fatal("expected user to be admin after SetUserAdmin(true)")
	}

	if err := r.SetUserAdmin(ctx, u.ID, false); err != nil {
		t.Fatalf("SetUserAdmin false: %v", err)
	}
	got, _ = r.GetUser(ctx, u.ID)
	if got.IsAdmin {
		t.Fatal("expected user not to be admin after SetUserAdmin(false)")
	}
}
