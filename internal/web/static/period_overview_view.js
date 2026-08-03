// Общий презентационный компонент обзора периода — тело для админ-модалки и
// публичной страницы «Обзор периода». Данные и колбэк массовых операций
// прокидывает контекст. Стили самодостаточны (не зависят от admin.js T/Btn).
const PO = {
  cardBorder: '#e5e7eb', hairline: '#f1f5f9', headingFg: '#0f172a',
  mutedFg: '#6b7280', dimFg: '#9ca3af', accent: ACCENT, danger: '#b91c1c',
};
// Порядок жизненного цикла: нет целей → черновик → к валидации → в работе → закрыто.
const PO_STATUS_TILES = [
  { key: 'no_goals',    label: 'Нет целей',   dot: '#cbd5e1', color: '#64748b' },
  { key: 'forming',     label: 'Черновик',    dot: '#f59e0b', color: '#92400e' },
  { key: 'ready',       label: 'К валидации', dot: '#3b82f6', color: '#1e40af' },
  { key: 'in_progress', label: 'В работе',    dot: '#22c55e', color: '#166534' },
  { key: 'closed',      label: 'Закрыто',     dot: '#9ca3af', color: '#4b5563' },
];

function POPrimaryBtn({ disabled, onClick, children }) {
  return <button type="button" onClick={onClick} disabled={disabled}
    style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '8px 16px',
      borderRadius: 20, border: '1.5px solid ' + PO.accent, background: PO.accent, color: 'white',
      fontSize: 13, fontWeight: 600, cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? .5 : 1,
      whiteSpace: 'nowrap' }}>{children}</button>;
}

