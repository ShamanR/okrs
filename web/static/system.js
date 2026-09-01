// OKR System-admin — React SPA (CDN React 18 + Babel standalone), mirrors admin.js conventions.
const {useState, useEffect, useCallback} = React;

// readCSRF / csrfHeaders — общие глобали из api.js (грузится раньше).
async function api(url, opts={}) {
  const res = await fetch(url, opts);
  if (res.status === 401) { window.location.href = '/login'; return null; }
  return res;
}
const get  = (u)    => api(u);
const post = (u, b) => api(u, {method:'POST', headers:csrfHeaders(), body: b===undefined?undefined:JSON.stringify(b)});
const put  = (u, b) => api(u, {method:'PUT',  headers:csrfHeaders(), body: JSON.stringify(b)});
const del  = (u)    => api(u, {method:'DELETE', headers:csrfHeaders()});
const patch = (u, b) => api(u, {method:'PATCH', headers:csrfHeaders(), body: JSON.stringify(b)});
// errMsg — читает текст ошибки из тела ответа ({error:"..."}), иначе падает на код
// статуса. res может быть null: api() возвращает null на 401, уже уводя на /login,
// и без этой проверки обработчик ошибки сам падал бы необработанным исключением.
async function errMsg(res){ if (!res) return 'Ошибка авторизации'; try { const j = await res.json(); return j.error || ('Ошибка '+res.status); } catch { return 'Ошибка '+res.status; } }

// Единая тема управляющих плоскостей (совпадает с admin.js T): тёмный сайдбар,
// фиолетовый акцент, светлая контент-область. Держит /system визуально в одном
// ряду с /admin и /settings.
const T = {
  sidebarBg:'#0c1220', sidebarText:'#f1f5f9', sidebarDim:'#94a3b8', sidebarMuted:'#64748b',
  sidebarSel:'#c4b5fd', sidebarSelBg:'rgba(124,58,237,0.15)',
  accent:'#7c3aed', link:'#2563eb',
  contentBg:'#edf0f4', cardBg:'#ffffff', cardBorder:'#e5e7eb', hairline:'#f1f5f9',
  headingFg:'#0f172a', bodyFg:'#111827', mutedFg:'#6b7280', dimFg:'#9ca3af',
  danger:'#dc2626', success:'#059669', warn:'#d97706', info:'#0891b2',
};

const C = { card:'#fff', border:'#e5e7eb', accent:'#2563eb', danger:'#b91c1c', ok:'#047857', muted:'#6b7280' };
const box = {background:C.card, border:'1px solid '+T.cardBorder, borderRadius:12, padding:16, marginBottom:16, boxShadow:'0 1px 3px rgba(15,23,42,0.04)'};
const btn = {padding:'6px 12px', border:'none', borderRadius:7, background:C.accent, color:'#fff', fontWeight:600, cursor:'pointer'};
const inp = {padding:'6px 10px', border:'1.5px solid '+C.border, borderRadius:7};
const th  = {textAlign:'left', padding:'6px 8px', borderBottom:'1px solid '+C.border};

