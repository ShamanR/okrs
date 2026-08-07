// Публичный раздел «Обзор периода» (admin-only). Мониторинг команд периода —
// та же вёрстка, что и админ-модалка (общий PeriodOverviewContent), период
// выбирается выпадающим списком в сайдбаре.
// useRef здесь не используется напрямую, но объявляется для общего PeriodSelect
// (period_select.js) — он берёт хуки из глобальной области бандла.
const { useState, useEffect, useRef } = React;

async function apiFetch(url, opts = {}) {
  const r = await fetch(url, opts);
  if (!r.ok) {
    if (r.status === 401) { location.href = '/login?next=' + encodeURIComponent(location.pathname); return null; }
    throw new Error(`HTTP ${r.status}`);
  }
  return r.status === 204 ? null : r.json();
}
const apiGet = url => apiFetch(url);
const apiPostJSON = (url, body) => apiFetch(url, { method: 'POST', headers: csrfHeaders(), body: JSON.stringify(body || {}) });

// Текущий активный период = самый короткий активный по датам (как FindPeriodForDate).
function pickDefaultPeriodId(periods) {
  if (!periods || periods.length === 0) return null;
  const active = periods.filter(p => p.status === 'active');
  if (active.length === 0) return periods[0].id;
  const dur = p => new Date(p.end_date) - new Date(p.start_date);
  active.sort((a, b) => (dur(a) - dur(b)) || (new Date(b.end_date) - new Date(a.end_date)));
  return active[0].id;
}

function App() {
  const [me, setMe] = useState(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [scopeSel, setScopeSel] = useState('my_teams'); // 'my_teams' | 'org'
  const [periods, setPeriods] = useState([]);
  const [periodId, setPeriodId] = useState(null);
  const [data, setData] = useState(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(false);

  useEffect(() => {
    Promise.all([apiGet('/api/v1/me'), apiGet('/api/v1/periods'), apiGet('/api/v1/config')]).then(([meData, per, cfg]) => {
      if (meData) setMe(meData);
      if (cfg) setIsAdmin(!!cfg.is_admin);
      const items = (per && per.items) || [];
      setPeriods(items);
      setPeriodId(pickDefaultPeriodId(items));
    }).catch(() => setErr(true));
  }, []);

  // Текущий выбранный период в ref — чтобы игнорировать ответы (overview и bulk),
  // относящиеся к уже смещённому выбору (гонка при переключении периода).
  const periodIdRef = useRef(null);
  const scopeRef = useRef('my_teams');
  const load = (pid, sc) => {
    if (!pid) return;
    setData(null); setErr(false);
    apiGet(`/api/v1/periods/${pid}/overview?scope=${sc}`)
      .then(d => { if (pid === periodIdRef.current && sc === scopeRef.current) setData(d); })
      .catch(() => { if (pid === periodIdRef.current && sc === scopeRef.current) setErr(true); });
  };
  useEffect(() => { periodIdRef.current = periodId; scopeRef.current = scopeSel; load(periodId, scopeSel); }, [periodId, scopeSel]);

  async function onApply(ep) {
    if (busy || !periodId) return;
    const pid = periodId;
    setBusy(true);
    try {
      await apiPostJSON(`/api/v1/periods/${pid}/teams/${ep}?scope=${scopeRef.current}`, {});
      if (pid === periodIdRef.current) load(pid, scopeRef.current);
    } catch { alert('Ошибка операции'); }
    finally { setBusy(false); }
  }

  const cur = periods.find(p => p.id === periodId);
  const total = data && data.summary ? data.summary.total_teams : null;

  return (
    <div className="app">
      <Sidebar
        user={me}
        active="period-overview"
        beforeSections={
          <div className="sidebar__period">
            <div className="sidebar__period-label">Период</div>
            <PeriodSelect periods={periods} periodId={periodId} onChange={setPeriodId} />
          </div>
        }
      />
      {/* .main из tracker.css — flex-колонка; переопределяем на обычный блок со
          скроллом, иначе карточка-флекс-элемент сжимается по высоте вьюпорта и
          обрезает нижний контент (управление периодом). */}
      <div className="main" style={{ display: 'block', padding: '24px 28px', overflow: 'auto' }}>
        {periods.length === 0
          ? <div style={{ color: '#6b7280', fontSize: 14 }}>Периодов пока нет.</div>
          : <>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 12, marginBottom: 18 }}>
                <div style={{ fontSize: 24, fontWeight: 800, color: '#0f172a', letterSpacing: '-.4px' }}>{cur ? cur.name : '—'}</div>
                {total != null && <div style={{ fontSize: 14, color: '#6b7280' }}>{scopeSel === 'org' ? 'вся организация' : 'мои команды'} · {total}</div>}
                <div style={{ flex: 1 }} />
                <ScopeToggle isAdmin={isAdmin} value={scopeSel} onChange={setScopeSel} />
              </div>
              {err
                ? <div style={{ color: '#b91c1c', fontSize: 13 }}>Не удалось загрузить обзор периода.</div>
                : <div style={{ background: 'white', border: '1px solid #e5e7eb', borderRadius: 14, boxShadow: '0 1px 3px rgba(15,23,42,0.04)', overflow: 'hidden' }}>
                    <PeriodOverviewContent data={data} busy={busy} onApply={onApply} isAdmin={isAdmin} scope={scopeSel} />
                  </div>}
            </>}
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
