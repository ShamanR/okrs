# Design: KR Description Display

**Date:** 2026-05-28
**Scope:** UI-only change in `tracker.js` + `tracker.css`

## Problem

`KeyResult.description` существует в доменной модели и возвращается API, но не отображается в `KRRow`. Многострочные описания при рендере не должны ломать flex-layout строки.

## Solution

Добавить рендер `kr.desc` под `kr.name` внутри `.kr-info` в компоненте `KRRow`.

### Компонент (tracker.js)

Внутри `.kr-info`, после `<div className="kr-name">`:

```jsx
{kr.desc && <div className="kr-desc">{kr.desc}</div>}
```

- Условный рендер: показывается только если `kr.desc` непустой.
- Текст вставляется как React text node — без `dangerouslySetInnerHTML` (архитектурное правило #8).

### CSS (tracker.css)

```css
.kr-desc {
  font-size: 11px;
  color: #6b7280;
  white-space: pre-wrap;
  word-break: break-word;
  margin-top: 2px;
  line-height: 1.4;
}
```

- `white-space: pre-wrap` — сохраняет переносы строк из поля description, не ломает layout.
- `word-break: break-word` — предотвращает горизонтальный overflow при длинных строках без пробелов.
- Цвет и размер согласованы с существующей системой: приглушённый серый `#6b7280`, мелкий шрифт.

## Constraints / Non-goals

- Нет изменений API, бэкенда, миграций.
- Нет изменений спек (domain model и API contract уже описывают `description`).
- Нет изменений в `KREditModal` — поле description там уже есть.

## Files changed

- `internal/web/static/tracker.js` — `KRRow`: добавить `kr.desc` рендер
- `internal/web/static/tracker.css` — добавить `.kr-desc`