function TenantsSection({tenants, reload, onOpenMembers}) {
  const [name,setName]=useState(''); const [slug,setSlug]=useState(''); const [err,setErr]=useState('');
  const [editId,setEditId]=useState(null);
  const [editName,setEditName]=useState('');
  const [editSlug,setEditSlug]=useState('');
  const [purgeDepth,setPurgeDepth]=useState({});
  const startEdit = (t)=>{ setErr(''); setEditId(t.id); setEditName(t.name); setEditSlug(t.slug); };
  const cancelEdit = ()=>{ setEditId(null); setEditName(''); setEditSlug(''); };
  const saveEdit = async (id)=>{ setErr('');
    const res = await patch(`/api/v1/system/tenants/${id}`, {name: editName.trim(), slug: editSlug.trim()});
    if (res.status===200){ cancelEdit(); reload(); } else setErr(await errMsg(res));
  };
  const create = async (e)=>{ e.preventDefault(); setErr('');
    const res = await post('/api/v1/system/tenants', {name, slug});
    if (res.status===201){ setName(''); setSlug(''); reload(); } else setErr(await errMsg(res));
  };
  const setStatus = async (id, action)=>{ setErr(''); const res = await post(`/api/v1/system/tenants/${id}/${action}`); if (res.status===204) reload(); else setErr(await errMsg(res)); };
  const purge = async (id)=>{
    const depth = purgeDepth[id] || 'quarter';
    const labels = {quarter:'старше квартала', year:'старше года', all:'все'};
    if (!confirm(`Очистить лог активности пространства #${id} (${labels[depth]})? Необратимо.`)) return;
    setErr('');
    const res = await post(`/api/v1/system/tenants/${id}/activity/purge`, {older_than: depth});
    if (res.status===200){ const j = await res.json(); setErr(`Пространство #${id}: удалено ${j.deleted}`); }
    else setErr(await errMsg(res));
  };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:10}}>Пространства</h2>
    <table style={{width:'100%',borderCollapse:'collapse'}}>
      <thead><tr>{['ID','Slug','Название','Статус',''].map(h=><th key={h} style={th}>{h}</th>)}</tr></thead>
      <tbody>{(tenants||[]).map(t=> editId===t.id
        ? <tr key={t.id}>
            <td style={{padding:'6px 8px'}}>{t.id}</td>
            <td style={{padding:'6px 8px'}}><input style={inp} value={editSlug} onChange={e=>setEditSlug(e.target.value)}/></td>
            <td style={{padding:'6px 8px'}}><input style={inp} value={editName} onChange={e=>setEditName(e.target.value)}/></td>
            <td style={{padding:'6px 8px',color:t.status==='suspended'?C.danger:C.ok}}>{t.status}</td>
            <td style={{padding:'6px 8px',display:'flex',gap:6}}>
              <button style={btn} onClick={()=>saveEdit(t.id)}>Сохранить</button>
              <button style={{...btn,background:C.muted}} onClick={cancelEdit}>Отмена</button>
            </td>
          </tr>
        : <tr key={t.id}>
            <td style={{padding:'6px 8px'}}>{t.id}</td>
            <td style={{padding:'6px 8px'}}>{t.slug}</td>
            <td style={{padding:'6px 8px'}}>{t.name}</td>
            <td style={{padding:'6px 8px',color:t.status==='suspended'?C.danger:C.ok}}>{t.status}</td>
            <td style={{padding:'6px 8px',display:'flex',gap:6}}>
              <button style={{...btn,background:C.accent}} onClick={()=>onOpenMembers(t.id)}>Участники</button>
              <button style={{...btn,background:C.muted}} onClick={()=>startEdit(t)}>Изменить</button>
              {t.status==='active'
                ? <button style={{...btn,background:C.danger}} onClick={()=>setStatus(t.id,'suspend')}>Suspend</button>
                : <button style={{...btn,background:C.ok}} onClick={()=>setStatus(t.id,'restore')}>Restore</button>}
              <select style={{...inp,padding:'4px 6px'}} value={purgeDepth[t.id]||'quarter'} onChange={e=>setPurgeDepth(d=>({...d,[t.id]:e.target.value}))}>
                <option value="quarter">Кв.</option><option value="year">Год</option><option value="all">Всё</option>
              </select>
              <button style={{...btn,background:C.danger}} onClick={()=>purge(t.id)}>Очистить лог</button>
            </td>
          </tr>)}</tbody>
    </table>
    <form onSubmit={create} style={{display:'flex',gap:8,marginTop:12,flexWrap:'wrap'}}>
      <input style={inp} placeholder="Название" value={name} onChange={e=>setName(e.target.value)} required/>
      <input style={inp} placeholder="slug" value={slug} onChange={e=>setSlug(e.target.value)} required/>
      <button style={btn} type="submit">Создать</button>
    </form>
    {err && <div style={{color:C.danger,marginTop:8}}>{err}</div>}
  </div>;
}

