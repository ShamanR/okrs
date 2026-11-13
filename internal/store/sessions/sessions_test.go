package sessions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"okrs/internal/store/sessions"
	"okrs/internal/store/testutil"
	"okrs/internal/store/users"

	"github.com/jackc/pgx/v5"
)

func setupSessionUser(t *testing.T, ctx context.Context, ur *users.UserRepository, key, name string) int64 {
	t.Helper()
	u, err := ur.UpsertUser(ctx, users.UpsertUserInput{
		ProviderSubjectKey: key,
		Provider:           "github",
		Subject:            key,
		DisplayName:        name,
	})
	if err != nil {
		t.Fatalf("upsert user %s: %v", name, err)
	}
	return u.ID
}

func TestSessionCreateGetDelete(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	ur := users.NewUserRepository(pool)
	userID := setupSessionUser(t, ctx, ur, "sess|u1", "SessUser1")
	r := sessions.NewSessionRepository(pool)

	sess, err := r.CreateSession(ctx, "tok-abc", userID, "github", time.Hour, "Mozilla/5.0", "127.0.0.1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID != "tok-abc" || sess.UserID != userID {
		t.Fatalf("unexpected session %+v", sess)
	}

	got, err := r.GetSession(ctx, "tok-abc")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != "tok-abc" {
		t.Fatalf("expected session tok-abc, got %s", got.ID)
	}

	if err := r.DeleteSession(ctx, "tok-abc"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	_, err = r.GetSession(ctx, "tok-abc")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestSessionExpired(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	ur := users.NewUserRepository(pool)
	userID := setupSessionUser(t, ctx, ur, "sess|u2", "SessUser2")
	r := sessions.NewSessionRepository(pool)

	// Negative TTL creates an already-expired session.
	_, err := r.CreateSession(ctx, "tok-expired", userID, "github", -time.Second, "", "")
	if err != nil {
		t.Fatalf("CreateSession expired: %v", err)
	}

	_, err = r.GetSession(ctx, "tok-expired")
	if err == nil {
		t.Fatal("expected error for expired session")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestSessionTouchUpdatesLastSeen(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	ur := users.NewUserRepository(pool)
	userID := setupSessionUser(t, ctx, ur, "sess|u3", "SessUser3")
	r := sessions.NewSessionRepository(pool)

	sess, _ := r.CreateSession(ctx, "tok-touch", userID, "github", time.Hour, "", "")
	initialLastSeen := sess.LastSeenAt

	time.Sleep(10 * time.Millisecond)

	if err := r.TouchSession(ctx, "tok-touch"); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}

	got, _ := r.GetSession(ctx, "tok-touch")
	if !got.LastSeenAt.After(initialLastSeen) {
		t.Fatalf("expected last_seen_at updated, initial=%s after=%s", initialLastSeen, got.LastSeenAt)
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()

	ur := users.NewUserRepository(pool)
	userID := setupSessionUser(t, ctx, ur, "sess|u4", "SessUser4")
	r := sessions.NewSessionRepository(pool)

	r.CreateSession(ctx, "tok-fresh", userID, "github", time.Hour, "", "")
	r.CreateSession(ctx, "tok-old", userID, "github", -time.Second, "", "")

	if err := r.DeleteExpiredSessions(ctx); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}

	// Fresh session still exists.
	if _, err := r.GetSession(ctx, "tok-fresh"); err != nil {
		t.Fatalf("fresh session missing after cleanup: %v", err)
	}
	// Expired session is gone.
	if _, err := r.GetSession(ctx, "tok-old"); err == nil {
		t.Fatal("expired session should have been deleted")
	}
}
