package users_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/testutil"
	"okrs/internal/store/users"
)

// Пользователь с поданной заявкой — не участник (Removed: внешней доставки ему
// нет), но его можно назвать в уведомлении о его же заявке (Requested).
func TestContactsByIDsMarksPendingRequest(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := users.NewUserRepository(pool)
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('pending-contact', 'google', 'pending-contact', 'Ольга', 'olga@example.com')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1, 1, 'user', 'requested')`,
		id); err != nil {
		t.Fatalf("membership: %v", err)
	}

	got, err := repo.ContactsByIDs(ctx, scope, []int64{id, 2})
	if err != nil {
		t.Fatalf("contacts: %v", err)
	}
	if c := got[id]; !c.Removed || !c.Requested || c.DisplayName != "Ольга" {
		t.Fatalf("заявитель: %+v, want Removed и Requested с именем", c)
	}
	if c := got[2]; c.Requested {
		t.Fatalf("системный пользователь заявок не подаёт: %+v", c)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM memberships WHERE user_id = $1 AND tenant_id = 1`, id); err != nil {
		t.Fatalf("deny: %v", err)
	}
	got, err = repo.ContactsByIDs(ctx, scope, []int64{id})
	if err != nil {
		t.Fatalf("contacts: %v", err)
	}
	if c := got[id]; !c.Removed || c.Requested {
		t.Fatalf("после отклонения — просто бывший участник: %+v", c)
	}
}
