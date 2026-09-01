package notificationchannels_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationchannels"
	"okrs/internal/store/testutil"
)

func newRepo(t *testing.T) (*notificationchannels.Repository, context.Context, domain.TenantScope, func()) {
	t.Helper()
	pool, cleanup := testutil.SetupDB(t)
	return notificationchannels.NewRepository(pool), context.Background(), domain.TenantScope{TenantID: 1}, cleanup
}

func TestUpsertThenGet(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	in := notificationchannels.Config{
		Channel: "mattermost", Enabled: true,
		Values:     map[string]any{"base_url": "https://mm.example.com"},
		SecretEnc:  []byte{0xDE, 0xAD, 0xBE, 0xEF},
		SecretHint: "••••4821",
	}
	if err := repo.Upsert(ctx, scope, in, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, ok, err := repo.Get(ctx, scope, "mattermost")
	if err != nil || !ok {
		t.Fatalf("get: %v ok=%v", err, ok)
	}
	if !got.Enabled || got.Values["base_url"] != "https://mm.example.com" {
		t.Fatalf("конфигурация не сохранилась: %+v", got)
	}
	if string(got.SecretEnc) != string(in.SecretEnc) {
		t.Fatalf("секрет не сохранился побайтово: %v", got.SecretEnc)
	}
	if got.UpdatedByUserID == nil || *got.UpdatedByUserID != 1 {
		t.Fatalf("не записан автор правки: %+v", got.UpdatedByUserID)
	}
}

// Повторный Upsert обязан обновлять, а не дублировать: ключ (tenant, channel).
func TestUpsertIsIdempotentPerChannel(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	c := notificationchannels.Config{Channel: "mattermost", Enabled: false, Values: map[string]any{}}
	if err := repo.Upsert(ctx, scope, c, 1); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	c.Enabled = true
	if err := repo.Upsert(ctx, scope, c, 1); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	list, err := repo.List(ctx, scope)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ожидалась одна строка, got %d", len(list))
	}
	if !list[0].Enabled {
		t.Fatal("второй upsert не обновил enabled")
	}
}

// Конфигурация одного тенанта не видна в другом.
func TestTenantIsolation(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	if err := repo.Upsert(ctx, scope, notificationchannels.Config{
		Channel: "mattermost", Values: map[string]any{},
	}, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	list, err := repo.List(ctx, domain.TenantScope{TenantID: 999})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("чужой тенант видит %d каналов", len(list))
	}
}

// Отсутствующий канал — не ошибка: тенант мог его никогда не настраивать.
func TestGetMissingChannelIsNotAnError(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	_, ok, err := repo.Get(ctx, scope, "nope")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Fatal("несуществующий канал не должен находиться")
	}
}

func TestIdentityRoundTrip(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	id := notificationchannels.Identity{
		UserID: 1, Channel: "mattermost",
		ExternalID: "mm-user-77", ExternalUsername: "ivan",
	}
	if err := repo.UpsertIdentity(ctx, scope, id); err != nil {
		t.Fatalf("upsert identity: %v", err)
	}
	got, ok, err := repo.GetIdentity(ctx, scope, 1, "mattermost")
	if err != nil || !ok {
		t.Fatalf("get identity: %v ok=%v", err, ok)
	}
	if got.ExternalID != "mm-user-77" || got.ExternalUsername != "ivan" {
		t.Fatalf("got %+v", got)
	}
}

// Один внешний аккаунт не может принадлежать двум пользователям одного тенанта.
// Пользователь id=2 (system:migration) заводится миграциями, поэтому доступен.
func TestExternalIDIsUniquePerTenantChannel(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	first := notificationchannels.Identity{UserID: 1, Channel: "mattermost", ExternalID: "shared"}
	if err := repo.UpsertIdentity(ctx, scope, first); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	second := notificationchannels.Identity{UserID: 2, Channel: "mattermost", ExternalID: "shared"}
	if err := repo.UpsertIdentity(ctx, scope, second); err == nil {
		t.Fatal("тот же external_id у второго пользователя должен отвергаться")
	}
}
