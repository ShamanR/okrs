package memberships

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/testutil"
)

// CreateRequest вставляет заявку только если у пользователя нет членства в
// пространстве, и атомарно сообщает, была ли вставка: на этом держится правило
// «одна заявка — одно уведомление» при одновременных отправках.
func TestCreateRequestInsertsOnlyOnce(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewMembershipRepository(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('github:cr','github','cr','CR') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	created, err := repo.CreateRequest(ctx, uid, 1)
	if err != nil || !created {
		t.Fatalf("первая заявка: created=%v err=%v, want true", created, err)
	}
	m, err := repo.Get(ctx, uid, 1)
	if err != nil || m.Status != domain.MembershipRequested || m.Role != domain.RoleUser {
		t.Fatalf("заявка записана неверно: %+v %v", m, err)
	}

	created, err = repo.CreateRequest(ctx, uid, 1)
	if err != nil || created {
		t.Fatalf("повторная заявка: created=%v err=%v, want false", created, err)
	}

	// Действующее членство заявка не трогает: ни статус, ни роль.
	if err := repo.SetStatus(ctx, uid, 1, domain.MembershipActive); err != nil {
		t.Fatalf("activate: %v", err)
	}
	created, err = repo.CreateRequest(ctx, uid, 1)
	if err != nil || created {
		t.Fatalf("заявка при действующем членстве: created=%v err=%v, want false", created, err)
	}
	if m, _ := repo.Get(ctx, uid, 1); m.Status != domain.MembershipActive {
		t.Fatalf("действующее членство понижено до заявки: %+v", m)
	}
}
