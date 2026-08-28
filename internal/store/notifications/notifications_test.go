package notifications_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/notifications"
	"okrs/internal/store/testutil"
)

func newRepo(t *testing.T) (*notifications.Repository, context.Context, domain.TenantScope, func()) {
	t.Helper()
	pool, cleanup := testutil.SetupDB(t)
	return notifications.NewRepository(pool), context.Background(), domain.TenantScope{TenantID: 1}, cleanup
}

// user id 1 — anonymous-local, id 2 — migration; оба заводятся миграциями,
// поэтому годятся как получатель и актор без дополнительной подготовки.
func input(key string) notifications.InsertInput {
	goalID := int64(10)
	return notifications.InsertInput{
		UserID: 1, Type: "goal_changed", Kind: "goal_fields_changed",
		ActorUserID: 2, GoalID: &goalID, EntityTitle: "Цель",
		Payload: map[string]any{"changed": map[string]any{}}, CoalesceKey: key,
	}
}

func TestInsertThenList(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	created, err := repo.Insert(ctx, scope, input("goal_changed:goal:10:2:100"))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if !created {
		t.Fatal("первая вставка должна создавать строку")
	}
	items, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].CoalesceCount != 1 {
		t.Fatalf("got %+v", items)
	}
}

// Схлопывание: второе событие с тем же ключом не создаёт строку, а увеличивает
// счётчик и снова помечает уведомление непрочитанным (спека §7.2).
func TestInsertCoalescesAndReopens(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	key := "goal_changed:goal:10:2:100"
	if _, err := repo.Insert(ctx, scope, input(key)); err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	if err := repo.MarkRead(ctx, scope, 1, nil, true); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	created, err := repo.Insert(ctx, scope, input(key))
	if err != nil {
		t.Fatalf("insert 2: %v", err)
	}
	if created {
		t.Fatal("повтор в том же бакете не должен создавать вторую строку")
	}
	items, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ожидалась одна строка, got %d", len(items))
	}
	if items[0].CoalesceCount != 2 {
		t.Errorf("coalesce_count: got %d, want 2", items[0].CoalesceCount)
	}
	if items[0].ReadAt != nil {
		t.Error("повтор обязан снова пометить уведомление непрочитанным")
	}
}

// Соседний бакет — отдельное уведомление: окно фиксированное, не скользящее.
func TestDifferentBucketCreatesSecondRow(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	if _, err := repo.Insert(ctx, scope, input("goal_changed:goal:10:2:100")); err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	if _, err := repo.Insert(ctx, scope, input("goal_changed:goal:10:2:101")); err != nil {
		t.Fatalf("insert 2: %v", err)
	}
	items, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ожидались две строки, got %d", len(items))
	}
}

func TestUnreadCountAndMarkRead(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	for _, k := range []string{"a", "b", "c"} {
		if _, err := repo.Insert(ctx, scope, input(k)); err != nil {
			t.Fatalf("insert %s: %v", k, err)
		}
	}
	if n, _ := repo.UnreadCount(ctx, scope, 1); n != 3 {
		t.Fatalf("unread: got %d, want 3", n)
	}

	items, _, _ := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
	if err := repo.MarkRead(ctx, scope, 1, []int64{items[0].ID}, false); err != nil {
		t.Fatalf("mark one: %v", err)
	}
	if n, _ := repo.UnreadCount(ctx, scope, 1); n != 2 {
		t.Fatalf("unread после точечной пометки: got %d, want 2", n)
	}

	if err := repo.MarkRead(ctx, scope, 1, nil, true); err != nil {
		t.Fatalf("mark all: %v", err)
	}
	if n, _ := repo.UnreadCount(ctx, scope, 1); n != 0 {
		t.Fatalf("unread после «прочитать всё»: got %d, want 0", n)
	}
}

