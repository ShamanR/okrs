<!-- gortex:communities:start -->
## Community Skills

| Area | Description | Explore |
|------|-------------|---------|
| Service Activity 64 Dirs | 1676 symbols | `analyze(operation:"communities", id:"community-140")` |
| Usecase Keyresult 34 Dirs | 1644 symbols | `analyze(operation:"communities", id:"community-157")` |
| Auth 40 Dirs | 1365 symbols | `analyze(operation:"communities", id:"community-287")` |
| Auth 67 Dirs | 1337 symbols | `analyze(operation:"communities", id:"community-76")` |
| Notifications Preferences 72 Dirs | 1269 symbols | `analyze(operation:"communities", id:"community-92")` |
| Usecase Period 38 Dirs | 1197 symbols | `analyze(operation:"communities", id:"community-153")` |
| Store Goals 10 Dirs | 854 symbols | `analyze(operation:"communities", id:"community-195")` |
| Http Dto 44 Dirs | 794 symbols | `analyze(operation:"communities", id:"community-82")` |
| Static 2 Dirs Map | 631 symbols | `analyze(operation:"communities", id:"community-36")` |
| Platform Logging 14 Dirs | 583 symbols | `analyze(operation:"communities", id:"community-286")` |
| Store Settings 20 Dirs | 553 symbols | `analyze(operation:"communities", id:"community-156")` |
| Render Export 16 Dirs | 510 symbols | `analyze(operation:"communities", id:"community-275")` |
| V1 Goals 9 Dirs | 498 symbols | `analyze(operation:"communities", id:"community-284")` |
| Platform Eventbus 12 Dirs | 450 symbols | `analyze(operation:"communities", id:"community-278")` |
| V1 Krs 14 Dirs | 448 symbols | `analyze(operation:"communities", id:"community-86")` |
| Core Progress 17 Dirs | 397 symbols | `analyze(operation:"communities", id:"community-230")` |
| Store Notifications | 354 symbols | `analyze(operation:"communities", id:"community-184")` |
| Activity Purge 15 Dirs | 263 symbols | `analyze(operation:"communities", id:"community-73")` |
| Admin Accessrequests 54 Dirs | 256 symbols | `analyze(operation:"communities", id:"community-94")` |
| Usecase Deliver | 246 symbols | `analyze(operation:"communities", id:"community-219")` |

<!-- gortex:communities:end -->

## Дизайн интерфейса и дизайн-система

> Этот раздел добавлен вручную. Если файл перегенерируется Gortex — восстановить
> из `design-system/README.md`, где те же правила продублированы.

Дизайн-система живёт в `design-system/`: карточки компонентов в `src/cards/` и
кликабельный прототип в `prototype/`. Это рабочий инструмент, а не нормативный
документ: поведение продукта описывает только `openspec/specs/`, и в OpenSpec
дизайн-система не описывается.

Превью инлайнят реальные `web/static/*.css` при сборке — стили не дублируются.

| что изменилось | что делать |
| --- | --- |
| только стили в `web/static/*.css` | ничего, достаточно пересобрать |
| разметка существующего компонента | обновить карточку в `design-system/src/cards/`, а если компонент есть в прототипе — и `design-system/prototype/` |
| появился новый переиспользуемый компонент | завести карточку, иначе он выпадет из системы |
| компонент удалён | удалить карточку |

После правок:

```sh
./design-system/build.sh
./design-system/prototype/build.sh
```

и залить результат в проект Claude Design (id в `design-system/README.md`).

Разметка карточек и прототипа повторяет продуктовые классы. Собственные классы
для продуктовых элементов там не заводятся: всё, что не является частью
продукта, помечается префиксом `.ds-` (карточки) или `.pt-` (прототип).

### Связь с OpenSpec

Если OpenSpec change затрагивает интерфейс:

- в `design.md` — раздел «Интерфейс»: какие компоненты дизайн-системы
  переиспользуются, какие добавляются, и ссылка на карточку или экран
  прототипа, по которому согласован вид;
- в `tasks.md` — пункт на согласование вида в прототипе **до** реализации
  и пункт на обновление дизайн-системы **после**;
- вид согласуется на прототипе, а не на скриншотах: прототип собран на
  реальном CSS и не расходится со стилями приложения.

Скриншоты в change добавляются только как историческая фиксация при архивации,
когда важно сохранить, как решение выглядело на момент принятия. Требовать их
в каждом change не нужно: они устаревают и не поддерживаются — в
`docs/screenshots/` уже лежат 13 файлов, на которые нет ни одной ссылки из
`docs/*.md`.
