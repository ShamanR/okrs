# Заметки по синхронизации дизайн-системы

- Репозиторий вне конвертера `/design-sync`: нет компонентного пакета и Storybook.
  Дизайн-система — standalone HTML-превью (`design-system/src/cards/`), которые при
  сборке инлайнят реальные `web/static/*.css`. Порядок сборки и заливки описан в
  `design-system/README.md`.
- Соответствие путей: `dist/components/<name>.html` → `components/<name>.html`,
  `dist/prototype.html` → `prototype/tracker.html`.
- `dist/prototype.local.html` **не заливается**: это вариант для локального просмотра
  с собственными `<!DOCTYPE>`/`<html>`, а артефактный движок оборачивает страницу сам.
- `dist/cards.json` не заливается: панель Design System строит индекс из маркеров
  `@dsCard` в первой строке каждой карточки.
- В проекте есть файлы, которыми управляет само приложение — `_ds_bundle.js`,
  `_ds_manifest.json`, `_adherence.oxlintrc.json`. Их синк не трогает.
- Авторизация DesignSync работает из сессии VSCode: отдельный `/design-login`
  в терминале не потребовался (проверено 2026-09-29).
