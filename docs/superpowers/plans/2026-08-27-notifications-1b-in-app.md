# Уведомления, фаза 1b: in-app колокольчик — план реализации

> **Для агентов:** ОБЯЗАТЕЛЬНЫЙ SUB-SKILL: используй superpowers:subagent-driven-development (рекомендуется) или superpowers:executing-plans, чтобы выполнять план задача за задачей. Шаги размечены чекбоксами (`- [ ]`).

**Цель:** работающий колокольчик уведомлений на всех страницах и экран настроек, где пользователь выбирает типы уведомлений и скоуп.

**Архитектура:** подписчик уведомлений слушает 13 из 22 типов событий на шине из фазы 1a. На каждое событие он резолвит получателей — лидов команды и её предков, отобранных по дистанции в дереве `teams.parent_id`, — одним рекурсивным запросом на батч, и пишет строки `notifications` со схлопыванием повторов через `ON CONFLICT`. Текст уведомления собирает сервер, фронт добавляет аватар, ссылку и время.

**Стек:** Go 1.25, PostgreSQL + pgx/v5 (рекурсивный CTE, `ON CONFLICT DO UPDATE`), chi, React 18 без сборщика (JSX через `@babel/standalone` в браузере).

**Спека:** [`docs/superpowers/specs/2026-08-26-notifications-design.md`](../specs/2026-08-26-notifications-design.md) — план опирается на неё, исполнителю нужно прочитать обе.

**Предусловие:** фаза [1a](2026-08-27-notifications-1a-event-bus.md) принята. Без шины и типов событий этот план не выполняется — он начинается с подписки на уже существующие `event.*`.

**Долг, унаследованный от 1a:** [список технического долга](2026-08-27-notifications-tech-debt.md). Два пункта оттуда обязаны быть закрыты **в этой фазе** и вплетены в задачи: graceful shutdown в `main` (Task 10, шаг 1а) и решение по CRLF перед обновлением golden-набора маршрутов (Task 6, шаг 7). Остальное — мелкий долг, который стоит подчищать попутно, когда задача и так трогает нужный файл.

**Чего в 1b нет:** внешних каналов (Telegram, Mattermost), доставок, шифрования секретов, админки каналов и entitlements-гейта — это фаза 2. Поэтому в матрице настроек не будет колонок каналов: канал ровно один, in-app.

## Глобальные ограничения

- **Коммиты не делает исполнитель.** По правилу 8 `CLAUDE.md` задачи заканчиваются прогоном тестов, а не `git commit`. Владелец репозитория коммитит сам.
- **Схема БД меняется только миграцией** (правило 2 в `specs/010-architecture-constraints.md`).
- **Никакой бизнес-логики в handlers** (правило 1 там же). Handler разбирает запрос, зовёт usecase или сервис и собирает DTO.
- **Слои:** `handler → usecase → service → store`. Usecase не обращается к репозиториям; сервис работает с одним репозиторием; порт объявляется на стороне потребителя.
- **Именование:** store — множественное число, service — единственное. Алиас импорта `<entity>svc`.
- **Пакет на URI:** путь пакета обработчика повторяет путь URI без сегментов-параметров и дефисов.
- **Никаких N+1** (правило 9 `CLAUDE.md`). Батчевые методы помечаются комментарием `// Батчевая операция: не превращать в цикл — это N+1.`
- **CSRF** обязателен для всех POST/PUT/PATCH/DELETE, вызываемых из браузера (правило 7 в `specs/010`).
- **Экранирование:** пользовательские данные рендерятся через React как текст, без `dangerouslySetInnerHTML` (правило 8 в `specs/010`).
- **Тесты с БД** поднимают контейнер через `testutil.SetupDB(t)`; без Docker тест скипается.
- **Golden-тест маршрутов** обновляется `go test ./internal/http -run RoutesGolden -update-routes` в той же задаче, где менялись маршруты.
- **Фронтенд без сборщика:** новый модуль — файл в `web/static/` + `<script type="text/babel">` в shell-шаблонах.
- **Язык:** комментарии в Go — по-английски, пользовательские строки в UI — по-русски.

---

## Карта файлов

**Создаются:**

| Файл | Ответственность |
|---|---|
| `migrations/044_notifications.up.sql` / `.down.sql` | `notifications`, `notification_preferences` |
| `internal/store/notifications/notifications.go` | вставка со схлопыванием, лента, счётчик, пометка, ретенция |
| `internal/store/notificationprefs/notificationprefs.go` | настройки и `ResolveRecipients` |
| `internal/service/notification/notification.go` | сервис уведомлений |
| `internal/service/notificationpref/notificationpref.go` | сервис настроек |
| `internal/render/notify/notify.go` | `kind + payload → {title, body}` |
| `internal/usecase/notification/notification.go` | fan-out: событие → получатели → строки |
| `internal/http/dto/notification.go` | DTO ответов |
| `internal/http/handlers/api/v1/notifications/` | лента |
| `internal/http/handlers/api/v1/notifications/unreadcount/` | счётчик бейджа |
| `internal/http/handlers/api/v1/notifications/read/` | пометка прочитанным |
| `internal/http/handlers/api/v1/notifications/preferences/` | настройки пользователя |
| `web/static/notifications.js` | `NotificationList`, панель колокольчика |

**Изменяются:**

| Файл | Что |
|---|---|
| `internal/store/store.go` | два новых репозитория в composite |
| `internal/http/httpdeps/httpdeps.go` | сервисы, usecase, подписка на шину |
| `internal/http/server.go` | регистрация четырёх маршрутов |
| `internal/scheduler/scheduler.go` | суточная петля ретенции |
| `web/static/sidebar.js` | колокольчик уведомлений вместо слота `bell` |
| `web/static/tracker.js` | снятие HCI-колокольчика, его запроса и мёртвого `HealthCheckInButton` |
| `web/static/settings.js` | секция «Уведомления» |
| `web/templates/*_shell.html` | подключение `notifications.js` |
| `seed_demo.sql` | демо-настройки и пара уведомлений |
| `specs/010`, `020`, `030`, `040`, `050`, `070` | правки по §15 спеки |

---

## Task 1: Миграция и репозиторий уведомлений

**Файлы:**
- Создать: `migrations/044_notifications.up.sql`, `migrations/044_notifications.down.sql`
- Создать: `internal/store/notifications/notifications.go`
- Тест: `internal/store/notifications/notifications_test.go`
- Изменить: `internal/store/store.go`

**Интерфейсы:**
- Производит:
  - `notifications.Notification` — доменная строка уведомления
  - `notifications.InsertInput` — вход для вставки со схлопыванием
  - `(*Repository).Insert(ctx, scope, in InsertInput) (created bool, err error)`
  - `(*Repository).InsertBatch(ctx, scope, ins []InsertInput) error`
  - `(*Repository).List(ctx, scope, userID int64, f ListFilter) ([]Notification, *Cursor, error)`
  - `(*Repository).UnreadCount(ctx, scope, userID int64) (int, error)`
  - `(*Repository).MarkRead(ctx, scope, userID int64, ids []int64, all bool) error`
  - `(*Repository).PurgeOlderThan(ctx, readerDays, anyDays int) (int64, error)`
  - `store.Store.Notifications *notifications.Repository`
- Используют: задачи 3 (сервис), 5 (fan-out), 10 (ретенция).

- [ ] **Шаг 1: Написать миграцию**

`migrations/044_notifications.up.sql` — только две таблицы фазы 1; `notification_channels`, `notification_identities` и `notification_link_tokens` появятся в фазе 2, потому что без каналов они пустые (YAGNI).

```sql
-- Уведомление адресовано конкретному получателю (user_id) и является одновременно
-- строкой in-app колокольчика. Ссылки на сущности намеренно НЕ внешние ключи:
-- уведомление переживает удаление цели, ровно как строка activity_events.
CREATE TABLE notifications (
    id             BIGSERIAL PRIMARY KEY,
    tenant_id      BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    type           TEXT   NOT NULL,
    kind           TEXT   NOT NULL,
    actor_user_id  BIGINT NOT NULL REFERENCES users(id),
    team_id        BIGINT,
    period_id      BIGINT,
    goal_id        BIGINT,
    kr_id          BIGINT,
    comment_id     BIGINT,
    entity_title   TEXT   NOT NULL DEFAULT '',
    payload_json   JSONB  NOT NULL DEFAULT '{}',
    coalesce_key   TEXT   NOT NULL,
    coalesce_count INT    NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at        TIMESTAMPTZ,
    CONSTRAINT notifications_type CHECK (
        type IN ('goal_comment','my_comment_resolved','goal_changed','kr_progress')),
    -- Ключ схлопывания: тип:сущность:актор:бакет. UNIQUE делает вставку
    -- идемпотентной между репликами — ON CONFLICT DO UPDATE вместо read-then-write.
    UNIQUE (tenant_id, user_id, coalesce_key)
);

-- Лента колокольчика: свежие сверху, в пределах одного получателя.
CREATE INDEX idx_notifications_feed ON notifications (tenant_id, user_id, created_at DESC);
-- Бейдж: COUNT по частичному индексу, без чтения прочитанных.
CREATE INDEX idx_notifications_unread ON notifications (tenant_id, user_id) WHERE read_at IS NULL;
-- Ретенция чистит по возрасту.
CREATE INDEX idx_notifications_created ON notifications (created_at);

-- Настройки пользователя. Отсутствие строки = дефолт (включено, scope=own,
-- channels={in_app}), поэтому бэкфилл на всех пользователей не нужен.
-- tenant_id в ключе обязателен: человек состоит в нескольких пространствах.
CREATE TABLE notification_preferences (
    tenant_id BIGINT  NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id   BIGINT  NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    type      TEXT    NOT NULL,
    enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    scope     TEXT,
    channels  TEXT[]  NOT NULL DEFAULT '{in_app}',
    PRIMARY KEY (tenant_id, user_id, type),
    CONSTRAINT notification_preferences_type CHECK (
        type IN ('goal_comment','my_comment_resolved','goal_changed','kr_progress')),
    -- Скоуп неприменим к адресным типам: у my_comment_resolved он всегда NULL.
    CONSTRAINT notification_preferences_scope CHECK (
        scope IS NULL OR scope IN ('own','own_and_children','subtree'))
);
```

`migrations/044_notifications.down.sql`:

```sql
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS notifications;
```

- [ ] **Шаг 2: Написать падающий тест репозитория**

`internal/store/notifications/notifications_test.go`:

```go
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

// Актор резолвится в том же запросе. Бывший участник пространства отдаётся
// плейсхолдером без имени и аватара — та же PII-гарантия, что в журнале.
func TestActorResolvedAndFormerMemberHidden(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	if _, err := repo.Insert(ctx, scope, input("a")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	items, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Актор — пользователь id 2 (system:migration), provider = 'system',
	// поэтому он показывается по имени, несмотря на отсутствие членства.
	if items[0].ActorRemoved {
		t.Error("системный пользователь не должен считаться удалённым участником")
	}
	if items[0].ActorDisplayName == "" {
		t.Error("имя актора обязано резолвиться тем же запросом")
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
```

- [ ] **Шаг 3: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/store/notifications/ -v`
Ожидается: FAIL — пакета нет. Если Docker недоступен, тест скипается: тогда прогнать после установки Docker, иначе задача не проверена.

- [ ] **Шаг 4: Реализовать репозиторий**

`internal/store/notifications/notifications.go`:

```go
// Package notifications persists per-recipient notifications. One row is both the
// in-app bell entry and the anchor external deliveries will hang off in phase 2.
package notifications

