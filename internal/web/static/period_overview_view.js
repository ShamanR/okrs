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

// ScopeToggle — segmented control «Мои команды / Вся организация». Сегмент
// «Вся организация» доступен только админам (охват всего тенанта).
function ScopeToggle({ isAdmin, value, onChange }) {
  const seg = (key, label) =>
    <button type="button" onClick={() => onChange(key)}
      style={{ padding: '6px 14px', border: 'none', cursor: 'pointer', fontSize: 13, fontWeight: 600, borderRadius: 8,
        background: value === key ? PO.accent : 'transparent', color: value === key ? '#fff' : PO.mutedFg }}>{label}</button>;
  return <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
    <span style={{ fontSize: 12, color: PO.dimFg, fontWeight: 600 }}>Охват</span>
    <div style={{ display: 'inline-flex', gap: 4, background: '#f1f5f9', padding: 4, borderRadius: 10 }}>
      {seg('my_teams', 'Мои команды')}
      {isAdmin && seg('org', 'Вся организация')}
    </div>
  </div>;
}

// Ярлыки и цвета корзин балансов (порядок фиксирован сервером).
const PO_DD_LABELS = { Delivery: 'Delivery', Discovery: 'Discovery' };
const PO_DD_COLORS = { Delivery: '#475569', Discovery: '#7c6cf0' };
const PO_FOCUS_LABELS = { PROFITABILITY: 'Profitability', STABILITY: 'Stability', SPEED_EFFICIENCY: 'Speed Efficiency', TECH_INDEPENDENCE: 'Tech Independency' };
const PO_FOCUS_COLORS = { PROFITABILITY: '#ef4444', STABILITY: '#22c55e', SPEED_EFFICIENCY: '#f59e0b', TECH_INDEPENDENCE: '#7c6cf0' };
const PO_PRIO_LABELS = { P0: 'P0 · критично', P1: 'P1 · высокий', P2: 'P2 · средний', P3: 'P3 · низкий' };
const PO_PRIO_COLORS = { P0: '#dc2626', P1: '#f59e0b', P2: '#3b82f6', P3: '#94a3b8' };
// Health-статусы KR (значения совпадают с трекером; ярлык done — «Closed»).
const PO_HEALTH_LABELS = { not_started: 'Not Started', on_track: 'On Track', at_risk: 'At Risk', done: 'Closed' };
const PO_HEALTH_COLORS = { not_started: '#6b7280', on_track: '#16a34a', at_risk: '#d97706', done: '#15803d' };

function POPrimaryBtn({ disabled, onClick, children }) {
  return <button type="button" onClick={onClick} disabled={disabled}
    style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '8px 16px',
      borderRadius: 20, border: '1.5px solid ' + PO.accent, background: PO.accent, color: 'white',
      fontSize: 13, fontWeight: 600, cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? .5 : 1,
      whiteSpace: 'nowrap' }}>{children}</button>;
}