function MembersSection({tenants, users, tid, setTid}) {
  const [members,setMembers]=useState([]);
  const [q,setQ]=useState(''); const [uid,setUid]=useState(''); const [role,setRole]=useState('user'); const [err,setErr]=useState('');
  const loadMembers = useCallback(async (id)=>{ if(!id){setMembers([]);return;} const res=await get(`/api/v1/system/tenants/${id}/members`); if(res&&res.ok) setMembers(await res.json()||[]); },[]);
  useEffect(()=>{ loadMembers(tid); },[tid,loadMembers]);
  const attach = async (e)=>{ e.preventDefault(); setErr('');
    if(!tid||!uid){ setErr('Выберите пространство и пользователя'); return; }
    const res = await post(`/api/v1/system/tenants/${tid}/members`, {user_id:Number(uid), role});
    if (res.status===201){ setUid(''); setQ(''); loadMembers(tid); } else setErr(await errMsg(res));
  };
  const filtered = (users||[]).filter(u=>((u.display_name||'')+' '+(u.email||'')).toLowerCase().includes(q.toLowerCase())).slice(0,50);
  // Requesters first, then active members.
  const ordered = [...members].sort((a,b)=>(a.status==='requested'?0:1)-(b.status==='requested'?0:1));
  const connect = async (m)=>{ setErr(''); const res=await post(`/api/v1/system/tenants/${tid}/members`, {user_id:m.user_id, role:m.role||'user'}); if(res.status===201) loadMembers(tid); else setErr(await errMsg(res)); };
  const deny = async (m)=>{ setErr(''); const res=await post(`/api/v1/system/tenants/${tid}/members/${m.user_id}/deny`); if(res.status===204) loadMembers(tid); else setErr(await errMsg(res)); };
  const remove = async (m)=>{ if(!confirm(`Удалить ${m.display_name||m.email||('пользователя #'+m.user_id)} из пространства?`)) return; setErr(''); const res=await del(`/api/v1/system/tenants/${tid}/members/${m.user_id}`); if(res.status===204) loadMembers(tid); else setErr(await errMsg(res)); };
  const changeRole = async (m, newRole)=>{ setErr(''); const res=await put(`/api/v1/system/tenants/${tid}/members/${m.user_id}/role`, {role:newRole}); if(res.status===204) loadMembers(tid); else setErr(await errMsg(res)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:10}}>Участники</h2>
    <select style={inp} value={tid} onChange={e=>setTid(e.target.value)}>
      <option value="">— выберите пространство —</option>
      {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
    </select>
    {tid && <table style={{width:'100%',borderCollapse:'collapse',marginTop:12}}>
      <thead><tr>{['Имя','Email','Роль','Статус',''].map(h=><th key={h} style={th}>{h}</th>)}</tr></thead>
      <tbody>{ordered.map(m=><tr key={m.user_id} style={m.status==='requested'?{background:'#fffbeb'}:null}>
        <td style={{padding:'6px 8px'}}>{m.display_name}</td><td style={{padding:'6px 8px'}}>{m.email}</td>
        <td style={{padding:'6px 8px'}}>{m.status==='requested'
          ? m.role
          : <select style={inp} value={m.role} onChange={e=>changeRole(m, e.target.value)}><option value="user">user</option><option value="admin">admin</option></select>}</td>
        <td style={{padding:'6px 8px',color:m.status==='requested'?C.muted:C.ok}}>{m.status}</td>
        <td style={{padding:'6px 8px',textAlign:'right'}}>{m.status==='requested'
          ? <span>
              <button style={{...btn,background:C.ok,marginRight:6}} onClick={()=>connect(m)}>Подключить</button>
              <button style={{...btn,background:C.muted}} onClick={()=>deny(m)}>Отклонить</button>
            </span>
          : <button style={{...btn,background:C.danger}} onClick={()=>remove(m)}>Удалить</button>}</td>
      </tr>)}</tbody>
    </table>}
    {tid && <form onSubmit={attach} style={{display:'flex',gap:8,marginTop:12,flexWrap:'wrap',alignItems:'center'}}>
      <input style={inp} placeholder="поиск пользователя" value={q} onChange={e=>setQ(e.target.value)}/>
      <select style={inp} value={uid} onChange={e=>setUid(e.target.value)}>
        <option value="">— пользователь —</option>
        {filtered.map(u=><option key={u.id} value={u.id}>{u.display_name}{u.email?' · '+u.email:''}</option>)}
      </select>
      <select style={inp} value={role} onChange={e=>setRole(e.target.value)}><option value="user">user</option><option value="admin">admin</option></select>
      <button style={btn} type="submit">Подключить</button>
    </form>}
    {err && <div style={{color:C.danger,marginTop:8}}>{err}</div>}
  </div>;
}

function RegistrationSection({tenants}) {
  const [val,setVal]=useState(''); const [msg,setMsg]=useState('');
  useEffect(()=>{ (async()=>{ const res=await get('/api/v1/system/settings'); if(res&&res.ok){ const j=await res.json(); setVal(j.default_registration_tenant_id==null?'':String(j.default_registration_tenant_id)); } })(); },[]);
  const save = async ()=>{ setMsg(''); const res=await put('/api/v1/system/settings/default-registration-tenant', {tenant_id: val===''?null:Number(val)}); setMsg(res.status===204?'Сохранено':await errMsg(res)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:6}}>Пространство регистрации по умолчанию</h2>
    <div style={{color:C.muted,marginBottom:8}}>Куда попадает новый пользователь без приглашения. «Нет» → страница-заглушка.</div>
    <div style={{display:'flex',gap:8,alignItems:'center',flexWrap:'wrap'}}>
      <select style={inp} value={val} onChange={e=>setVal(e.target.value)}>
        <option value="">— нет —</option>
        {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
      </select>
      <button style={btn} onClick={save}>Сохранить</button>
      {msg && <span style={{color:msg==='Сохранено'?C.ok:C.danger}}>{msg}</span>}
    </div>
  </div>;
}

const KNOWN_ENT = ['sso','subdomains','file_uploads','max_users'];
function EntitlementsSection({tenants}) {
  const [tid,setTid]=useState(''); const [ent,setEnt]=useState({}); const [k,setK]=useState(''); const [v,setV]=useState(''); const [msg,setMsg]=useState('');
  const load = useCallback(async(id)=>{ if(!id){setEnt({});return;} const res=await get(`/api/v1/system/tenants/${id}/entitlements`); if(res&&res.ok) setEnt(await res.json()||{}); },[]);
  useEffect(()=>{ load(tid); },[tid,load]);
  const save = async (key, value)=>{ setMsg(''); const res=await put(`/api/v1/system/tenants/${tid}/entitlements`, {[key]: value}); if(res.status===204){ load(tid); setMsg('Сохранено'); } else setMsg(await errMsg(res)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:6}}>Entitlements</h2>
    <div style={{color:C.muted,marginBottom:10}}>В OSS не ограничивают (всё включено); запись — задел для SaaS-сборки.</div>
    <select style={inp} value={tid} onChange={e=>setTid(e.target.value)}>
      <option value="">— выберите пространство —</option>
      {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
    </select>
    {tid && <div style={{marginTop:12,display:'flex',flexDirection:'column',gap:8}}>
      {KNOWN_ENT.map(key=>{
        const cur = ent[key];
        if (key==='max_users') {
          return <div key={key} style={{display:'flex',gap:8,alignItems:'center'}}>
            <code style={{minWidth:160}}>entitlement.{key}</code>
            <input style={inp} type="number" defaultValue={cur!=null?cur:''} onBlur={e=>save(key, Number(e.target.value))}/>
          </div>;
        }
        return <div key={key} style={{display:'flex',gap:8,alignItems:'center'}}>
          <code style={{minWidth:160}}>entitlement.{key}</code>
          <button style={{...btn,background: cur===true?C.ok:C.muted}} onClick={()=>save(key, cur!==true)}>{cur===true?'on':'off'}</button>
        </div>;
      })}
      <div style={{display:'flex',gap:8,marginTop:8,flexWrap:'wrap',alignItems:'center'}}>
        <input style={inp} placeholder="ключ (без entitlement.)" value={k} onChange={e=>setK(e.target.value)}/>
        <input style={inp} placeholder='значение JSON (true / 50 / "x")' value={v} onChange={e=>setV(e.target.value)}/>
        <button style={btn} onClick={()=>{ if(!k){setMsg('Укажите ключ');return;} try{ save(k, JSON.parse(v)); }catch{ setMsg('Значение должно быть валидным JSON'); } }}>Сохранить ключ</button>
      </div>
      {msg && <div style={{color:msg==='Сохранено'?C.ok:C.danger}}>{msg}</div>}
    </div>}
  </div>;
}

// Каналы уведомлений: список берётся из сборки, а выдача пространству — это
// обычный entitlement. Отдельного эндпоинта записи здесь нет намеренно: он бы
// дублировал /entitlements и дал второй путь к тем же данным.
function NotificationChannelsSection({tenants}) {
  const [channels,setChannels]=useState([]);
  const [loadErr,setLoadErr]=useState('');
  const [tid,setTid]=useState(''); const [ent,setEnt]=useState({}); const [msg,setMsg]=useState('');
  useEffect(()=>{ (async()=>{ const r=await get('/api/v1/system/notification-channels'); if(r&&r.ok){ const j=await r.json(); setChannels(j.channels||[]); } else { setLoadErr(await errMsg(r)); } })(); },[]);
  const load = useCallback(async(id)=>{ if(!id){setEnt({});return;} const r=await get(`/api/v1/system/tenants/${id}/entitlements`); if(r&&r.ok) setEnt(await r.json()||{}); },[]);
  useEffect(()=>{ load(tid); },[tid,load]);
  const toggle = async (key, on)=>{ setMsg(''); const r=await put(`/api/v1/system/tenants/${tid}/entitlements`, {[key]: on}); if(r&&r.status===204){ load(tid); setMsg('Сохранено'); } else setMsg(await errMsg(r)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:6}}>Каналы уведомлений</h2>
    <div style={{color:C.muted,marginBottom:10}}>
      Список каналов задаётся сборкой. Здесь выбирается, какие из них доступны пространству;
      подключает и настраивает канал уже администратор пространства. Внутренние уведомления
      (колокольчик) доступны всегда и в этот список не входят.
    </div>
    {loadErr && <div style={{color:C.danger,marginBottom:10}}>Список каналов не загрузился: {loadErr}</div>}
    {!loadErr && !channels.length && <div style={{color:C.muted}}>В этой сборке нет внешних каналов.</div>}
    {!loadErr && !!channels.length && <select style={inp} value={tid} onChange={e=>setTid(e.target.value)}>
      <option value="">— выберите пространство —</option>
      {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
    </select>}
    {tid && <div style={{marginTop:12,display:'flex',flexDirection:'column',gap:8}}>
      {channels.map(ch=>{
        const on = ent[ch.entitlement_key]===true;
        return <div key={ch.name} style={{display:'flex',gap:8,alignItems:'center'}}>
          <span style={{minWidth:160,fontWeight:600}}>{ch.title}</span>
          <code style={{minWidth:220,color:C.muted}}>entitlement.{ch.entitlement_key}</code>
          <button style={{...btn,background: on?C.ok:C.muted}} onClick={()=>toggle(ch.entitlement_key, !on)}>
            {on?'доступен':'выключен'}
          </button>
        </div>;
      })}
      {msg && <div style={{color:msg==='Сохранено'?C.ok:C.danger}}>{msg}</div>}
    </div>}
  </div>;
}

function MessagesSection() {
  const [msg, setMsg] = useState('');
  const [saved, setSaved] = useState('');
  useEffect(()=>{ (async()=>{ const r=await get('/api/v1/system/settings'); if(r&&r.ok){ const j=await r.json(); setMsg(j.no_access_message||''); } })(); },[]);
  const save = async ()=>{ setSaved(''); const r=await put('/api/v1/system/settings/no-access-message', {message: msg}); setSaved(r.status===204?'Сохранено':await errMsg(r)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:6}}>Сообщение «нет доступа»</h2>
    <div style={{color:C.muted,marginBottom:10}}>Markdown. Показывается на странице /no-access. Пусто → текст по умолчанию.</div>
    <MarkdownEditor value={msg} onChange={setMsg} rows={6}/>
    <div style={{marginTop:8,display:'flex',gap:8,alignItems:'center'}}>
      <button style={btn} onClick={save}>Сохранить</button>
      {saved && <span style={{color:saved==='Сохранено'?C.ok:C.danger}}>{saved}</span>}
    </div>
  </div>;
}

function UsersSection({users, reload}) {
  const [meId,setMeId]=useState(0); const [err,setErr]=useState('');
  useEffect(()=>{ (async()=>{ const res=await get('/api/v1/me'); if(res&&res.ok){ const me=await res.json(); setMeId(me.id); } })(); },[]);
  const toggle = async (u)=>{ setErr(''); const res=await put(`/api/v1/system/users/${u.id}/system-admin`, {is_system_admin: !u.is_system_admin}); if(res.status===204) reload(); else setErr(await errMsg(res)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:10}}>Пользователи</h2>
    {err && <div style={{color:C.danger,marginBottom:8}}>{err}</div>}
    <table style={{width:'100%',borderCollapse:'collapse'}}>
      <thead><tr>{['Имя','Email','System-admin'].map(h=><th key={h} style={th}>{h}</th>)}</tr></thead>
      <tbody>{(users||[]).map(u=><tr key={u.id}>
        <td style={{padding:'6px 8px'}}>{u.display_name}</td>
        <td style={{padding:'6px 8px'}}>{u.email}</td>
        <td style={{padding:'6px 8px'}}>
          <input type="checkbox" checked={!!u.is_system_admin} disabled={u.id===meId} onChange={()=>toggle(u)}/>
          {u.id===meId && <span style={{color:C.muted,marginLeft:6}}>(вы)</span>}
        </td>
      </tr>)}</tbody>
    </table>
  </div>;
}

// ── SHELL ────────────────────────────────────────────────────────────────────
// Sidebar живёт в общем модуле sidebar.js (грузится ПЕРЕД этим скриптом).
// Зеркалит admin.js: тёмный сайдбар с контекстной навигацией + верхняя строка-
// хлебные крошки + скролл-регион контента.
const SYSTEM_SECTIONS = [
  {id:'tenants',       label:'Пространства', icon:'🏢'},
  {id:'members',       label:'Участники',    icon:'👥'},
  {id:'users',         label:'Пользователи', icon:'🔑'},
  {id:'registration',  label:'Регистрация',  icon:'📝'},
  {id:'entitlements',  label:'Entitlements', icon:'🎚'},
  {id:'notifications', label:'Уведомления',  icon:'🔔'},
  {id:'messages',      label:'Сообщения',    icon:'💬'},
];

function Shell({section, setSection, currentUser, children}) {
  const cur = SYSTEM_SECTIONS.find(s=>s.id===section);
  return <div style={{display:'flex',height:'100vh',overflow:'hidden'}}>
    <Sidebar user={currentUser} active={null} showSections={false}>
      <div className="sidebar__context">
        <div className="sidebar__section-label">Система</div>
        {SYSTEM_SECTIONS.map(s=>(
          <button
            key={s.id}
            onClick={()=>setSection(s.id)}
            className={`sidebar__navlink${s.id===section?' sidebar__navlink--active':''}`}
          >
            <span className="sidebar__navlink-icon">{s.icon}</span>{s.label}
          </button>
        ))}
      </div>
    </Sidebar>
    <div style={{flex:1,display:'flex',flexDirection:'column',overflow:'hidden',background:T.contentBg}}>
      <div style={{padding:'0 24px',background:'white',borderBottom:'1px solid '+T.cardBorder,display:'flex',alignItems:'center',height:54,gap:14,flexShrink:0}}>
        <a href="/teamOkrs" style={{display:'inline-flex',alignItems:'center',gap:7,padding:'6px 12px 6px 10px',background:'#f5f3ff',color:'#6d28d9',border:'1px solid #ddd6fe',borderRadius:20,fontSize:12,fontWeight:600,textDecoration:'none',flexShrink:0}}>
          <span style={{fontSize:13,lineHeight:1}}>←</span> OKR Tracker
        </a>
        <div style={{width:1,height:20,background:T.cardBorder}}/>
        <div style={{display:'flex',alignItems:'baseline',gap:10,flex:1}}>
          <span style={{fontSize:11,color:T.dimFg,fontWeight:600,textTransform:'uppercase',letterSpacing:.5}}>Система</span>
          <span style={{color:T.dimFg,fontSize:12}}>/</span>
          <span style={{fontSize:17,fontWeight:800,color:T.headingFg,letterSpacing:'-.2px'}}>{cur?.label}</span>
        </div>
      </div>
      <div style={{flex:1,overflow:'auto'}}>
        <div style={{padding:'20px 24px 24px'}}>{children}</div>
      </div>
    </div>
  </div>;
}

function App() {
  const [me,setMe]=useState(null);
  const [tenants,setTenants]=useState([]); const [users,setUsers]=useState([]);
  const [section,setSection]=useState(()=>localStorage.getItem('okr_system_section')||'tenants');
  const [membersTid,setMembersTid]=useState('');
  const openMembers = useCallback((id)=>{ setMembersTid(String(id)); setSection('members'); },[]);
  const reloadTenants = useCallback(async()=>{ const res=await get('/api/v1/system/tenants'); if(res&&res.ok) setTenants(await res.json()||[]); },[]);
  const reloadUsers = useCallback(async()=>{ const res=await get('/api/v1/system/users'); if(res&&res.ok) setUsers(await res.json()||[]); },[]);
  useEffect(()=>{ (async()=>{ const res=await get('/api/v1/me'); if(res&&res.ok) setMe(await res.json()); })(); },[]);
  useEffect(()=>{ reloadTenants(); reloadUsers(); },[reloadTenants,reloadUsers]);
  useEffect(()=>{ localStorage.setItem('okr_system_section',section); },[section]);
  useEffect(()=>{
    const label = (SYSTEM_SECTIONS.find(s=>s.id===section)||{}).label;
    document.title = label ? `Система · ${label}` : 'Система · Управление';
  },[section]);
  return <Shell section={section} setSection={setSection} currentUser={me}>
    {section==='tenants' && <TenantsSection tenants={tenants} reload={reloadTenants} onOpenMembers={openMembers}/>}
    {section==='members' && <MembersSection tenants={tenants} users={users} tid={membersTid} setTid={setMembersTid}/>}
    {section==='users' && <UsersSection users={users} reload={reloadUsers}/>}
    {section==='registration' && <RegistrationSection tenants={tenants}/>}
    {section==='entitlements' && <EntitlementsSection tenants={tenants}/>}
    {section==='notifications' && <NotificationChannelsSection tenants={tenants}/>}
    {section==='messages' && <MessagesSection/>}
  </Shell>;
}

ReactDOM.createRoot(document.getElementById('root')).render(<App/>);
