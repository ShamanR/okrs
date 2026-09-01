# Уведомления: шина доменных событий, in-app колокольчик, Telegram и Mattermost — дизайн

**Дата:** 2026-08-26
**Статус:** утверждён, реализация не начата
**Область:** новая подсистема уведомлений + перевод существующего журнала активности на общую шину доменных событий. Внешний контракт существующих URI не меняется; добавляются 11 новых маршрутов.

---

## 1. Задача

Пользователь должен получать уведомления о том, что происходит с целями, за которые он отвечает.

**Типы уведомлений:**

1. оставлен комментарий к цели;
2. решён **мой** комментарий к цели;
3. внесено изменение в цель;
4. обновлён прогресс KR.

**Скоупы** (для каких целей уведомлять):

- только цели, где я руководитель;
- цели, где я руководитель, + цели на уровень ниже;
- цели, где я руководитель, + любая глубина вниз.

**Каналы сейчас:** in-app (колокольчик), Telegram, Mattermost.
**Каналы в будущем:** Slack, почта, webhook.

**Правила подключения каналов:**

- in-app доступен всем тенантам всегда;
- **список каналов, допустимых для пространства, задаёт пользователь системного уровня** — тот, кто создаёт тенанты и управляет entitlements. Администратор пространства этот список расширить не может;
- из уже допустимых каналов администратор пространства включает, выключает и настраивает нужные: сам прописывает секреты, пути и токены;
- в SaaS-инсталляции платные каналы должны открываться тенанту при оплате тарифа — **это будущая работа**: сейчас системный администратор выдаёт разрешение вручную, интерфейса оплаты и предложений «подключить канал» в админке пространства нет;
- в настройках пользователь выбирает, какие уведомления и в каком скоупе получать; если в тенанте подключено больше одного канала — ещё и в каких каналах.

**Дополнительное требование, поставленное в ходе обсуждения:** механика доставки строится как шина доменных событий с регистрацией слушателей на Go-каналах, каждый тип события — отдельная структура, подписка возможна на конкретный тип. Существующий модуль журналирования (лог событий) переводится на эту же шину и сам кастует события в свой формат хранения.

---

## 2. Исходное состояние

| Что | Факт |
|---|---|
| Журнал активности | `activity_events` (миграция `039`), append-only. 22 action-константы, 4 категории |
| Точки записи в журнал | 24 вызова `s.activity.Record(...)` / `RecordBatch(...)` в 5 файлах слоя `usecase`: `goal/goal.go` (11), `keyresult/keyresult.go` (5), `goal/comments.go` (4), `goal/links.go` (2), `period/bulkstatus.go` (2) |
| Форма записи | usecase собирает `domain.ActivityEvent{Category, Action, TeamID, …, Payload: map[string]any{…}}` вручную в каждой из 24 точек |
| Гарантия записи | best-effort: `activity.Service.Record` глотает и логирует ошибку, чтобы журнал не ронял мутацию |
| Шина событий | отсутствует |
| Колокольчик | `SidebarBell` в `web/static/sidebar.js` занят Health Check-in; рендерится только на трекере, счётчик передаётся хостом |
| Настройки пользователя | `store/usersettings` — key/value **без tenant-колонки**; страница `/settings` с секциями «Описание команд», «Мой сайдбар», «Мои пространства» |
| Настройки тенанта | `store/tenantsettings` — key/value + `entitlement.*`; админка `/admin` → Настройки: «Общие», «Доступ», «Обратная связь», «Health check-in» |
| Иерархия | `teams.parent_id` (дерево), `teams.lead_udid` (лид), `memberships` (активность участника в тенанте) |
| Фоновые задачи | `internal/scheduler`: обновление кэша health check-in, суточные снапшоты прогресса под Postgres advisory-lock |
| Сеймы расширения | `auth.Register` (OAuth-провайдеры), `entitlements.Register`, `nomembership.Register`, `auth.RegisterResolveStrategy` |
| Последняя миграция | `043_goal_links` |

### 2.1. Mismatch со спеками, обнаруженный при анализе

1. **`specs/020-domain-model.md`, раздел ActivityEvent** — перечень `action` содержит 21 значение, в коде их 22: отсутствует `kr_note_updated` (`internal/core/domain/models.go:292`). Спека отстала от кода. Правится в фазе 1, где этот перечень и так переписывается под типы событий.
2. **`specs/010-architecture-constraints.md`, раздел «Граница service / usecase»** — правило сформулировано как «метод принадлежит сервису сущности, если трогает не более одного репозитория **и** не пишет в журнал активности». После перевода на шину usecase в журнал не пишет вообще; критерий переформулируется в «…и не публикует доменное событие». Правится в фазе 1.
3. **`specs/010-architecture-constraints.md`, правило 10** («провайдер-специфичный код не должен появляться за пределами `internal/auth/providers/{name}`») сформулировано только про auth. Каналы уведомлений вводят вторую такую же ось; правило обобщается в фазе 2.

4. **`specs/010-architecture-constraints.md`, раздел «OSS / SaaS split»** — сказано, что приватный `okrs-saas` «импортирует `okrs/app`, blank-import'ит пакеты с SaaS-регистрациями, выбирает их по имени в `Config`». Регистрирующие функции (`auth.Register`, `auth.RegisterResolveStrategy`, `entitlements.Register`, `nomembership.Register`) при этом лежат в `internal/`, а `okrs/internal/...` по правилу видимости Go импортируется только кодом внутри модуля `okrs` — из отдельного модуля вызвать их нельзя. Значит описанный путь расширения работает только для кода, живущего внутри этого же модуля.

   **В этой спеке не чиним:** переделка сеймов auth/entitlements/nomembership — самостоятельная задача, к уведомлениям отношения не имеющая, и тянуть её сюда значит смешать две несвязанные вещи. Но форму каналов мы с неё не копируем (см. §10) — иначе требование «расширять каналы из внешнего репозитория» осталось бы невыполнимым.

Иных расхождений между кодом и спеками в затрагиваемой области не обнаружено.

---

## 3. Решения, принятые при обсуждении

Сводка, чтобы дальнейший текст читался без контекста переписки.

| Вопрос | Решение |
|---|---|
| Что значит «цель, где я руководитель» | Я указан в `teams.lead_udid` команды-владельца цели |
| По какому дереву считаются скоупы | По дереву команд `teams.parent_id` (не по `goal_links`) |
| Колокольчик | Health Check-in убирается из сайдбара полностью; единственный колокольчик — уведомления. Панель HCI временно остаётся без точки входа, новое место для неё — вне данной спеки |
| Адресат внешних каналов | Только личные сообщения пользователю. Общих каналов нет и в модели данных не закладывается |
| Шумность | Каждое событие — отдельное уведомление, повторы схлопываются |
| Адресные vs скоупные типы | Два класса. «Решён мой комментарий» приходит всегда, независимо от скоупа, и селектора скоупа не имеет |
| Telegram-бот | Свой на тенант; токен прописывает администратор пространства |
| Хранение секретов | Отдельная таблица, шифрование AES-256-GCM ключом из env |
| Расширяемость каналов | Публичный пакет-контракт `notifychannel` в корне модуля + опция `app.Config.NotificationChannels`. Свой канал можно добавить из внешнего репозитория, собрав его рядом с `main`. Глобального реестра и blank-import'ов нет |
| Кто разрешает канал тенанту | Пользователь системного уровня (`is_system_admin`), вручную, через `entitlement.notifications.<name>` в панели `/system`. Администратор пространства только включает, выключает и настраивает **из уже разрешённого**, расширить список не может и неразрешённых каналов не видит |
| Тарифы и оплата | Откладываются. Ключи и точка проверки готовы, автоматическая простановка при оплате — будущая работа SaaS-биллинга; интерфейса оплаты и upsell в админке пространства нет |
| Механика доставки | Fan-out в момент мутации через шину доменных событий (не outbox-воркер поверх журнала) |
| Гранулярность подписки | На конкретный тип события; тип = отдельная Go-структура |
| Журнал | Первый подписчик шины; сам кастует события в строку `activity_events` |
| Объём | Одна спека, два плана внедрения |

