package users_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	teamsvc "okrs/internal/service/team"
	usersvc "okrs/internal/service/user"
	useruc "okrs/internal/usecase/user"
	"testing"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	apiusers "okrs/internal/http/handlers/api/v1/users"
	"okrs/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupDB(t *testing.T) (*pgxpool.Pool, *store.Store, func()) {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	testutil.RequireDockerOrSkip(t, err)

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := testutil.RunMigrations(dbURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return pool, store.New(pool), func() {
		pool.Close()
		_ = container.Terminate(ctx)
	}
}

// insertUser creates a user and returns their integer id and udid.
func insertUser(t *testing.T, pool *pgxpool.Pool, name, email string) (int64, string) {
	t.Helper()
	var id int64
	var udid string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (provider_subject_key, provider, subject, display_name, email, created_at, updated_at, last_login_at)
		VALUES ($1, 'google', $2, $3, $4, NOW(), NOW(), NOW())
		RETURNING id, udid`,
		"google:"+name, name, name, email,
	).Scan(&id, &udid)
	if err != nil {
		t.Fatalf("insert user %s: %v", name, err)
	}
	return id, udid
}

func insertTeamWithLead(t *testing.T, pool *pgxpool.Pool, name, lead, leadUDID string) int64 {
	t.Helper()
	var id int64
	var err error
	if leadUDID != "" {
		err = pool.QueryRow(context.Background(),
			`INSERT INTO teams (name, lead, lead_udid) VALUES ($1, $2, $3) RETURNING id`, name, lead, leadUDID,
		).Scan(&id)
	} else {
		err = pool.QueryRow(context.Background(),
			`INSERT INTO teams (name, lead) VALUES ($1, $2) RETURNING id`, name, lead,
		).Scan(&id)
	}
	if err != nil {
		t.Fatalf("insert team %s: %v", name, err)
	}
	return id
}

func grantAccess(t *testing.T, pool *pgxpool.Pool, userID, teamID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO user_hierarchy_grants (user_id, team_id, created_by_user_id) VALUES ($1, $2, $1)`,
		userID, teamID,
	); err != nil {
		t.Fatalf("grant: %v", err)
	}
}

func doGet(t *testing.T, handler http.Handler, url string, scopeIDs []int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	ctx := auth.WithAllowedTeamIDs(req.Context(), scopeIDs)
	ctx = auth.WithTenant(ctx, &domain.Tenant{ID: 1})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func parseUsers(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse body: %v\nbody: %s", err, w.Body.String())
	}
	return out
}

// newSearchUC собирает usecase поиска поверх тестового стора: поиск пересекает гранты
// вызывающего с иерархией команд, поэтому ему нужны и users, и teams, и кэш грантов.
func newSearchUC(st *store.Store) *useruc.UseCase {
	return useruc.New(useruc.Deps{
		Users:  usersvc.New(st.Users),
		Teams:  teamsvc.New(st.Teams),
		Grants: store.NewGrantsCache(st.Grants),
	})
}

