# Сбор обратной связи по инструменту — design

Дата: 2026-06-16

## Цель

Собирать обратную связь с пользователей через внешнюю Google-форму:

1. постоянный пункт «Обратная связь» в гамбургер-меню (под «Документация»);
2. модальное всплывающее окно с просьбой поделиться обратной связью;
3. раздел в админке (Настройки) для управления сбором: вкл/выкл окна, вкл/выкл
   пункта меню, ссылка на опрос, частота показа окна.

## Поведение всплывающего окна (итоговая логика)

- НЕ показывать в первый заход — дать пользователю поработать минимум 2 суток.
- Интервал охлаждения между показами, чтобы не перегружать (настройка «частота»).
- После долгого перерыва окно снова даёт пользователю время поработать (2 дня).
- Модальное, закрывается крестиком (а также Esc / клик по overlay).
- Закрытие приостанавливает показ на «частоту» дней.
- Клик по ссылке опроса считается закрытием (пауза = «частота»).

## Хранение настроек

`system_settings` — generic key/value (JSON), схема не меняется → **миграция не нужна**.
Новые ключи:

| Ключ | Тип | Дефолт | Назначение |
|---|---|---|---|
| `feedback_url` | string | `""` | Ссылка на форму опроса |
| `feedback_popup_enabled` | bool | `false` | Вкл/выкл всплывающего окна |
| `feedback_menu_link_enabled` | bool | `false` | Вкл/выкл пункта в гамбургер-меню |
| `feedback_frequency_days` | int | `30` | Интервал охлаждения (дней) |

Если `feedback_url` пустой — ни окно, ни пункт меню не показываются, независимо
от тумблеров (показывать нечего).

## Backend (Go)

### `/api/v1/config` (любой авторизованный пользователь)

Расширить `configResponse` полями:

```json
{
  "feedback_url": "https://...",
  "feedback_popup_enabled": true,
  "feedback_menu_link_enabled": true,
  "feedback_frequency_days": 30
}
```

Это единственный канал, через который `header.js` получает конфиг фидбэка.

### `/api/v1/admin/settings/feedback` (admin)

Новые методы в `internal/http/handlers/api/v1/admin/handler.go` по образцу
general-settings:

- `GET` → `{feedback_url, feedback_popup_enabled, feedback_menu_link_enabled, feedback_frequency_days}`
- `POST` body `{feedback_url, feedback_popup_enabled, feedback_menu_link_enabled, feedback_frequency_days}`

Валидация:
- `feedback_url` — пустой или корректный http(s) (`isValidHTTPURL`);
- `feedback_frequency_days >= 1` (иначе 400).

Регистрация роутов в `internal/http/server.go` рядом с general-settings (≈271-272).

Тесты: `config/handler_test.go`, `admin/handler_test.go`.

## Frontend — окно и пункт меню (`header.js` + `header.css`)

`HeaderNavMenu` уже грузится на всех shell и уже тянет `/api/v1/config`.
Расширить этот fetch (хранить весь конфиг фидбэка в state).

### Пункт меню

«Обратная связь» (иконка 💬) — сразу **под** «Документация».
Условие показа: `feedback_menu_link_enabled && feedback_url`.
Открывает `feedback_url` в новой вкладке (`target=_blank rel=noopener`).

### Модальное окно `FeedbackNudge`

Компонент внутри фрагмента `HeaderNavMenu`. Дизайн в стиле сайта (overlay +
центрированная карточка, палитра/радиусы как у `.nav-menu*`): заголовок,
текст-просьба, primary-кнопка «Поделиться обратной связью» (→ `feedback_url`,
новая вкладка), крестик ✕ справа сверху. Закрытие: крестик, Esc, клик по
overlay — всё считается «закрытием».

Окно показывается на всех страницах инструмента (включая админку).

## Логика показа (трекинг через cookies)

Три cookie (`max-age` ~2 года, `SameSite=Lax`, `path=/`):

- `okr_fb_start` — начало текущего периода вовлечения;
- `okr_fb_seen` — последний визит;
- `okr_fb_dismissed` — последнее закрытие окна.

На каждой загрузке (после получения конфига), `freqMs = feedback_frequency_days * 86400000`:

```
now = Date.now()
если нет okr_fb_start ИЛИ нет okr_fb_seen ИЛИ (now − okr_fb_seen > freqMs):
    okr_fb_start = now          // первый заход ИЛИ возврат после долгого перерыва
okr_fb_seen = now
```

Показываем окно, когда выполнено всё:

```
feedback_popup_enabled && feedback_url
&& (now − okr_fb_start ≥ 2 суток)                                 // grace
&& (нет okr_fb_dismissed ИЛИ now − okr_fb_dismissed ≥ freqMs)     // охлаждение
```

При закрытии (крестик / Esc / overlay / клик по ссылке): `okr_fb_dismissed = now`.

Свойства:
- первый заход — не показываем (grace 2 дня);
- поработал ≥2 дней — показали;
- повтор — не раньше чем через `frequency` дней;
- долгий перерыв (визитов не было дольше `frequency` дней) → `okr_fb_start`
  сбрасывается → снова 2 дня grace перед показом.

Порог «долгого перерыва» намеренно привязан к настройке `frequency`
(без отдельного знача, YAGNI). Константа grace = 2 суток (бизнес-правило, не настраивается).

## Frontend — админка (`admin.js`)

Новый `FeedbackSettingsPanel` (по образцу `GeneralSettingsPanel`) в секции
`settings`: поле ссылки на опрос, два тумблера (окно / пункт меню), числовое
поле частоты (дней). Читает/пишет `/api/v1/admin/settings/feedback`.
Добавляется как ещё одна карточка в блок `section==='settings'`.

## Specs (в том же change set)

- `040-api-contract.md` — новые поля `/api/v1/config` + эндпоинты `/api/v1/admin/settings/feedback`.
- `030-user-flows.md` — флоу сбора обратной связи.
- `050-permissions-and-lifecycle.md` — новые ключи настроек и кто ими управляет (admin).
- Миграции/seed — не трогаем (структура таблиц не меняется).

## Definition of done

- обновлены specs (040, 030, 050);
- расширен config-эндпоинт + новые admin-эндпоинты с валидацией;
- header.js/header.css: пункт меню + модальное окно + cookie-логика;
- admin.js: панель настроек фидбэка;
- тесты на config- и admin-хендлеры;
- `go build ./...`, `go vet ./...`, `go test ./...` зелёные.
