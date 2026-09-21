## Why

Персональную сводку Health Check-in ранее заменил обычный колокольчик уведомлений: кнопки ⚡ и
панели сводки в интерфейсе больше нет. Но бэкенд-эндпоинт сводки, отдельная страница настроек в
админке, стили, документация и спецификация `health-checkin` остались. Спецификация описывает
поведение, которым пользователь воспользоваться не может, а часть настроек на странице
«⚡ Health Check-in» на самом деле управляет другими экранами (доской, деревом, обзором периода).
Механику нужно удалить, а нужные пороги перенести туда, где их ожидают найти.

## What Changes

- **BREAKING** Удаляется эндпоинт `GET /api/v1/health-checkin` вместе с расчётом сводки: зоной
  ответственности, категориями и счётчиком проблем.
- **BREAKING** Удаляются эндпоинты `GET/POST /api/v1/admin/settings/health-checkin`, раздел
  «⚡ Health Check-in» в админке и маршрут `/admin/health-checkin`.
- В раздел админки «Настройки» добавляется блок «Пороги прогресса» с эндпоинтами
  `GET/POST /api/v1/admin/settings/progress-thresholds`. В блоке четыре порога, которыми
  пользуются другие экраны:
  - «нет обновлений» (дни);
  - допустимое отставание от темпа;
  - порог «в плане»;
  - допуск по весам.
- Удаляются настройки, нужные только сводке: глубина спуска по комментариям, число решённых
  комментариев, состав счётчика и неиспользуемое время жизни кэша.
- Ключ `health_checkin_config` заменяется четырьмя отдельными ключами настроек пространства.
  Миграция переносит ранее сохранённые значения, поэтому поведение экранов не меняется.
- Ответ `GET /api/v1/config` не меняется: поля `stale_days`, `behind_margin` и `green_threshold`
  остаются и читаются из новых ключей.
- Удаляются неиспользуемые стили `.hci-*`, упоминания Health Check-in в README и `docs/`, а также
  связанные скриншоты.
- Кэш данных периода остаётся без изменения поведения, но переезжает из пакета `healthcheckin`,
  потому что на нём держатся обзор периода и снимки прогресса. Это внутренний рефакторинг.
- Capability `health-checkin` удаляется целиком. Правила, которые нужны оставшимся экранам,
  переносятся в соответствующие capabilities.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `health-checkin`: удаляются все requirements. После archive каталог спецификации удаляется.
- `settings`:
  - добавляется requirement «Пороги оценки прогресса»: редактирование в разделе «Настройки»,
    значения по умолчанию, валидация и доступ;
  - «Производные настройки в интерфейсе участника» больше не ссылается на сводку Health
    Check-in.
- `team-okr-board`: добавляется requirement о предупреждении «нет обновлений» на карточке цели.
  Раньше это правило было записано только в `health-checkin`.
- `progress-tracking`: из «Единые правила расчёта» убирается health check-in как раздел, где
  считается прогресс. Purpose спецификации обновляется при archive так же.

Requirements `period-overview` и `periods`, как и пороговые requirements `progress-tracking`, уже
ссылаются на «настройки пространства» и остаются без изменений.

## Противоречия между кодом и текущими specs

- `health-checkin`, «Счётчик просмотра решённых комментариев считается на клиенте»: клиентской
  отметки просмотра нет, как и точки входа в сводку. Панель удалили вместе с переходом на
  колокольчик, а спецификацию не обновили. Requirement удаляется.
- `health-checkin`, «Настройки Health Check-in»: среди настроек указано «время жизни кэша», но
  сервер его не читает, время жизни кэша задано константой (5 минут). Настройка удаляется.
- `health-checkin`, «Единый порог «нет обновлений» в интерфейсе» обещает одинаковое правило для
  карточки и сводки. На деле карточка считает дни с последнего обновления цели, а сводка считала
  от обновлений прогресса ключевых результатов. Новый requirement в `team-okr-board` описывает
  фактическое поведение карточки.
- Раньше отставание и допуск по весам не проверялись на отрицательные значения. Новый блок
  отклоняет их как ошибку валидации.

## Impact

- **Удаляемый бэкенд:**
  - `internal/service/healthcheckin`, `internal/usecase/healthcheckin`;
  - `internal/http/handlers/api/v1/healthcheckin`;
  - `internal/http/handlers/api/v1/admin/settings/healthcheckin`;
  - регистрация всего этого в `internal/http/server.go`, `internal/http/httpdeps` и
    `internal/http/handlers/web/shell`.
- **Новый бэкенд:**
  - чтение порогов в `internal/service/settings`;
  - обработчик `internal/http/handlers/api/v1/admin/settings/progressthresholds`.
- **Переносимый бэкенд** (кэш периода, загрузчик, вспомогательные функции, настройка интервала
  снимков прогресса) и его потребители:
  - `internal/usecase/period`;
  - `internal/service/settings`;
  - `internal/scheduler`;
  - `internal/http/handlers/api/v1/admin/admincommon`;
  - `internal/http/handlers/api/v1/config`.
- **API:**
  - удаляются `GET /api/v1/health-checkin` и `GET/POST /api/v1/admin/settings/health-checkin`;
  - добавляются `GET/POST /api/v1/admin/settings/progress-thresholds`;
  - `GET /api/v1/config` не меняется.
- **Данные:** миграция 047 переносит четыре значения из `health_checkin_config` в отдельные
  ключи `tenant_settings` и удаляет старый ключ. Seed-комментарий нужно обновить.
- **Frontend:**
  - `web/static/admin.js`: удаляется раздел и `HealthCheckInSettingsPanel`, добавляется блок
    «Пороги прогресса» в раздел «Настройки»;
  - `web/static/tracker.css`: удаляется блок `.hci-*`;
  - комментарии в `web/static/tracker.js` и `web/static/ui.js`.
- **Документация:**
  - `README.md`;
  - `docs/mid-quarter.md`, `docs/images/README.md`;
  - скриншоты `docs/screenshots/admin-health-checkin.png` и `docs/images/health-checkin.png`.
  - Исторические `docs/old_specs` и `docs/superpowers` не меняются.
