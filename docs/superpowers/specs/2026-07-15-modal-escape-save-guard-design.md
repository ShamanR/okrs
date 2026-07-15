# Закрытие модалок по Escape с guard несохранённых изменений

Дата: 2026-07-15

## Проблема

Поведение закрытия модальных окон в приложении неоднородно и неполно:

- Модалки трекера (`KRProgressModal`, `KREditModal`, `GoalModal`, `ConfirmModal`
  в `internal/web/static/tracker.js`) построены на хуке `useOverlayClose` и
  закрываются **только кликом по оверлею** — Escape не обрабатывается вообще.
- Общий компонент `Modal` в `internal/web/static/admin.js` закрывается по
  Escape, но **сразу**, без предупреждения о несохранённых правках — легко
  потерять заполненную форму.

Требования (единые для всех модальных окон приложения):

1. Модалка закрывается по Escape.
2. Если в форме есть несохранённые изменения — при попытке закрытия появляется
   окно-подтверждение «Сохранить изменения?»: Enter — сохранить, повторный
   Escape — выйти без сохранения.
3. Поведение единообразно для всех модалок.

## Решение

Вынести общую логику закрытия в переиспользуемый хук и общий компонент
подтверждения, чтобы поведение и внешний вид совпадали в трекере и админке
(consistency, CLAUDE.md #12). Обе страницы уже грузят `ui.js` и `components.css`
— это общий дом для новых сущностей.

### Компоненты

1. **`useModalClose({ isDirty, canSave = true, onSave, onClose })`** — хук в
   `internal/web/static/ui.js`. Владеет всей механикой закрытия:
   - Ставит один `keydown`-листенер на `document`.
   - Ведёт **модульный стек модалок** (push при монтировании, pop при
     размонтировании); на клавиши реагирует только верхняя модалка стека.
   - Блокирует скролл body (`overflow: hidden`), восстанавливает при закрытии.
   - Держит состояние `confirming` (показывается ли окно-подтверждение).
   - Возвращает `{ requestClose, confirming, confirmEl }`.

   **State machine `requestClose()`** (вызывается из Escape, крестика ×, клика по
   оверлею):
   - `confirming === true` → игнор (окном-подтверждением управляет его
     собственная обработка клавиш).
   - `!isDirty` → сразу `onClose()`.
   - `isDirty` → `setConfirming(true)`.

   **Обработка клавиш** (только когда модалка — верх стека и `!e.defaultPrevented`):
   - `confirming === false`: `Escape` → `requestClose()`.
   - `confirming === true`:
     - `Enter` → если `canSave`: `onSave()` (затем закрытие по завершении);
       иначе no-op. `preventDefault`.
     - `Escape` → закрыть без сохранения (`onClose()`). `preventDefault`.

2. **`<ModalCloseConfirm>`** — общий презентационный компонент в `ui.js`,
   рендерит окно-подтверждение поверх модалки. Стили — классы `.modal-confirm*`
   в `components.css` (грузится и трекером, и админкой). Заголовок «Есть
   несохранённые изменения», подзаголовок «Сохранить изменения перед выходом?»,
   строка-подсказка «**Enter** — сохранить и выйти · **Esc** — выйти без
   сохранения». Кнопки: «Отмена» (вернуться к форме), «Не сохранять» (danger-soft,
   красная — Escape), «Сохранить» (primary, Enter, disabled при `!canSave`).
   Автофокус на «Сохранить». Клик вне окошка → «Отмена».

### Определение `isDirty`

Сравнение текущего состояния формы со снапшотом начальных значений:

- **Admin-редакторы** (`PeriodModalBody`, `TeamEditor`, `UserModal`) уже имеют
  объект `initial` → `isDirty = JSON.stringify(f) !== JSON.stringify(initial)`.
- **Tracker-модалки**: снапшот исходной формы (через `useRef` при первом
  рендере) + черновики. В `KRProgressModal` учитываются `form`, `note`, и
  `descDraft` (когда `descEditing`).

### Охват

| Модалка | Escape закрывает | Guard сохранения |
|---|---|---|
| `KRProgressModal`, `KREditModal`, `GoalModal` (tracker) | да | да (по `isDirty`) |
| Admin `Modal` → `PeriodModalBody` / `TeamEditor` / `UserModal` | да | да (по `isDirty`) |
| `ConfirmModal` (удаление) | да | нет (нет редактируемых полей) |
| Feedback-nudge (`sidebar.js`) | уже закрывается по Escape | — (не форма) |

## Изменения

1. **`internal/web/static/ui.js`** — добавить `useModalClose` (хук + модульный
   стек модалок) и `<ModalCloseConfirm>`.

2. **`internal/web/static/components.css`** — добавить классы `.modal-confirm*`
   для окна-подтверждения (переиспользуют визуальный язык `ConfirmModal`).

3. **`internal/web/static/tracker.js`**:
   - `KRProgressModal`, `KREditModal`, `GoalModal` — перевести с `useOverlayClose`
     на `useModalClose`; вычислять `isDirty` и `canSave`; крестик/оверлей →
     `requestClose`; рендерить `confirmEl`.
   - `ConfirmModal` — перевести на `useModalClose` без `isDirty` (Escape просто
     закрывает).
   - Вложенные выпадашки внутри модалок (`TeamCombobox` и кастомные
     комбобоксы/селекты, обрабатывающие Escape) — вызывать `e.preventDefault()`
     при «поедании» Escape, чтобы хук их пропускал (Escape закрывает дропдаун,
     а не модалку). Нативные `<select>` обрабатываются браузером.

4. **`internal/web/static/admin.js`**:
   - `Modal` — делегировать все интенты закрытия (×, оверлей, Escape) в
     `requestClose` из `useModalClose`; убрать собственный `keydown`/scroll-lock,
     когда используется guard (их берёт хук). Проброс `isDirty`/`canSave`/`onSave`
     из тела редактора (через `closeRef`-мост, детали — в плане).
   - `PeriodModalBody`, `TeamEditor`, `UserModal` — вычислять `isDirty`/`canSave`,
     подключить хук, рендерить `confirmEl`.
   - `TeamCombobox` (admin) — `e.preventDefault()` при закрытии дропдауна по Escape.

5. **Tests** — фронтенд-логики автотестов в проекте нет (server-rendered React
   без бандлера/тест-раннера). Поведение проверяется вручную по чек-листу (см.
   «Проверка»). Go-тесты не затрагиваются.

## Проверка (ручная)

- Escape на чистой форме → закрывается сразу.
- Escape/×/клик по оверлею на изменённой форме → окно «Сохранить изменения?».
- В окне: Enter → сохраняет и закрывает; Escape → выходит без сохранения;
  «Продолжить редактирование»/клик вне → возврат к форме.
- Невалидная форма (`canSave=false`) + Enter в окне → «Сохранить» неактивна,
  ничего не происходит.
- Escape в открытом дропдансе (`TeamCombobox`) закрывает дропдаун, а не модалку.
- Удаление (`ConfirmModal`): Escape закрывает без окна-подтверждения.
- Вложенность (`ConfirmModal` поверх формы): Escape закрывает только верхнюю.

## Вне scope

- Полный focus-trap и ARIA-разметка (кроме автофокуса на кнопку подтверждения).
- `system.js` продолжает использовать нативный `confirm()` для деструктивных
  системных действий — это не модальные окна приложения.
- Схема таблиц не меняется → seed/demo и product-specs не затрагиваются.
