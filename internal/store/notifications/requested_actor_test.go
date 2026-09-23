package notifications_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/notifications"
	"okrs/internal/store/testutil"
)

// Заявитель с поданной заявкой ещё не участник, но уведомление о его заявке
// обязано называть его по имени. После отклонения (членство удалено) он снова
// показывается как бывший участник.
func TestRequestedActorIsNamedUntilRequestIsGone(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notifications.NewRepository(pool)
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	var requester int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name, avatar_url)
		VALUES ('google:requester', 'google', 'requester', 'Заявитель', 'https://avatar.example/r.png')
		RETURNING id`).Scan(&requester); err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1, 1, 'user', 'requested')`,
		requester); err != nil {
		t.Fatalf("membership: %v", err)
	}
	if _, err := repo.Insert(ctx, scope, notifications.InsertInput{
		UserID: 1, Type: "access_requested", Kind: "access_requested",
		ActorUserID: requester, EntityTitle: "Default", CoalesceKey: "req",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	one := func() notifications.Notification {
		t.Helper()
		items, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, n := range items {
			if n.ActorUserID == requester {
				return n
			}
		}
		t.Fatalf("уведомление о заявке не найдено: %+v", items)
		return notifications.Notification{}
	}

	if n := one(); n.ActorRemoved || n.ActorDisplayName != "Заявитель" || n.ActorAvatarURL == "" {
		t.Fatalf("пока заявка ждёт решения, заявитель назван по имени: %+v", n)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM memberships WHERE user_id = $1 AND tenant_id = 1`, requester); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if n := one(); !n.ActorRemoved || n.ActorDisplayName != "" {
		t.Fatalf("после отклонения заявитель показывается как бывший участник: %+v", n)
	}
}