---

## 4. Шина доменных событий

### 4.1. Пакеты

- **`internal/core/event`** — доменные события: по структуре на тип, чистые данные без I/O. Группа `core/` предназначена ровно для этого.
- **`internal/platform/eventbus`** — машинерия шины. Группа `platform/` уже держит registry-сеймы (`entitlements`, `nomembership`); шина с регистрацией слушателей — такой же сейм.

Заводить пакет в корне `internal/` правило группировки из `010` запрещает.

### 4.2. Форма события

```go
package event

type Kind string

// Event — маркер. Kind даёт ключ маршрутизации без reflect. Context отдаёт встроенный
// Meta, чтобы подписчик читал scope/actor без type switch по всем 22 типам.
type Event interface {
    Kind() Kind
    Context() Meta
}

// Meta — общий контекст, встраивается в каждое событие.
type Meta struct {
    Scope      domain.TenantScope
    ActorID    int64
    TeamID     *int64
    PeriodID   *int64
    OccurredAt time.Time
}

// Context реализует Event.Context: раз Meta встроена в каждое событие, Kind — это
// всё, что новому типу события нужно объявить самому.
func (m Meta) Context() Meta { return m }

type CommentAdded struct {
    Meta
    GoalID, CommentID int64
    GoalTitle, Text   string
}

func (CommentAdded) Kind() Kind { return KindCommentAdded }

type CommentResolved struct {
    Meta
    GoalID, CommentID int64
    GoalTitle         string
    AuthorUserID      int64 // адресат уведомления: автор таски
}

type KRProgressUpdated struct {
    Meta
    GoalID, KRID       int64
    GoalTitle, KRTitle string
    KRKind             domain.KRKind // журнальный payload всегда нёс kind — событие тоже
    Before, After      int
}

type GoalFieldsChanged struct {
    Meta
    GoalID  int64
    Title   string
    Changed map[string][2]any // field → {before, after}
}
```

`CommentResolved.AuthorUserID` заполняется в точке публикации, где автор таски уже прочитан, — благодаря этому подписчику уведомлений не нужен join к `goal_comments`.

**Все KR-события несут `GoalID`, а не только `KRID`.** Это не избыточность: уведомление о правке KR адресуется как «изменение в цели» и схлопывается по цели (§6.1, §7.2), а ссылка ведёт на цель. Без `GoalID` в событии подписчику пришлось бы догружать его запросом на каждое KR-событие — ровно тот N+1, которого требует избегать правило 9 CLAUDE.md. В точке публикации цель уже прочитана.

### 4.3. Полный перечень типов

21 структура. Изначально их было 22 — по одной на каждую существующую `ActivityAction`; сейчас типов на одну меньше, чем action-констант, потому что `KRProgressUpdated` и `KRNoteUpdated` заменены одним `KRCheckedIn` (см. `docs/superpowers/plans/2026-08-31-kr-checkin-notifications.md`): чек-ин — одно действие пользователя, а разворачивает его в две строки журнала (`kr_progress` и `kr_note_updated`) уже подписчик. Обе action-константы при этом сохранены — на них ссылаются уже записанные строки журнала.

`GoalCreated`, `GoalDeleted`, `GoalCopied`, `GoalMoved`, `GoalFieldsChanged`, `GoalOwnerChanged`, `GoalShared`, `GoalUnshared`, `GoalLinked`, `GoalUnlinked`, `KRCreated`, `KRDeleted`, `KRFieldsChanged`, `KRCheckedIn`, `StatusChanged`, `CommentAdded`, `CommentResolved`, `CommentReopened`, `CommentDeleted`, `ReplyAdded`, `ReplyDeleted`.

Близкие пары намеренно **не** склеиваются в одну структуру с булевым полем (`CommentResolved`/`CommentReopened`, `GoalLinked`/`GoalUnlinked`, `CommentDeleted`/`ReplyDeleted`): подписка «решён мой комментарий» нужна без «переоткрыт», а с булевым полем фильтровать пришлось бы внутри обработчика — то есть терять ровно ту гранулярность, ради которой типизация и вводится.

### 4.4. Интерфейс шины

```go
package eventbus

// Handler всегда принимает срез: одиночный Publish отдаёт срез из одного элемента.
// Так батчевый слушатель по построению не может выродиться в N+1.
type Handler[T event.Event] func(ctx context.Context, evs []T) error

func Subscribe[T event.Event](b *Bus, name string, h Handler[T], opts ...Option)
func SubscribeAll(b *Bus, name string, h Handler[event.Event], opts ...Option)

func New(logger *slog.Logger) *Bus
func (b *Bus) Publish(ctx context.Context, ev event.Event)
func (b *Bus) PublishBatch(ctx context.Context, evs []event.Event)
func (b *Bus) Start(ctx context.Context)
func (b *Bus) Close(timeout time.Duration) error
```

`Subscribe` — пакетная функция, а не метод: у методов Go не бывает параметров типа.

**Маршрутизация:** `map[event.Kind][]*subscriber` плюс отдельный список wildcard-подписчиков. Ключ берётся из нулевого значения `T` при регистрации (`var zero T; zero.Kind()`) — без `reflect`. Внутри `Subscribe` живёт единственный type-assert `any(ev).(T)`; наружу интерфейс типобезопасен.

`SubscribeAll` существует ради журнала: ему нужны все 22 типа, и перечислять их 22 вызовами — шум, который разъедется при добавлении 23-го.

**Опции:** `WithBuffer(n)` — размер канала подписчика (по умолчанию 256); `WithMode(Sync|Async)`; `WithTimeout(d)` — таймаут одного вызова обработчика в async-режиме (по умолчанию 30 с).

### 4.5. Исполнение

Каждая регистрация получает **свой буферизованный канал и свою goroutine**: медленный подписчик не тормозит остальных, а порядок событий внутри одного подписчика сохраняется (одна goroutine — FIFO).

**Склейка батча.** Async-воркер перед вызовом обработчика дочёрпывает уже готовое из своего канала неблокирующим `select`, собирая срез. Всплеск публикаций превращается в один `RecordBatch`, а не в N вставок.

**Переполнение.** `Publish` шлёт неблокирующим `select` с `default`. Буфер полон → событие для этого подписчика дропается, пишется `Warn` и инкрементируется счётчик дропов. Пользовательский запрос шина не блокирует и не роняет никогда — это ровно та гарантия, которую сегодня даёт `activity.Service.Record`.

**Паника** в обработчике перехватывается `recover`, логируется, подписчик продолжает работу.

**Режимы.** Журнал подписывается в режиме `Sync`, уведомления — `Async`.

Обоснование асимметрии: журнал сегодня пишется до ответа на мутацию, и перевод его в async создал бы окно, в котором лента и счётчики после мутации события ещё не видят, — тихая регрессия ради нулевой выгоды, запись одной строки и так дешёвая. Уведомлениям async обязателен: там резолв получателей и вставка доставок, держать на этом HTTP-ответ незачем. `Sync` вызывает обработчик прямо в `Publish` (канал не задействован), `Async` — через канал; регистрация в обоих случаях одна и та же, разница только в опции.

**Контекст.** Async-обработчик не может использовать ctx запроса — он отменяется, как только handler вернул ответ. Шина детачит контекст через `context.WithoutCancel(ctx)` (значения — трейс, логгер — сохраняются) и накладывает собственный таймаут.

**Долговечность.** Шина внутрипроцессная: событие, лежащее в буфере в момент падения процесса, теряется. Для журнала это не регрессия (best-effort и сегодня), для уведомлений приемлемо: потеря уведомления не искажает данные. Долговечность доставки **во внешние каналы** обеспечивается на уровне БД — строкой `notification_deliveries` с ретраями (см. §7.3), а не буфером шины. При штатной остановке (SIGTERM в K8s) `Close` дренирует буферы с таймаутом.

### 4.6. Сборка