import (
	"context"
	"encoding/json"
	"time"

	"okrs/internal/core/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Notification is one delivered notification.
type Notification struct {
	ID            int64
	Type          string
	Kind          string
	ActorUserID   int64
	TeamID        *int64
	PeriodID      *int64
	GoalID        *int64
	KRID          *int64
	CommentID     *int64
	EntityTitle   string
	Payload       map[string]any
	CoalesceCount int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ReadAt        *time.Time

	// Actor is resolved on read, in the same query — a second lookup per row would
	// be N+1, and the renderer needs the name for every notification.
	ActorDisplayName string
	ActorAvatarURL   string
	// ActorRemoved marks an actor who is no longer an active member of the tenant.
	// Same PII rule as the activity journal: name and avatar of a former member are
	// not exposed, only a neutral placeholder.
	ActorRemoved bool
}

// InsertInput is one notification to store. CoalesceKey is built by the caller
// (usecase/notification) and encodes type:entity:actor:time-bucket.
type InsertInput struct {
	UserID      int64
	Type        string
	Kind        string
	ActorUserID int64
	TeamID      *int64
	PeriodID    *int64
	GoalID      *int64
	KRID        *int64
	CommentID   *int64
	EntityTitle string
	Payload     map[string]any
	CoalesceKey string
}

// Cursor is the keyset pagination position, mirroring store/activity.
type Cursor struct {
	CreatedAt time.Time
	ID        int64
}

type ListFilter struct {
	UnreadOnly bool
	Limit      int
	Cursor     *Cursor
}

const insertSQL = `
INSERT INTO notifications
  (tenant_id, user_id, type, kind, actor_user_id, team_id, period_id, goal_id, kr_id,
   comment_id, entity_title, payload_json, coalesce_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (tenant_id, user_id, coalesce_key) DO UPDATE
   SET coalesce_count = notifications.coalesce_count + 1,
       updated_at     = now(),
       payload_json   = EXCLUDED.payload_json,
       -- A repeat inside the window is new information: light the badge again.
       read_at        = NULL
RETURNING (xmax = 0) AS inserted`

// Insert stores one notification, coalescing into an existing row when the key
// matches. Returns whether a new row was created (false = coalesced).
//
// One atomic statement, so concurrent replicas cannot duplicate a row: the unique
// index arbitrates instead of a read-then-write race.
func (r *Repository) Insert(ctx context.Context, scope domain.TenantScope, in InsertInput) (bool, error) {
	raw, err := marshalPayload(in.Payload)
	if err != nil {
		return false, err
	}
	var inserted bool
	err = r.db.QueryRow(ctx, insertSQL,
		scope.TenantID, in.UserID, in.Type, in.Kind, in.ActorUserID,
		in.TeamID, in.PeriodID, in.GoalID, in.KRID, in.CommentID,
		in.EntityTitle, raw, in.CoalesceKey,
	).Scan(&inserted)
	return inserted, err
}

// InsertBatch stores many notifications in a single pipelined round-trip.
// Батчевая операция: не превращать в цикл Insert — это N+1 на горячем пути fan-out.
func (r *Repository) InsertBatch(ctx context.Context, scope domain.TenantScope, ins []InsertInput) error {
	if len(ins) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for _, in := range ins {
		raw, err := marshalPayload(in.Payload)
		if err != nil {
			return err
		}
		b.Queue(insertSQL,
			scope.TenantID, in.UserID, in.Type, in.Kind, in.ActorUserID,
			in.TeamID, in.PeriodID, in.GoalID, in.KRID, in.CommentID,
			in.EntityTitle, raw, in.CoalesceKey)
	}
	br := r.db.SendBatch(ctx, b)
	defer br.Close()
	for range ins {
		var inserted bool
		if err := br.QueryRow().Scan(&inserted); err != nil {
			return err
		}
	}
	return nil
}

func marshalPayload(p map[string]any) ([]byte, error) {
	if p == nil {
		p = map[string]any{}
	}
	return json.Marshal(p)
}

// List returns a page of the recipient's notifications, newest first.
func (r *Repository) List(ctx context.Context, scope domain.TenantScope, userID int64, f ListFilter) ([]Notification, *Cursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	args := []any{scope.TenantID, userID}
	// The actor is joined here rather than looked up per row: one query, no N+1.
	// A former member (no active membership, and not a system user) is returned as a
	// neutral placeholder — the journal applies the same PII rule.
	q := `SELECT n.id, n.type, n.kind, n.actor_user_id, n.team_id, n.period_id, n.goal_id,
	             n.kr_id, n.comment_id, n.entity_title, n.payload_json, n.coalesce_count,
	             n.created_at, n.updated_at, n.read_at,
	             CASE WHEN m.user_id IS NULL AND u.provider <> 'system'
	                  THEN '' ELSE u.display_name END,
	             CASE WHEN m.user_id IS NULL AND u.provider <> 'system'
	                  THEN '' ELSE COALESCE(u.avatar_url, '') END,
	             (m.user_id IS NULL AND u.provider <> 'system')
	        FROM notifications n
	        JOIN users u ON u.id = n.actor_user_id
	        LEFT JOIN memberships m
	               ON m.user_id = u.id AND m.tenant_id = n.tenant_id AND m.status = 'active'
	       WHERE n.tenant_id = $1 AND n.user_id = $2`
	if f.UnreadOnly {
		q += ` AND n.read_at IS NULL`
	}
	if f.Cursor != nil {
		args = append(args, f.Cursor.CreatedAt, f.Cursor.ID)
		q += ` AND (n.created_at, n.id) < ($3, $4)`
	}
	args = append(args, limit+1) // +1 запрашивается, чтобы узнать, есть ли следующая страница
	q += ` ORDER BY n.created_at DESC, n.id DESC LIMIT $` + strconv.Itoa(len(args))

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		var raw []byte
		if err := rows.Scan(&n.ID, &n.Type, &n.Kind, &n.ActorUserID, &n.TeamID, &n.PeriodID,
			&n.GoalID, &n.KRID, &n.CommentID, &n.EntityTitle, &raw, &n.CoalesceCount,
			&n.CreatedAt, &n.UpdatedAt, &n.ReadAt,
			&n.ActorDisplayName, &n.ActorAvatarURL, &n.ActorRemoved); err != nil {
			return nil, nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &n.Payload)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *Cursor
	if len(out) > limit {
		last := out[limit-1]
		next = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		out = out[:limit]
	}
	return out, next, nil
}

// UnreadCount powers the bell badge. Partial-index COUNT for one user — cheap enough
// that caching it across K8s replicas would buy staleness, not speed.
func (r *Repository) UnreadCount(ctx context.Context, scope domain.TenantScope, userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE tenant_id = $1 AND user_id = $2 AND read_at IS NULL`,
		scope.TenantID, userID).Scan(&n)
	return n, err
}

// MarkRead marks the given ids read, or everything unread when all is true.
// Scoped by user_id, so one user can never mark another's notifications.
func (r *Repository) MarkRead(ctx context.Context, scope domain.TenantScope, userID int64, ids []int64, all bool) error {
	if all {
		_, err := r.db.Exec(ctx,
			`UPDATE notifications SET read_at = now()
			  WHERE tenant_id = $1 AND user_id = $2 AND read_at IS NULL`,
			scope.TenantID, userID)
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = now()
		  WHERE tenant_id = $1 AND user_id = $2 AND id = ANY($3) AND read_at IS NULL`,
		scope.TenantID, userID, ids)
	return err
}

// PurgeOlderThan drops read notifications older than readDays and anything older
// than anyDays. Cross-tenant on purpose: it is the retention pass, not a user action.
func (r *Repository) PurgeOlderThan(ctx context.Context, readDays, anyDays int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM notifications
		 WHERE (read_at IS NOT NULL AND created_at < now() - make_interval(days => $1))
		    OR created_at < now() - make_interval(days => $2)`,
		readDays, anyDays)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
```

Импорты пакета: `context`, `encoding/json`, `strconv`, `time`, `okrs/internal/core/domain`, `github.com/jackc/pgx/v5`, `github.com/jackc/pgx/v5/pgxpool`.

- [ ] **Шаг 5: Подключить репозиторий в composite**

`internal/store/store.go` — добавить поле и его инициализацию:

```go
	Notifications *notifications.Repository
```
```go
		Notifications: notifications.NewRepository(db),
```

- [ ] **Шаг 6: Прогнать тесты**

Запустить: `go test ./internal/store/notifications/ ./internal/store/ -count=1 -v`
Ожидается: PASS, шесть тестов репозитория.

- [ ] **Шаг 7: Проверить обратимость миграции**

Откат обязан быть рабочим: иначе неудачный деплой невозможно отыграть.

Запустить: `go test ./internal/store/... -count=1`
Ожидается: PASS. Дополнительно вручную: применить `044.up.sql`, затем `044.down.sql`, затем снова `up` — ошибок нет (в `down` таблицы удаляются в порядке, обратном созданию, поэтому FK не мешают).

---

## Task 2: Настройки и резолв получателей

Сердце фичи. Один рекурсивный запрос отвечает на вопрос «кому адресовать событие, случившееся в команде T».

**Файлы:**
- Создать: `internal/store/notificationprefs/notificationprefs.go`
- Тест: `internal/store/notificationprefs/notificationprefs_test.go`
- Изменить: `internal/store/store.go`

**Интерфейсы:**
- Производит:
  - `notificationprefs.Preference{Type string; Enabled bool; Scope string; Channels []string}`
  - `(*Repository).GetAll(ctx, scope, userID int64) ([]Preference, error)` — с подстановкой дефолтов
  - `(*Repository).Set(ctx, scope, userID int64, p Preference) error`
  - `notificationprefs.Target{TeamID, ActorID int64}` и `notificationprefs.Recipient{Ord int; UserID int64; Channels []string}`
  - `(*Repository).ResolveRecipients(ctx, scope, notifType string, targets []Target) ([]Recipient, error)`
  - `(*Repository).ResolveAddressed(ctx, scope, notifType string, userIDs []int64) ([]Recipient, error)`
  - `store.Store.NotificationPrefs *notificationprefs.Repository`
- Используют: задачи 3 (сервис) и 5 (fan-out).

- [ ] **Шаг 1: Написать падающий тест**

`internal/store/notificationprefs/notificationprefs_test.go`:

```go
package notificationprefs_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// tree строит дерево команд корень → середина → лист, у каждой свой лид,
// и делает всех лидов активными участниками тенанта 1.
// Возвращает id команд и id пользователей-лидов.
func tree(t *testing.T, pool *pgxpool.Pool) (teamIDs [3]int64, leadIDs [3]int64) {
	t.Helper()
	ctx := context.Background()
	names := []string{"Корень", "Середина", "Лист"}
	var parent *int64
	for i, name := range names {
		var udid string
		err := pool.QueryRow(ctx, `
			INSERT INTO users (provider_subject_key, provider, subject, display_name)
			VALUES ($1,'system',$1,$2) RETURNING id, udid`,
			"lead-"+name, "Лид "+name).Scan(&leadIDs[i], &udid)
		if err != nil {
			t.Fatalf("создать лида %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,1,'user','active')`,
			leadIDs[i]); err != nil {
			t.Fatalf("членство %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO teams (name, type, parent_id, tenant_id, lead_udid)
			VALUES ($1,'team',$2,1,$3) RETURNING id`,
			name, parent, udid).Scan(&teamIDs[i]); err != nil {
			t.Fatalf("создать команду %s: %v", name, err)
		}
		p := teamIDs[i]
		parent = &p
	}
	return teamIDs, leadIDs
}

func has(rs []notificationprefs.Recipient, userID int64) bool {
	for _, r := range rs {
		if r.UserID == userID {
			return true
		}
	}
	return false
}

// Дефолт (строки настроек нет) — scope=own: событие в листе уведомляет только
// лида листа, ни середину, ни корень.
func TestResolveDefaultScopeIsOwn(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}

	rs, err := repo.ResolveRecipients(context.Background(), scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !has(rs, leads[2]) {
		t.Error("лид собственной команды обязан получить уведомление")
	}
	if has(rs, leads[1]) || has(rs, leads[0]) {
		t.Error("при scope=own предки не уведомляются")
	}
}

// own_and_children: лид середины получает событие из листа (дистанция 1),
// лид корня — нет (дистанция 2).
func TestResolveOwnAndChildren(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	for _, id := range []int64{leads[0], leads[1]} {
		if err := repo.Set(ctx, scope, id, notificationprefs.Preference{
			Type: "goal_changed", Enabled: true, Scope: "own_and_children", Channels: []string{"in_app"},
		}); err != nil {
			t.Fatalf("set: %v", err)
		}
	}

	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !has(rs, leads[1]) {
		t.Error("лид на дистанции 1 обязан получить уведомление")
	}
	if has(rs, leads[0]) {
		t.Error("лид на дистанции 2 не должен получать при own_and_children")
	}
}

// subtree: событие из листа доходит до корня на любой глубине.
func TestResolveSubtree(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if err := repo.Set(ctx, scope, leads[0], notificationprefs.Preference{
		Type: "goal_changed", Enabled: true, Scope: "subtree", Channels: []string{"in_app"},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !has(rs, leads[0]) {
		t.Error("при subtree корень обязан получить событие из листа")
	}
}

// Актор не уведомляется о собственном действии — и исключается ПОШТУЧНО:
// лид, оказавшийся автором одного события в батче, должен получить остальные.
func TestActorExcludedPerEventNotPerBatch(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}

	// Два события: в первом актор — лид листа, во втором — посторонний (id 1).
	rs, err := repo.ResolveRecipients(context.Background(), scope, "goal_changed",
		[]notificationprefs.Target{
			{TeamID: teams[2], ActorID: leads[2]},
			{TeamID: teams[2], ActorID: 1},
		})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, r := range rs {
		if r.Ord == 0 && r.UserID == leads[2] {
			t.Error("актор получил уведомление о собственном действии")
		}
	}
	found := false
	for _, r := range rs {
		if r.Ord == 1 && r.UserID == leads[2] {
			found = true
		}
	}
	if !found {
		t.Error("лид не получил уведомление о ЧУЖОМ действии в том же батче")
	}
}

// Выключенный тип не приносит уведомлений вообще.
func TestDisabledTypeYieldsNoRecipients(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if err := repo.Set(ctx, scope, leads[2], notificationprefs.Preference{
		Type: "goal_changed", Enabled: false, Scope: "own", Channels: []string{"in_app"},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if has(rs, leads[2]) {
		t.Error("выключенный тип не должен давать получателей")
	}
}

// Лид, исключённый из пространства, уведомления не получает, хотя
// teams.lead_udid у команды остался заполненным.
func TestInactiveMemberExcluded(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `DELETE FROM memberships WHERE user_id = $1 AND tenant_id = 1`, leads[2]); err != nil {
		t.Fatalf("удалить членство: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if has(rs, leads[2]) {
		t.Error("бывший участник пространства не должен получать уведомления")
	}
}

// Soft-deleted команда в середине цепочки обрывает подъём: предки за ней
// не считаются частью поддерева.
func TestSoftDeletedTeamBreaksChain(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if err := repo.Set(ctx, scope, leads[0], notificationprefs.Preference{
		Type: "goal_changed", Enabled: true, Scope: "subtree", Channels: []string{"in_app"},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE teams SET deleted_at = now() WHERE id = $1`, teams[1]); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if has(rs, leads[0]) {
		t.Error("удалённая команда в цепочке должна обрывать подъём к предкам")
	}
}

// GetAll подставляет дефолты для типов, у которых строки нет: все четыре типа
// должны вернуться всегда, иначе экран настроек покажет пустоту новому пользователю.
func TestGetAllReturnsDefaultsForMissingRows(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	prefs, err := repo.GetAll(context.Background(), scope, 1)
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(prefs) != 4 {
		t.Fatalf("ожидались все 4 типа, got %d", len(prefs))
	}
	for _, p := range prefs {
		if !p.Enabled {
			t.Errorf("%s: дефолт должен быть включён", p.Type)
		}
		if p.Type == "my_comment_resolved" {
			if p.Scope != "" {
				t.Errorf("у адресного типа скоуп неприменим, got %q", p.Scope)
			}
			continue
		}
		if p.Scope != "own" {
			t.Errorf("%s: дефолтный скоуп own, got %q", p.Type, p.Scope)
		}
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/store/notificationprefs/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 3: Реализовать репозиторий**

`internal/store/notificationprefs/notificationprefs.go`:

```go
// Package notificationprefs persists per-user notification preferences and answers
// the question the fan-out actually asks: who is subscribed to an event that
// happened in team T.
//
// Resolution lives here rather than in a separate read model because it is a read of
// preferences enriched with the team tree — not a second entity.
package notificationprefs

import (
	"context"

	"okrs/internal/core/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Notification types. Scoped types resolve through the team tree; addressed types
// carry their recipient in the event itself.
const (
	TypeGoalComment       = "goal_comment"
	TypeMyCommentResolved = "my_comment_resolved"
	TypeGoalChanged       = "goal_changed"
	TypeKRProgress        = "kr_progress"
)

// AllTypes is the order the settings screen renders.
var AllTypes = []string{TypeGoalComment, TypeMyCommentResolved, TypeGoalChanged, TypeKRProgress}

// IsAddressed reports whether a type is addressed rather than scope-based. An
// addressed type has no scope selector: it is delivered to a specific person.
func IsAddressed(t string) bool { return t == TypeMyCommentResolved }

// Scope values.
const (
	ScopeOwn             = "own"
	ScopeOwnAndChildren  = "own_and_children"
	ScopeSubtree         = "subtree"
)

type Preference struct {
	Type     string
	Enabled  bool
	Scope    string
	Channels []string
}

// Target is one event's addressing input: the team it happened in and who did it.
type Target struct {
	TeamID  int64
	ActorID int64
}

// Recipient is one resolved addressee. Ord is the index of the originating Target,
// so the caller maps results back onto its batch.
type Recipient struct {
	Ord      int
	UserID   int64
	Channels []string
}

// defaultPreference is what applies when the user has never touched settings:
// enabled, own team only, in-app. Missing rows are the norm, not an exception —
// that is why nothing is backfilled on user creation.
func defaultPreference(t string) Preference {
	p := Preference{Type: t, Enabled: true, Channels: []string{"in_app"}}
	if !IsAddressed(t) {
		p.Scope = ScopeOwn
	}
	return p
}

// GetAll returns all four types, substituting defaults for rows that do not exist.
func (r *Repository) GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]Preference, error) {
	rows, err := r.db.Query(ctx,
		`SELECT type, enabled, COALESCE(scope, ''), channels
		   FROM notification_preferences WHERE tenant_id = $1 AND user_id = $2`,
		scope.TenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stored := make(map[string]Preference)
	for rows.Next() {
		var p Preference
		if err := rows.Scan(&p.Type, &p.Enabled, &p.Scope, &p.Channels); err != nil {
			return nil, err
		}
		stored[p.Type] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Preference, 0, len(AllTypes))
	for _, t := range AllTypes {
		if p, ok := stored[t]; ok {
			out = append(out, p)
			continue
		}
		out = append(out, defaultPreference(t))
	}
	return out, nil
}

// Set upserts one preference row.
func (r *Repository) Set(ctx context.Context, scope domain.TenantScope, userID int64, p Preference) error {
	var scopeVal any
	if !IsAddressed(p.Type) && p.Scope != "" {
		scopeVal = p.Scope
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channels)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, user_id, type) DO UPDATE
		   SET enabled = EXCLUDED.enabled, scope = EXCLUDED.scope, channels = EXCLUDED.channels`,
		scope.TenantID, userID, p.Type, p.Enabled, scopeVal, p.Channels)
	return err
}

