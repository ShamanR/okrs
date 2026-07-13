# Stale-предупреждение только для статуса «в работе»

Дата: 2026-07-13

## Проблема

Колокольчик Health Check-in в сайдбаре показывает `total_problems` — сумму
категорий с `in_counter: true`. Категория `stale` («N дней без обновления»)
подсвечивается как проблема даже для целей команд, которые ещё не переведены в
статус «в работе» (`in_progress`).

Корень — денилист в `computeCategories`:

```go
trackStale := status != domain.TeamPeriodStatusForming && status != domain.TeamPeriodStatusReady
```

Он исключает только `forming` и `ready`. Для команды **без записи** в
`team_period_statuses` (нет строки за период) `data.Statuses[teamID]` возвращает
zero-value `""`, и условие `"" != forming && "" != ready` истинно → stale-цели
такой (ещё не запущенной) команды попадают в счётчик колокольчика.

В остальном коде отсутствующий статус дефолтится в `no_goals`
(`service.go:395`, `service.go:566`), но в health check-in этот дефолт не
применяется.

## Решение

Заменить денилист на позитивное правило: категория `stale` считается проблемой
**только когда статус команды за период = `in_progress`**.

Это:
- закрывает дыру с пустым/отсутствующим статусом (главная причина бага);
- сохраняет исключение `forming` / `ready` (черновик и к валидации);
- убирает шум по `closed` — «нет обновлений» не имеет смысла для завершённого
  периода; соответствует формулировке «пока цели не переведены в работу».

## Изменения

1. **Backend** — `internal/service/healthcheckin.go`:
   `trackStale := status == domain.TeamPeriodStatusInProgress`, обновить
   комментарий.
2. **Frontend** — `internal/web/static/tracker.js` (`GoalCard`):
   `staleTracked = periodStatus === 'in_progress'`. Спека требует «то же
   правило» для предупреждения на карточке цели → держим бэкенд и фронтенд
   консистентными. `periodStatus` на фронте дефолтится в `no_goals`, пустым не
   бывает, поэтому позитивное правило безопасно.
3. **Spec** — `specs/040-api-contract.md` (правило stale): переписать в
   позитивную формулировку — считается только для `in_progress`.
4. **Tests** — `internal/service/healthcheckin_test.go`: добавить кейсы
   «пустой статус → 0 stale» и «closed → 0 stale»; существующие кейсы
   (`in_progress` → 1, `forming`/`ready` → 0) остаются валидными.

## Вне scope

- Категории `formation_errors`, `awaiting_validation`, `lagging`, `no_goals` —
  их логика по статусам не меняется.
- Схема таблиц не меняется → seed/demo обновлять не нужно.