`app.New` создаёт шину → передаёт в `httpdeps.Build` (единственное место, где известен полный граф зависимостей), там регистрируются подписчики → `bus.Start(ctx)` и `defer bus.Close(5*time.Second)` рядом с уже существующим запуском `scheduler`. Построение роутера остаётся чистой сборкой без goroutine — правило из `010` соблюдается.

---

## 5. Журнал как подписчик

Публикация заменяет прямую запись. В `usecase/goal`, `usecase/keyresult`, `usecase/period` поле `activity *activitysvc.Service` заменяется узким портом на стороне потребителя:

```go
type Publisher interface { Publish(ctx context.Context, ev event.Event) }
```

Каждая из 24 точек переписывается с ручной сборки журнальной строки на публикацию семантического события:

```go
// было
s.activity.Record(ctx, scope, domain.ActivityEvent{
    ActorUserID: authorUserID, Category: domain.ActivityDiscussion, Action: domain.ActionCommentAdded,
    TeamID: &teamID, PeriodID: &periodID, GoalID: &goalID, CommentID: &commentID,
    EntityTitle: g.Title, Payload: map[string]any{"text": text},
})

// стало
s.events.Publish(ctx, event.CommentAdded{
    Meta:      event.Meta{Scope: scope, ActorID: authorUserID, TeamID: &teamID, PeriodID: &periodID},
    GoalID:    goalID,
    CommentID: commentID,
    GoalTitle: g.Title,
    Text:      text,
})
```

`service/activity` получает файл `journal.go` — единственное место, знающее форму строки `activity_events`:

```go
// Handle — подписчик шины. Кастует доменные события в журнальные строки.
func (s *Service) Handle(ctx context.Context, evs []event.Event) error {
    rows := make([]domain.ActivityEvent, 0, len(evs))
    for _, ev := range evs {
        if row, ok := toRow(ev); ok {
            rows = append(rows, row)
        }
    }
    return s.RecordBatch(ctx, scopeOf(evs), rows) // батч, не цикл Record
}

// toRow — type switch: событие → {category, action, entity_title, payload_json}.
func toRow(ev event.Event) (domain.ActivityEvent, bool) { … }
```

События в батче могут принадлежать разным тенантам (разные запросы, обслуженные одним инстансом), поэтому `Handle` группирует срез по `Meta.Scope` и делает по одному `RecordBatch` на тенант.

**Выигрыш помимо самой шины:** сегодня `Category`, `Action` и ручная сборка `Payload` (`map[string]any{"before": …, "after": …}`) размазаны по 24 местам в пяти файлах usecase. После переезда они собраны в одном `toRow`, и usecase про журнал не знает ничего.

`Record`/`RecordBatch` в `service/activity` сохраняются, но становятся деталью подписчика. Публичные чтения журнала (`List`, `TreeCounts`, `CategoryCounts`, `Purge`) не меняются.

---

## 6. Классы уведомлений и типы

| Тип | Класс | Порождающие события | Скоуп |
|---|---|---|---|
| `goal_comment` | скоупный | `CommentAdded`, `ReplyAdded` | да |
| `my_comment_resolved` | адресный | `CommentResolved` | нет |
| `goal_changed` | скоупный | `GoalCreated`, `GoalCopied`, `GoalMoved`, `GoalDeleted`, `GoalFieldsChanged`, `GoalOwnerChanged`, `KRCreated`, `KRFieldsChanged`, `KRDeleted` | да |
| `kr_progress` | скоупный | `KRCheckedIn` | да |

Подписчик уведомлений регистрируется ровно на эти 13 типов событий; остальные 8 до него не доходят. Это и есть польза от гранулярной подписки.

### 6.1. Границы типа «внесено изменение в цель»

`goal_changed` намеренно широкий: для лида «в цели что-то поменялось» — одно понятие, независимо от того, поправили заголовок цели или удалили вложенный KR. Поэтому в него входят изменения **и самой цели, и её KR**, включая появление и исчезновение того и другого.

**Копирование и перенос цели считаются созданием.** `GoalCopied` и `GoalMoved` отдельным поводом для уведомления не являются: для команды-получателя копия — это появившаяся цель, для команды-источника перенос — исчезнувшая. Отдельными типами событий на шине они остаются (журналу нужны свои `goal_copied` / `goal_moved`), но текст уведомления у них общий с созданием и удалением.

**Не входят** — с обоснованием, потому что граница здесь неочевидна:

- `GoalShared`, `GoalUnshared`, `GoalLinked`, `GoalUnlinked` — это изменения **отношений между целями и командами**, а не содержимого цели. Сама цель после шаринга не меняется ни на поле. Отдельного типа уведомления они пока не получают;
- изменение заметки к KR — в журнале она отнесена к категории `discussion`, то есть продукт классифицирует её как обсуждение, а не как изменение состава, и тянуть её в `goal_changed` значило бы разойтись с собственной классификацией. Отдельного события у заметки больше нет: она приезжает внутри `KRCheckedIn` и уведомляет как `kr_progress` вместе с остальным чек-ином (тип `goal_changed` её по-прежнему не включает);
- `StatusChanged` — статус команды в периоде, а не цель;
- `CommentReopened`, `CommentDeleted`, `ReplyDeleted` — обсуждение; уведомления о них не заводим (переоткрытие таски и удаления шумны и малоинформативны).

**Схлопывание работает на пользу этой широты.** Ключ `goal_changed` строится по цели (§7.2), поэтому правка цели вместе с двумя её KR в одном окне даёт одно уведомление «×3», а не три отдельных.

**Скоупный** тип адресуется лидам команды-владельца и её предков, отобранным по дистанции. **Адресный** тип адресуется конкретному пользователю, известному из самого события; селектора скоупа в настройках не имеет.

Актор никогда не получает уведомление о собственном действии.

---

## 7. Модель данных

Одна миграция `044_notifications`, пять таблиц.

### 7.1. `notification_preferences` — настройки пользователя

```sql
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
    CONSTRAINT notification_preferences_scope CHECK (
        scope IS NULL OR scope IN ('own','own_and_children','subtree'))
);
```

**Отсутствие строки = дефолт:** `enabled = TRUE`, `scope = 'own'`, `channels = {in_app}`. Резолв получателей делает `LEFT JOIN` + `COALESCE` к этим значениям, поэтому бэкфилл на всех пользователей не нужен, а новый участник пространства получает разумное поведение сразу. Строка появляется только тогда, когда человек что-то поменял.

`tenant_id` в ключе обязателен: пользователь состоит в нескольких пространствах и настраивает их независимо. Существующий `user_settings` для этого не годится — в нём нет tenant-колонки.

У адресного типа `my_comment_resolved` значение `scope` всегда `NULL`.

### 7.2. `notifications` — уведомление, оно же in-app

```sql
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
    UNIQUE (tenant_id, user_id, coalesce_key)
);
CREATE INDEX ON notifications (tenant_id, user_id, created_at DESC);
CREATE INDEX ON notifications (tenant_id, user_id) WHERE read_at IS NULL;
```

`user_id` — **получатель**. `kind` хранит `event.Kind` и нужен рендеру текста.

Ссылки на сущности (`team_id`, `period_id`, `goal_id`, `kr_id`, `comment_id`) — **не** внешние ключи, по той же причине, что и в `activity_events`: уведомление переживает удаление сущности, на которую ссылается.

**Схлопывание повторов.** `coalesce_key` собирается как `type:entity:actor_user_id:bucket`, где:

- `entity` — `kr:<kr_id>` для `kr_progress`, иначе `goal:<goal_id>`. Комментарии схлопываются по цели, а не по комментарию: три реплики в обсуждении одной цели — это одно уведомление, а не три;
- `bucket = floor(unix_seconds / 600)` — окно 10 минут.

Вставка:

```sql
INSERT INTO notifications (…) VALUES (…)
ON CONFLICT (tenant_id, user_id, coalesce_key) DO UPDATE
   SET coalesce_count = notifications.coalesce_count + 1,
       updated_at     = now(),
       payload_json   = EXCLUDED.payload_json,
       read_at        = NULL;
```