const resolveSQL = `
WITH RECURSIVE chain AS (
    SELECT src.ord, src.actor_id, t.id, t.parent_id, t.lead_udid, 0 AS distance
      FROM unnest($1::bigint[], $4::bigint[]) WITH ORDINALITY AS src(team_id, actor_id, ord)
      JOIN teams t ON t.id = src.team_id AND t.deleted_at IS NULL
    UNION ALL
    SELECT c.ord, c.actor_id, t.id, t.parent_id, t.lead_udid, c.distance + 1
      FROM teams t JOIN chain c ON t.id = c.parent_id
     WHERE t.deleted_at IS NULL
)
SELECT c.ord - 1, u.id, COALESCE(p.channels, '{in_app}'::text[])
  FROM chain c
  JOIN users u       ON u.udid = c.lead_udid
  JOIN memberships m ON m.user_id = u.id AND m.tenant_id = $2 AND m.status = 'active'
  LEFT JOIN notification_preferences p
         ON p.tenant_id = $2 AND p.user_id = u.id AND p.type = $3
 WHERE u.id <> c.actor_id
   AND COALESCE(p.enabled, TRUE)
   AND CASE COALESCE(p.scope, 'own')
         WHEN 'own'              THEN c.distance = 0
         WHEN 'own_and_children' THEN c.distance <= 1
         ELSE TRUE
       END`

// ResolveRecipients answers "who must be notified" for a whole batch of events at
// once: $1 and $4 are parallel arrays of team and actor, one pair per event, and the
// result carries Ord so the caller maps rows back onto its batch.
//
// Actor exclusion is per event (c.actor_id travels down the recursion), not per
// batch: a lead who authored one event must still be notified about the others.
//
// Батчевая операция: не превращать в цикл по событиям — это N+1 на горячем пути.
func (r *Repository) ResolveRecipients(ctx context.Context, scope domain.TenantScope, notifType string, targets []Target) ([]Recipient, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	teamIDs := make([]int64, len(targets))
	actorIDs := make([]int64, len(targets))
	for i, t := range targets {
		teamIDs[i], actorIDs[i] = t.TeamID, t.ActorID
	}
	rows, err := r.db.Query(ctx, resolveSQL, teamIDs, scope.TenantID, notifType, actorIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recipient
	for rows.Next() {
		var rc Recipient
		if err := rows.Scan(&rc.Ord, &rc.UserID, &rc.Channels); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// ResolveAddressed filters explicitly addressed recipients (e.g. the author of a
// resolved task) by their preferences. No tree walk: the recipient is already known.
//
// Батчевая операция: не превращать в цикл — это N+1.
func (r *Repository) ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]Recipient, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT src.ord - 1, u.id, COALESCE(p.channels, '{in_app}'::text[])
		  FROM unnest($1::bigint[]) WITH ORDINALITY AS src(user_id, ord)
		  JOIN users u       ON u.id = src.user_id
		  JOIN memberships m ON m.user_id = u.id AND m.tenant_id = $2 AND m.status = 'active'
		  LEFT JOIN notification_preferences p
		         ON p.tenant_id = $2 AND p.user_id = u.id AND p.type = $3
		 WHERE COALESCE(p.enabled, TRUE)`,
		userIDs, scope.TenantID, notifType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recipient
	for rows.Next() {
		var rc Recipient
		if err := rows.Scan(&rc.Ord, &rc.UserID, &rc.Channels); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
```

`WITH ORDINALITY` нумерует с единицы, поэтому в `SELECT` стоит `c.ord - 1` — наружу отдаётся индекс среза, а не порядковый номер SQL.

- [ ] **Шаг 4: Подключить репозиторий в composite**

`internal/store/store.go`:

```go
	NotificationPrefs *notificationprefs.Repository
```
```go
		NotificationPrefs: notificationprefs.NewRepository(db),
```

- [ ] **Шаг 5: Прогнать тесты**

Запустить: `go test ./internal/store/notificationprefs/ -count=1 -v`
Ожидается: PASS, восемь тестов.

- [ ] **Шаг 6: Проверить, что резолв батча — один запрос**

Запустить: `rg -n 'ResolveRecipients' -A 25 internal/store/notificationprefs/notificationprefs.go | rg -c 'r\.db\.Query'`
Ожидается: `1`. Если запросов больше одного — резолв выродился в цикл, и правило 9 нарушено.

---

## Task 3: Сервисы уведомлений и настроек

Тонкий слой: каждый сервис работает ровно с одним репозиторием, как требует `specs/010`.

**Файлы:**
- Создать: `internal/service/notification/notification.go`
- Создать: `internal/service/notificationpref/notificationpref.go`
- Тест: `internal/service/notificationpref/notificationpref_test.go`

**Интерфейсы:**
- Потребляет: репозитории из задач 1 и 2.
- Производит:
  - `notificationsvc.New(repo Repo) *Service` с методами `Create`, `CreateBatch`, `List`, `UnreadCount`, `MarkRead`, `Purge`
  - `notificationprefsvc.New(repo Repo) *Service` с методами `GetAll`, `Set`, `Resolve`, `ResolveAddressed`
  - `notificationprefsvc.ErrInvalidType`, `ErrInvalidScope` — валидация входа
- Используют: задачи 5 (fan-out), 6 и 7 (handlers), 10 (ретенция).

- [ ] **Шаг 1: Написать падающий тест валидации**

Валидация — единственная нетривиальная логика в этих сервисах, её и проверяем. Остальное — проброс, который покрыт тестами репозиториев.

`internal/service/notificationpref/notificationpref_test.go`:

```go
package notificationpref_test

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/core/domain"
	notificationprefsvc "okrs/internal/service/notificationpref"
	"okrs/internal/store/notificationprefs"
)

// fakeRepo фиксирует, что дошло до стора: валидация обязана отсекать мусор
// ДО записи, а не полагаться на CHECK-ограничение в БД.
type fakeRepo struct{ saved []notificationprefs.Preference }

func (f *fakeRepo) GetAll(context.Context, domain.TenantScope, int64) ([]notificationprefs.Preference, error) {
	return nil, nil
}

func (f *fakeRepo) Set(_ context.Context, _ domain.TenantScope, _ int64, p notificationprefs.Preference) error {
	f.saved = append(f.saved, p)
	return nil
}

func (f *fakeRepo) ResolveRecipients(context.Context, domain.TenantScope, string, []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func (f *fakeRepo) ResolveAddressed(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func TestSetRejectsUnknownType(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: "made_up", Enabled: true, Scope: "own"})
	if !errors.Is(err, notificationprefsvc.ErrInvalidType) {
		t.Fatalf("got %v, want ErrInvalidType", err)
	}
	if len(repo.saved) != 0 {
		t.Error("невалидный тип не должен доходить до стора")
	}
}

func TestSetRejectsUnknownScope(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "everything"})
	if !errors.Is(err, notificationprefsvc.ErrInvalidScope) {
		t.Fatalf("got %v, want ErrInvalidScope", err)
	}
}

// У адресного типа скоуп неприменим: даже если клиент его прислал, он
// затирается, иначе в БД появится строка, противоречащая CHECK-ограничению.
func TestSetClearsScopeForAddressedType(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeMyCommentResolved, Enabled: true, Scope: "subtree"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if repo.saved[0].Scope != "" {
		t.Fatalf("скоуп адресного типа должен обнуляться, got %q", repo.saved[0].Scope)
	}
}

// Пустой список каналов означал бы «уведомление некуда доставить»: тихо
// починить осмысленнее, чем сохранить бесполезную настройку.
func TestSetDefaultsEmptyChannelsToInApp(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if len(repo.saved[0].Channels) != 1 || repo.saved[0].Channels[0] != "in_app" {
		t.Fatalf("got %v, want [in_app]", repo.saved[0].Channels)
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/service/notificationpref/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 3: Реализовать сервис настроек**

`internal/service/notificationpref/notificationpref.go`:

```go
// Package notificationpref is the notification-preferences entity service: reads,
// validated writes, and recipient resolution for the fan-out.
package notificationpref

import (
	"context"
	"errors"
	"slices"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationprefs"
)

var (
	ErrInvalidType  = errors.New("notificationpref: unknown notification type")
	ErrInvalidScope = errors.New("notificationpref: unknown scope")
)

// Repo is the port this service needs. Declared consumer-side, per specs/010.
type Repo interface {
	GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error)
	Set(ctx context.Context, scope domain.TenantScope, userID int64, p notificationprefs.Preference) error
	ResolveRecipients(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error)
	ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error)
}

type Service struct{ repo Repo }

func New(repo Repo) *Service { return &Service{repo: repo} }

func (s *Service) GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error) {
	return s.repo.GetAll(ctx, scope, userID)
}

// Set validates before writing. The DB CHECK constraints are a backstop, not the
// place a user-facing error should come from.
func (s *Service) Set(ctx context.Context, scope domain.TenantScope, userID int64, p notificationprefs.Preference) error {
	if !slices.Contains(notificationprefs.AllTypes, p.Type) {
		return ErrInvalidType
	}
	if notificationprefs.IsAddressed(p.Type) {
		// Scope is meaningless for an addressed type; drop whatever the client sent.
		p.Scope = ""
	} else {
		if p.Scope == "" {
			p.Scope = notificationprefs.ScopeOwn
		}
		valid := []string{notificationprefs.ScopeOwn, notificationprefs.ScopeOwnAndChildren, notificationprefs.ScopeSubtree}
		if !slices.Contains(valid, p.Scope) {
			return ErrInvalidScope
		}
	}
	if len(p.Channels) == 0 {
		// An empty channel set means "nowhere to deliver". In phase 1b in_app is the
		// only channel, so fixing it quietly beats storing a useless preference.
		p.Channels = []string{"in_app"}
	}
	return s.repo.Set(ctx, scope, userID, p)
}

// Батчевая операция: не превращать в цикл по событиям — это N+1.
func (s *Service) Resolve(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return s.repo.ResolveRecipients(ctx, scope, notifType, targets)
}

// Батчевая операция: не превращать в цикл — это N+1.
func (s *Service) ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error) {
	return s.repo.ResolveAddressed(ctx, scope, notifType, userIDs)
}
```

- [ ] **Шаг 4: Реализовать сервис уведомлений**

`internal/service/notification/notification.go`:

```go
// Package notification is the notifications entity service: create, read, mark read,
// retention. One repository, no business rules — the fan-out lives in usecase.
package notification

import (
	"context"

	"okrs/internal/core/domain"
	"okrs/internal/store/notifications"
)

// Repo is the port this service needs. Declared consumer-side, per specs/010.
type Repo interface {
	Insert(ctx context.Context, scope domain.TenantScope, in notifications.InsertInput) (bool, error)
	InsertBatch(ctx context.Context, scope domain.TenantScope, ins []notifications.InsertInput) error
	List(ctx context.Context, scope domain.TenantScope, userID int64, f notifications.ListFilter) ([]notifications.Notification, *notifications.Cursor, error)
	UnreadCount(ctx context.Context, scope domain.TenantScope, userID int64) (int, error)
	MarkRead(ctx context.Context, scope domain.TenantScope, userID int64, ids []int64, all bool) error
	PurgeOlderThan(ctx context.Context, readDays, anyDays int) (int64, error)
}

type Service struct{ repo Repo }

func New(repo Repo) *Service { return &Service{repo: repo} }

func (s *Service) Create(ctx context.Context, scope domain.TenantScope, in notifications.InsertInput) (bool, error) {
	return s.repo.Insert(ctx, scope, in)
}

// Батчевая операция: не превращать в цикл Create — это N+1 на горячем пути fan-out.
func (s *Service) CreateBatch(ctx context.Context, scope domain.TenantScope, ins []notifications.InsertInput) error {
	return s.repo.InsertBatch(ctx, scope, ins)
}

func (s *Service) List(ctx context.Context, scope domain.TenantScope, userID int64, f notifications.ListFilter) ([]notifications.Notification, *notifications.Cursor, error) {
	return s.repo.List(ctx, scope, userID, f)
}

func (s *Service) UnreadCount(ctx context.Context, scope domain.TenantScope, userID int64) (int, error) {
	return s.repo.UnreadCount(ctx, scope, userID)
}

func (s *Service) MarkRead(ctx context.Context, scope domain.TenantScope, userID int64, ids []int64, all bool) error {
	return s.repo.MarkRead(ctx, scope, userID, ids, all)
}