// Уведомления одного тенанта не видны в другом: изоляция обязана держаться
// на уровне запроса, а не на аккуратности вызывающего.
func TestTenantIsolation(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	if _, err := repo.Insert(ctx, scope, input("a")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	other := domain.TenantScope{TenantID: 999}
	items, _, err := repo.List(ctx, other, 1, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("чужой тенант видит %d уведомлений", len(items))
	}
}

// Актор резолвится в том же запросе. Миграция 028 делает КАЖДОГО пользователя,
// включая системных, активным участником тенанта 1 — поэтому проверка не может
// полагаться на «естественное» отсутствие членства у системного пользователя: она
// обязана явно снять членство и явно завести не-системного актора без него.
//
//   - user 2 (system:migration) — членство снимается вручную; провайдер 'system'
//     обязан оставаться исключением и показываться по имени даже без членства.
//   - отдельный не-системный пользователь без единой строки в memberships (он заведён
//     уже после бэкфилла 028, поэтому её и не может быть) обязан прийти как
//     плейсхолдер: ActorRemoved=true, имя и аватар пустые.
func TestActorResolvedAndFormerMemberHidden(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notifications.NewRepository(pool)
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	if _, err := pool.Exec(ctx, `DELETE FROM memberships WHERE user_id = 2 AND tenant_id = 1`); err != nil {
		t.Fatalf("strip membership: %v", err)
	}

	var formerActorID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name, avatar_url)
		VALUES ('google:former-actor', 'google', 'former-actor', 'Former Actor', 'https://avatar.example/former.png')
		RETURNING id`).Scan(&formerActorID)
	if err != nil {
		t.Fatalf("insert former actor: %v", err)
	}

	if _, err := repo.Insert(ctx, scope, input("a")); err != nil { // actor 2 (system), no membership
		t.Fatalf("insert (system actor): %v", err)
	}
	formerIn := input("b")
	formerIn.ActorUserID = formerActorID
	if _, err := repo.Insert(ctx, scope, formerIn); err != nil {
		t.Fatalf("insert (former-member actor): %v", err)
	}

	items, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var system, former *notifications.Notification
	for i := range items {
		switch items[i].ActorUserID {
		case 2:
			system = &items[i]
		case formerActorID:
			former = &items[i]
		}
	}
	if system == nil || former == nil {
		t.Fatalf("got %+v", items)
	}

	if system.ActorRemoved {
		t.Error("системный пользователь не должен считаться удалённым участником, даже без членства")
	}
	if system.ActorDisplayName == "" {
		t.Error("имя системного актора обязано резолвиться тем же запросом")
	}

	if !former.ActorRemoved {
		t.Error("не-системный актор без активного членства обязан считаться удалённым участником")
	}
	if former.ActorDisplayName != "" || former.ActorAvatarURL != "" {
		t.Errorf("PII бывшего участника не должно утекать: got name=%q avatar=%q", former.ActorDisplayName, former.ActorAvatarURL)
	}
}

// UnreadOnly обязан отфильтровывать прочитанные строки, а не только красить их иначе.
func TestListUnreadOnly(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	for _, k := range []string{"a", "b"} {
		if _, err := repo.Insert(ctx, scope, input(k)); err != nil {
			t.Fatalf("insert %s: %v", k, err)
		}
	}
	items, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("setup: got %d items, want 2", len(items))
	}
	if err := repo.MarkRead(ctx, scope, 1, []int64{items[0].ID}, false); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	unread, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20, UnreadOnly: true})
	if err != nil {
		t.Fatalf("list unread: %v", err)
	}
	if len(unread) != 1 || unread[0].ID != items[1].ID {
		t.Fatalf("unread-only: got %+v, want just id %d", unread, items[1].ID)
	}
}

// Пагинация: вторая страница продолжает ровно там, где остановилась первая — без
// повтора и без пропуска строки.
func TestListPaginatesWithCursor(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	for _, k := range []string{"a", "b", "c", "d", "e"} {
		if _, err := repo.Insert(ctx, scope, input(k)); err != nil {
			t.Fatalf("insert %s: %v", k, err)
		}
	}

	first, cursor, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 3})
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("page 1: got %d items, want 3", len(first))
	}
	if cursor == nil {
		t.Fatal("page 1: expected a next-page cursor")
	}

	second, cursor2, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 3, Cursor: cursor})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("page 2: got %d items, want 2", len(second))
	}
	if cursor2 != nil {
		t.Fatal("page 2: expected no further cursor")
	}

	seen := map[int64]bool{}
	for _, n := range first {
		seen[n.ID] = true
	}
	for _, n := range second {
		if seen[n.ID] {
			t.Errorf("id %d repeated across pages", n.ID)
		}
		seen[n.ID] = true
	}
	if len(seen) != 5 {
		t.Fatalf("pagination lost or duplicated rows: got %d unique ids, want 5", len(seen))
	}
}

// Ретенция мерит возраст по updated_at, не created_at: строка, которая старая по
// рождению, но недавно тронута схлопыванием, обязана пережить обе ветки PurgeOlderThan.
func TestPurgeOlderThan(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notifications.NewRepository(pool)
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	if _, err := repo.Insert(ctx, scope, input("stale-read")); err != nil {
		t.Fatalf("insert stale-read: %v", err)
	}
	if err := repo.MarkRead(ctx, scope, 1, nil, true); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if _, err := repo.Insert(ctx, scope, input("ancient-unread")); err != nil {
		t.Fatalf("insert ancient-unread: %v", err)
	}
	// Старая по рождению, но «тронута» недавно — имитация свежего схлопывания.
	if _, err := repo.Insert(ctx, scope, input("still-active")); err != nil {
		t.Fatalf("insert still-active: %v", err)
	}

	backdate := func(key string, alsoUpdated bool) {
		t.Helper()
		q := `UPDATE notifications SET created_at = now() - interval '400 days'`
		if alsoUpdated {
			q += `, updated_at = now() - interval '400 days'`
		}
		q += ` WHERE coalesce_key = $1`
		if _, err := pool.Exec(ctx, q, key); err != nil {
			t.Fatalf("backdate %s: %v", key, err)
		}
	}
	backdate("stale-read", true)
	backdate("ancient-unread", true)
	backdate("still-active", false) // updated_at остаётся свежим

	n, err := repo.PurgeOlderThan(ctx, 30, 365)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 2 {
		t.Fatalf("purged: got %d, want 2", n)
	}

	var survivorKey string
	if err := pool.QueryRow(ctx,
		`SELECT coalesce_key FROM notifications WHERE tenant_id = $1 AND user_id = 1`,
		scope.TenantID).Scan(&survivorKey); err != nil {
		t.Fatalf("survivor lookup: %v", err)
	}
	if survivorKey != "still-active" {
		t.Fatalf("survivor: got %q, want %q", survivorKey, "still-active")
	}
}

// InsertBatch обязан оставаться одним round-trip: это горячий путь fan-out.
func TestInsertBatchIsBatched(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	ins := []notifications.InsertInput{input("k1"), input("k2"), input("k3")}
	if err := repo.InsertBatch(ctx, scope, ins); err != nil {
		t.Fatalf("insert batch: %v", err)
	}
	if n, _ := repo.UnreadCount(ctx, scope, 1); n != 3 {
		t.Fatalf("после батча непрочитанных %d, want 3", n)
	}
}