Один атомарный запрос, корректный при конкурентной вставке с нескольких реплик, без предварительного `SELECT`.

Два следствия, принятые сознательно:

- **окно фиксированное, а не скользящее.** События в 10:04:59 и 10:05:01 попадают в разные бакеты и не схлопываются. Скользящее окно потребовало бы «прочитать, потом записать» с гонкой между репликами; артефакт границы дешевле;
- **повтор в окне снова помечает уведомление непрочитанным** (`read_at = NULL`). В окне пришло новое изменение — это новая информация, её нужно подсветить.

### 7.3. `notification_deliveries` — доставка во внешние каналы

```sql
CREATE TABLE notification_deliveries (
    id              BIGSERIAL PRIMARY KEY,
    notification_id BIGINT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel         TEXT   NOT NULL,
    status          TEXT   NOT NULL DEFAULT 'pending',
    attempts        INT    NOT NULL DEFAULT 0,
    last_error      TEXT,
    send_after      TIMESTAMPTZ NOT NULL,
    sent_at         TIMESTAMPTZ,
    UNIQUE (notification_id, channel),
    CONSTRAINT notification_deliveries_status CHECK (
        status IN ('pending','sent','failed','skipped'))
);
CREATE INDEX ON notification_deliveries (send_after) WHERE status = 'pending';
```

Канал `in_app` строк доставки **не создаёт** — сама `notifications` и есть in-app.

Строки доставки заводятся только при **создании** уведомления. Если вставка схлопнулась в существующее (сработал `ON CONFLICT DO UPDATE`), новых строк доставки не появляется — событие уедет во внешний канал в составе уже запланированной отправки. Это и есть механизм схлопывания для мессенджеров.

`send_after = created_at + окно схлопывания`. Это даёт схлопывание для мессенджеров бесплатно: воркер просыпается уже после окна и отправляет одно сообщение с итоговым `coalesce_count` («Пётр обновил прогресс 3 KR»), а не три сообщения подряд.

Ретрай — тем же полем: `attempts++`, `send_after = now() + backoff` (экспонента от 30 с, потолок 1 час), после 6 попыток → `failed` с сохранённым `last_error`. Статус `skipped` — доставка невозможна по не-технической причине (нет привязки аккаунта, пустой email), причина в `last_error`.

### 7.4. `notification_channels` — конфигурация канала в тенанте

```sql
CREATE TABLE notification_channels (
    tenant_id          BIGINT  NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel            TEXT    NOT NULL,
    enabled            BOOLEAN NOT NULL DEFAULT FALSE,
    config_json        JSONB   NOT NULL DEFAULT '{}',
    secret_enc         BYTEA,
    secret_hint        TEXT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by_user_id BIGINT REFERENCES users(id),
    PRIMARY KEY (tenant_id, channel)
);
```

`config_json` — несекретная часть: `base_url` для Mattermost, `bot_username` и `poll_offset` для Telegram. `secret_enc` — AES-256-GCM, nonce внутри значения. `secret_hint` — маска вида `••••4821` для показа в UI.

Ключ шифрования берётся из env `NOTIFICATIONS_SECRET_KEY` (32 байта в base64). Если ключ не задан, каналы, требующие секрета, недоступны, админка объясняет причину, а in-app работает — коробочная установка не обязана ничего настраивать. Плейнтекст секрета **никогда** не покидает сервер: наружу уходит только `secret_hint`.

### 7.5. `notification_identities` и `notification_link_tokens` — привязка аккаунтов

```sql
CREATE TABLE notification_identities (
    tenant_id         BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id           BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    channel           TEXT   NOT NULL,
    external_id       TEXT   NOT NULL,
    external_username TEXT,
    linked_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, channel),
    UNIQUE (tenant_id, channel, external_id)
);

CREATE TABLE notification_link_tokens (
    token      TEXT PRIMARY KEY,
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    channel    TEXT   NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);
```

**Telegram** — deep-link `t.me/<bot_username>?start=<token>`: одноразовый токен живёт 15 минут, бот получает `/start <token>`, сервер сохраняет `chat_id` в `external_id`. Без этого отправить в личку невозможно — Telegram не позволяет боту писать первым.

**Mattermost** — привязки не требует: `GET /api/v4/users/email/{email}` резолвит пользователя по email, который уже есть в `users.email`. Строка `notification_identities` заводится автоматически при первой отправке и работает как кэш. Если email пуст или пользователь в Mattermost не найден — доставка получает статус `skipped` с причиной, видимой в настройках.

### 7.6. Ретенция

Уведомления копятся быстрее журнала — строка на каждого получателя. Чистка в суточном фоновом проходе: прочитанные старше 90 дней и любые старше 180 дней. Константы, без пользовательской настройки: данных о том, что кому-то нужно иначе, пока нет.

### 7.7. Seed

`seed_demo.sql` обновляется по правилу 7 CLAUDE.md: демо-пользователям проставляются настройки уведомлений и заводится несколько строк `notifications`, чтобы колокольчик в демо не был пустым.

---

## 8. Резолв получателей

Событие произошло в команде `T`. Получатели скоупного типа — лиды `T` и её предков, у которых дистанция укладывается в выбранный скоуп.

```sql
WITH RECURSIVE chain AS (
    SELECT src.ord, src.actor_id, t.id, t.parent_id, t.lead_udid, 0 AS distance
      FROM unnest($1::bigint[], $4::bigint[]) WITH ORDINALITY AS src(team_id, actor_id, ord)
      JOIN teams t ON t.id = src.team_id AND t.deleted_at IS NULL
    UNION ALL
    SELECT c.ord, c.actor_id, t.id, t.parent_id, t.lead_udid, c.distance + 1
      FROM teams t JOIN chain c ON t.id = c.parent_id
     WHERE t.deleted_at IS NULL
)
SELECT c.ord, u.id,
       COALESCE(p.scope, 'own'), COALESCE(p.channels, '{in_app}'::text[])
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
       END;
```

`$1` и `$4` — параллельные массивы «команда события» и «актор события» для всех событий батча, `$2` — тенант, `$3` — тип уведомления. Результат раскладывается обратно по событиям через `ord` — порядковый номер во входных массивах.

Актор исключается **поштучно**, через перенесённый по цепочке `c.actor_id`, а не одним списком на весь батч: лид, оказавшийся автором одного события в батче, должен получить уведомления об остальных.

**Батчевость обязательна.** Батч из 50 событий четырёх типов даёт **не больше 4 запросов** (по одному на тип), а не 50. Метод помечается комментарием как батчевый: превращение его в цикл — регрессия по правилу 9 CLAUDE.md, а не рефакторинг.

`JOIN memberships` не косметика: лид, вычищенный из пространства, уведомления получать не должен, а `teams.lead_udid` при этом остаётся заполненным.

Адресные типы резолв не задействуют вовсе: получатель известен из события (`CommentResolved.AuthorUserID`), проверяются только `enabled` и список каналов.

---

## 9. Раскладка по слоям

Соблюдаются правила `010`: store — множественное число, service — единственное; сервис работает с одним репозиторием; usecase оркестрирует сервисы и не ходит в репозитории.

| Слой | Пакет | Ответственность |
|---|---|---|
| store | `store/notifications` | агрегат «уведомление + его доставки» |
| store | `store/notificationprefs` | настройки и `ResolveRecipients` |
| store | `store/notificationchannels` | конфигурация каналов тенанта |
| store | `store/notificationidentities` | привязки аккаунтов и одноразовые токены |
| service | `service/notification` | создание, чтение, пометка прочитанным, claim и завершение доставок |
| service | `service/notificationpref` | чтение-запись настроек, резолв получателей |
| service | `service/notificationchannel` | конфигурация каналов, шифрование и дешифровка секрета |
| service | `service/notificationidentity` | выпуск токена, привязка, отвязка |
| usecase | `usecase/notification` | `Handle` (событие → получатели → уведомления → доставки), `Deliver` (воркер), `Link` |
| render | `render/notify` | `kind + payload → {title, body}` |