function PeriodOverviewContent({ data, busy, onApply }) {
  // Храним селектор категории (kind/key), а не снимок команд, чтобы drill-down
  // пересчитывался от актуальных data после массовой операции или смены периода.
  const [drill, setDrill] = useState(null); // {title, kind:'status'|'err'|'goals', key?}
  if (!data) return <div style={{ padding: '40px 22px', textAlign: 'center', color: PO.mutedFg }}>Загрузка обзора…</div>;
  const s = data.summary;
  const allTeams = data.teams || [];

  // Статус строки уже нормализован сервером в бакет плитки — фильтруем по нему напрямую.
  const teamsByStatus = k => allTeams.filter(t => t.status === k);
  const teamsWithErr = () => allTeams.filter(t => t.weight_error);
  const teamsWithGoals = () => allTeams.filter(t => t.goals_count > 0);
  const drillTeams = () => {
    if (!drill) return [];
    if (drill.kind === 'status') return teamsByStatus(drill.key);
    if (drill.kind === 'err') return teamsWithErr();
    return teamsWithGoals();
  };

  // Тот же предикат, что и на сервере (computeBulkAffected): затрагиваются команды
  // с целями, чей статус не равен целевому.
  const affectActivate = teamsWithGoals().filter(t => t.status !== 'in_progress').length;
  const affectClose = teamsWithGoals().filter(t => t.status !== 'closed').length;
  const skipNoGoals = s.total_teams - s.teams_with_goals;

  const tile = (label, value, sub, accent, onClick) =>
    <div onClick={onClick} style={{ flex: '1 1 150px', minWidth: 140, background: 'white', border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '14px 16px', cursor: onClick ? 'pointer' : 'default' }}>
      <div style={{ fontSize: 12, color: PO.mutedFg, fontWeight: 600 }}>{label}</div>
      <div style={{ fontSize: 26, fontWeight: 800, color: accent || PO.headingFg, marginTop: 4 }}>{value}</div>
      {sub && <div style={{ fontSize: 11, color: PO.dimFg, marginTop: 2 }}>{sub}</div>}
    </div>;

  return <div style={{ padding: '18px 22px' }}>
    <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, marginBottom: 10 }}>Команды по статусам · всего {s.total_teams}</div>
    <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
      {PO_STATUS_TILES.map(st => tile(
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 7, height: 7, borderRadius: 999, background: st.dot }} />{st.label}</span>,
        (s.by_status && s.by_status[st.key]) || 0, 'показать состав', st.color,
        () => setDrill({ title: st.label, kind: 'status', key: st.key })
      ))}
    </div>

    <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, margin: '18px 0 10px' }}>Качество и результат</div>
    <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
      {tile('Команды с целями', `${s.teams_with_goals}/${s.total_teams}`, 'только они участвуют в массовых операциях', PO.accent, () => setDrill({ title: 'Команды с целями', kind: 'goals' }))}
      {tile('Ошибки весов', s.weight_error_count, 'сумма весов целей ≠ 100%', '#b91c1c', () => setDrill({ title: 'Ошибки весов', kind: 'err' }))}
      {tile('Средний прогресс', `${s.avg_progress}%`, `по ${s.teams_with_goals} командам с целями`, PO.accent)}
    </div>

    {drill && (() => { const dt = drillTeams(); return <div style={{ marginTop: 14, border: '1px solid ' + PO.cardBorder, borderRadius: 12, overflow: 'hidden' }}>
      <div style={{ padding: '10px 14px', background: '#f8fafc', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ fontSize: 12.5, fontWeight: 700, color: PO.headingFg }}>{drill.title} · {dt.length}</span>
        <button onClick={() => setDrill(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: PO.mutedFg, fontSize: 16 }}>×</button>
      </div>
      <div style={{ maxHeight: 220, overflowY: 'auto' }}>
        {dt.length === 0
          ? <div style={{ padding: '16px', textAlign: 'center', color: PO.dimFg, fontSize: 12.5 }}>Пусто</div>
          : dt.map(t => <div key={t.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 10, padding: '8px 14px', borderTop: '1px solid ' + PO.hairline, fontSize: 12.5 }}>
              <span style={{ color: PO.headingFg, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{(t.path || []).join(' › ') || t.name}</span>
              <span style={{ color: t.weight_error ? PO.danger : PO.mutedFg, flexShrink: 0 }}>{t.goals_count > 0 ? `${t.progress}% · веса ${t.weight_sum}` : 'нет целей'}</span>
            </div>)}
      </div>
    </div>; })()}

    <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, margin: '18px 0 10px' }}>Массовые операции · только админ</div>
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      <div style={{ border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '14px 16px', display: 'flex', alignItems: 'center', gap: 14 }}>
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 13.5, fontWeight: 700, color: PO.headingFg }}>Перевести все команды в «В работе»</div>
          <div style={{ fontSize: 12, color: PO.mutedFg, marginTop: 3 }}>Затронет только команды, у которых есть хотя бы одна цель в этом периоде. Цели блокируются от редактирования, остаётся обновление прогресса.</div>
        </div>
        <div style={{ fontSize: 11.5, color: PO.dimFg, textAlign: 'right' }}>затронет {affectActivate}<br />пропустим {skipNoGoals} без целей</div>
        <POPrimaryBtn disabled={busy || affectActivate === 0} onClick={() => onApply('activate')}>Применить</POPrimaryBtn>
      </div>
      <div style={{ border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '14px 16px', display: 'flex', alignItems: 'center', gap: 14 }}>
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 13.5, fontWeight: 700, color: PO.headingFg }}>Закрыть цели всех команд периода</div>
          <div style={{ fontSize: 12, color: PO.mutedFg, marginTop: 3 }}>Команды без целей не трогаем. У остальных статус становится «Закрыто» — доступны только комментарии.</div>
        </div>
        <div style={{ fontSize: 11.5, color: PO.dimFg, textAlign: 'right' }}>затронет {affectClose}<br />пропустим {skipNoGoals} без целей</div>
        <POPrimaryBtn disabled={busy || affectClose === 0} onClick={() => onApply('close')}>Применить</POPrimaryBtn>
      </div>
    </div>
  </div>;
}
