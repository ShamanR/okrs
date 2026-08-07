// ProgressChart — прогресс целей за период во времени. Голый глобал (без бандлера).
// Ось X — даты старт→конец периода, ось Y — 0–100%. Пунктирная диагональ — ориентир
// ровного заполнения. Точки до старта / после конца периода клампятся к краям и
// рисуются ромбами. Наведение на точку подсвечивает её и показывает дату замера.

// Короткая русская дата замера: "2026-08-05" → "5 авг 2026".
const PC_MONTHS = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];
function pcFormatDate(iso) {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso || '');
  if (!m) return iso || '';
  return `${parseInt(m[3], 10)} ${PC_MONTHS[parseInt(m[2], 10) - 1]} ${m[1]}`;
}

function ProgressChart({ series }) {
  const [hover, setHover] = React.useState(null); // индекс точки под курсором
  if (!series || !series.points || !series.points.length) {
    return <div style={{ color: '#94a3b8', padding: 24, fontSize: 13 }}>Нет данных о прогрессе за период.</div>;
  }
  const W = 900, H = 320, padL = 40, padR = 20, padT = 20, padB = 30;
  const x0 = padL, x1 = W - padR, y0 = H - padB, y1 = padT;
  const start = Date.parse(series.period_start), end = Date.parse(series.period_end);
  const span = Math.max(1, end - start);
  const xOf = (dateStr) => {
    const t = Date.parse(dateStr);
    const clamped = Math.min(Math.max(t, start), end); // до старта / после конца → край
    return x0 + ((clamped - start) / span) * (x1 - x0);
  };
  const yOf = (p) => y0 + (Math.min(Math.max(p, 0), 100) / 100) * (y1 - y0);
  const pts = series.points.map(pt => ({
    x: xOf(pt.date), y: yOf(pt.progress),
    edge: Date.parse(pt.date) < start || Date.parse(pt.date) > end,
    p: pt.progress, date: pt.date,
  }));
  // Линия всегда начинается из левого нижнего угла (старт периода, 0%), а не от
  // первой точки прогресса.
  const origin = { x: x0, y: y0 };
  const linePts = [origin, ...pts];
  const line = linePts.map((p, i) => `${i ? 'L' : 'M'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
  const last = pts[pts.length - 1];
  const area = `${line} L${last.x.toFixed(1)},${y0} L${x0},${y0} Z`;
  const grid = [0, 25, 50, 75, 100];

  const hp = hover != null ? pts[hover] : null;
  const tipLabel = hp ? `${pcFormatDate(hp.date)} · ${hp.p}%` : '';
  const tipW = Math.max(70, tipLabel.length * 6.6 + 16);
  const tipX = hp ? Math.min(Math.max(hp.x - tipW / 2, x0), x1 - tipW) : 0;
  const tipY = hp ? Math.max(hp.y - 34, y1) : 0;

  return (
    <svg viewBox={`0 0 ${W} ${H}`} style={{ width: '100%', height: 'auto' }}>
      {grid.map(g => (
        <g key={g}>
          <line x1={x0} y1={yOf(g)} x2={x1} y2={yOf(g)} stroke="#eef2f7" />
          <text x={x0 - 6} y={yOf(g) + 3} textAnchor="end" fontSize="10" fill="#94a3b8">{g}%</text>
        </g>
      ))}
      <line x1={x0} y1={y0} x2={x1} y2={y1} stroke="#cbd5e1" strokeDasharray="4 4" />
      <path d={area} fill="rgba(124,108,240,0.10)" />
      <path d={line} fill="none" stroke="#7c6cf0" strokeWidth="2" />

      {/* Вертикальная направляющая до точки под курсором. */}
      {hp && <line x1={hp.x} y1={y0} x2={hp.x} y2={hp.y} stroke="#7c6cf0" strokeOpacity="0.35" strokeDasharray="3 3" />}

      {pts.map((p, i) => {
        const active = i === hover;
        const mark = p.edge
          ? <rect x={p.x - (active ? 5 : 4)} y={p.y - (active ? 5 : 4)} width={active ? 10 : 8} height={active ? 10 : 8}
              transform={`rotate(45 ${p.x} ${p.y})`} fill="#7c6cf0" />
          : <circle cx={p.x} cy={p.y} r={active ? 5 : 3.5} fill={active ? '#7c6cf0' : '#fff'} stroke="#7c6cf0" strokeWidth="2" />;
        return (
          <g key={i} onMouseEnter={() => setHover(i)} onMouseLeave={() => setHover(h => (h === i ? null : h))} style={{ cursor: 'pointer' }}>
            {mark}
            {/* Прозрачная увеличенная зона наведения. */}
            <circle cx={p.x} cy={p.y} r="12" fill="transparent" />
          </g>
        );
      })}

      {/* Тултип с датой и значением замера. */}
      {hp && <g pointerEvents="none">
        <rect x={tipX} y={tipY} width={tipW} height="22" rx="6" fill="#0f172a" opacity="0.92" />
        <text x={tipX + tipW / 2} y={tipY + 15} textAnchor="middle" fontSize="11" fill="#fff">{tipLabel}</text>
      </g>}

      <text x={x0} y={H - 8} fontSize="10" fill="#94a3b8">{series.period_start}</text>
      <text x={x1} y={H - 8} textAnchor="end" fontSize="10" fill="#94a3b8">{series.period_end}</text>
    </svg>
  );
}