`ResolveRecipients` живёт в `store/notificationprefs`, потому что отвечает на вопрос «кто подписан», — это чтение настроек, обогащённое деревом команд, а не отдельная сущность.

`usecase/notification.Handle` — это и есть подписчик шины, регистрируемый на свои 13 типов событий с `WithMode(Async)`.

Четыре store-пакета вместо двух — следствие правила «репозиторий отвечает ровно за одну сущность или агрегат». Слияние сократило бы число файлов ценой расхождения со спекой.

---

## 10. Каналы как публичный сейм

**Требование:** тип канала должен быть расширяемым из **внешнего репозитория** — сторонний модуль подключает `okrs` как зависимость, пишет собственный канал и собирает его рядом с `main`.

Это исключает форму, применённую к OAuth-провайдерам и entitlements: их реестры (`auth.Register`, `entitlements.Register`) лежат в `internal/`, а по правилу видимости Go пакет `okrs/internal/...` импортируется только кодом внутри модуля `okrs`. Внешний модуль такой `Register` вызвать не может физически. Поэтому сейм каналов строится иначе: **публичный пакет с интерфейсами + подключение через опцию `app.Config`**, без глобального реестра.

### 10.1. Публичный контракт

Новый пакет в корне модуля, рядом с `app` и `web`:

```go
// Package notifychannel — публичный контракт канала доставки уведомлений.
// Только типы и интерфейсы: ни I/O, ни импортов internal/*.
package notifychannel

// Target — адресат. Канал использует то поле, которым умеет адресовать.
type Target struct {
    ExternalID string // сохранённая привязка (telegram chat_id и т.п.)
    Email      string // для каналов, резолвящих адресата сами (Mattermost)
}

type Message struct {
    Title, Body string // Markdown
    URL         string // ссылка на цель
}

type Sender interface {
    Send(ctx context.Context, target Target, msg Message) error
}

// Settings — конфигурация канала в конкретном тенанте.
// Secret расшифрован ядром; хранение и шифрование канала не касаются.
type Settings struct {
    Values map[string]any
    Secret string
}

type FieldKind string // text | url | secret

type Field struct {
    Key, Label, Hint string
    Required         bool
    Kind             FieldKind
}

// Descriptor описывает форму настройки канала в админке.
// Благодаря ему админка не знает ни про Telegram, ни про чужие каналы.
type Descriptor struct {
    Name        string // ключ канала: хранится в БД, попадает в настройки пользователя
    Title       string
    SecretField string // какое поле шифровать; "" — секрета нет
    Fields      []Field
}

// Channel — единица подключения: описание плюс конструктор отправителя.
type Channel struct {
    Descriptor Descriptor
    New        func(Settings) (Sender, error)
}

// Linker реализуется каналом, которому нужна явная привязка аккаунта
// (одноразовый токен и deep-link), а не резолв адресата по email.
// Необязательный интерфейс: канал без него адресуется через Target.Email.
type Linker interface {
    LinkURL(s Settings, token string) string
}
```

Пакет намеренно не содержит ничего, кроме типов: внешний автор канала получает зависимость, которая не тянет ни базу, ни HTTP-слой коробки.

### 10.2. Подключение — опцией, близко к main

```go
// app.Config
type Config struct {
    …
    // NotificationChannels — каналы, доступные этой сборке. nil → только in-app.
    NotificationChannels []notifychannel.Channel
}
```

Сборка в `cmd/server` (и точно так же в чужом `main`):

```go
app.New(app.Config{
    Pool: pool,
    NotificationChannels: []notifychannel.Channel{
        telegram.Channel(),      // okrs/notifychannel/telegram   — коробочный
        mattermost.Channel(),    // okrs/notifychannel/mattermost — коробочный
        smsgw.Channel(cfg.SMS),  // из внешнего репозитория
    },
})
```

Коробочные каналы живут в публичных подпакетах `okrs/notifychannel/telegram` и `okrs/notifychannel/mattermost` и экспортируют `Channel()`. Следствия:

- канал, который не подключили, **не компилируется** в бинарь — коробка без Telegram не тащит его код;
- у внешнего автора есть две работающие реализации как образец контракта;
- реестра по имени нет, blank-import'ов нет, порядок сборки виден в одном месте — в `main`.

`app.New` собирает из слайса `map[Descriptor.Name]Channel`, проверяет уникальность имён (дубликат → ошибка сборки, а не молчаливое затирание) и передаёт карту внутрь через `httpdeps.Build`. Внутренние слои импортируют публичный `okrs/notifychannel` ради типов — это разрешено, ограничение видимости работает только в обратную сторону.

`in_app` каналом в этом смысле **не является**: он не имеет `Sender`, не настраивается администратором и обслуживается самой таблицей `notifications`. В списке `NotificationChannels` его нет и быть не может.

### 10.3. Что это требует от остальной системы

Расширяемость должна доходить до данных, иначе она декоративна:

- **никаких CHECK-ограничений на имена каналов.** `notification_channels.channel`, `notification_deliveries.channel` и элементы `notification_preferences.channels` — свободный текст. Новый канал не требует миграции. (Ограничение `CHECK` на `notification_preferences.type` остаётся: типы уведомлений фиксированы, расширяемы только каналы.)
- **канал, оставшийся в БД, но не подключённый в этой сборке** (выключили опцию, откатили бинарь) — не ошибка: строки доставки получают статус `skipped` с причиной «канал недоступен в этой сборке», а в админке пространства такой канал **не показывается вовсе**. Карточке взяться неоткуда: экран рендерится по дескрипторам каналов, собранных в этот бинарь, а у отсутствующего в сборке канала дескриптора нет. Это согласуется с §13.4 — там же обосновано, почему недоступный канал не появляется даже строкой «недоступно». Данные не теряются, при возврате канала всё оживает.
- **шифрование секрета делает ядро**, а не канал: `service/notificationchannel` шифрует поле, названное в `Descriptor.SecretField`, и отдаёт каналу уже расшифрованное значение в `Settings.Secret`. Внешний канал получает шифрование бесплатно и не может его случайно обойти.
- **привязка аккаунта.** Канал, реализующий `Linker`, получает механику одноразовых токенов и deep-link (как Telegram); канал, не реализующий его, адресуется по `Target.Email` (как Mattermost). Третий вариант ядру не нужен.
- **гейт доступности** — по имени канала, и это два независимых условия, оба обязаны быть истинны (подробности и обоснование — §10.5): явная выдача пространству, читаемая как административное данное через `settings.Service.TenantEntitlements` (ключ `notifications.<Descriptor.Name>` без префикса), И тариф не запрещает — тот же канал под ключом `entitlement.notifications.<Descriptor.Name>`, проверенным `entitlements.Entitlements.Has`. Внешний канал гейтится тем же механизмом без правок ядра. `in_app` не гейтится никогда.

### 10.5. Два уровня управления каналом

Полномочия разведены: **какие каналы вообще доступны пространству** решает пользователь системного уровня, **что из доступного включено и как настроено** — администратор пространства.

| Уровень | Кто | Что может | Где |
|---|---|---|---|
| Системный | `users.is_system_admin` — тот, кто создаёт тенанты и управляет entitlements | Разрешить или запретить пространству канал: ключ `entitlement.notifications.<name>` | Панель `/system` → «Entitlements» |
| Тенант | `memberships.role = admin` | Включить, выключить и настроить канал **из числа разрешённых**: секреты, пути, токены | `/admin` → Настройки → «Уведомления» |

**Это два разных вопроса, и ни один не отвечает за другой.** `entitlements.Entitlements.Has` отвечает на вопрос «разблокирован ли класс возможностей в тарифе» — тот же вопрос, что и для `sso`, `subdomains`, `max_users`. Для коробочной сборки правильный ответ на него — «всё разблокировано», и реализация `unlimited` в этом качестве совершенно верна. А «какие каналы уведомлений выданы этому конкретному пространству» — вопрос другого рода: административное назначение, конкретное данное, которое системный администратор ввёл руками, а не производное от тарифа. Проверять доступность канала одним только `entitlements.Has` значит смешать эти два вопроса — `unlimited` откроет канал молча, и в коробочной сборке (единственной, что существует сегодня) любой канал стал бы доступен любому пространству без какой-либо явной выдачи. Это и произошло на практике: администратор пространства видел в `/admin` канал, который системный администратор не выдавал через `/system`.

