// BalanceBars — переиспользуемый горизонтальный «баланс» целей (полоса + count + %)
// с кликом по полосе для drill-down. Голый глобал (без бандлера) — как ProgressBar.
function BalanceBars({ title, subtitle, items, labels, colors, onSelect }) {
  const max = Math.max(1, ...items.map(i => i.count));
  return (
    <div style={{ flex: '1 1 260px', minWidth: 240 }}>
      <div style={{ fontWeight: 700, fontSize: 14.5, color: '#0f172a' }}>{title}</div>
      {subtitle && <div style={{ fontSize: 12, color: '#64748b', margin: '2px 0 10px' }}>{subtitle}</div>}
      {items.map(it => (
        <div key={it.key} onClick={() => onSelect && it.count > 0 && onSelect(it.key)}
             style={{ display: 'grid', gridTemplateColumns: '130px 1fr 72px', alignItems: 'center',
                      gap: 10, padding: '5px 0', cursor: (onSelect && it.count > 0) ? 'pointer' : 'default' }}>
          <div style={{ fontSize: 13, color: it.count ? '#0f172a' : '#94a3b8' }}>{(labels && labels[it.key]) || it.key}</div>
          <div style={{ height: 10, background: '#eef2f7', borderRadius: 6 }}>
            <div style={{ width: `${(it.count / max) * 100}%`, height: '100%', borderRadius: 6,
                          background: (colors && colors[it.key]) || '#7c6cf0' }} />
          </div>
          <div style={{ fontSize: 13, textAlign: 'right', color: '#334155' }}>
            <b>{it.count}</b> · {it.percent}%
          </div>
        </div>
      ))}
    </div>
  );
}