// Purge is the retention pass, run from the scheduler.
func (s *Service) Purge(ctx context.Context, readDays, anyDays int) (int64, error) {
	return s.repo.PurgeOlderThan(ctx, readDays, anyDays)
}
```

- [ ] **Шаг 5: Прогнать тесты**

Запустить: `go test ./internal/service/notificationpref/ ./internal/service/notification/ -count=1 -v`
Ожидается: PASS, четыре теста валидации.

- [ ] **Шаг 6: Проверить, что сервисы не тянут лишнего**

Запустить: `go list -deps ./internal/service/notification/ ./internal/service/notificationpref/ | rg 'okrs/internal/(usecase|http)'`
Ожидается: пустой вывод. Сервис не знает ни про usecase, ни про HTTP.

---

## Task 4: Рендер текста уведомления

Текст собирает сервер: в фазе 2 те же строки уйдут в Telegram и Mattermost, и держать шаблоны отдельно в Go и в JS означало бы гарантированное расхождение формулировок.

**Файлы:**
- Создать: `internal/render/notify/notify.go`
- Тест: `internal/render/notify/notify_test.go`

**Интерфейсы:**
- Потребляет: `event.Kind` (фаза 1a).
- Производит: `notify.Text{Title, Body string}` и `notify.Render(in notify.Input) Text`, где `Input{Kind event.Kind; ActorName, EntityTitle string; Count int; Payload map[string]any}`.
- Используют: задача 6 (сборка DTO ленты).

- [ ] **Шаг 1: Написать падающий тест**

`internal/render/notify/notify_test.go`:

```go
package notify_test

import (
	"strings"
	"testing"

	"okrs/internal/core/event"
	"okrs/internal/render/notify"
)

func TestRenderCommentAdded(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindCommentAdded, ActorName: "Пётр", EntityTitle: "Снизить отток", Count: 1,
		Payload: map[string]any{"text": "Уточните метрику"},
	})
	if !strings.Contains(got.Title, "Пётр") || !strings.Contains(got.Title, "комментарий") {
		t.Errorf("заголовок: %q", got.Title)
	}
	if !strings.Contains(got.Body, "Снизить отток") {
		t.Errorf("тело должно называть цель: %q", got.Body)
	}
}

// Схлопнутое уведомление обязано сообщать, сколько событий за ним стоит,
// иначе «×3» в интерфейсе окажется единственным намёком.
func TestRenderMentionsCoalesceCount(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRProgressUpdated, ActorName: "Пётр", EntityTitle: "MAU", Count: 3,
		Payload: map[string]any{"after": map[string]any{"progress": 60}},
	})
	if !strings.Contains(got.Title, "3") {
		t.Errorf("заголовок схлопнутого уведомления должен нести счётчик: %q", got.Title)
	}
}

func TestRenderProgressShowsPercent(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindKRProgressUpdated, ActorName: "Пётр", EntityTitle: "MAU", Count: 1,
		Payload: map[string]any{
			"before": map[string]any{"progress": float64(10)},
			"after":  map[string]any{"progress": float64(60)},
		},
	})
	if !strings.Contains(got.Body, "60") {
		t.Errorf("тело должно показывать новый процент: %q", got.Body)
	}
}

func TestRenderMyCommentResolved(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindCommentResolved, ActorName: "Анна", EntityTitle: "Снизить отток", Count: 1,
	})
	if !strings.Contains(got.Title, "Анна") || !strings.Contains(strings.ToLower(got.Title), "решил") {
		t.Errorf("заголовок: %q", got.Title)
	}
}

// Неизвестный kind не должен приводить к пустой строке в интерфейсе:
// уведомление уже создано, и показать его надо хоть как-то.
func TestRenderUnknownKindHasFallback(t *testing.T) {
	got := notify.Render(notify.Input{Kind: "made_up", ActorName: "Пётр", EntityTitle: "Цель", Count: 1})
	if got.Title == "" {
		t.Fatal("для неизвестного kind обязан быть запасной заголовок")
	}
}

