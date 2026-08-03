// Общий компонент выбора периода (сайдбар). Используется трекером и обзором периода.
// CSS-классы period-select__* — в tracker.css.
const TRK_PERIOD_STATUS = {
  future: { label: 'Планируется', dot: 'rgb(59,130,196)', bg: 'rgba(59,130,196,0.15)', fg: 'rgb(59,130,196)' },
  active: { label: 'В работе', dot: 'rgb(31,157,85)', bg: 'rgba(31,157,85,0.15)', fg: 'rgb(31,157,85)' },
  closed: { label: 'Закрыто', dot: 'rgb(107,114,128)', bg: 'rgba(107,114,128,0.14)', fg: 'rgb(107,114,128)' },
};
function fmtPeriodDate(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit' });
}
function fmtDateRange(a, b) { return `${fmtPeriodDate(a)} – ${fmtPeriodDate(b)}`; }
function PeriodSelect({ periods, periodId, onChange }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState(null);
  const triggerRef = useRef(null);
  const cur = periods.find(p => p.id === periodId);
  const st = cur && TRK_PERIOD_STATUS[cur.status];
  const openMenu = () => {
    const r = triggerRef.current.getBoundingClientRect();
    setPos({ top: r.bottom + 6, left: r.left });
    setOpen(true);
  };
  return <div className="period-select" style={{ position: 'relative' }}>
    <button ref={triggerRef} type="button" className="period-select__trigger" onClick={() => open ? setOpen(false) : openMenu()}>
      {st && <span className="period-select__dot" style={{ background: st.dot }} />}
      <span className="period-select__name">{cur ? cur.name : '—'}</span>
      <span className="period-select__chev">▾</span>
    </button>
    {open && pos && <>
      <div className="period-select__backdrop" onClick={() => setOpen(false)} />
      <div className="period-select__menu" style={{ top: pos.top, left: pos.left }}>
        <div className="period-select__group">Актуальные и будущие</div>
        {periods.map(p => {
          const s = TRK_PERIOD_STATUS[p.status] || TRK_PERIOD_STATUS.closed;
          return <button key={p.id} type="button"
            className={'period-select__item' + (p.id === periodId ? ' is-selected' : '')}
            onClick={() => { onChange(p.id); setOpen(false); }}>
            <span className="period-select__indent" style={{ width: p.depth * 12 }} />
            <span className="period-select__dot" style={{ background: s.dot }} />
            <span className="period-select__item-name">{p.name}</span>
            <span className="period-select__range">{fmtDateRange(p.start_date, p.end_date)}</span>
            <span className="period-select__badge-wrap"><span className="period-select__badge" style={{ background: s.bg, color: s.fg }}>{s.label}</span></span>
          </button>;
        })}
      </div>
    </>}
  </div>;
}
