// Shared Markdown render + edit module.
// Loaded as text/babel BEFORE tracker.js / admin.js, so these globals are
// available to both React apps. Depends on globals: React, marked, DOMPurify.

// Rendered output is restricted to a small, safe subset. Anything outside the
// allowlist (scripts, images, iframes, tables, etc.) is stripped to plain text.
const MD_ALLOWED_TAGS = ['p', 'br', 'strong', 'em', 'b', 'i', 'ul', 'ol', 'li', 'a',
  'h1', 'h2', 'h3', 'blockquote', 'code', 'pre'];
const MD_ALLOWED_ATTR = ['href', 'target', 'rel'];

// Force every link to open safely in a new tab. Runs once at load.
if (typeof DOMPurify !== 'undefined' && DOMPurify.addHook) {
  DOMPurify.addHook('afterSanitizeAttributes', node => {
    if (node.tagName === 'A') {
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer');
    }
  });
}

function renderMarkdown(text) {
  if (!text) return '';
  const raw = marked.parse(String(text), { breaks: true, gfm: true });
  return DOMPurify.sanitize(raw, {
    ALLOWED_TAGS: MD_ALLOWED_TAGS,
    ALLOWED_ATTR: MD_ALLOWED_ATTR,
    ALLOWED_URI_REGEXP: /^(?:https?:|mailto:)/i,
  });
}

// Read component: renders sanitized Markdown HTML. Renders null for empty text.
function Markdown({ text, className }) {
  const html = renderMarkdown(text);
  if (!html) return null;
  return React.createElement('div', {
    className: 'md-content' + (className ? ' ' + className : ''),
    dangerouslySetInnerHTML: { __html: html },
  });
}