func TestUsersEndpoint_NoParams_Returns400(t *testing.T) {
	_, st, cleanup := setupDB(t)
	defer cleanup()

	h := apiusers.New(newSearchUC(st), usersvc.New(st.Users))
	handler := http.HandlerFunc(h.Get)
	w := doGet(t, handler, "/api/v1/users", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUsersEndpoint_QParam_EmptyString_ReturnsRecent(t *testing.T) {
	pool, st, cleanup := setupDB(t)
	defer cleanup()

	insertUser(t, pool, "Alice", "alice@example.com")
	insertUser(t, pool, "Bob", "bob@example.com")

	h := apiusers.New(newSearchUC(st), usersvc.New(st.Users))
	handler := http.HandlerFunc(h.Get)
	w := doGet(t, handler, "/api/v1/users?q=", nil) // nil scope = admin
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	users := parseUsers(t, w)
	if len(users) < 2 {
		t.Fatalf("expected ≥2 users, got %d", len(users))
	}
}

func TestUsersEndpoint_IDsMode_ReturnsByUDID(t *testing.T) {
	pool, st, cleanup := setupDB(t)
	defer cleanup()

	_, udid1 := insertUser(t, pool, "Alice", "alice@example.com")
	_, udid2 := insertUser(t, pool, "Bob", "bob@example.com")
	insertUser(t, pool, "Carol", "carol@example.com")

	h := apiusers.New(newSearchUC(st), usersvc.New(st.Users))
	handler := http.HandlerFunc(h.Get)
	url := "/api/v1/users?ids[]=" + udid1 + "&ids[]=" + udid2
	w := doGet(t, handler, url, []int64{}) // even empty scope — ids[] skips scope
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	users := parseUsers(t, w)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	names := map[string]bool{}
	for _, u := range users {
		names[u["display_name"].(string)] = true
	}
	if !names["Alice"] || !names["Bob"] {
		t.Fatalf("unexpected users: %v", names)
	}
}

func TestUsersEndpoint_ScopedSearch_OnlyGrantedAndLeads(t *testing.T) {
	pool, st, cleanup := setupDB(t)
	defer cleanup()

	// Alice has a grant to teamA (in scope).
	aliceID, _ := insertUser(t, pool, "Alice", "alice@example.com")
	// Bob is a lead of teamA (in scope).
	_, bobUDID := insertUser(t, pool, "Bob", "bob@example.com")
	// Carol has a grant to teamB (outside scope).
	carolID, _ := insertUser(t, pool, "Carol", "carol@example.com")
	// Dave has no grant and is not a lead.
	insertUser(t, pool, "Dave", "dave@example.com")

	teamA := insertTeamWithLead(t, pool, "TeamA", "Bob", bobUDID)
	teamB := insertTeamWithLead(t, pool, "TeamB", "", "")

	grantAccess(t, pool, aliceID, teamA)
	grantAccess(t, pool, carolID, teamB)

	h := apiusers.New(newSearchUC(st), usersvc.New(st.Users))
	handler := http.HandlerFunc(h.Get)

	// Scope = only teamA.
	w := doGet(t, handler, "/api/v1/users?q=", []int64{teamA})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	users := parseUsers(t, w)
	names := map[string]bool{}
	for _, u := range users {
		names[u["display_name"].(string)] = true
	}
	if !names["Alice"] {
		t.Errorf("Alice (grant to teamA) should be visible")
	}
	if !names["Bob"] {
		t.Errorf("Bob (lead of teamA) should be visible")
	}
	if names["Carol"] {
		t.Errorf("Carol (grant to teamB only) should NOT be visible")
	}
	if names["Dave"] {
		t.Errorf("Dave (no grant, no lead) should NOT be visible")
	}
}

func TestUsersEndpoint_ScopedSearch_ParentGrantCoversChild(t *testing.T) {
	pool, st, cleanup := setupDB(t)
	defer cleanup()

	// Alice has a grant to parentTeam.
	// Scope is the childTeam (child of parentTeam).
	// Alice should still be visible because her grant to parent covers child.
	aliceID, _ := insertUser(t, pool, "Alice", "alice@example.com")

	parentID := insertTeamWithLead(t, pool, "Parent", "", "")
	var childID int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO teams (name, parent_id) VALUES ('Child', $1) RETURNING id`, parentID,
	).Scan(&childID); err != nil {
		t.Fatalf("insert child team: %v", err)
	}

	grantAccess(t, pool, aliceID, parentID)

	h := apiusers.New(newSearchUC(st), usersvc.New(st.Users))
	handler := http.HandlerFunc(h.Get)

	// Scope = childTeam only.
	w := doGet(t, handler, "/api/v1/users?q=", []int64{childID})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	users := parseUsers(t, w)
	names := map[string]bool{}
	for _, u := range users {
		names[u["display_name"].(string)] = true
	}
	if !names["Alice"] {
		t.Errorf("Alice (grant to parent) should be visible when scope is child")
	}
}

func TestUsersEndpoint_EmptyScope_ReturnsEmpty(t *testing.T) {
	pool, st, cleanup := setupDB(t)
	defer cleanup()

	insertUser(t, pool, "Alice", "alice@example.com")

	h := apiusers.New(newSearchUC(st), usersvc.New(st.Users))
	handler := http.HandlerFunc(h.Get)

	// Empty scope (no grants at all).
	w := doGet(t, handler, "/api/v1/users?q=", []int64{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	users := parseUsers(t, w)
	if len(users) != 0 {
		t.Errorf("expected 0 users with empty scope, got %d", len(users))
	}
}

func TestUsersEndpoint_LedTeam_IncludedInResponse(t *testing.T) {
	pool, st, cleanup := setupDB(t)
	defer cleanup()

	aliceID, aliceUDID := insertUser(t, pool, "Alice", "alice@example.com")
	teamID := insertTeamWithLead(t, pool, "Platform", "Alice", aliceUDID)
	grantAccess(t, pool, aliceID, teamID)

	h := apiusers.New(newSearchUC(st), usersvc.New(st.Users))
	handler := http.HandlerFunc(h.Get)

	w := doGet(t, handler, "/api/v1/users?q=", nil) // admin
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	users := parseUsers(t, w)
	var alice map[string]any
	for _, u := range users {
		if u["display_name"] == "Alice" {
			alice = u
		}
	}
	if alice == nil {
		t.Fatal("Alice not found in response")
	}
	if alice["led_team"] != "Platform" {
		t.Errorf("expected led_team=Platform, got %v", alice["led_team"])
	}
}
