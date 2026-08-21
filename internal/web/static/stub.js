// Страницы-заглушки «в разработке» (/activity-log).
// Рендерят тот же общий Sidebar из sidebar.js + плейсхолдер контента.
const STUB_SECTIONS = {
  '/activity-log': { id: 'activity-log', title: 'Лог активностей', icon: '🕑' },
};

function StubApp() {
  const [me, setMe] = React.useState(null);
  React.useEffect(() => {
    fetch('/api/v1/me', { credentials: 'include' })
      .then(r => (r.ok ? r.json() : null))
      .then(d => { if (d) setMe(d); })
      .catch(() => {});
  }, []);
  const meta = STUB_SECTIONS[location.pathname] || { id: null, title: 'Раздел', icon: '•' };
  React.useEffect(() => { document.title = meta.title; }, [meta.title]);
  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <Sidebar user={me} active={meta.id} />
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: 12 }}>
        <div style={{ fontSize: 44 }}>{meta.icon}</div>
        <div style={{ fontSize: 22, fontWeight: 700, color: '#111827' }}>{meta.title}</div>
        <div style={{ fontSize: 14, color: '#6b7280' }}>Раздел в разработке</div>
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<StubApp />);
