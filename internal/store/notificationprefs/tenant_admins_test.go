package notificationprefs_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

// member создаёт пользователя с членством в тенанте и возвращает его id.
func member(t *testing.T, pool *pgxpool.Pool, key string, tenantID int64, role, status string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ($1,'system',$1,$1) RETURNING id`, key).Scan(&id); err != nil {
		t.Fatalf("создать %s: %v", key, err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,$2,$3,$4)`,
		id, tenantID, role, status); err != nil {
		t.Fatalf("членство %s: %v", key, err)
	}
	return id
}

func enable(t *testing.T, repo *notificationprefs.Repository, tenantID, userID int64, on bool) {
	t.Helper()
	if err := repo.Set(context.Background(), domain.TenantScope{TenantID: tenantID}, userID,
		notificationprefs.Preference{Type: notificationprefs.TypeAccessRequested, Enabled: on}); err != nil {
		t.Fatalf("set: %v", err)
	}
}

// Заявку получают только активные администраторы пространства заявки, которые
// сами включили тип: системный тип выключен по умолчанию.
func TestResolveTenantAdmins(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := notificationprefs.NewRepository(pool)

	var otherTenant int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO tenants (slug, name) VALUES ('other','Другое') RETURNING id`).Scan(&otherTenant); err != nil {
		t.Fatalf("тенант: %v", err)
	}

	optedIn := member(t, pool, "admin-on", 1, "admin", "active")
	neverChose := member(t, pool, "admin-default", 1, "admin", "active")
	optedOut := member(t, pool, "admin-off", 1, "admin", "active")
	plain := member(t, pool, "user-on", 1, "user", "active")
	pendingAdmin := member(t, pool, "admin-requested", 1, "admin", "requested")
	foreignAdmin := member(t, pool, "admin-foreign", otherTenant, "admin", "active")
	enable(t, repo, 1, optedIn, true)
	enable(t, repo, 1, optedOut, false)
	enable(t, repo, 1, plain, true)
	enable(t, repo, 1, pendingAdmin, true)
	enable(t, repo, otherTenant, foreignAdmin, true)

	// Два события в одной пачке: заявителей двое, второй из них — сам optedIn
	// (невозможно в жизни, но проверяет поштучное исключение автора).
	requester := member(t, pool, "requester", 1, "user", "requested")
	rs, err := repo.ResolveTenantAdmins(ctx, domain.TenantScope{TenantID: 1},
		notificationprefs.TypeAccessRequested, []int64{requester, optedIn})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	byOrd := map[int][]int64{}
	for _, r := range rs {
		byOrd[r.Ord] = append(byOrd[r.Ord], r.UserID)
	}
	if len(byOrd[0]) != 1 || byOrd[0][0] != optedIn {
		t.Errorf("событие 0: получатели %v, want только %d", byOrd[0], optedIn)
	}
	if len(byOrd[1]) != 0 {
		t.Errorf("событие 1: автор не получает уведомления о собственном действии, got %v", byOrd[1])
	}
	for _, r := range rs {
		switch r.UserID {
		case neverChose:
			t.Error("администратор, не включавший тип, не должен получать уведомление")
		case optedOut, plain, pendingAdmin, foreignAdmin:
			t.Errorf("пользователь %d не должен получать уведомление о заявке", r.UserID)
		}
	}
}

func TestResolveTenantAdminsEmptyBatch(t *testing.T) {
	repo := notificationprefs.NewRepository(nil)
	rs, err := repo.ResolveTenantAdmins(context.Background(), domain.TenantScope{TenantID: 1},
		notificationprefs.TypeAccessRequested, nil)
	if err != nil || rs != nil {
		t.Fatalf("пустая пачка: %v, %v", rs, err)
	}
}