Поэтому доступность канала — конъюнкция двух условий, проверяемых на каждом из четырёх путей гейта (`internal/service/notificationchannel`: `Available`, `IsAvailable`, `Save`, `Sender`, через общий хелпер `channelAvailable`):

1. **явная выдача** — `ChannelGrants.TenantEntitlements` (в проде — `settings.Service.TenantEntitlements`, кэшированный снимок `tenant_settings`) содержит ключ `notifications.<name>` со значением JSON `true`. Значение `false` или отсутствие ключа — не выдано; отсутствие ключа и явное «выключили» неразличимы для гейта и оба ведут к «недоступен», хотя это разные события в истории тенанта.
2. **тариф не запрещает** — `entitlements.Entitlements.Has(scope, "entitlement.notifications.<name>")` истинно.

В коробке второе условие истинно всегда, и результат решает первое — ровно то поведение, которого ждёт коробочный администратор. В ограничивающей SaaS-сборке тариф может дополнительно запретить канал независимо от выдачи, и оба слоя честны: ни один не переопределяет другой молча.

Администратор пространства **не может** расширить список доступных ему каналов — только распорядиться выданным. Выдача проставляется вручную системным администратором через `PUT /api/v1/system/tenants/{id}/entitlements` под ключом `entitlement.notifications.<name>` — тем же маршрутом и тем же ключом, под которым позже эту запись может прочитать и снапшот-реализация тарифной проверки для SaaS (см. package doc `internal/platform/entitlements`). В OSS-реализации `unlimited` тарифная сторона ничего не читает вовсе — `Has` отвечает `true` на любой ключ независимо от того, что где записано, — поэтому единственная запись, проставленная этим маршрутом, на практике решает и тариф (тривиально) и выдачу (по существу).

**Привязка к тарифам в эту спеку не входит.** Автоматическая простановка `entitlement.notifications.*` при оплате — будущая работа на стороне SaaS-биллинга; ключи и точка их проверки для неё готовы, но ни интерфейса оплаты, ни предложения «купить канал» в админке пространства сейчас не появляется (§13.4).

**Проверка серверная, в трёх местах** — иначе разграничение декоративно и обходится curl'ом:

1. `PUT /api/v1/admin/settings/notifications/{channel}` для неразрешённого канала → `404`, независимо от того, что показал UI. Именно `404`, а не `403`: ответ обязан быть неотличим от ответа по каналу, которого в сборке нет вообще, иначе он сам становится оракулом — перебирая имена, администратор пространства вычислит полный каталог каналов продукта, включая невыданные его пространству. Обоснование целиком — в §13.4; то же зафиксировано в `specs/040-api-contract.md`. Не «чинить» обратно на `403`;
2. канал, разрешение на который отозвали, перестаёт участвовать в fan-out: строки доставки для него не создаются;
3. уже созданные `pending`-доставки отозванного канала завершаются как `skipped` с причиной, а не отправляются.

**Список каналов сборки нужен и системной панели.** Сейчас `/system` → «Entitlements» рендерит тумблеры по захардкоженному в JS списку `KNOWN_ENT` (`web/static/system.js:173`) плюс поле «произвольный ключ». Для каналов это не годится: набор зависит от того, что подано в `app.Config.NotificationChannels`, и внешний канал в захардкоженный массив не попадёт никогда. Поэтому добавляется read-only маршрут `GET /api/v1/system/notification-channels`, отдающий `{name, title}` подключённых каналов, а панель рендерит тумблер `entitlement.notifications.<name>` по каждому. Канал из чужого репозитория появляется в системной панели сам, без правок фронтенда.

### 10.4. Приём входящих сообщений

Long-polling Telegram (§11) — деталь **реализации канала Telegram**, а не ядра: ядро не знает, что у какого-то канала бывают входящие. Канал, которому нужен фоновый приём, реализует необязательный интерфейс:

```go
// Receiver — канал умеет принимать входящие (привязку аккаунта).
type Receiver interface {
    // Receive вызывается фоновой петлёй под лидер-локом.
    // Возвращает обнаруженные привязки: токен из /start → внешний id.
    Receive(ctx context.Context, s Settings) ([]Link, error)
}

type Link struct{ Token, ExternalID, Username string }
```

Ядро само ничего не знает про `getUpdates`: петля в `internal/scheduler` обходит подключённые каналы, реализующие `Receiver`, под advisory-lock и сохраняет полученные `Link`. Смещение опроса канал хранит в своих `Settings.Values` — ядро отдаёт их обратно на запись через тот же `service/notificationchannel`.

Так внешний канал с входящими (Slack Events, свой мессенджер) встраивается без правок ядра.

---

## 11. Фоновые задачи

Три петли в `internal/scheduler`.

**Доставка** — каждые 5 секунд:

```sql
SELECT … FROM notification_deliveries
 WHERE status = 'pending' AND send_after <= now()
 ORDER BY send_after LIMIT 100
 FOR UPDATE SKIP LOCKED;
```

`SKIP LOCKED`, а не advisory-lock как у снапшотов прогресса: здесь нужна пропускная способность, и все реплики должны разбирать очередь параллельно, не мешая друг другу. Успех → `sent` + `sent_at`; ошибка → `attempts++` и новый `send_after` по экспоненте.

**Приём входящих** — обход подключённых каналов, реализующих `notifychannel.Receiver` (§10.4), под advisory-lock: здесь, в отличие от доставки, лидер обязан быть один, иначе реплики будут воровать друг у друга апдейты. Полученные `Link` сохраняются как привязки аккаунтов.

Ядро при этом не знает, что происходит внутри `Receive`. У коробочного Telegram это long-polling `getUpdates` по каждому включённому боту тенанта, со смещением, которое канал хранит в собственных `Settings.Values` (`poll_offset`).

Выбор polling вместо webhook — решение **канала Telegram**, а не архитектуры: webhook требует публичного HTTPS-ingress, которого у корпоративной self-hosted установки за периметром может не быть, тогда как исходящий доступ к `api.telegram.org` нужен в любом случае — polling работает всюду, где работает отправка. **Известное ограничение:** лидер держит по одному long-poll соединению на каждый тенант с включённым Telegram; на тысячах тенантов это упрётся в потолок. Заменить его на webhook можно правкой одного канала, не трогая ядро.

**Ретенция** — раз в сутки, в уже существующем суточном проходе.

---

## 12. API

```text
GET    /api/v1/notifications                      список, курсор, ?unread=1
GET    /api/v1/notifications/unread-count         счётчик для бейджа
POST   /api/v1/notifications/read                 {ids:[…]} | {all:true}
GET    /api/v1/notifications/preferences          настройки + каналы, доступные в тенанте
PUT    /api/v1/notifications/preferences
GET    /api/v1/notifications/identities           статус привязок
POST   /api/v1/notifications/identities/{ch}/link → {url, expires_at}
DELETE /api/v1/notifications/identities/{ch}

GET    /api/v1/admin/settings/notifications                      только разрешённые каналы + дескрипторы + статус
PUT    /api/v1/admin/settings/notifications/{channel}
POST   /api/v1/admin/settings/notifications/{channel}/test

GET    /api/v1/system/notification-channels                      каналы, собранные в этот бинарь
```

Последний маршрут — системного уровня (`is_system_admin`), read-only: отдаёт `{name, title}` подключённых каналов, чтобы панель `/system` могла отрисовать тумблеры `entitlement.notifications.<name>` по фактическому составу сборки, включая каналы из внешних репозиториев (§10.5). Управление самими разрешениями идёт через уже существующий `PUT /api/v1/system/tenants/{id}/entitlements` — новых маршрутов для этого не нужно.