function PeriodOverviewContent({ data, busy, onApply, isAdmin, scope }) {
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
      {tile('Средний прогресс', `${s.avg_progress}%`, `по ${s.progress_teams} командам с целями (без черновиков)`, PO.accent)}
    </div>

    {/* Drill-down состава по статусам / качеству — сразу под плитками. */}
    {drill && (drill.kind === 'status' || drill.kind === 'err' || drill.kind === 'goals') && (() => { const dt = drillTeams(); return <div style={{ marginTop: 14, border: '1px solid ' + PO.cardBorder, borderRadius: 12, overflow: 'hidden' }}>
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

    <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, margin: '18px 0 10px' }}>Балансы целей · клик по полосе — состав</div>
    <div style={{ display: 'flex', gap: 28, flexWrap: 'wrap' }}>
      <BalanceBars title="Discovery / Delivery" subtitle="Соотношение исследовательской и поставочной работы"
        items={(data.balances && data.balances.discovery_delivery) || []} labels={PO_DD_LABELS} colors={PO_DD_COLORS}
        onSelect={k => setDrill({ kind: 'balance', field: 'work_type', key: k, title: 'Discovery / Delivery: ' + (PO_DD_LABELS[k] || k) })} />
      <BalanceBars title="Стратегические фокусы" subtitle="Profitability · Stability · Speed Efficiency · Tech Independency"
        items={(data.balances && data.balances.focuses) || []} labels={PO_FOCUS_LABELS} colors={PO_FOCUS_COLORS}
        onSelect={k => setDrill({ kind: 'balance', field: 'focus_type', key: k, title: 'Фокус: ' + (PO_FOCUS_LABELS[k] || k) })} />
      <BalanceBars title="Приоритеты" subtitle="Распределение целей по приоритету P0–P3"
        items={(data.balances && data.balances.priorities) || []} labels={PO_PRIO_LABELS} colors={PO_PRIO_COLORS}
        onSelect={k => setDrill({ kind: 'balance', field: 'priority', key: k, title: 'Приоритет: ' + (PO_PRIO_LABELS[k] || k) })} />
      <BalanceBars title="Статусы KR" subtitle="Распределение key results по health-статусу"
        items={(data.balances && data.balances.health) || []} labels={PO_HEALTH_LABELS} colors={PO_HEALTH_COLORS}
        onSelect={k => setDrill({ kind: 'krhealth', key: k, title: 'Статус KR: ' + (PO_HEALTH_LABELS[k] || k) })} />
    </div>

    {/* Drill-down целей баланса — сразу под полосами. */}
    {drill && drill.kind === 'balance' && (() => {
      const dg = (data.goals || []).filter(g => g[drill.field] === drill.key);
      return <div style={{ marginTop: 14, border: '1px solid ' + PO.cardBorder, borderRadius: 12, overflow: 'hidden' }}>
        <div style={{ padding: '10px 14px', background: '#f8fafc', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: 12.5, fontWeight: 700, color: PO.headingFg }}>{drill.title} · {dg.length}</span>
          <button onClick={() => setDrill(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: PO.mutedFg, fontSize: 16 }}>×</button>
        </div>
        <div style={{ maxHeight: 220, overflowY: 'auto' }}>
          {dg.length === 0
            ? <div style={{ padding: '16px', textAlign: 'center', color: PO.dimFg, fontSize: 12.5 }}>Пусто</div>
            : dg.map(g => <div key={g.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 10, padding: '8px 14px', borderTop: '1px solid ' + PO.hairline, fontSize: 12.5 }}>
                <span style={{ color: PO.headingFg, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{g.title} <span style={{ color: PO.dimFg }}>· {g.team_name}</span></span>
                <span style={{ color: PO.mutedFg, flexShrink: 0 }}>{g.progress}%</span>
              </div>)}
        </div>
      </div>;
    })()}

    {/* Drill-down KR по health-статусу — список key results в выбранном статусе. */}
    {drill && drill.kind === 'krhealth' && (() => {
      const dk = (data.krs || []).filter(kr => kr.health_status === drill.key);
      return <div style={{ marginTop: 14, border: '1px solid ' + PO.cardBorder, borderRadius: 12, overflow: 'hidden' }}>
        <div style={{ padding: '10px 14px', background: '#f8fafc', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: 12.5, fontWeight: 700, color: PO.headingFg }}>{drill.title} · {dk.length}</span>
          <button onClick={() => setDrill(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: PO.mutedFg, fontSize: 16 }}>×</button>
        </div>
        <div style={{ maxHeight: 220, overflowY: 'auto' }}>
          {dk.length === 0
            ? <div style={{ padding: '16px', textAlign: 'center', color: PO.dimFg, fontSize: 12.5 }}>Пусто</div>
            : dk.map(kr => <div key={kr.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 10, padding: '8px 14px', borderTop: '1px solid ' + PO.hairline, fontSize: 12.5 }}>
                <span style={{ color: PO.headingFg, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{kr.title} <span style={{ color: PO.dimFg }}>· {kr.goal_title} · {kr.team_name}</span></span>
                <span style={{ color: PO.mutedFg, flexShrink: 0 }}>{kr.progress}%</span>
              </div>)}
        </div>
      </div>;
    })()}

    <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, margin: '22px 0 4px' }}>Прогресс целей за период</div>
    <div style={{ fontSize: 12, color: PO.mutedFg, marginBottom: 10 }}>Пунктирная диагональ — ориентир ровного заполнения периода. Ромбы по краям — прогресс, зафиксированный до начала или после окончания периода.</div>
    <div style={{ border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '12px 14px' }}>
      <ProgressChart series={data.progress} />
    </div>

    {/* Массовые операции: для «моих команд» — руководителю над своими; для «всей
        организации» — только админу. Область действия = текущий охват. */}
    {(scope !== 'org' || isAdmin) && (() => {
      const scopeWord = scope === 'org' ? 'всех команд организации' : 'моих команд';
      return <>
      <div style={{ fontSize: 11, color: PO.dimFg, fontWeight: 700, textTransform: 'uppercase', letterSpacing: .5, margin: '22px 0 10px' }}>Управление периодом · {scope === 'org' ? 'вся организация' : 'мои команды'}</div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        <div style={{ border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '14px 16px', display: 'flex', alignItems: 'center', gap: 14 }}>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 13.5, fontWeight: 700, color: PO.headingFg }}>Перевести в «В работе»</div>
            <div style={{ fontSize: 12, color: PO.mutedFg, marginTop: 3 }}>Затронет только команды {scopeWord}, у которых есть хотя бы одна цель в этом периоде. Цели блокируются от редактирования, остаётся обновление прогресса.</div>
          </div>
          <div style={{ fontSize: 11.5, color: PO.dimFg, textAlign: 'right' }}>затронет {affectActivate}<br />пропустим {skipNoGoals} без целей</div>
          <POPrimaryBtn disabled={busy || affectActivate === 0} onClick={() => onApply('activate')}>Применить</POPrimaryBtn>
        </div>
        <div style={{ border: '1px solid ' + PO.cardBorder, borderRadius: 12, padding: '14px 16px', display: 'flex', alignItems: 'center', gap: 14 }}>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 13.5, fontWeight: 700, color: PO.headingFg }}>Закрыть цели периода</div>
            <div style={{ fontSize: 12, color: PO.mutedFg, marginTop: 3 }}>Команды {scopeWord} без целей не трогаем. У остальных статус становится «Закрыто» — доступны только комментарии.</div>
          </div>
          <div style={{ fontSize: 11.5, color: PO.dimFg, textAlign: 'right' }}>затронет {affectClose}<br />пропустим {skipNoGoals} без целей</div>
          <POPrimaryBtn disabled={busy || affectClose === 0} onClick={() => onApply('close')}>Применить</POPrimaryBtn>
        </div>
      </div>
      </>;
    })()}
  </div>;
}
