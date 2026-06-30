# Customizable markdown messages (no-access + empty-hierarchy) — design

Статус: проектная спека. Спек A из связки «кастомизация интерфейса». Делает два текста
редактируемыми из админ-интерфейсов и рендерит их как markdown:

1. **No-access** — сообщение на странице `/no-access` (глобально, system-admin).
2. **Пустая иерархия** — текст в трекере, когда у пользователя нет доступных узлов (per-tenant,
   tenant-admin).

Спек B (invite-ссылки) — отдельно, после.

## Контекст

Оба текста сейчас захардкожены: no-access — в `no_access.js`; пустая иерархия — в `tracker.js`
(«Нет доступа к командам» / «За доступом обратитесь к администратору», ~стр. 1979). Нужно вынести
их в настройки и поддержать markdown. В проекте уже есть общий `markdown.js` с компонентами
`Markdown({text})` (рендер через `marked` + `DOMPurify`) и `MarkdownEditor({value,onChange})`;
`admin_shell` и `tracker_shell` уже грузят `marked`+`dompurify`+`markdown.js`. `system_shell` и
`no_membership` — нет (добавим).

## Общие принципы

- Сообщения хранятся как **markdown-исходник** в key/value-настройках.
- Рендер — только через `Markdown` (marked + DOMPurify); никакого сырого HTML (XSS-правило `010`).
- Если значение не задано/пустое — показывается **дефолт** (текущие строки), поведение не меняется.
- Слои: явный `domain.TenantScope`; запись `no_access_message` — system-only, `empty_hierarchy_message`
  — продуктовый ключ tenant-admin (правило write-authority из settings tier).

## Req 1 — no-access сообщение (глобально, system-admin)

### Хранение

`system_settings.no_access_message` (markdown-строка). Глобальный инстанс-ключ (страница
показывается до членства в любом тенанте).

### Редактирование (`/system`)

Новая секция «Сообщения» в React-панели `/system` (`system.js`): `MarkdownEditor` + кнопка
«Сохранить». Требует загрузки markdown-либ в `system_shell`.

- `GET /api/v1/system/settings` — расширяется полем `no_access_message` (рядом с
  `default_registration_tenant_id`).
- `PUT /api/v1/system/settings/no-access-message` — body `{"message": "<markdown>"}` → `204`.
  Пишет `system_settings` через `SettingsService.SystemSet`. Под `RequireSystemAdmin`.

### Рендер на `/no-access`

Страница — authed-без-membership; `/api/v1/system/settings` под system-гейтом, поэтому ей
недоступен. Доставка — **server-side инъекция**:

- `StubHandler.Render` (в `server.go`) читает `system_settings.no_access_message` через
  `settingsSvc.SystemGet` и передаёт в шаблон `no-membership` как данные.
- Шаблон кладёт исходник в скрытый элемент: `<div id="na-msg-src" hidden>{{.NoAccessMessage}}</div>`
  (html/template экранирует в HTML-контексте → без инъекции).
- `no_access.js` читает `document.getElementById('na-msg-src').textContent`; если непусто —
  рендерит `<Markdown text={msg}/>`, иначе — текущий дефолтный абзац. Форма join-request остаётся.
- `no_membership` shell дополняется `marked`+`dompurify`+`markdown.js`.

Новый публичный эндпоинт не вводится.

## Req 2 — пустая иерархия (per-tenant, tenant-admin)

### Хранение

`tenant_settings.empty_hierarchy_message` (markdown-строка), per-tenant продуктовый ключ.

### Редактирование (`/admin?section=settings`)

В панель «Общие» (`GeneralSettingsPanel` в `admin.js`, рядом с `documentation_url`) добавляется
`MarkdownEditor` для «Сообщение при отсутствии доступа к командам».

- Admin general settings GET (`GET /api/v1/admin/settings/general`) расширяется полем
  `empty_hierarchy_message`; POST — принимает и пишет его через `SetTenantProduct`
  (продуктовый ключ; `entitlement.*`-guard не затрагивает).

### Рендер (трекер)

- `/api/v1/config` (`configResponse`) расширяется полем `empty_hierarchy_message` (per-tenant
  чтение, как `documentation_url`).
- `tracker.js` пустое состояние (хардкод ~1979): если `config.empty_hierarchy_message` непусто —
  `<Markdown text={...}/>`, иначе текущие две строки. `tracker_shell` уже грузит markdown-либы.

## Обработка ошибок

- Пустое значение настройки → дефолтный текст (не ошибка).
- Мутации: `204` успех; `4xx/5xx` → инлайн-сообщение в соответствующей панели.
- system no-access write не-system-admin → `403`; admin general POST не-tenant-admin → `403` (гейты).

## Тестирование

- **Backend (Go, httptest/testcontainers):**
  - `GET /api/v1/system/settings` возвращает `no_access_message`; `PUT …/no-access-message` пишет
    и читается обратно; гейт (не-admin → `403`).
  - admin general GET/POST round-trip для `empty_hierarchy_message` (tenant-scoped, в
    `tenant_settings`); запись идёт продуктовым путём.
  - `/api/v1/config` возвращает `empty_hierarchy_message` активного тенанта.
- **Frontend:** ручная проверка по DoD (`010`): markdown редактируется в `/system` и `/admin`,
  корректно рендерится на `/no-access` и в трекере; при пустом значении — дефолты; разметка
  санитайзится (никакого исполнения скриптов).

## Вне scope

- Markdown на прочих экранах; локализация/мультиязычность сообщений; вложения/картинки.
- Invite-ссылки (Спек B).
