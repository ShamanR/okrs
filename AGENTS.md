<!-- gortex:communities:start -->
## Community Skills

| Area | Description | Explore |
| ------ | ------------- | --------- |
| Usecase Keyresult 34 Dirs | 1699 symbols | `analyze(operation:"communities", id:"community-159")` |
| Auth 67 Dirs | 1326 symbols | `analyze(operation:"communities", id:"community-70")` |
| Service Activity 63 Dirs | 1276 symbols | `analyze(operation:"communities", id:"community-139")` |
| Auth 42 Dirs | 1274 symbols | `analyze(operation:"communities", id:"community-285")` |
| Accessrequests Approve 70 Dirs | 1246 symbols | `analyze(operation:"communities", id:"community-87")` |
| Usecase Period 26 Dirs | 839 symbols | `analyze(operation:"communities", id:"community-153")` |
| Static 2 Dirs | 783 symbols | `analyze(operation:"communities", id:"community-34")` |
| Store Settings 19 Dirs | 662 symbols | `analyze(operation:"communities", id:"community-156")` |
| Store Activity 6 Dirs | 647 symbols | `analyze(operation:"communities", id:"community-197")` |
| Http Dto 43 Dirs | 643 symbols | `analyze(operation:"communities", id:"community-76")` |
| V1 Goals 10 Dirs | 579 symbols | `analyze(operation:"communities", id:"community-281")` |
| Platform Logging 13 Dirs | 577 symbols | `analyze(operation:"communities", id:"community-284")` |
| Usecase Notification 17 Dirs | 507 symbols | `analyze(operation:"communities", id:"community-158")` |
| Store Goals 5 Dirs | 506 symbols | `analyze(operation:"communities", id:"community-170")` |
| Core Progress 22 Dirs | 486 symbols | `analyze(operation:"communities", id:"community-228")` |
| Platform Eventbus 12 Dirs | 465 symbols | `analyze(operation:"communities", id:"community-143")` |
| V1 Goals 12 Dirs | 439 symbols | `analyze(operation:"communities", id:"community-80")` |
| Mattermost 7 Dirs | 396 symbols | `analyze(operation:"communities", id:"community-273")` |
| Store Notifications 2 Dirs | 376 symbols | `analyze(operation:"communities", id:"community-188")` |
| Usecase Goal 4 Dirs | 312 symbols | `analyze(operation:"communities", id:"community-212")` |

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