// Все 13 kind, порождающих уведомления, должны рендериться осмысленно —
// иначе новое событие в фазе 2 молча даст пустой заголовок.
func TestRenderCoversEveryNotifyingKind(t *testing.T) {
	kinds := []event.Kind{
		event.KindCommentAdded, event.KindReplyAdded, event.KindCommentResolved,
		event.KindGoalCreated, event.KindGoalCopied, event.KindGoalMoved, event.KindGoalDeleted,
		event.KindGoalFieldsChanged, event.KindGoalOwnerChanged,
		event.KindKRCreated, event.KindKRFieldsChanged, event.KindKRDeleted,
		event.KindKRProgressUpdated,
	}
	if len(kinds) != 13 {
		t.Fatalf("перечислено %d kind, ожидалось 13", len(kinds))
	}
	for _, k := range kinds {
		got := notify.Render(notify.Input{Kind: k, ActorName: "Пётр", EntityTitle: "Цель", Count: 1})
		if got.Title == "" || got.Title == notify.FallbackTitle {
			t.Errorf("%s: нет собственного заголовка (got %q)", k, got.Title)
		}
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/render/notify/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 3: Реализовать рендер**

`internal/render/notify/notify.go`:

```go
// Package notify renders a notification into human text. Server-side on purpose:
// phase 2 sends the same strings to Telegram and Mattermost, and keeping templates
// in both Go and JS would guarantee the wordings drift apart.
//
// Mirrors internal/render/export, which renders OKRs to Markdown.
package notify

import (
	"fmt"

	"okrs/internal/core/event"
)

// FallbackTitle is used when a kind has no dedicated wording. Exported so tests can
// assert that every notifying kind has its own text rather than falling through.
const FallbackTitle = "Обновление по цели"

type Input struct {
	Kind        event.Kind
	ActorName   string
	EntityTitle string
	Count       int // coalesce_count: >1 means several events collapsed into one
	Payload     map[string]any
}

type Text struct {
	Title string
	Body  string
}

// Render builds the notification's title and body.
func Render(in Input) Text {
	title, body := wording(in)
	if in.Count > 1 {
		title = fmt.Sprintf("%s (%d)", title, in.Count)
	}
	return Text{Title: title, Body: body}
}

func wording(in Input) (title, body string) {
	actor := in.ActorName
	if actor == "" {
		actor = "Кто-то"
	}
	switch in.Kind {
	case event.KindCommentAdded:
		return actor + " оставил комментарий", quoteOr(in, "text", in.EntityTitle)
	case event.KindReplyAdded:
		return actor + " ответил в обсуждении", quoteOr(in, "text", in.EntityTitle)
	case event.KindCommentResolved:
		return actor + " решил ваш комментарий", in.EntityTitle

	case event.KindGoalCreated:
		return actor + " создал цель", in.EntityTitle
	case event.KindGoalCopied:
		return actor + " скопировал цель", in.EntityTitle
	case event.KindGoalMoved:
		return actor + " перенёс цель", in.EntityTitle
	case event.KindGoalDeleted:
		return actor + " удалил цель", in.EntityTitle
	case event.KindGoalFieldsChanged:
		return actor + " изменил цель", in.EntityTitle
	case event.KindGoalOwnerChanged:
		return actor + " сменил владельца цели", in.EntityTitle

	case event.KindKRCreated:
		return actor + " добавил ключевой результат", in.EntityTitle
	case event.KindKRFieldsChanged:
		return actor + " изменил ключевой результат", in.EntityTitle
	case event.KindKRDeleted:
		return actor + " удалил ключевой результат", in.EntityTitle

	case event.KindKRProgressUpdated:
		return actor + " обновил прогресс", progressBody(in)
	}
	return FallbackTitle, in.EntityTitle
}

// quoteOr returns the payload's text field, falling back to the entity title when
// the payload is missing — a notification must never render as an empty line.
func quoteOr(in Input, field, fallback string) string {
	if s, ok := in.Payload[field].(string); ok && s != "" {
		return s
	}
	return fallback
}

func progressBody(in Input) string {
	after, ok := percent(in.Payload, "after")
	if !ok {
		return in.EntityTitle
	}
	if before, ok := percent(in.Payload, "before"); ok {
		return fmt.Sprintf("%s: %d%% → %d%%", in.EntityTitle, before, after)
	}
	return fmt.Sprintf("%s: %d%%", in.EntityTitle, after)
}

// percent digs progress out of the {before|after: {progress: N}} payload. Values
// arriving from JSONB are float64, values built in-process are int — handle both.
func percent(payload map[string]any, side string) (int, bool) {
	m, ok := payload[side].(map[string]any)
	if !ok {
		return 0, false
	}
	switch v := m["progress"].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}
```

- [ ] **Шаг 4: Прогнать тесты**

Запустить: `go test ./internal/render/notify/ -count=1 -v`
Ожидается: PASS, шесть тестов, включая покрытие всех 13 kind.

- [ ] **Шаг 5: Проверить чистоту пакета**

Запустить: `go list -deps ./internal/render/notify/ | rg 'okrs/internal/(store|service|usecase|http)'`
Ожидается: пустой вывод. Рендер зависит только от `core/event` — его можно вызывать откуда угодно, включая отправку в мессенджеры в фазе 2.

---

## Task 5: Fan-out — подписчик уведомлений

**Файлы:**
- Создать: `internal/usecase/notification/notification.go`
- Создать: `internal/usecase/notification/mapping.go`
- Тест: `internal/usecase/notification/notification_test.go`

**Интерфейсы:**
- Потребляет: `event.*` (фаза 1a), сервисы из задачи 3.
- Производит:
  - `notificationuc.New(deps Deps) *UseCase`, где `Deps{Notifications NotificationWriter; Prefs PrefResolver}` — оба порта объявлены здесь
  - `(*UseCase).Handle(ctx context.Context, evs []event.Event) error` — сигнатура под `eventbus.SubscribeAll`
  - `notificationuc.CoalesceWindow = 10 * time.Minute`
- Используют: задача 10 (подписка при сборке).

- [ ] **Шаг 1: Написать падающий тест отбора и схлопывания**

`internal/usecase/notification/notification_test.go`:

```go
package notification_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	notificationuc "okrs/internal/usecase/notification"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/notifications"
)

type fakeWriter struct{ rows []notifications.InsertInput }

func (f *fakeWriter) CreateBatch(_ context.Context, _ domain.TenantScope, ins []notifications.InsertInput) error {
	f.rows = append(f.rows, ins...)
	return nil
}

// fakePrefs возвращает одного и того же получателя на каждое событие батча.
type fakePrefs struct{ calls int }

func (f *fakePrefs) Resolve(_ context.Context, _ domain.TenantScope, _ string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	f.calls++
	out := make([]notificationprefs.Recipient, 0, len(targets))
	for i := range targets {
		out = append(out, notificationprefs.Recipient{Ord: i, UserID: 42, Channels: []string{"in_app"}})
	}
	return out, nil
}

func (f *fakePrefs) ResolveAddressed(_ context.Context, _ domain.TenantScope, _ string, userIDs []int64) ([]notificationprefs.Recipient, error) {
	f.calls++
	out := make([]notificationprefs.Recipient, 0, len(userIDs))
	for i, id := range userIDs {
		out = append(out, notificationprefs.Recipient{Ord: i, UserID: id, Channels: []string{"in_app"}})
	}
	return out, nil
}

func teamPtr(v int64) *int64 { return &v }

func meta() event.Meta {
	return event.Meta{
		Scope:      domain.TenantScope{TenantID: 1},
		ActorID:    7,
		TeamID:     teamPtr(3),
		OccurredAt: time.Unix(1_700_000_000, 0),
	}
}

func newUC() (*notificationuc.UseCase, *fakeWriter, *fakePrefs) {
	w, p := &fakeWriter{}, &fakePrefs{}
	return notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p}), w, p
}

// События, которым не соответствует ни один тип уведомления, до записи не доходят.
// Это фиксирует границу goal_changed из спеки §6.1.
func TestNonNotifyingEventsAreIgnored(t *testing.T) {
	uc, w, _ := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalShared{Meta: meta(), GoalID: 1, Title: "Цель"},
		event.GoalLinked{Meta: meta(), ChildGoalID: 1, Title: "Цель"},
		event.KRNoteUpdated{Meta: meta(), GoalID: 1, KRID: 2, KRTitle: "KR"},
		event.StatusChanged{Meta: meta(), TeamTitle: "Команда"},
		event.CommentReopened{Meta: meta(), GoalID: 1, CommentID: 2, GoalTitle: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 0 {
		t.Fatalf("эти события не должны порождать уведомлений, got %d", len(w.rows))
	}
}

// Правка цели и двух её KR одним автором в одном окне даёт ОДИН ключ схлопывания:
// ключ строится по цели, а не по KR (спека §6.1, §7.2).
func TestGoalAndItsKRsShareOneCoalesceKey(t *testing.T) {
	uc, w, _ := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
		event.KRFieldsChanged{Meta: meta(), GoalID: 10, KRID: 20, KRTitle: "KR-1"},
		event.KRDeleted{Meta: meta(), GoalID: 10, KRID: 21, KRTitle: "KR-2"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 3 {
		t.Fatalf("ожидались три вставки (схлопывает БД, не usecase), got %d", len(w.rows))
	}
	first := w.rows[0].CoalesceKey
	for _, r := range w.rows {
		if r.CoalesceKey != first {
			t.Fatalf("ключи разошлись: %q vs %q — ключ обязан строиться по цели", first, r.CoalesceKey)
		}
	}
}

// А вот kr_progress схлопывается по KR: два разных KR — два разных уведомления.
func TestProgressCoalescesPerKR(t *testing.T) {
	uc, w, _ := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.KRProgressUpdated{Meta: meta(), GoalID: 10, KRID: 20, KRTitle: "A", After: 50},
		event.KRProgressUpdated{Meta: meta(), GoalID: 10, KRID: 21, KRTitle: "B", After: 70},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if w.rows[0].CoalesceKey == w.rows[1].CoalesceKey {
		t.Fatal("прогресс разных KR не должен схлопываться в одно уведомление")
	}
}

// Адресный тип не ходит в резолв по дереву: получатель уже известен из события.
func TestAddressedTypeUsesAuthorFromEvent(t *testing.T) {
	uc, w, _ := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.CommentResolved{Meta: meta(), GoalID: 10, CommentID: 5, GoalTitle: "Цель", AuthorUserID: 99},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 1 || w.rows[0].UserID != 99 {
		t.Fatalf("уведомление должно уйти автору таски, got %+v", w.rows)
	}
	if w.rows[0].Type != notificationprefs.TypeMyCommentResolved {
		t.Errorf("тип: got %q", w.rows[0].Type)
	}
}

// Автор, решивший собственную таску, уведомления не получает.
func TestAuthorResolvingOwnCommentGetsNothing(t *testing.T) {
	uc, w, _ := newUC()
	m := meta()
	err := uc.Handle(context.Background(), []event.Event{
		event.CommentResolved{Meta: m, GoalID: 10, CommentID: 5, GoalTitle: "Цель", AuthorUserID: m.ActorID},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 0 {
		t.Fatal("собственное действие не должно порождать уведомление себе")
	}
}

// Резолв вызывается один раз на тип, а не на событие: иначе батч из 50 событий
// даст 50 рекурсивных запросов (правило 9 CLAUDE.md).
func TestResolveCalledOncePerTypeNotPerEvent(t *testing.T) {
	uc, _, p := newUC()
	evs := make([]event.Event, 0, 10)
	for i := 0; i < 10; i++ {
		evs = append(evs, event.GoalFieldsChanged{Meta: meta(), GoalID: int64(i), Title: "Цель"})
	}
	if err := uc.Handle(context.Background(), evs); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("резолв вызван %d раз на 10 событий одного типа, want 1", p.calls)
	}
}

// События без team_id адресовать некому: тихо пропускаем, а не падаем.
func TestEventWithoutTeamIsSkipped(t *testing.T) {
	uc, w, _ := newUC()
	m := meta()
	m.TeamID = nil
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m, GoalID: 1, Title: "Цель"},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 0 {
		t.Fatal("событие без команды нельзя заскоупить — уведомлений быть не должно")
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/usecase/notification/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 3: Реализовать отображение события в тип уведомления**

`internal/usecase/notification/mapping.go`:

```go
package notification

import (
	"okrs/internal/core/event"
	"okrs/internal/store/notificationprefs"
)

// notifyType maps an event onto the notification type it produces, or "" when the
// event produces none. This function IS the boundary described in spec §6.1 —
// widening it is a product decision, not a refactor.
func notifyType(ev event.Event) string {
	switch ev.(type) {
	case event.CommentAdded, event.ReplyAdded:
		return notificationprefs.TypeGoalComment

	case event.CommentResolved:
		return notificationprefs.TypeMyCommentResolved

	case event.GoalCreated, event.GoalCopied, event.GoalMoved, event.GoalDeleted,
		event.GoalFieldsChanged, event.GoalOwnerChanged,
		event.KRCreated, event.KRFieldsChanged, event.KRDeleted:
		return notificationprefs.TypeGoalChanged

	case event.KRProgressUpdated:
		return notificationprefs.TypeKRProgress
	}
	// Deliberately silent: goal_shared, goal_linked, kr_note_updated, status_changed,
	// comment_reopened and the deletions notify nobody (spec §6.1).
	return ""
}

// anchor is what a notification points at: the goal (or KR, for progress) plus the
// ids stored on the row.
type anchor struct {
	goalID    *int64
	krID      *int64
	commentID *int64
	title     string
	// addressee is set only for addressed types.
	addressee int64
}

// anchorOf extracts the anchor from an event. Every KR event carries GoalID, which
// is why a KR change can be addressed and coalesced as a change to its goal without
// an extra query.
func anchorOf(ev event.Event) anchor {
	id := func(v int64) *int64 { return &v }
	switch e := ev.(type) {
	case event.CommentAdded:
		return anchor{goalID: id(e.GoalID), commentID: id(e.CommentID), title: e.GoalTitle}
	case event.ReplyAdded:
		return anchor{goalID: id(e.GoalID), commentID: id(e.CommentID), title: e.GoalTitle}
	case event.CommentResolved:
		return anchor{goalID: id(e.GoalID), commentID: id(e.CommentID), title: e.GoalTitle, addressee: e.AuthorUserID}

	case event.GoalCreated:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalCopied:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalMoved:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalDeleted:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalFieldsChanged:
		return anchor{goalID: id(e.GoalID), title: e.Title}
	case event.GoalOwnerChanged:
		return anchor{goalID: id(e.GoalID), title: e.Title}

	case event.KRCreated:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRFieldsChanged:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRDeleted:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	case event.KRProgressUpdated:
		return anchor{goalID: id(e.GoalID), krID: id(e.KRID), title: e.KRTitle}
	}
	return anchor{}
}

// payloadOf carries only what the renderer needs, not the whole journal payload.
func payloadOf(ev event.Event) map[string]any {
	switch e := ev.(type) {
	case event.CommentAdded:
		return map[string]any{"text": e.Text}
	case event.ReplyAdded:
		return map[string]any{"text": e.Text}
	case event.KRProgressUpdated:
		return map[string]any{
			"before":     map[string]any{"progress": e.Before},
			"after":      map[string]any{"progress": e.After},
			"goal_title": e.GoalTitle,
		}
	}
	return map[string]any{}
}
```

- [ ] **Шаг 4: Реализовать fan-out**

`internal/usecase/notification/notification.go`:

```go
// Package notification turns domain events into per-recipient notifications.
//
// It is the bus subscriber registered for the 13 event types that notify anyone;
// the other 9 never reach it. Registered asynchronously — resolving recipients and
// inserting rows has no business holding up an HTTP response.
package notification

import (
	"context"
	"fmt"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/notifications"
)

// CoalesceWindow is how long repeats collapse into one notification. Fixed buckets,
// not a sliding window: a sliding one needs read-then-write and races between
// replicas, and the boundary artefact is the cheaper trade (spec §7.2).
const CoalesceWindow = 10 * time.Minute

// NotificationWriter and PrefResolver are consumer-side ports, per specs/010.
type NotificationWriter interface {
	CreateBatch(ctx context.Context, scope domain.TenantScope, ins []notifications.InsertInput) error
}

type PrefResolver interface {
	Resolve(ctx context.Context, scope domain.TenantScope, notifType string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error)
	ResolveAddressed(ctx context.Context, scope domain.TenantScope, notifType string, userIDs []int64) ([]notificationprefs.Recipient, error)
}

type Deps struct {
	Notifications NotificationWriter
	Prefs         PrefResolver
}

type UseCase struct {
	notifications NotificationWriter
	prefs         PrefResolver
}

func New(deps Deps) *UseCase {
	return &UseCase{notifications: deps.Notifications, prefs: deps.Prefs}
}

// pending is one event already classified, awaiting its recipients.
type pending struct {
	ev     event.Event
	anchor anchor
	typ    string
}

// Handle is the bus subscriber. It groups the batch by (tenant, notification type),
// resolves recipients once per group, and writes all rows in one batch.
//
// Батчевая операция: резолв и вставка идут на группу, не на событие — иначе батч из
// 50 событий даёт 50 рекурсивных запросов и 50 вставок (правило 9 CLAUDE.md).
func (u *UseCase) Handle(ctx context.Context, evs []event.Event) error {
	type groupKey struct {
		tenantID int64
		typ      string
	}
	groups := make(map[groupKey][]pending)

	for _, ev := range evs {
		typ := notifyType(ev)
		if typ == "" {
			continue
		}
		a := anchorOf(ev)
		m := ev.Context()
		if notificationprefs.IsAddressed(typ) {
			// Nobody is notified about their own action.
			if a.addressee == 0 || a.addressee == m.ActorID {
				continue
			}
		} else if m.TeamID == nil {
			// Without a team the event cannot be scoped to anyone.
			continue
		}
		k := groupKey{tenantID: m.Scope.TenantID, typ: typ}
		groups[k] = append(groups[k], pending{ev: ev, anchor: a, typ: typ})
	}

	for k, items := range groups {
		scope := domain.TenantScope{TenantID: k.tenantID}
		recipients, err := u.resolve(ctx, scope, k.typ, items)
		if err != nil {
			return err
		}
		rows := make([]notifications.InsertInput, 0, len(recipients))
		for _, rc := range recipients {
			p := items[rc.Ord]
			rows = append(rows, u.row(p, rc.UserID))
		}
		if err := u.notifications.CreateBatch(ctx, scope, rows); err != nil {
			return err
		}
	}
	return nil
}

// resolve picks the addressing strategy for the group's type: addressed types carry
// their recipient, scoped types walk the team tree.
func (u *UseCase) resolve(ctx context.Context, scope domain.TenantScope, typ string, items []pending) ([]notificationprefs.Recipient, error) {
	if notificationprefs.IsAddressed(typ) {
		userIDs := make([]int64, len(items))
		for i, p := range items {
			userIDs[i] = p.anchor.addressee
		}
		return u.prefs.ResolveAddressed(ctx, scope, typ, userIDs)
	}
	targets := make([]notificationprefs.Target, len(items))
	for i, p := range items {
		m := p.ev.Context()
		targets[i] = notificationprefs.Target{TeamID: *m.TeamID, ActorID: m.ActorID}
	}
	return u.prefs.Resolve(ctx, scope, typ, targets)
}

func (u *UseCase) row(p pending, userID int64) notifications.InsertInput {
	m := p.ev.Context()
	return notifications.InsertInput{
		UserID:      userID,
		Type:        p.typ,
		Kind:        string(p.ev.Kind()),
		ActorUserID: m.ActorID,
		TeamID:      m.TeamID,
		PeriodID:    m.PeriodID,
		GoalID:      p.anchor.goalID,
		KRID:        p.anchor.krID,
		CommentID:   p.anchor.commentID,
		EntityTitle: p.anchor.title,
		Payload:     payloadOf(p.ev),
		CoalesceKey: coalesceKey(p, m),
	}
}

// coalesceKey is type:entity:actor:bucket.
//
// The entity is the KR only for kr_progress; everything else keys on the goal, so a
// goal edited together with two of its KRs collapses into one "×3" notification.
func coalesceKey(p pending, m event.Meta) string {
	entity := "goal:0"
	if p.typ == notificationprefs.TypeKRProgress && p.anchor.krID != nil {
		entity = fmt.Sprintf("kr:%d", *p.anchor.krID)
	} else if p.anchor.goalID != nil {
		entity = fmt.Sprintf("goal:%d", *p.anchor.goalID)
	}
	at := m.OccurredAt
	if at.IsZero() {
		at = time.Now()
	}
	bucket := at.Unix() / int64(CoalesceWindow.Seconds())
	return fmt.Sprintf("%s:%s:%d:%d", p.typ, entity, m.ActorID, bucket)
}
```

- [ ] **Шаг 5: Прогнать тесты**

Запустить: `go test ./internal/usecase/notification/ -count=1 -v`
Ожидается: PASS, семь тестов.

- [ ] **Шаг 6: Проверить, что usecase не ходит в репозитории**

Пакеты `store/notifications` и `store/notificationprefs` импортируются здесь **только ради типов** (`notifications.InsertInput`, `notificationprefs.Target`, `Recipient`, константы `Type*`). Это разрешённый импорт-ради-типа — тот же приём, что в `usecase/export`, который импортирует `usecase/okrboard`, чтобы назвать тип в собственном порте. Признак, отличающий его от нарушения: в пакете нет ни одного обращения к методам репозитория, и подмена реализации в тесте не требует стора вовсе — что и доказывает `fakeWriter`/`fakePrefs` из шага 1.

Запустить: `rg -n 'Repository|pgxpool|db\.Query|db\.Exec' internal/usecase/notification/`
Ожидается: пустой вывод (`rg` вернёт код 1). Usecase обращается только к своим портам.

Проверять через `go list -deps ... | rg pgx` **нельзя**: `store/notifications` тянет pgx транзитивно ради собственной реализации, поэтому драйвер окажется в списке зависимостей и на корректном коде.

---

## Task 6: API ленты уведомлений

**Файлы:**
- Создать: `internal/http/dto/notification.go`
- Создать: `internal/http/handlers/api/v1/notifications/{handler.go,routes.go}`
- Создать: `internal/http/handlers/api/v1/notifications/unreadcount/{handler.go,routes.go}`
- Создать: `internal/http/handlers/api/v1/notifications/read/{handler.go,routes.go}`
- Тест: `internal/http/handlers/api/v1/notifications/routes_test.go`
- Изменить: `internal/http/server.go`, `internal/http/testdata/routes.golden`

**Интерфейсы:**
- Потребляет: `notificationsvc.Service` (задача 3), `notify.Render` (задача 4).
- Производит:
  - `dto.Notification`, `dto.NotificationList`, `dto.UnreadCount`
  - `notifications.New(svc NotificationReader) *Handler` с методом `Get`
  - `unreadcount.New(svc UnreadCounter) *Handler` с методом `Get`
  - `read.New(svc ReadMarker) *Handler` с методом `Post`
- Используют: задачи 8 и 9 (фронт).

- [ ] **Шаг 1: Написать DTO**

`internal/http/dto/notification.go`:

```go
package dto

// Notification is one bell entry. Title and Body are rendered server-side so the
// wording lives in one place and phase 2's messengers reuse it verbatim.
type Notification struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Count       int    `json:"count"`
	CreatedAt   string `json:"created_at"`
	Read        bool   `json:"read"`
	ActorName   string `json:"actor_name"`
	ActorAvatar string `json:"actor_avatar,omitempty"`
	// URL is where clicking the notification navigates. Empty when the target is gone.
	URL string `json:"url,omitempty"`
}

type NotificationList struct {
	Items      []Notification `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type UnreadCount struct {
	Count int `json:"count"`
}
```

- [ ] **Шаг 2: Написать падающий тест маршрутов и поведения**

`internal/http/handlers/api/v1/notifications/routes_test.go`:

```go
package notifications_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/notifications"
	storenotif "okrs/internal/store/notifications"
)

type fakeSvc struct{ items []storenotif.Notification }

// Сигнатура обязана совпадать с портом NotificationReader дословно, включая
// context.Context: иначе фейк не удовлетворит интерфейс и тест не соберётся.
func (f *fakeSvc) List(_ context.Context, _ domain.TenantScope, _ int64, _ storenotif.ListFilter) ([]storenotif.Notification, *storenotif.Cursor, error) {
	return f.items, nil, nil
}

// Заголовок и тело собираются на сервере: клиент не должен знать формулировок.
func TestGetRendersTitleAndBody(t *testing.T) {
	goalID := int64(5)
	svc := &fakeSvc{items: []storenotif.Notification{{
		ID: 1, Type: "goal_comment", Kind: "comment_added",
		ActorDisplayName: "Пётр", EntityTitle: "Снизить отток",
		Payload: map[string]any{"text": "Уточните метрику"},
		CoalesceCount: 1, CreatedAt: time.Now(), GoalID: &goalID,
	}}}

	r := chi.NewRouter()
	notifications.RegisterRoutes(r, notifications.New(svc))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	// В реальном запросе scope и user кладёт middleware; тест подставляет их сам —
	// хелпер описан в шаге 4.
	req = withScopeAndUser(req, 1, 42)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	var got struct {
		Items []struct {
			Title, Body, URL string
			Count            int
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("got %d items", len(got.Items))
	}
	if got.Items[0].Title == "" || got.Items[0].Body == "" {
		t.Errorf("сервер обязан отдавать готовый текст: %+v", got.Items[0])
	}
	if got.Items[0].URL == "" {
		t.Error("уведомление с целью обязано нести ссылку")
	}
}

// Без tenant-скоупа — 403, а не паника и не пустой список.
func TestGetWithoutScopeIsForbidden(t *testing.T) {
	r := chi.NewRouter()
	notifications.RegisterRoutes(r, notifications.New(&fakeSvc{}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", w.Code)
	}
}
```

- [ ] **Шаг 3: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/http/handlers/api/v1/notifications/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 4: Написать хелпер контекста для теста**

В том же тестовом файле, чтобы тест не зависел от middleware:

```go
// withScopeAndUser кладёт в запрос tenant-скоуп и пользователя так же, как это
// делает middleware chain. Хелпер нужен, потому что handler читает их из контекста,
// а поднимать весь роутер ради двух значений — избыточно.
func withScopeAndUser(r *http.Request, tenantID, userID int64) *http.Request {
	ctx := auth.ContextWithTenantScope(r.Context(), domain.TenantScope{TenantID: tenantID})
	ctx = auth.ContextWithUserID(ctx, userID)
	return r.WithContext(ctx)
}
```

Если экспортированных конструкторов контекста в `internal/auth` нет, добавить их рядом с существующими `TenantScopeFromContext`/`UserIDFromContext` — они нужны и другим тестам handler'ов, и это меньшее зло, чем поднимать полный middleware chain в юнит-тесте.

- [ ] **Шаг 5: Реализовать три обработчика**

`internal/http/handlers/api/v1/notifications/handler.go`:

```go
// Package notifications serves GET /api/v1/notifications — the bell feed.
package notifications

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/dto"
	"okrs/internal/render/notify"
	storenotif "okrs/internal/store/notifications"
)

// NotificationReader is the port this handler needs. *notification.Service satisfies it.
type NotificationReader interface {
	List(ctx context.Context, scope domain.TenantScope, userID int64, f storenotif.ListFilter) ([]storenotif.Notification, *storenotif.Cursor, error)
}

type Handler struct{ svc NotificationReader }

func New(svc NotificationReader) *Handler { return &Handler{svc: svc} }

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	userID := auth.UserIDFromContext(r.Context())

	f := storenotif.ListFilter{
		UnreadOnly: r.URL.Query().Get("unread") == "1",
		Limit:      20,
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			f.Limit = n
		}
	}
	if c := r.URL.Query().Get("cursor"); c != "" {
		cur, err := decodeCursor(c)
		if err != nil {
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid cursor", map[string]string{"cursor": "invalid"})
			return
		}
		f.Cursor = cur
	}

	items, next, err := h.svc.List(r.Context(), scope, userID, f)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load notifications", nil)
		return
	}

	out := dto.NotificationList{Items: make([]dto.Notification, 0, len(items))}
	for _, n := range items {
		out.Items = append(out.Items, toDTO(n))
	}
	if next != nil {
		out.NextCursor = encodeCursor(next)
	}
	v1.WriteJSON(w, http.StatusOK, out)
}

func toDTO(n storenotif.Notification) dto.Notification {
	actor := n.ActorDisplayName
	if n.ActorRemoved || actor == "" {
		// Former member: neutral placeholder, no name and no avatar.
		actor = "Бывший участник"
	}
	text := notify.Render(notify.Input{
		Kind:        event.Kind(n.Kind),
		ActorName:   actor,
		EntityTitle: n.EntityTitle,
		Count:       n.CoalesceCount,
		Payload:     n.Payload,
	})
	d := dto.Notification{
		ID: n.ID, Type: n.Type, Kind: n.Kind,
		Title: text.Title, Body: text.Body,
		Count:     n.CoalesceCount,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
		Read:      n.ReadAt != nil,
		ActorName: actor,
		URL:       targetURL(n),
	}
	if !n.ActorRemoved {
		d.ActorAvatar = n.ActorAvatarURL
	}
	return d
}

// targetURL builds the link the bell entry navigates to. Empty when there is no goal
// to open — the notification still renders, it just is not clickable.
func targetURL(n storenotif.Notification) string {
	if n.GoalID == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("/?goal_id=")
	b.WriteString(strconv.FormatInt(*n.GoalID, 10))
	if n.TeamID != nil {
		b.WriteString("&team_id=")
		b.WriteString(strconv.FormatInt(*n.TeamID, 10))
	}
	if n.PeriodID != nil {
		b.WriteString("&period_id=")
		b.WriteString(strconv.FormatInt(*n.PeriodID, 10))
	}
	return b.String()
}

// Cursor encoding keeps the keyset position opaque to the client, the same contract
// the activity feed uses.
func encodeCursor(c *storenotif.Cursor) string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(c.ID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (*storenotif.Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, strconv.ErrSyntax
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return &storenotif.Cursor{CreatedAt: at, ID: id}, nil
}
```

`internal/http/handlers/api/v1/notifications/routes.go`:

```go
package notifications

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/notifications", h.Get)
}
```

`internal/http/handlers/api/v1/notifications/unreadcount/handler.go`:

```go
// Package unreadcount serves GET /api/v1/notifications/unread-count — the bell badge.
package unreadcount

import (
	"context"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/dto"
)

// UnreadCounter is the port this handler needs. *notification.Service satisfies it.
type UnreadCounter interface {
	UnreadCount(ctx context.Context, scope domain.TenantScope, userID int64) (int, error)
}

type Handler struct{ svc UnreadCounter }

func New(svc UnreadCounter) *Handler { return &Handler{svc: svc} }

// Get is polled every 60s by the sidebar. Deliberately uncached server-side: it is a
// COUNT over a partial index for one user, and caching it across K8s replicas would
// buy staleness rather than speed.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	n, err := h.svc.UnreadCount(r.Context(), scope, auth.UserIDFromContext(r.Context()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to count notifications", nil)
		return
	}
	v1.WriteJSON(w, http.StatusOK, dto.UnreadCount{Count: n})
}
```

```go
package unreadcount

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/notifications/unread-count", h.Get)
}
```

`internal/http/handlers/api/v1/notifications/read/handler.go`:

```go
// Package read serves POST /api/v1/notifications/read — marking notifications read.
package read

import (
	"context"
	"encoding/json"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
)

// ReadMarker is the port this handler needs. *notification.Service satisfies it.
type ReadMarker interface {
	MarkRead(ctx context.Context, scope domain.TenantScope, userID int64, ids []int64, all bool) error
}

type Handler struct{ svc ReadMarker }

func New(svc ReadMarker) *Handler { return &Handler{svc: svc} }

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	if !req.All && len(req.IDs) == 0 {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "ids or all required",
			map[string]string{"ids": "required"})
		return
	}
	// The service scopes the update by user_id, so one user can never mark another's.
	if err := h.svc.MarkRead(r.Context(), scope, auth.UserIDFromContext(r.Context()), req.IDs, req.All); err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to mark read", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

```go
package read

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/api/v1/notifications/read", h.Post)
}
```

- [ ] **Шаг 6: Зарегистрировать маршруты**

`internal/http/server.go` — в группе, где уже монтируются tenant-scoped API под CSRF (рядом с `apime.RegisterRoutes`):

```go
			notifications.RegisterRoutes(r, notifications.New(s.deps.Notifications))
			unreadcount.RegisterRoutes(r, unreadcount.New(s.deps.Notifications))
			read.RegisterRoutes(r, read.New(s.deps.Notifications))
```

`POST /api/v1/notifications/read` обязан попасть в группу под CSRF-middleware (правило 7 в `specs/010`): он вызывается из браузера и меняет состояние.

- [ ] **Шаг 7: Разобраться с CRLF, затем обновить golden-тест маршрутов**

**Сначала прочитать:** [технический долг, п. 1.2](2026-08-27-notifications-tech-debt.md). Кратко: `internal/http/testdata/routes.golden` и `specs/070-code-structure.md` лежат в CRLF, а `TestRoutesGolden` и `TestSpecRouteTableMatchesRouter` сравнивают с LF-выводом роутера, поэтому **падают всегда** — в golden 142 строки при 142 фактических маршрутах, различий по существу нет. Это состояние дерева, а не регрессия.

Здесь оно перестаёт быть безобидным: задача добавляет маршруты, и `-update-routes` на CRLF-дереве либо перезапишет файл в LF (шумный дифф на 142 строки поверх трёх осмысленных), либо не даст ожидаемого результата.

Выбрать один путь и записать выбор в отчёт:

1. нормализовать `routes.golden` и `specs/070-code-structure.md` в LF **отдельно** от изменений этой задачи, чтобы шум не смешался с содержательным диффом;
2. добавить `.gitattributes` с `* text=auto eol=lf` — лечит причину, но затрагивает всё дерево и должно идти отдельной задачей;
3. научить оба теста игнорировать `\r` при сравнении — самое узкое изменение, не трогает данные.

Рекомендация: вариант 3 для этой задачи (минимальный радиус), вариант 2 — отдельной задачей позже.

Затем запустить: `go test ./internal/http -run RoutesGolden -update-routes && git diff --stat internal/http/testdata/routes.golden`
Ожидается: в golden добавились ровно три строки — `GET /api/v1/notifications`, `GET /api/v1/notifications/unread-count`, `POST /api/v1/notifications/read`. Больше ничего не изменилось. Если дифф показывает 142 изменённые строки — сработала перезапись окончаний строк, откатить и вернуться к выбору выше.

- [ ] **Шаг 8: Прогнать тесты**

Запустить: `go test ./internal/http/... -count=1`
Ожидается: PASS, включая golden.

---

## Task 7: API настроек уведомлений

**Файлы:**
- Создать: `internal/http/handlers/api/v1/notifications/preferences/{handler.go,routes.go}`
- Тест: `internal/http/handlers/api/v1/notifications/preferences/handler_test.go`
- Изменить: `internal/http/dto/notification.go`, `internal/http/server.go`, `internal/http/testdata/routes.golden`

**Интерфейсы:**
- Потребляет: `notificationprefsvc.Service` (задача 3).
- Производит: `dto.NotificationPreference`, `dto.NotificationPreferences`; `preferences.New(svc PrefService) *Handler` с методами `Get` и `Put`.
- Используют: задача 9 (экран настроек).

- [ ] **Шаг 1: Дополнить DTO**

В `internal/http/dto/notification.go`:

```go
// NotificationPreference is one row of the settings matrix.
// Scope is empty for addressed types, where it does not apply.
type NotificationPreference struct {
	Type     string   `json:"type"`
	Enabled  bool     `json:"enabled"`
	Scope    string   `json:"scope,omitempty"`
	Channels []string `json:"channels"`
	// Addressed marks a type that has no scope selector, so the UI renders a dash
	// instead of a dropdown without hardcoding the type name.
	Addressed bool `json:"addressed"`
}

type NotificationPreferences struct {
	Items []NotificationPreference `json:"items"`
	// Channels available in this tenant. Phase 1b always returns ["in_app"]; the UI
	// shows channel columns only when there is more than one.
	Channels []string `json:"channels"`
}
```

- [ ] **Шаг 2: Написать падающий тест**

`internal/http/handlers/api/v1/notifications/preferences/handler_test.go`:

```go
package preferences_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"okrs/internal/http/handlers/api/v1/notifications/preferences"
)