Пакеты обработчиков повторяют пути (`handlers/api/v1/notifications/preferences` и так далее), общий для группы код — в лист-пакет `notificationcommon`. Все мутирующие маршруты под CSRF (правило 7 из `010`), админские — под `RequireTenantAdminMiddleware`. `internal/http/testdata/routes.golden` обновляется в том же change set через `go test ./internal/http -run RoutesGolden -update-routes`.

Входящего вебхука Telegram в контракте нет — приём идёт long-polling'ом.

**Кэша на счётчике непрочитанных нет, сознательно.** Это `COUNT` по partial-индексу для одного пользователя. Кэширование в памяти инстанса в K8s-среде дало бы разные числа на разных репликах при том, что запрос и так дешёвый. Бейдж обновляется опросом раз в 60 секунд и при возврате фокуса на вкладку; пользовательское ожидание «увижу в течение минуты» для колокольчика адекватное.

---

## 13. UI

### 13.1. Рендер текста — на сервере

Текст уведомления нужен в трёх местах: колокольчик, Telegram, Mattermost. Держать шаблоны и в Go, и в JS — гарантированное расхождение формулировок, поэтому текст собирает сервер (`internal/render/notify`, симметрично существующему `internal/render/export`): plain-текст для in-app, Markdown для мессенджеров. Фронт добавляет только аватар, ссылку и относительное время.

### 13.2. Колокольчик

Живёт внутри `Sidebar` в `web/static/sidebar.js`, а не передаётся хостом, — поэтому появляется на всех SPA-shell разом (трекер, админка, настройки, system, страница без доступа). Модуль уже самодостаточен и сам ходит в `/api/v1/config`, так что запрос счётчика туда ложится без новых зависимостей.

**Колокольчик Health Check-in из сайдбара убирается полностью.** Существующий `SidebarBell` не дублируется и не обобщается под две иконки — он перепрофилируется под уведомления: разметка и стили остаются, меняются источник счётчика и обработчик клика. Проп-слот `bell` удаляется из API компонента `Sidebar`, трекер перестаёт его передавать.

Следствия в `web/static/tracker.js`:

- состояние `hciData` / `hciOpen` и компонент `HealthCheckInPanel` **удалены полностью** (задача 8, решение владельца) — не оставлены недостижимым кодом на будущее. Панель временно недоступна пользователю; новое место для входа в Health Check-in определяется отдельной задачей, вне этой спеки;
- запрос health check-in при загрузке трекера снимается вместе с колокольчиком — данные некому показывать, а запрос не бесплатный;
- заодно удаляется `HealthCheckInButton` (`tracker.js:2607`) — компонент определён, но не используется ни в одном месте уже сейчас, то есть это мёртвый код, а не потеря функциональности.

Код панели удалён вместе с точкой входа, а не оставлен недостижимым — репозиторий не любит копить такой код (в слоистом рефакторинге такие пакеты вычищались). Технический долг здесь не код, а продуктовый пробел: у Health Check-in нет входа до отдельной задачи о новом месте для него.

Панель — общий компонент `NotificationList` (новый `web/static/notifications.js`, подключается в партиале `spa-vendor` рядом с `sidebar.js`): аватар актора, серверный текст, пометка «×3» при схлопывании, время, клик → переход на цель, действие «Отметить все прочитанными».

### 13.3. Настройки пользователя

Новая секция «Уведомления» в `web/static/settings.js`, рядом с «Описание команд», «Мой сайдбар», «Мои пространства».

Матрица: строка на тип уведомления, тумблер включения, селектор скоупа, колонки-чекбоксы каналов. У адресного типа `my_comment_resolved` вместо селектора скоупа — прочерк: скоуп к нему неприменим.

**Колонки каналов появляются только если в тенанте включён больше одного канала.** Если работает только in-app, колонок нет вовсе — остаются тумблер и скоуп. Это требование заказчика, и оно же удерживает экран простым в коробочной установке.

Ниже — блок «Подключённые аккаунты»: Telegram с кнопкой «Подключить» (открывает выданный deep-link), статусом «Подключён как @name» и действием «Отключить»; Mattermost с пояснением «подключается автоматически по вашему email» либо предупреждением, если email пуст.

### 13.4. Админка тенанта

`/admin` → Настройки → «Уведомления», рядом с «Общие», «Доступ», «Обратная связь», «Health check-in».

Карточка на канал, **форма генерится из `Descriptor`** — админка не знает о существовании Telegram, и Slack появится в ней сам. Секрет показан маской `••••4821`, и она стоит **плейсхолдером в пустом поле ввода**, а не рядом с кнопкой «Заменить»: пустое поле по контракту означает «не менять сохранённый», поэтому замена секрета — это просто ввод нового значения, и отдельное действие «Заменить» ничего бы не добавило. Плюс тумблер «Включён» и кнопка «Проверить» (шлёт тестовое сообщение администратору). **Последней ошибки доставки на карточке нет**: доставок ещё не существует — таблица и воркер появляются в фазе 2a-2, вместе с ними появится и это поле.

**Показываются только разрешённые пространству каналы.** Неразрешённый канал не появляется вообще — ни карточкой с замком, ни строкой «недоступно». Никакого upsell, предложений тарифа и намёков на существование других каналов: администратор пространства видит ровно тот набор, которым может распоряжаться.

Это требование к API, а не к разметке: `GET /api/v1/admin/settings/notifications` **не отдаёт** дескрипторы неразрешённых каналов, поэтому состав сборки через тенантный API не разведать. `PUT` по неразрешённому каналу возвращает `404` — тот же ответ, что и по каналу, которого в сборке нет, на случай прямого запроса мимо UI. Два разных ответа свели бы на нет всё, что сказано абзацем выше: `403` означал бы «канал есть, но не ваш», и перебор имён вернул бы администратору полный каталог каналов продукта. Поэтому оба случая отвечают одинаково, и `403` здесь — регресс, а не уточнение.

### 13.5. Системная панель

`/system` получает **отдельный раздел сайдбара «Уведомления»** (изначально задумывался блок внутри «Entitlements»; при реализации выбран свой раздел: каналы не приходится искать внутри чужого экрана, а место под будущие настройки уведомлений системного уровня остаётся): тумблер `entitlement.notifications.<name>` на каждый канал, собранный в этот бинарь. Список приходит из `GET /api/v1/system/notification-channels`, а не из захардкоженного `KNOWN_ENT` (`web/static/system.js:173`), — иначе канал из внешнего репозитория в панели не появился бы никогда.

Существующее поле «произвольный ключ» остаётся как было: оно и сейчас позволяет выставить любой entitlement вручную, но полагаться на него для штатной операции «разрешить пространству Telegram» неправильно.

**Переключатель работает в любой сборке, включая коробочную.** Источник истины о том, выдан ли канал пространству, — не тарифная реализация `entitlements.Entitlements`, а сама запись `entitlement.notifications.<name>` в `tenant_settings`, которую пишет этот переключатель; `internal/service/notificationchannel` читает её напрямую как административное данное через `ChannelGrants` (обёртка над `settings.Service.TenantEntitlements`) и требует её истинности как первого из двух условий гейта — см. §10.5. Тарифная проверка `entitlements.Has` по тому же ключу остаётся вторым, независимым условием: в OSS-реализации `unlimited` она истинна всегда, поэтому решает именно выдача, и переключатель здесь и то, что видит администратор тенанта в `/admin`, всегда согласованы. Так гейт устроен ровно потому, что «какие каналы выданы» — не тарифный вопрос (§10.5); никакого флага `entitlements_enforced` и предупреждения панели поэтому не нужно, а более раннее решение с таким флагом было ошибкой проектирования: оно снова сводило выдачу к ответу тарифной реализации, то есть к тому же смешению вопросов, которое описано и исправлено в §10.5.

Новые стили — в `components.css` и `sidebar.css`. Ни нового вендора, ни сборщика (правило 5 из `010`).

---

## 14. Тесты

