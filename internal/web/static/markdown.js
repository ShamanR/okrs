// Shared Markdown render + edit module.
// Loaded as text/babel BEFORE tracker.js / admin.js, so these globals are
// available to both React apps. Depends on globals: React, marked, DOMPurify.

// Restrict rendered output to the agreed minimal feature set. Anything richer
// (headings, code blocks, images, scripts, tables) is stripped to plain text.
const MD_ALLOWED_TAGS = ['p', 'br', 'strong', 'em', 'b', 'i', 'ul', 'ol', 'li', 'a'];
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

// Apply a markdown transform to the current textarea selection.
function applyMarkdownFormat(el, kind, onChange) {
  const start = el.selectionStart;
  const end = el.selectionEnd;
  const value = el.value;
  const sel = value.slice(start, end);
  let inserted;
  let selFrom;
  let selTo;
  if (kind === 'bold' || kind === 'italic') {
    const mark = kind === 'bold' ? '**' : '*';
    const inner = sel || (kind === 'bold' ? 'жирный' : 'курсив');
    inserted = mark + inner + mark;
    selFrom = start + mark.length;
    selTo = selFrom + inner.length;
  } else if (kind === 'ul' || kind === 'ol') {
    const lines = (sel || 'пункт').split('\n');
    inserted = lines.map((l, i) => (kind === 'ul' ? '- ' : (i + 1) + '. ') + l).join('\n');
    selFrom = start;
    selTo = start + inserted.length;
  } else if (kind === 'link') {
    const label = sel || 'текст';
    inserted = '[' + label + '](url)';
    selFrom = start + inserted.length - 4; // caret over "url"
    selTo = selFrom + 3;
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

const MD_TOOLBAR = [
  { kind: 'bold', label: 'B', title: 'Жирный', style: { fontWeight: 800 } },
  { kind: 'italic', label: 'I', title: 'Курсив', style: { fontStyle: 'italic' } },
  { kind: 'ul', label: '•', title: 'Маркированный список', style: {} },
  { kind: 'ol', label: '1.', title: 'Нумерованный список', style: {} },
  { kind: 'link', label: '🔗', title: 'Ссылка', style: {} },
];

// Write component: toolbar + textarea + preview toggle.
function MarkdownEditor({ value, onChange, rows, placeholder, textareaClassName, textareaStyle }) {
  const [preview, setPreview] = React.useState(false);
  const ref = React.useRef(null);
  const val = value || '';
  const fmt = kind => { if (ref.current) applyMarkdownFormat(ref.current, kind, onChange); };
  return React.createElement('div', { className: 'md-editor' },
    React.createElement('div', { className: 'md-editor__bar' },
      React.createElement('div', { className: 'md-editor__tools' },
        MD_TOOLBAR.map(b => React.createElement('button', {
          key: b.kind, type: 'button', className: 'md-editor__btn', title: b.title,
          style: b.style, disabled: preview,
          onMouseDown: e => e.preventDefault(), // keep textarea selection
          onClick: () => fmt(b.kind),
        }, b.label))
      ),
      React.createElement('button', {
        type: 'button',
        className: 'md-editor__tab' + (preview ? ' md-editor__tab--active' : ''),
        onClick: () => setPreview(p => !p),
      }, preview ? 'Редактор' : 'Превью')
    ),
    preview
      ? React.createElement('div', { className: 'md-editor__preview' },
          val.trim()
            ? React.createElement(Markdown, { text: val })
            : React.createElement('div', { className: 'md-editor__empty' }, 'Нечего показать'))
      : React.createElement('textarea', {
          ref,
          value: val,
          onChange: e => onChange(e.target.value),
          rows: rows || 3,
          placeholder: placeholder || '',
          className: textareaClassName || 'form-textarea',
          style: textareaStyle,
        })
  );
}