// GET обязан вернуть все четыре типа, даже если пользователь ничего не настраивал:
// иначе экран настроек у нового пользователя будет пустым.
func TestGetReturnsAllFourTypes(t *testing.T) {
	r := chi.NewRouter()
	preferences.RegisterRoutes(r, preferences.New(newFakeSvc()))

	req := withScopeAndUser(httptest.NewRequest(http.MethodGet, "/api/v1/notifications/preferences", nil), 1, 42)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Items    []struct{ Type, Scope string; Addressed bool } `json:"items"`
		Channels []string                                       `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got.Items) != 4 {
		t.Fatalf("got %d типов, want 4", len(got.Items))
	}
	// В фазе 1b канал ровно один — фронт по этому признаку скрывает колонки каналов.
	if len(got.Channels) != 1 || got.Channels[0] != "in_app" {
		t.Fatalf("channels: %v, want [in_app]", got.Channels)
	}
	for _, it := range got.Items {
		if it.Type == "my_comment_resolved" {
			if !it.Addressed {
				t.Error("адресный тип должен быть помечен addressed")
			}
			if it.Scope != "" {
				t.Errorf("у адресного типа скоуп неприменим, got %q", it.Scope)
			}
		}
	}
}

// Невалидный тип — 400 с полем в details, а не 500.
func TestPutRejectsUnknownType(t *testing.T) {
	r := chi.NewRouter()
	preferences.RegisterRoutes(r, preferences.New(newFakeSvc()))

	body := strings.NewReader(`{"items":[{"type":"made_up","enabled":true,"scope":"own","channels":["in_app"]}]}`)
	req := withScopeAndUser(httptest.NewRequest(http.MethodPut, "/api/v1/notifications/preferences", body), 1, 42)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", w.Code, w.Body.String())
	}
}
```

Фейк сервиса и `withScopeAndUser` — те же по форме, что в задаче 6; написать их в этом же тестовом файле (копия из четырёх строк честнее общего тест-пакета: иначе два handler-теста свяжутся через общий тип).

- [ ] **Шаг 3: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/http/handlers/api/v1/notifications/preferences/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 4: Реализовать обработчик**

`internal/http/handlers/api/v1/notifications/preferences/handler.go`:

```go
// Package preferences serves GET/PUT /api/v1/notifications/preferences.
package preferences

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	v1 "okrs/internal/http/handlers/api/v1"
	"okrs/internal/http/dto"
	notificationprefsvc "okrs/internal/service/notificationpref"
	"okrs/internal/store/notificationprefs"
)

// PrefService is the port this handler needs. *notificationpref.Service satisfies it.
type PrefService interface {
	GetAll(ctx context.Context, scope domain.TenantScope, userID int64) ([]notificationprefs.Preference, error)
	Set(ctx context.Context, scope domain.TenantScope, userID int64, p notificationprefs.Preference) error
}

type Handler struct{ svc PrefService }

func New(svc PrefService) *Handler { return &Handler{svc: svc} }

// availableChannels are the channels this build can deliver to. Phase 1b has only
// in-app; phase 2 replaces this with the tenant's entitled channel list.
var availableChannels = []string{"in_app"}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v1.SetAPICacheControl(w)
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	prefs, err := h.svc.GetAll(r.Context(), scope, auth.UserIDFromContext(r.Context()))
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to load preferences", nil)
		return
	}
	out := dto.NotificationPreferences{
		Items:    make([]dto.NotificationPreference, 0, len(prefs)),
		Channels: availableChannels,
	}
	for _, p := range prefs {
		out.Items = append(out.Items, dto.NotificationPreference{
			Type: p.Type, Enabled: p.Enabled, Scope: p.Scope, Channels: p.Channels,
			Addressed: notificationprefs.IsAddressed(p.Type),
		})
	}
	v1.WriteJSON(w, http.StatusOK, out)
}

// Put replaces the caller's preferences. Whole-matrix replace rather than per-row
// patch: the settings screen edits the matrix as one form.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		v1.WriteError(w, http.StatusForbidden, "FORBIDDEN", "forbidden", nil)
		return
	}
	var req struct {
		Items []dto.NotificationPreference `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid payload", nil)
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	for _, it := range req.Items {
		err := h.svc.Set(r.Context(), scope, userID, notificationprefs.Preference{
			Type: it.Type, Enabled: it.Enabled, Scope: it.Scope, Channels: it.Channels,
		})
		switch {
		case errors.Is(err, notificationprefsvc.ErrInvalidType):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown notification type",
				map[string]string{"type": "invalid"})
			return
		case errors.Is(err, notificationprefsvc.ErrInvalidScope):
			v1.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown scope",
				map[string]string{"scope": "invalid"})
			return
		case err != nil:
			v1.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to save preferences", nil)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Цикл по `req.Items` делает до четырёх `Set` — это не N+1: типов ровно четыре, они фиксированы `CHECK`-ограничением, и рост здесь невозможен. Заводить батчевый upsert ради четырёх строк — преждевременная оптимизация.

`internal/http/handlers/api/v1/notifications/preferences/routes.go`:

```go
package preferences

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/notifications/preferences", h.Get)
	r.Put("/api/v1/notifications/preferences", h.Put)
}
```

- [ ] **Шаг 5: Зарегистрировать маршруты и обновить golden**

`internal/http/server.go`, в той же группе под CSRF:

```go
			preferences.RegisterRoutes(r, preferences.New(s.deps.NotificationPrefs))
```

Запустить: `go test ./internal/http -run RoutesGolden -update-routes && git diff internal/http/testdata/routes.golden`
Ожидается: добавились ровно две строки — `GET` и `PUT /api/v1/notifications/preferences`.

- [ ] **Шаг 6: Прогнать тесты**

Запустить: `go test ./internal/http/... -count=1 -v`
Ожидается: PASS.

---

## Task 8: Колокольчик в сайдбаре

Колокольчик живёт внутри `Sidebar`, а не передаётся хостом, — поэтому появляется на всех SPA-страницах разом. Модуль `sidebar.js` уже самодостаточен (сам ходит в `/api/v1/config` и `/api/v1/session/tenants`), так что запрос счётчика туда ложится без новых зависимостей.

**Файлы:**
- Создать: `web/static/notifications.js`
- Изменить: `web/static/sidebar.js`, `web/static/sidebar.css`
- Изменить: `web/static/tracker.js` (снятие HCI-колокольчика)
- Изменить: `web/templates/tracker_shell.html`, `admin_shell.html`, `settings_shell.html`, `system_shell.html`, `stub_shell.html`, `no_membership.html`, `activity_shell.html`, `goal_tree_shell.html`, `period_overview_shell.html`

**Интерфейсы:**
- Потребляет: `GET /api/v1/notifications`, `GET /api/v1/notifications/unread-count`, `POST /api/v1/notifications/read` (задача 6).
- Производит глобальные компоненты: `NotificationList({items, onRead})`, `NotificationsPanel({open, onClose})`, `NotificationsBell()`.

- [ ] **Шаг 1: Написать модуль уведомлений**

`web/static/notifications.js`:

```jsx
// NotificationList / NotificationsPanel / NotificationsBell — колокольчик уведомлений.
// Подключается ПЕРЕД sidebar.js во всех SPA-shell: сайдбар рендерит колокольчик сам,
// не получая его пропом, поэтому уведомления доступны на каждой странице.
//
// Использует React.useState/useEffect (не top-level деструктуризацию), чтобы не
// конфликтовать с app-скриптами, делящими ту же глобальную область.

// Опрос счётчика раз в минуту. Кэша на сервере нет намеренно: это COUNT по
// частичному индексу для одного пользователя, а кэш в памяти инстанса дал бы
// разные числа на разных репликах K8S.
const NOTIF_POLL_MS = 60000;

function _notifCSRF() {
  const m = document.cookie.match(/(?:^|;\s*)okr_csrf_token=([^;]*)/);
  return m ? decodeURIComponent(m[1]) : '';
}

// Относительное время: «5 мин назад». Абсолютная дата остаётся в title.
function _notifAgo(iso) {
  const then = new Date(iso).getTime();
  if (!then) return '';
  const mins = Math.floor((Date.now() - then) / 60000);
  if (mins < 1) return 'только что';
  if (mins < 60) return mins + ' мин назад';
  const hours = Math.floor(mins / 60);
  if (hours < 24) return hours + ' ч назад';
  const days = Math.floor(hours / 24);
  if (days < 30) return days + ' дн назад';
  return new Date(iso).toLocaleDateString('ru-RU');
}

// NotificationList — список записей. Вынесен отдельно от панели, чтобы его можно
// было переиспользовать на отдельной странице уведомлений, не переписывая.
function NotificationList({ items, onRead }) {
  if (!items.length) {
    return (
      <div className="notif__empty">
        <span className="notif__empty-icon">🔕</span>
        <span>Пока нет уведомлений</span>
      </div>
    );
  }
  return (
    <div className="notif__list">
      {items.map(n => (
        <a
          key={n.id}
          className={`notif__item${n.read ? '' : ' notif__item--unread'}`}
          href={n.url || undefined}
          onClick={() => onRead(n.id)}
        >
          <div className="notif__item-head">
            {/* Текст пришёл с сервера и рендерится как текст: никакого
                dangerouslySetInnerHTML — правило 8 в specs/010. */}
            <span className="notif__item-title">{n.title}</span>
            <span className="notif__item-time" title={n.created_at}>{_notifAgo(n.created_at)}</span>
          </div>
          <div className="notif__item-body">{n.body}</div>
        </a>
      ))}
    </div>
  );
}

// NotificationsPanel — выпадающая панель. Данные грузит при открытии, а не при
// монтировании: на большинстве заходов панель не открывают вовсе.
function NotificationsPanel({ open, onClose, onChanged }) {
  const [items, setItems] = React.useState([]);
  const [loading, setLoading] = React.useState(false);

  React.useEffect(() => {
    if (!open) return;
    setLoading(true);
    fetch('/api/v1/notifications?limit=30', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(d => setItems((d && d.items) || []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [open]);

  React.useEffect(() => {
    if (!open) return;
    const onKey = e => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  const markRead = (ids, all) => {
    fetch('/api/v1/notifications/read', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': _notifCSRF() },
      body: JSON.stringify(all ? { all: true } : { ids }),
    }).then(r => {
      if (!r.ok) return;
      setItems(prev => prev.map(n => (all || ids.includes(n.id) ? { ...n, read: true } : n)));
      onChanged();
    });
  };

  if (!open) return null;
  const hasUnread = items.some(n => !n.read);
  return (
    <>
      <div className="notif__backdrop" onClick={onClose} />
      <div className="notif__panel">
        <div className="notif__panel-head">
          <span className="notif__panel-title">Уведомления</span>
          {hasUnread && (
            <button className="notif__mark-all" onClick={() => markRead(null, true)}>
              Отметить все прочитанными
            </button>
          )}
          <button className="notif__close" onClick={onClose} aria-label="Закрыть">✕</button>
        </div>
        {loading
          ? <div className="notif__empty">Загрузка…</div>
          : <NotificationList items={items} onRead={id => markRead([id], false)} />}
      </div>
    </>
  );
}

// NotificationsBell — иконка с бейджем плюс панель. Рендерится сайдбаром на всех
// страницах; счётчик опрашивается раз в минуту и при возврате фокуса на вкладку.
function NotificationsBell() {
  const [count, setCount] = React.useState(0);
  const [open, setOpen] = React.useState(false);

  const refresh = React.useCallback(() => {
    fetch('/api/v1/notifications/unread-count', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(d => { if (d) setCount(d.count || 0); })
      .catch(() => {});
  }, []);

  React.useEffect(() => {
    refresh();
    const timer = setInterval(refresh, NOTIF_POLL_MS);
    const onFocus = () => refresh();
    window.addEventListener('focus', onFocus);
    return () => { clearInterval(timer); window.removeEventListener('focus', onFocus); };
  }, [refresh]);

  return (
    <>
      <button className="sidebar__bell" onClick={() => setOpen(o => !o)} aria-label="Уведомления">
        <span className="sidebar__bell-icon">🔔</span>
        <span className={`sidebar__bell-badge${count === 0 ? ' sidebar__bell-badge--zero' : ''}`}>{count}</span>
      </button>
      <NotificationsPanel open={open} onClose={() => setOpen(false)} onChanged={refresh} />
    </>
  );
}
```

- [ ] **Шаг 2: Перевести сайдбар на собственный колокольчик**

В `web/static/sidebar.js`:

1. Удалить компонент `SidebarBell` (строки 53–61) — он больше не нужен как отдельный переиспользуемый примитив: колокольчик остался один, и его разметка переехала в `NotificationsBell`. Классы `.sidebar__bell*` при этом сохраняются, поэтому стили не трогаются.
2. В `SidebarTenant` убрать проп `bell` и рендерить колокольчик самостоятельно:

```jsx
function SidebarTenant({ user }) {
  // …без изменений…
      {/* Колокольчик уведомлений живёт здесь, а не приходит пропом: так он
          появляется на всех SPA-страницах разом, а не только на трекере. */}
      <NotificationsBell />
  // …
}
```

3. В `Sidebar` убрать проброс `bell` в `SidebarTenant`; проп `bell` удалить из сигнатуры компонента.

- [ ] **Шаг 3: Снять колокольчик Health Check-in с трекера**

В `web/static/tracker.js`:

1. убрать проп `bell={…}` из `<Sidebar>` (строки 3086–3088);
2. удалить состояние `hciData`/`hciOpen` (строки 2806–2807), эффект загрузки health check-in (около 2818) и эффект пометки просмотренных (2824–2825);
3. удалить рендер `<HealthCheckInPanel …>` (3195–3198);
4. удалить объявления `HealthCheckInPanel`, `HealthCheckInButton` (2607), `hciSeenKey`, `hciUnseenResolved`, `hciMarkResolvedSeen`.

`HealthCheckInButton` уже сейчас нигде не используется — это мёртвый код, и его удаление не теряет функциональности. Панель же становилась бы недостижимой: по решению заказчика вход в Health Check-in временно убирается совсем, новое место для него — отдельная задача (спека §13.2). Удаляем весь блок целиком, а не оставляем недостижимым: непроверяемый код гниёт быстрее, чем восстанавливается из истории git.

Запрос health check-in при загрузке трекера уходит вместе с колокольчиком — показывать эти данные больше некому.

- [ ] **Шаг 4: Подключить модуль во всех shell-шаблонах**

В каждом shell, где подключён `sidebar.js`, добавить строку **перед** ним:

```html
<script type="text/babel" src="/static/notifications.js" data-presets="react"></script>
```

Порядок важен: `sidebar.js` вызывает `NotificationsBell`, а babel-модули выполняются в порядке подключения.

Запустить: `rg -l 'sidebar.js' web/templates/`
Ожидается: список шаблонов; в каждом из них должна появиться строка с `notifications.js`. Проверить, что списки совпадают: `rg -l 'notifications.js' web/templates/`.

- [ ] **Шаг 5: Добавить стили панели**

В `web/static/sidebar.css` — стили `.notif__*` (панель, список, элемент, бейдж непрочитанного, пустое состояние, backdrop). Существующие `.sidebar__bell*` не менять: их использует новый колокольчик.

- [ ] **Шаг 6: Проверить, что шаблоны собираются**

Запустить: `go test ./internal/http -run Templates -count=1`
Ожидается: PASS. Тест шаблонов ловит опечатку в имени партиала на этапе сборки.

- [ ] **Шаг 7: Ручная проверка**

Поднять `docker compose up`, открыть трекер, админку и настройки.
Ожидается: на каждой странице в шапке сайдбара один колокольчик с бейджем; колокольчика Health Check-in нет нигде; клик открывает панель; клик по уведомлению помечает его прочитанным и ведёт на цель; «Отметить все прочитанными» обнуляет бейдж.

---

## Task 9: Экран настроек уведомлений

**Файлы:**
- Изменить: `web/static/settings.js`, `web/static/settings.css`

**Интерфейсы:**
- Потребляет: `GET`/`PUT /api/v1/notifications/preferences` (задача 7).
- Производит: секцию `notifications` в `SECTION_META` и компонент `NotificationsSettings`.

- [ ] **Шаг 1: Добавить секцию в навигацию**

В `web/static/settings.js`:

```jsx
const SECTION_META = {
  descriptions: { label: 'Описание команд', hint: 'Только команды, где вы лид', icon: '📝' },
  notifications: { label: 'Уведомления', hint: 'Что и о чём присылать', icon: '🔔' },
  sidebar: { label: 'Мой сайдбар', hint: 'Какие узлы показывать', icon: '☰' },
  spaces: { label: 'Мои пространства', hint: 'Тенанты и заявки', icon: '🏢' },
};
```

и в список доступных секций:

```jsx
  const sections = useMemo(
    () => [...(isLead ? ['descriptions'] : []), 'notifications', 'sidebar', 'spaces'],
    [isLead],
  );
```

Секция доступна всем, а не только лидам: адресный тип «решён мой комментарий» приходит любому, кто оставляет комментарии, независимо от того, руководит ли он командой.

- [ ] **Шаг 2: Написать компонент**

```jsx
// Подписи типов уведомлений. Тексты живут на клиенте: сервер отдаёт ключи, а не
// готовые ярлыки, чтобы не смешивать контракт API с языком интерфейса.
const NOTIF_TYPE_LABELS = {
  goal_comment: { label: 'Комментарий к цели', hint: 'Кто-то оставил комментарий или ответил' },
  my_comment_resolved: { label: 'Решён мой комментарий', hint: 'Приходит всегда, независимо от охвата' },
  goal_changed: { label: 'Изменение в цели', hint: 'Правки цели и её ключевых результатов, создание и удаление' },
  kr_progress: { label: 'Обновление прогресса KR', hint: 'Изменился процент выполнения' },
};

const NOTIF_SCOPES = [
  { value: 'own', label: 'Только мои команды' },
  { value: 'own_and_children', label: 'Мои команды и уровень ниже' },
  { value: 'subtree', label: 'Мои команды и всё поддерево' },
];

function NotificationsSettings() {
  const [items, setItems] = useState([]);
  const [channels, setChannels] = useState([]);
  const [status, setStatus] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    (async () => {
      const data = await apiGet('/api/v1/notifications/preferences');
      setItems(data?.items || []);
      setChannels(data?.channels || []);
      setLoading(false);
    })();
  }, []);

  // Колонки каналов показываются только когда каналов больше одного. В фазе 1b
  // канал ровно один (in-app), поэтому колонок нет и экран остаётся простым.
  const showChannels = channels.length > 1;

  const patch = (type, changes) =>
    setItems(prev => prev.map(it => (it.type === type ? { ...it, ...changes } : it)));

  const save = async () => {
    setStatus('');
    const res = await apiPut('/api/v1/notifications/preferences', { items });
    setStatus(res?.ok ? 'Сохранено' : 'Не удалось сохранить');
  };

  if (loading) return <div className="settings__loading">Загрузка…</div>;

  return (
    <div className="settings__section">
      <h2 className="settings__title">Уведомления</h2>
      <p className="settings__hint">
        Уведомления приходят по целям команд, где вы руководитель. Охват задаёт,
        насколько глубоко вниз по структуре смотреть.
      </p>

      <table className="notif-prefs">
        <thead>
          <tr>
            <th>Тип</th>
            <th>Присылать</th>
            <th>Охват</th>
            {showChannels && channels.map(c => <th key={c}>{c}</th>)}
          </tr>
        </thead>
        <tbody>
          {items.map(it => {
            const meta = NOTIF_TYPE_LABELS[it.type] || { label: it.type, hint: '' };
            return (
              <tr key={it.type}>
                <td>
                  <div className="notif-prefs__label">{meta.label}</div>
                  <div className="notif-prefs__hint">{meta.hint}</div>
                </td>
                <td>
                  <input
                    type="checkbox"
                    checked={it.enabled}
                    onChange={e => patch(it.type, { enabled: e.target.checked })}
                  />
                </td>
                <td>
                  {/* У адресного типа охват неприменим: получатель — конкретный
                      человек, а не срез структуры. Признак приходит с сервера,
                      чтобы клиент не хардкодил имя типа. */}
                  {it.addressed ? (
                    <span className="notif-prefs__na">—</span>
                  ) : (
                    <select
                      value={it.scope || 'own'}
                      disabled={!it.enabled}
                      onChange={e => patch(it.type, { scope: e.target.value })}
                    >
                      {NOTIF_SCOPES.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
                    </select>
                  )}
                </td>
                {showChannels && channels.map(c => (
                  <td key={c}>
                    <input
                      type="checkbox"
                      checked={(it.channels || []).includes(c)}
                      disabled={!it.enabled}
                      onChange={e => patch(it.type, {
                        channels: e.target.checked
                          ? [...(it.channels || []), c]
                          : (it.channels || []).filter(x => x !== c),
                      })}
                    />
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>

      <div className="settings__actions">
        <button className="btn" onClick={save}>Сохранить</button>
        {status && <span className={status === 'Сохранено' ? 'settings__ok' : 'settings__err'}>{status}</span>}
      </div>
    </div>
  );
}
```

и в рендер секций:

```jsx
      {active === 'notifications' && <NotificationsSettings />}
```

- [ ] **Шаг 2а: Проверить наличие `apiPut`**

Запустить: `rg -n 'function apiPut|const apiPut' web/static/settings.js web/static/api.js`
Ожидается: функция найдена. Если её нет — добавить в `web/static/api.js` рядом с существующими хелперами, используя общий `csrfHeaders` (единый CSRF-слой, `specs/010`), а не собственное чтение cookie.

- [ ] **Шаг 3: Добавить стили**

В `web/static/settings.css` — `.notif-prefs`, `.notif-prefs__label`, `.notif-prefs__hint`, `.notif-prefs__na`. Таблица должна оставаться читаемой при трёх колонках (фаза 1b) и при шести (фаза 2 с каналами).

- [ ] **Шаг 4: Ручная проверка**

Открыть `/settings?section=notifications`.
Ожидается: четыре строки; у «Решён мой комментарий» вместо селектора охвата прочерк; колонок каналов нет; выключение типа гасит селектор охвата; «Сохранить» показывает «Сохранено»; после перезагрузки страницы значения сохранились.

- [ ] **Шаг 5: Проверить сквозной сценарий**

Двумя пользователями в демо-данных: один — лид команды, второй — участник. От имени второго изменить цель в команде первого.
Ожидается: у первого бейдж колокольчика вырос, в панели появилось уведомление «… изменил цель». Если выставить у первого охват «только мои команды», а изменить цель в дочерней команде — уведомления быть не должно.

---

## Task 10: Сборка, ретенция, seed и спеки

Последняя задача связывает написанное в работающее приложение и приводит документацию в соответствие.

**Файлы:**
- Изменить: `internal/http/httpdeps/httpdeps.go`, `internal/scheduler/scheduler.go`, `app/app.go`
- Изменить: `seed_demo.sql`
- Изменить: `specs/010-architecture-constraints.md`, `020-domain-model.md`, `030-user-flows.md`, `040-api-contract.md`, `050-permissions-and-lifecycle.md`, `070-code-structure.md`, `README.md`

**Интерфейсы:**
- Потребляет: всё из задач 1–9.
- Производит: зарегистрированного подписчика уведомлений на шине и суточную петлю ретенции.

- [ ] **Шаг 1: Собрать граф зависимостей**

`internal/http/httpdeps/httpdeps.go` — добавить сервисы, usecase и подписку:

```go
	notifications := notificationsvc.New(st.Notifications)
	notificationPrefs := notificationprefsvc.New(st.NotificationPrefs)

	notificationUC := notificationuc.New(notificationuc.Deps{
		Notifications: notifications,
		Prefs:         notificationPrefs,
	})

	// Асинхронно: резолв получателей и вставка строк не должны задерживать
	// HTTP-ответ. Журнал, наоборот, подписан синхронно (фаза 1a).
	eventbus.SubscribeAll(bus, "notifications", notificationUC.Handle, eventbus.WithMode(eventbus.Async))
```

и в `Deps`:

```go
	Notifications     *notificationsvc.Service
	NotificationPrefs *notificationprefsvc.Service
```

`notificationUC.Handle` подписывается через `SubscribeAll`, а не 13 раз через `Subscribe`: отбор типов уже сделан внутри — функция `notifyType` возвращает `""` для остальных девяти. Один источник правды про границу лучше, чем список из 13 регистраций, который разъедется с `notifyType` при первом же изменении.

- [ ] **Шаг 1а: Добавить graceful shutdown в `cmd/server/main.go`**

Долг из фазы 1a ([п. 1.1](2026-08-27-notifications-tech-debt.md)), который здесь перестаёт быть теоретическим. Сейчас `App.Close`, а значит и `bus.Close`, **не выполняются никогда**: `main` не ловит сигналы, `http.ListenAndServe` либо блокируется навсегда, либо возвращает ошибку в `os.Exit(1)`, а тот пропускает `defer`.

В фазе 1a терять было нечего — единственный подписчик синхронный. Здесь появляется асинхронный подписчик уведомлений с буфером, и его дренаж при остановке становится настоящим: без этого шага события, лежащие в канале в момент SIGTERM, теряются молча.

```go
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: addr, Handler: a.Handler}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			stop()
		}
	}()
	<-ctx.Done()

	// Порядок остановки существенен: сначала перестаём принимать запросы, затем
	// дренируем шину (иначе подписчик получит события уже после закрытия пула),
	// и только потом закрываем пул БД.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = a.Close(5 * time.Second)
```

Существующий `defer pool.Close()` при этом впервые начинает работать — он страдал от той же дыры.

Запустить: `go build ./... && go vet ./cmd/...`
Ожидается: чисто. Проверка вручную: запустить сервер, послать `SIGTERM`, убедиться, что процесс завершается сам, а не по таймауту оболочки.

- [ ] **Шаг 2: Добавить петлю ретенции**

`internal/scheduler/scheduler.go`:

```go
// notificationRetentionLockKey is a fixed advisory-lock key so only one replica runs
// the daily purge (deleting twice is harmless, but the scan is not free).
const notificationRetentionLockKey = 918273646

const (
	notificationRetentionInterval = 24 * time.Hour
	// Read notifications survive 90 days, anything at all survives 180. Constants
	// until there is evidence someone needs different — see spec §7.6.
	notificationReadRetentionDays = 90
	notificationAnyRetentionDays  = 180
)

// NotificationPurger removes notifications past retention. Narrow port so the
// scheduler does not depend on the whole notification service.
type NotificationPurger interface {
	Purge(ctx context.Context, readDays, anyDays int) (int64, error)
}
```

Поле `Notifications NotificationPurger` добавляется в `Deps`, петля запускается из `Start` рядом с `startProgressSnapshotLoop`, по образцу существующей: тикер, попытка взять advisory-lock, вызов `Purge`, логирование числа удалённых строк.

Ключ блокировки берётся отличный от `progressSnapshotLockKey` (918273645) — совпадение заставило бы две независимые задачи ждать друг друга.

- [ ] **Шаг 3: Обновить seed**

`seed_demo.sql` — по правилу 7 `CLAUDE.md` демо должно соответствовать структуре таблиц. Добавить в конец:

```sql
-- Демо-настройки уведомлений: один пользователь смотрит всё поддерево,
-- остальные остаются на дефолте (строк нет — дефолт подставляется на чтении).
INSERT INTO notification_preferences (tenant_id, user_id, type, enabled, scope, channels)
SELECT 1, u.id, 'goal_changed', TRUE, 'subtree', '{in_app}'
  FROM users u WHERE u.provider_subject_key = 'system:anonymous-local'
ON CONFLICT DO NOTHING;

-- Пара уведомлений, чтобы колокольчик в демо не был пустым.
INSERT INTO notifications
  (tenant_id, user_id, type, kind, actor_user_id, team_id, period_id, goal_id,
   entity_title, payload_json, coalesce_key, coalesce_count)
SELECT 1, 1, 'goal_changed', 'goal_fields_changed', 2, g.team_id, g.period_id, g.id,
       g.title, '{}'::jsonb, 'demo:goal:' || g.id, 1
  FROM goals g WHERE g.tenant_id = 1 ORDER BY g.id LIMIT 2
ON CONFLICT DO NOTHING;
```

Запустить: `go test ./internal/store -run Seed -count=1`
Ожидается: PASS. Тест сида ловит рассинхрон со схемой.

- [ ] **Шаг 4: Прогнать всё**

Запустить: `go build ./... && go vet ./... && go test ./... -count=1`
Ожидается: PASS.

- [ ] **Шаг 5: Обновить спеки**

Правки по §15 дизайн-дока, в объёме фазы 1b:

- **`010-architecture-constraints.md`** — в раздел «Слои» добавить `render/notify` и `usecase/notification`; в таблицу репозиториев — `Notifications` и `NotificationPrefs`; в описание `sidebar.js` (строка 12) добавить `notifications.js` к перечню общих модулей и переписать упоминание `SidebarBell` — теперь это колокольчик уведомлений, доступный на всех страницах, а не слот, заполняемый трекером.
- **`020-domain-model.md`** — сущности `Notification` и `NotificationPreference` с инвариантами: отсутствие строки настроек = дефолт; ключ схлопывания и фиксированное окно; повтор в окне снова помечает непрочитанным; ссылки на сущности — не FK. Заодно дописать пропущенный `kr_note_updated` в перечень action (mismatch №1 из дизайн-дока).
- **`030-user-flows.md`** — флоу «настроить уведомления»; строка 169 («Справа от названия — колокольчик Health Check-in, только на трекере») переписывается под уведомления на всех страницах; строка 270 уточняется.
- **`040-api-contract.md`** — пять маршрутов: `GET /api/v1/notifications`, `GET /api/v1/notifications/unread-count`, `POST /api/v1/notifications/read`, `GET`/`PUT /api/v1/notifications/preferences`.
- **`050-permissions-and-lifecycle.md`** — свои уведомления и настройки читает и меняет только сам пользователь; серверная проверка идёт по `user_id`, а не по телу запроса.
- **`070-code-structure.md`** — карта «URI → пакет обработчика» для четырёх новых пакетов.
- **`README.md`** — секция API.

- [ ] **Шаг 6: Проверить, что спеки и код сошлись**

Запустить: `go test ./internal/http -run SpecRoutes -count=1`
Ожидается: PASS. Тест сверяет маршруты с `040-api-contract.md`; если он падает — спека и код разошлись, и это надо чинить здесь, а не потом.

---

## Приёмка фазы 1b

- [ ] **Шаг 1: Полный прогон**

Запустить: `go build ./... && go vet ./... && go test ./... -count=1`
Ожидается: PASS. Тесты с БД требуют Docker; без него они скипаются, и тогда приёмка не пройдена.

- [ ] **Шаг 2: Проверить отсутствие N+1 на горячем пути**

Запустить: `rg -n 'Батчевая операция' internal/store/notifications/ internal/store/notificationprefs/ internal/service/notification/ internal/service/notificationpref/ internal/usecase/notification/`
Ожидается: не меньше пяти помеченных методов — `InsertBatch`, `ResolveRecipients`, `ResolveAddressed`, `CreateBatch`, `Handle`. Пометка не украшение: она объясняет следующему, почему нельзя «упростить» метод в цикл.

- [ ] **Шаг 3: Проверить границу уведомлений**

Запустить: `go test ./internal/usecase/notification/ -run TestNonNotifyingEvents -v`
Ожидается: PASS. Это исполняемая версия §6.1 спеки: шаринг, связи, заметка к KR, статус и переоткрытие комментария уведомлений не порождают.

- [ ] **Шаг 4: Сквозной сценарий вручную**

`docker compose up`, два пользователя из демо-данных.

1. Пользователь A — лид команды, охват «только мои команды». Пользователь B меняет цель в этой команде → у A растёт бейдж, в панели уведомление «B изменил цель».
2. B меняет цель в дочерней команде → уведомления нет.
3. A переключает охват на «мои команды и уровень ниже», B повторяет → уведомление появилось.
4. B правит ту же цель ещё дважды в течение десяти минут → уведомление осталось одним, с пометкой «(3)».
5. A отмечает уведомление прочитанным, B правит цель снова в том же окне → уведомление снова непрочитанное.
6. A меняет собственную цель → уведомления себе нет.
7. B оставляет комментарий, A отмечает его решённым → у B появляется «A решил ваш комментарий», независимо от того, руководит ли B чем-нибудь.

Пункты 4 и 5 проверяют схлопывание, которое автотесты видят только по ключу; здесь видно, как оно выглядит для человека.

- [ ] **Шаг 5: Проверить, что лента активности не пострадала**

Открыть страницу активности после сценария выше.
Ожидается: все действия из шагов 1–7 в ленте, с прежними формулировками. Уведомления — отдельная подсистема, и появление их не должно было ничего изменить в журнале.