**Шина** (`platform/eventbus`): маршрутизация по типу — подписчик на `CommentAdded` не получает `KRProgressUpdated`; `SubscribeAll` получает всё; изоляция подписчиков при панике в одном; дроп при переполнении буфера с инкрементом счётчика; дренаж в `Close`; независимость async-обработчика от отмены ctx запроса; склейка батча из нескольких публикаций.

**Журнал** (`service/activity`): табличный тест `toRow` по всем 22 типам событий — он же страховка от того, что при добавлении 23-го про журнал забудут; группировка батча по тенантам.

**Резолв получателей**: табличный тест на дереве из четырёх уровней — все три скоупа, исключение актора, исключение неактивного участника, команда без `lead_udid`, soft-deleted команда в середине цепочки.

**Схлопывание**: два события в одном бакете дают одну строку с `coalesce_count = 2` и сброшенным `read_at`; события в соседних бакетах дают две строки; правка цели и двух её KR одним автором в одном окне даёт **одно** уведомление `goal_changed` с `count = 3` — проверка того, что ключ строится по цели, а не по KR.

**Отбор событий**: табличный тест «событие → тип уведомления или ничего» по всем 22 типам — фиксирует границу `goal_changed` из §6.1, в том числе что `GoalShared`, `GoalLinked`, `KRNoteUpdated` и `StatusChanged` уведомлений не порождают, а `GoalCopied` и `GoalMoved` порождают.

**Доставка**: успех, ретрай с backoff, переход в `failed` после лимита попыток, `SKIP LOCKED` не выдаёт одну строку двум конкурентным claim'ам.

**Полномочия по каналам** (§10.5): `GET /api/v1/admin/settings/notifications` не отдаёт неразрешённые каналы — ни дескриптор, ни имя, то есть состав сборки через тенантный API не разведать; tenant-admin получает `404` на `PUT` неразрешённого канала, неотличимый от ответа по каналу, которого нет в сборке (§13.4), — проверка серверная, а не в UI; отзыв `entitlement.notifications.<name>` останавливает создание новых доставок; уже созданные `pending`-доставки отозванного канала завершаются как `skipped`, а не отправляются; `GET /api/v1/system/notification-channels` доступен только `is_system_admin` и перечисляет фейковый канал, поданный опцией.

**Каналы**: фейковый канал, поданный через `app.Config.NotificationChannels`, проходит весь путь до `Send` — это тест самого требования «канал можно добавить извне»; дубликат `Descriptor.Name` роняет `app.New` с внятной ошибкой; канал, присутствующий в БД, но не поданный в опции, даёт доставку `skipped`, а не панику; round-trip шифрования секрета; отсутствие плейнтекста секрета в ответах API; `Descriptor` фейкового канала долетает до ответа админского API и рендерит форму.

**Usecase**: `usecase/notification.Handle` на `FakeBus`, добавляемом в `service/servicetest`. Существующие тесты usecase, проверяющие журнал через fake-репозиторий активности, переезжают на `FakeBus`.

**Маршруты**: обновлённый `routes.golden`.

---

## 15. Правки спек — в том же change set

- **`010-architecture-constraints.md`** — пакеты `core/event`, `platform/eventbus`, `render/notify` в разделе «Слои»; переформулировка критерия service/usecase; обобщение правила 10 на каналы уведомлений; новые store-репозитории в таблице; в описании `sidebar.js` (строка 12) к перечню общих модулей добавляется `notifications.js`, а `SidebarBell` переописывается как колокольчик уведомлений, доступный на всех страницах, а не слот, заполняемый трекером.

  Плюс правка утверждения **«Публичных пакетов ровно два: `app` и `web`»** — их становится больше: добавляется `notifychannel` (контракт) и его коробочные реализации `notifychannel/telegram`, `notifychannel/mattermost`. Формулировка переписывается в правило: *публичны фасад приложения, SSR-ассеты и контракты точек расширения, предназначенных для внешних модулей; всё остальное — `internal/`*. Заодно в раздел «OSS / SaaS split» добавляется описание расширения каналов опцией `app.Config.NotificationChannels`.
- **`020-domain-model.md`** — сущности `Notification`, `NotificationPreference`, `NotificationChannel`, `NotificationIdentity` с инвариантами; добавление пропущенного `kr_note_updated` в перечень action.
- **`030-user-flows.md`** — флоу настройки уведомлений, привязки Telegram, подключения канала администратором; плюс два места, где колокольчик описан как Health Check-in: строка 169 («Справа от названия — колокольчик Health Check-in (только на трекере…)») переписывается под уведомления на всех страницах, строка 270 («шапка тенанта с колокольчиком») уточняется.
- **`040-api-contract.md`** — 12 новых маршрутов (11 тенантных + системный `GET /api/v1/system/notification-channels`).
- **`050-permissions-and-lifecycle.md`** — три уровня полномочий: свои уведомления и настройки читает и меняет только сам пользователь; включение, выключение и настройку каналов из разрешённых делает tenant-admin; **список разрешённых пространству каналов задаёт только system-admin** через `entitlement.notifications.<name>`. Явно фиксируется, что tenant-admin расширить этот список не может.
- **`070-code-structure.md`** — карта «URI → пакет обработчика».
- **`README.md`** — секция API.

---

## 16. Фазы внедрения

**Фаза 1 — движок и in-app.**
Шина событий, 22 типа событий, перевод журнала на шину, модель данных, резолв получателей, подписчик уведомлений, рендер текста, API уведомлений и настроек, колокольчик в сайдбаре, секция «Уведомления» в настройках пользователя.
Результат: работающий колокольчик. Внешних каналов ещё нет, поэтому колонок каналов в матрице настроек не видно.

**Фаза 2 — внешние каналы.**
Публичный пакет-контракт `notifychannel`, опция `app.Config.NotificationChannels`, коробочные реализации Telegram и Mattermost, шифрование секретов, воркер доставки, воркер приёма входящих, привязка аккаунтов, админка каналов, разграничение полномочий system-admin / tenant-admin (§10.5) и блок каналов в системной панели.
Результат: уведомления в личку в Telegram и Mattermost, настраиваемые администратором пространства.

Каждая фаза получает собственный план реализации.

---

## 17. Что сознательно не делается

- **Общие каналы** (уведомления в групповой чат) — только личные сообщения. Групповой чат не согласуется с персональными скоупами и создаёт риск показать цели тем, кому они не видны.
- **Дайджесты** (сводка раз в час или день) — есть схлопывание повторов; дайджест добавится отдельной спекой, если объём этого потребует.
- **Webhook-приём Telegram** — polling покрывает все среды; webhook добавится как альтернативный режим канала, если упрёмся в число тенантов.
- **Slack, почта, webhook-канал** — контракт `notifychannel` рассчитан на них (отправка, привязка через `Linker`, входящие через `Receiver`), но реализации в эту спеку не входят. Добавляются как отдельные пакеты и строка в `main` — ядро при этом не меняется, и то же верно для канала из чужого репозитория.
- **Настраиваемая ретенция и окно схлопывания** — константы до появления данных о том, что кому-то нужно иначе.
- **Уведомления по дереву целей `goal_links`** — скоупы считаются по дереву команд; декомпозиция целей на адресацию не влияет.
- **Привязка каналов к тарифам и оплате** — `entitlement.notifications.<name>` проставляется системным администратором вручную. Автоматическая выдача при оплате, интерфейс оплаты и предложение «подключить канал» в админке пространства — будущая работа на стороне SaaS-биллинга. Точка подключения готова: биллингу достаточно дёрнуть существующий `PUT /api/v1/system/tenants/{id}/entitlements`, ядро уведомлений при этом не меняется.
- **Новое место для Health Check-in** — колокольчик HCI из сайдбара убирается, но переносить его вход куда-либо эта спека не берётся. Панель остаётся в коде без точки входа; где ей жить дальше — отдельная задача. Приняли осознанно: смешивать перекладку чужой фичи с запуском уведомлений значит удлинить фазу 1 и размыть её проверяемый результат.