// Read component with collapse: same sanitized HTML as <Markdown>, but the
// rendered height is capped and the rest is revealed on demand. The height cap
// itself lives in CSS (--md-collapse-max on the host class), so the markup is
// never truncated: long descriptions stay whole in the DOM and keep their list,
// heading and paragraph structure in the collapsed view.
//
// The collapsed state is deliberately ephemeral - it is not persisted anywhere,
// so a reload, a team switch or a period switch brings every description back
// collapsed.
function CollapsibleMarkdown({ text, className }) {
  const html = renderMarkdown(text);
  const bodyRef = React.useRef(null);
  const [expanded, setExpanded] = React.useState(false);
  const [overflows, setOverflows] = React.useState(false);

  // A rewritten description comes back collapsed, like every other one.
  React.useEffect(() => { setExpanded(false); }, [text]);

  // Whether there is anything hidden can only be measured, not guessed: the same
  // text renders to a different height depending on its markup, the card width
  // and where lines wrap. Measuring is skipped while expanded - the cap is lifted
  // then, so scrollHeight would always equal clientHeight - and the toggle stays
  // visible anyway to collapse back.
  React.useLayoutEffect(() => {
    const el = bodyRef.current;
    if (!el) return undefined;
    // Everything the sanitized markup can make focusable: links, and <pre> blocks,
    // which the browser treats as focusable scrollers when their lines overflow.
    const focusables = el.querySelectorAll('a[href], pre');
    if (expanded) {
      focusables.forEach(n => n.removeAttribute('tabindex'));
      return undefined;
    }
    const measure = () => {
      // 1px of slack absorbs sub-pixel rounding of the em-based height cap.
      setOverflows(el.scrollHeight - el.clientHeight > 1);
      // overflow:hidden clips only visually: a link below the cut is still a Tab
      // stop, and focusing it scrolls this box, showing a shifted slice while the
      // description still reads as collapsed. Keep the cut-off part out of the tab
      // order; the toggle is the keyboard way in.
      const cut = el.getBoundingClientRect().bottom + 1;
      focusables.forEach(n => {
        if (n.getBoundingClientRect().bottom > cut) n.setAttribute('tabindex', '-1');
        else n.removeAttribute('tabindex');
      });
    };
    measure();
    if (typeof ResizeObserver === 'undefined') return undefined;
    // Re-measure on width changes (window resize, sidebar collapse) and once
    // webfonts settle, both of which change the rendered height.
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [html, expanded]);

  if (!html) return null;
  const clipped = overflows && !expanded;
  const showToggle = overflows || expanded;
  return React.createElement('div', {
    className: 'md-collapsible'
      + (clipped ? ' md-collapsible--clipped' : '')
      + (expanded ? ' md-collapsible--expanded' : ''),
  },
    React.createElement('div', {
      ref: bodyRef,
      className: 'md-collapsible__body md-content' + (className ? ' ' + className : ''),
      dangerouslySetInnerHTML: { __html: html },
    }),
    showToggle && React.createElement('button', {
      type: 'button',
      className: 'md-collapsible__toggle',
      'aria-expanded': expanded,
      // The host card reacts to clicks (open the editor) and to drags (reorder);
      // neither must fire from this control. A draggable ancestor cannot be stopped
      // from here — dragstart is dispatched at the ancestor and never reaches this
      // button — so the ancestor checks for [data-no-drag] on mousedown instead.
      draggable: false,
      'data-no-drag': '',
      onDragStart: e => { e.preventDefault(); e.stopPropagation(); },
      onMouseDown: e => e.stopPropagation(),
      onClick: e => { e.stopPropagation(); setExpanded(v => !v); },
    },
      React.createElement('span', { className: 'md-collapsible__caret' }, expanded ? '\u25b2' : '\u25bc'),
      React.createElement('span', { className: 'md-collapsible__label' }, expanded ? 'Свернуть' : 'Показать полностью'))
  );
}

// Apply a markdown transform to the current textarea selection.
function applyMarkdownFormat(el, kind, onChange) {
  const start = el.selectionStart;
  const end = el.selectionEnd;
  const value = el.value;
  const sel = value.slice(start, end);
  let inserted;
  let selFrom;
  let selTo;
  if (kind === 'bold' || kind === 'italic' || kind === 'code') {
    const mark = kind === 'bold' ? '**' : kind === 'italic' ? '*' : '`';
    const inner = sel || (kind === 'bold' ? 'жирный' : kind === 'italic' ? 'курсив' : 'код');
    inserted = mark + inner + mark;
    selFrom = start + mark.length;
    selTo = selFrom + inner.length;
  } else if (kind === 'link') {
    const label = sel || 'текст';
    inserted = '[' + label + '](url)';
    selFrom = start + inserted.length - 4; // caret over "url"
    selTo = selFrom + 3;
  } else if (kind === 'ul' || kind === 'ol' || kind === 'quote' || kind === 'heading') {
    const placeholder = kind === 'quote' ? 'цитата' : kind === 'heading' ? 'заголовок' : 'пункт';
    const lines = (sel || placeholder).split('\n');
    const prefix = i => kind === 'ul' ? '- ' : kind === 'ol' ? (i + 1) + '. ' : kind === 'quote' ? '> ' : '# ';
    inserted = lines.map((l, i) => prefix(i) + l).join('\n');
    selFrom = start;
    selTo = start + inserted.length;
  } else {
    return;
  }
  const next = value.slice(0, start) + inserted + value.slice(end);
  onChange(next);
  // Restore focus/selection after React re-render.
  requestAnimationFrame(() => {
    el.focus();
    el.setSelectionRange(selFrom, selTo);
  });
}

// Monochrome inline icons (reusable immutable elements).
function mdIconList() {
  return React.createElement('svg', { width: 15, height: 15, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round' },
    React.createElement('line', { x1: 8, y1: 6, x2: 20, y2: 6 }),
    React.createElement('line', { x1: 8, y1: 12, x2: 20, y2: 12 }),
    React.createElement('line', { x1: 8, y1: 18, x2: 20, y2: 18 }),
    React.createElement('line', { x1: 3.5, y1: 6, x2: 3.51, y2: 6 }),
    React.createElement('line', { x1: 3.5, y1: 12, x2: 3.51, y2: 12 }),
    React.createElement('line', { x1: 3.5, y1: 18, x2: 3.51, y2: 18 })
  );
}
function mdIconLink() {
  return React.createElement('svg', { width: 14, height: 14, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round' },
    React.createElement('path', { d: 'M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71' }),
    React.createElement('path', { d: 'M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71' })
  );
}

const MD_TOOLBAR = [
  { kind: 'bold', node: 'B', title: 'Жирный', style: { fontWeight: 800 } },
  { kind: 'italic', node: 'I', title: 'Курсив', style: { fontStyle: 'italic', fontFamily: 'Georgia, "Times New Roman", serif' } },
  { kind: 'heading', node: 'H', title: 'Заголовок', style: { fontWeight: 700 } },
  'divider',
  { kind: 'ul', node: mdIconList(), title: 'Маркированный список', style: {} },
  { kind: 'ol', node: '1.', title: 'Нумерованный список', style: { fontWeight: 600, fontSize: 12 } },
  'divider',
  { kind: 'quote', node: '❝', title: 'Цитата', style: { fontSize: 15, lineHeight: 1 } },
  { kind: 'code', node: '</>', title: 'Код', style: { fontFamily: 'ui-monospace, Menlo, monospace', fontSize: 11, fontWeight: 600 } },
  { kind: 'link', node: mdIconLink(), title: 'Ссылка', style: {} },
];

const MD_HINT = ': **жирный**, *курсив*, # заголовок, - список, [ссылка](url), `код`';

// Write component: toolbar + textarea/preview + markdown hint.
function MarkdownEditor({ value, onChange, rows, placeholder, textareaClassName, textareaStyle, onKeyDown }) {
  const [preview, setPreview] = React.useState(false);
  const ref = React.useRef(null);
  const val = value || '';
  const fmt = kind => { if (ref.current) applyMarkdownFormat(ref.current, kind, onChange); };
  return React.createElement('div', { className: 'md-editor' },
    React.createElement('div', { className: 'md-editor__bar' },
      React.createElement('div', { className: 'md-editor__tools' },
        MD_TOOLBAR.map((b, i) => b === 'divider'
          ? React.createElement('span', { key: 'd' + i, className: 'md-editor__divider' })
          : React.createElement('button', {
              key: b.kind, type: 'button', className: 'md-editor__btn', title: b.title,
              style: b.style, disabled: preview,
              onMouseDown: e => e.preventDefault(), // keep textarea selection
              onClick: () => fmt(b.kind),
            }, b.node))
      ),
      React.createElement('div', { className: 'md-editor__tabs' },
        React.createElement('button', {
          type: 'button', className: 'md-editor__tab' + (!preview ? ' md-editor__tab--active' : ''),
          onClick: () => setPreview(false),
        }, 'Написать'),
        React.createElement('button', {
          type: 'button', className: 'md-editor__tab' + (preview ? ' md-editor__tab--active' : ''),
          onClick: () => setPreview(true),
        }, 'Просмотр')
      )
    ),
    preview
      ? React.createElement('div', { className: 'md-editor__preview', style: textareaStyle },
          val.trim()
            ? React.createElement(Markdown, { text: val })
            : React.createElement('div', { className: 'md-editor__empty' }, 'Нечего показать'))
      : React.createElement('textarea', {
          ref,
          value: val,
          onChange: e => onChange(e.target.value),
          onKeyDown,
          rows: rows || 3,
          placeholder: placeholder || '',
          className: textareaClassName || 'form-textarea',
          style: textareaStyle,
        }),
    !preview && React.createElement('div', { className: 'md-editor__hint' },
      'Поддерживается ', React.createElement('strong', null, 'Markdown'), MD_HINT)
  );
}
