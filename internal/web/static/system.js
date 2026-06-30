// OKR System-admin — React SPA (CDN React 18 + Babel standalone), mirrors admin.js conventions.
const {useState, useEffect, useCallback} = React;

function readCSRF() {
  return document.cookie.split(';').map(c=>c.trim()).find(c=>c.startsWith('okr_csrf_token='))?.split('=')[1] || '';
}
function csrfHeaders(extra={}) { return {'X-CSRF-Token': readCSRF(), 'Content-Type':'application/json', ...extra}; }
async function api(url, opts={}) {
  const res = await fetch(url, opts);
  if (res.status === 401) { window.location.href = '/login'; return null; }
  return res;
}
const get  = (u)    => api(u);
const post = (u, b) => api(u, {method:'POST', headers:csrfHeaders(), body: b===undefined?undefined:JSON.stringify(b)});
const put  = (u, b) => api(u, {method:'PUT',  headers:csrfHeaders(), body: JSON.stringify(b)});
async function errMsg(res){ try { const j = await res.json(); return j.error || ('Ошибка '+res.status); } catch { return 'Ошибка '+res.status; } }

const C = { card:'#fff', border:'#e5e7eb', accent:'#2563eb', danger:'#b91c1c', ok:'#047857', muted:'#6b7280' };
const box = {background:C.card, border:'1px solid '+C.border, borderRadius:10, padding:16, marginBottom:16};
const btn = {padding:'6px 12px', border:'none', borderRadius:7, background:C.accent, color:'#fff', fontWeight:600, cursor:'pointer'};
const inp = {padding:'6px 10px', border:'1.5px solid '+C.border, borderRadius:7};
const th  = {textAlign:'left', padding:'6px 8px', borderBottom:'1px solid '+C.border};

function TenantsSection({tenants, reload}) {
  const [name,setName]=useState(''); const [slug,setSlug]=useState(''); const [err,setErr]=useState('');
  const create = async (e)=>{ e.preventDefault(); setErr('');
    const res = await post('/api/v1/system/tenants', {name, slug});
    if (res.status===201){ setName(''); setSlug(''); reload(); } else setErr(await errMsg(res));
  };
  const setStatus = async (id, action)=>{ setErr(''); const res = await post(`/api/v1/system/tenants/${id}/${action}`); if (res.status===204) reload(); else setErr(await errMsg(res)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:10}}>Тенанты</h2>
    <table style={{width:'100%',borderCollapse:'collapse'}}>
      <thead><tr>{['ID','Slug','Название','Статус',''].map(h=><th key={h} style={th}>{h}</th>)}</tr></thead>
      <tbody>{(tenants||[]).map(t=><tr key={t.id}>
        <td style={{padding:'6px 8px'}}>{t.id}</td><td style={{padding:'6px 8px'}}>{t.slug}</td>
        <td style={{padding:'6px 8px'}}>{t.name}</td>
        <td style={{padding:'6px 8px',color:t.status==='suspended'?C.danger:C.ok}}>{t.status}</td>
        <td style={{padding:'6px 8px'}}>
          {t.status==='active'
            ? <button style={{...btn,background:C.danger}} onClick={()=>setStatus(t.id,'suspend')}>Suspend</button>
            : <button style={{...btn,background:C.ok}} onClick={()=>setStatus(t.id,'restore')}>Restore</button>}
        </td></tr>)}</tbody>
    </table>
    <form onSubmit={create} style={{display:'flex',gap:8,marginTop:12,flexWrap:'wrap'}}>
      <input style={inp} placeholder="Название" value={name} onChange={e=>setName(e.target.value)} required/>
      <input style={inp} placeholder="slug" value={slug} onChange={e=>setSlug(e.target.value)} required/>
      <button style={btn} type="submit">Создать</button>
    </form>
    {err && <div style={{color:C.danger,marginTop:8}}>{err}</div>}
  </div>;
}

function MembersSection({tenants, users}) {
  const [tid,setTid]=useState(''); const [members,setMembers]=useState([]);
  const [q,setQ]=useState(''); const [uid,setUid]=useState(''); const [role,setRole]=useState('user'); const [err,setErr]=useState('');
  const loadMembers = useCallback(async (id)=>{ if(!id){setMembers([]);return;} const res=await get(`/api/v1/system/tenants/${id}/members`); if(res&&res.ok) setMembers(await res.json()||[]); },[]);
  useEffect(()=>{ loadMembers(tid); },[tid,loadMembers]);
  const attach = async (e)=>{ e.preventDefault(); setErr('');
    if(!tid||!uid){ setErr('Выберите тенант и пользователя'); return; }
    const res = await post(`/api/v1/system/tenants/${tid}/members`, {user_id:Number(uid), role});
    if (res.status===201){ setUid(''); setQ(''); loadMembers(tid); } else setErr(await errMsg(res));
  };
  const filtered = (users||[]).filter(u=>((u.display_name||'')+' '+(u.email||'')).toLowerCase().includes(q.toLowerCase())).slice(0,50);
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:10}}>Участники</h2>
    <select style={inp} value={tid} onChange={e=>setTid(e.target.value)}>
      <option value="">— выберите тенант —</option>
      {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
    </select>
    {tid && <table style={{width:'100%',borderCollapse:'collapse',marginTop:12}}>
      <thead><tr>{['Имя','Email','Роль','Статус'].map(h=><th key={h} style={th}>{h}</th>)}</tr></thead>
      <tbody>{members.map(m=><tr key={m.user_id}>
        <td style={{padding:'6px 8px'}}>{m.display_name}</td><td style={{padding:'6px 8px'}}>{m.email}</td>
        <td style={{padding:'6px 8px'}}>{m.role}</td><td style={{padding:'6px 8px',color:m.status==='requested'?C.muted:C.ok}}>{m.status}</td>
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
    <h2 style={{fontSize:15,marginBottom:6}}>Тенант регистрации по умолчанию</h2>
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
      <option value="">— выберите тенант —</option>
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

function App() {
  const [tenants,setTenants]=useState([]); const [users,setUsers]=useState([]); const [tab,setTab]=useState('tenants');
  const reloadTenants = useCallback(async()=>{ const res=await get('/api/v1/system/tenants'); if(res&&res.ok) setTenants(await res.json()||[]); },[]);
  useEffect(()=>{ reloadTenants(); (async()=>{ const res=await get('/api/v1/system/users'); if(res&&res.ok) setUsers(await res.json()||[]); })(); },[reloadTenants]);
  const tabBtn = (id,label)=><button onClick={()=>setTab(id)} style={{...btn,background:tab===id?C.accent:'#94a3b8'}}>{label}</button>;
  return <div style={{maxWidth:920,margin:'0 auto',padding:'24px 16px'}}>
    <h1 style={{fontSize:20,marginBottom:16}}>Система · Управление</h1>
    <div style={{display:'flex',gap:8,marginBottom:16,flexWrap:'wrap'}}>
      {tabBtn('tenants','Тенанты')}{tabBtn('members','Участники')}{tabBtn('registration','Регистрация')}{tabBtn('entitlements','Entitlements')}
    </div>
    {tab==='tenants' && <TenantsSection tenants={tenants} reload={reloadTenants}/>}
    {tab==='members' && <MembersSection tenants={tenants} users={users}/>}
    {tab==='registration' && <RegistrationSection tenants={tenants}/>}
    {tab==='entitlements' && <EntitlementsSection tenants={tenants}/>}
  </div>;
}

ReactDOM.createRoot(document.getElementById('root')).render(<App/>);
