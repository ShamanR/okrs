// No-membership page — React SPA (CDN React 18 + Babel standalone), uses the shared Sidebar.
// Served as a static file (like admin.js/system.js): inline Babel/JSX in an html/template
// <script> would be mangled by template JS-context escaping.
const {useState, useEffect} = React;

// Customizable markdown message injected server-side into a hidden element.
const customMsg = (document.getElementById('na-msg-src')?.textContent || '').trim();

function readCSRF() {
  return document.cookie.split(';').map(c=>c.trim()).find(c=>c.startsWith('okr_csrf_token='))?.split('=')[1] || '';
}

function NoAccess() {
  const [me, setMe] = useState(null);
  const [slug, setSlug] = useState('');
  const [msg, setMsg] = useState(null); // {text, ok}
  useEffect(() => {
    fetch('/api/v1/me', { credentials:'include' }).then(r => r.ok ? r.json() : null).then(setMe).catch(()=>{});
  }, []);
  const submit = async (e) => {
    e.preventDefault(); setMsg(null);
    const res = await fetch('/api/v1/onboarding/join-request', {
      method:'POST', credentials:'include',
      headers:{'Content-Type':'application/json','Accept':'application/json','X-CSRF-Token':readCSRF()},
      body: JSON.stringify({ slug: slug.trim() })
    });
    if (res.status === 204) setMsg({text:'Запрос отправлен. Дождитесь подтверждения администратора.', ok:true});
    else if (res.status === 404) setMsg({text:'Организация с таким slug не найдена.', ok:false});
    else if (res.status === 409) setMsg({text:'Вы уже состоите в этой организации.', ok:false});
    else { const b = await res.json().catch(()=>({})); setMsg({text:(b.error||('Ошибка '+res.status)), ok:false}); }
  };
  return <div style={{display:'flex',height:'100vh',overflow:'hidden'}}>
    <Sidebar user={me} showSections={false} />
    <div style={{flex:1,overflowY:'auto'}}>
      <div className="na-wrap"><div className="na-card">
        <h1>Нет доступа</h1>
        {customMsg
          ? <div className="na-card-md"><Markdown text={customMsg}/></div>
          : <p>У вашей учётной записи нет доступа ни к одной организации. Обратитесь к администратору
            или запросите доступ по короткому имени (slug) организации.</p>}
        <form className="na-form" onSubmit={submit}>
          <input placeholder="slug организации" value={slug} onChange={e=>setSlug(e.target.value)} required/>
          <button type="submit">Запросить доступ</button>
        </form>
        {msg && <div className={'na-msg ' + (msg.ok ? 'ok' : 'err')}>{msg.text}</div>}
      </div></div>
    </div>
  </div>;
}

ReactDOM.createRoot(document.getElementById('root')).render(<NoAccess/>);
