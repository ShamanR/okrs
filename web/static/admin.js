// OKR Admin — React SPA (CDN React 18 + Babel standalone)
const {useState, useMemo, useEffect, useRef, useCallback} = React;

// ── API ──────────────────────────────────────────────────────────────────────
// readCSRF / csrfHeaders — общие глобали из api.js (грузится раньше).
async function apiFetch(url, opts={}) {
  const res = await fetch(url, opts);
  if (res.status === 401) { window.location.href = '/login'; return null; }
  return res;
}
const apiGet  = url       => apiFetch(url);
const apiPost = (url, body) => apiFetch(url, {method:'POST',   headers:csrfHeaders(), body:JSON.stringify(body)});
const apiPut  = (url, body) => apiFetch(url, {method:'PUT',    headers:csrfHeaders(), body:JSON.stringify(body)});
const apiPatch= (url, body) => apiFetch(url, {method:'PATCH',  headers:csrfHeaders(), body:JSON.stringify(body)});
const apiDel  = url       => apiFetch(url, {method:'DELETE', headers:{'X-CSRF-Token':readCSRF()}});
// readErr — читает текст ошибки из тела ответа ({error:"..."}), иначе падает на код
// статуса. res может быть null (401 уже увёл на /login в apiFetch).
async function readErr(res) {
  if (!res) return 'Ошибка авторизации';
  try { const j = await res.json(); return j.error || ('Ошибка ' + res.status); }
  catch { return 'Ошибка ' + res.status; }
}

// ── CONSTANTS ────────────────────────────────────────────────────────────────
// TEAM_TYPE_LABEL / TEAM_TYPE_ORDER / TEAM_TYPE_COLOR — общие константы из ui.js (грузится раньше).

const T = {
  sidebarBg:'#0c1220', sidebarText:'#f1f5f9', sidebarDim:'#94a3b8', sidebarMuted:'#64748b',
  sidebarSel:'#c4b5fd', sidebarSelBg:'rgba(124,58,237,0.15)',
  accent:'#7c3aed', link:'#2563eb',
  contentBg:'#edf0f4', cardBg:'#ffffff', cardBorder:'#e5e7eb', hairline:'#f1f5f9',
  headingFg:'#0f172a', bodyFg:'#111827', mutedFg:'#6b7280', dimFg:'#9ca3af',
  danger:'#dc2626', success:'#059669', warn:'#d97706', info:'#0891b2',
};

// ── HELPERS ──────────────────────────────────────────────────────────────────
function flatHierOrder(teams) {
  const out = [], byParent = {};
  for (const t of teams) {
    const key = t.parent_id != null ? t.parent_id : '__root__';
    (byParent[key] = byParent[key] || []).push(t);
  }
  function walk(parent) {
    const kids = (byParent[parent] || []).slice().sort(
      (a,b)=>(TEAM_TYPE_ORDER[a.type]-TEAM_TYPE_ORDER[b.type])||a.name.localeCompare(b.name));
    for (const k of kids) { out.push(k); walk(k.id); }
  }
  walk('__root__');
  return out;
}
function teamDepth(teams, id) {
  let d=0, cur=teams.find(t=>t.id===id);
  while (cur && cur.parent_id != null) { cur=teams.find(t=>t.id===cur.parent_id); d++; if(d>20)break; }
  return d;
}
function teamPath(teams, id) {
  const out=[]; let cur=teams.find(t=>t.id===id);
  while (cur) { out.unshift(cur); cur=cur.parent_id!=null?teams.find(t=>t.id===cur.parent_id):null; if(out.length>20)break; }
  return out;
}
function descendantIds(teams, rootId) {
  const out=new Set([rootId]); let changed=true;
  while (changed) { changed=false; for(const t of teams){if(t.parent_id!=null&&out.has(t.parent_id)&&!out.has(t.id)){out.add(t.id);changed=true;}} }
  return out;
}
function fmtDate(iso) {
  if (!iso) return '';
  return iso.slice(0,10);
}
function fmtDateTime(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  return d.toLocaleDateString('ru-RU')+' '+d.toLocaleTimeString('ru-RU',{hour:'2-digit',minute:'2-digit'});
}
function avatarInitials(name) {
  if (!name) return '?';
  return name.trim().split(/\s+/).map(w=>w[0]).join('').toUpperCase().slice(0,2);
}
const AVATAR_COLORS = ['#2563eb','#7c3aed','#059669','#d97706','#dc2626','#0891b2','#6366f1','#be185d'];
function avatarColor(id) { return AVATAR_COLORS[Math.abs(id||0) % AVATAR_COLORS.length]; }

// ── PRIMITIVES ───────────────────────────────────────────────────────────────
function Avatar({user, size=28}) {
  if (!user) return <div style={{width:size,height:size,borderRadius:'50%',background:'#e5e7eb',display:'inline-flex',alignItems:'center',justifyContent:'center',color:T.dimFg,fontSize:size*.4,fontWeight:700,flexShrink:0}}>?</div>;
  if (user.AvatarURL) return <img src={user.AvatarURL} style={{width:size,height:size,borderRadius:'50%',objectFit:'cover',flexShrink:0}}/>;
  return <div style={{width:size,height:size,borderRadius:'50%',background:avatarColor(user.ID),display:'inline-flex',alignItems:'center',justifyContent:'center',color:'white',fontSize:size*.38,fontWeight:700,letterSpacing:'-.3px',flexShrink:0}}>{avatarInitials(user.DisplayName)}</div>;
}
function TypeBadge({type}) {
  const c = TEAM_TYPE_COLOR[type]||T.mutedFg;
  return <span style={{display:'inline-block',padding:'2px 9px',fontSize:10.5,fontWeight:700,color:'white',background:c,borderRadius:10,letterSpacing:.3,textTransform:'uppercase',flexShrink:0,lineHeight:1.4}}>{TEAM_TYPE_LABEL[type]||type}</span>;
}
function Btn({variant='secondary', onClick, disabled, children, style={}, danger, size='md', type='button'}) {
  const pad = size==='sm'?'5px 12px':'8px 16px', fs = size==='sm'?12:13;
  let bg='white', color='#374151', border='1.5px solid #e5e7eb';
  if (variant==='primary'){bg=T.accent;color='white';border='1.5px solid '+T.accent;}
  else if (variant==='accent'){bg=T.link;color='white';border='1.5px solid '+T.link;}
  else if (variant==='ghost'){bg='transparent';border='1.5px solid transparent';}
  if (danger){color=T.danger;border='1.5px solid #fecaca';bg='white';}
  return <button type={type} onClick={onClick} disabled={disabled} style={{display:'inline-flex',alignItems:'center',justifyContent:'center',gap:6,padding:pad,borderRadius:20,border,background:bg,color,fontSize:fs,fontWeight:600,cursor:disabled?'not-allowed':'pointer',opacity:disabled?.5:1,transition:'all .12s',whiteSpace:'nowrap',...style}}>{children}</button>;
}
function Chip({children, color='#6b7280', bg}) {
  return <span style={{display:'inline-block',padding:'2px 8px',fontSize:10.5,fontWeight:700,color,background:bg||(color+'15'),borderRadius:10,letterSpacing:.3,textTransform:'uppercase',lineHeight:1.45}}>{children}</span>;
}
const inpStyle = {width:'100%',padding:'9px 13px',borderRadius:9,border:'1.5px solid #e5e7eb',fontSize:14,outline:'none',background:'white',color:T.bodyFg};
function Field({label, hint, children, required}) {
  return <div style={{marginBottom:14}}>
    <div style={{fontSize:12,fontWeight:600,color:'#475569',marginBottom:6,display:'flex',alignItems:'center',gap:6}}>
      {label}{required&&<span style={{color:T.danger}}>*</span>}
      {hint&&<span style={{fontSize:11,fontWeight:400,color:T.dimFg}}>· {hint}</span>}
    </div>
    {children}
  </div>;
}
function RowAction({title, onClick, disabled, danger, children}) {
  return <button onClick={e=>{e.stopPropagation();if(!disabled)onClick(e);}} disabled={disabled} title={title}
    style={{width:28,height:28,borderRadius:7,border:'1px solid '+T.cardBorder,background:'white',color:disabled?T.dimFg:(danger?T.danger:T.bodyFg),cursor:disabled?'not-allowed':'pointer',opacity:disabled?.4:1,fontSize:12,padding:0,lineHeight:1,display:'inline-flex',alignItems:'center',justifyContent:'center',transition:'all .12s'}}
    onMouseEnter={e=>{if(disabled)return;e.currentTarget.style.background=danger?'#fef2f2':'#f5f3ff';e.currentTarget.style.borderColor=danger?'#fecaca':'#c4b5fd';}}
    onMouseLeave={e=>{if(disabled)return;e.currentTarget.style.background='white';e.currentTarget.style.borderColor=T.cardBorder;}}>
    {children}
  </button>;
}
function Modal({open, title, subtitle, onClose, children, width=640, guarded=false, closeRef}) {
  // Закрываем по оверлею только если и нажатие, и отпускание мыши были на нём самом —
  // иначе выделение текста с выносом курсора за пределы окна закрывало бы модалку.
  const downOnOverlay=useRef(false);
  // guarded — закрытием (×, оверлей, Escape) управляет useModalClose в теле редактора
  // через closeRef.current (=requestClose): он покажет guard несохранённых изменений.
  const fire=()=>{ if (guarded && closeRef && closeRef.current) closeRef.current(); else onClose(); };
  const onMouseDown=e=>{downOnOverlay.current=e.target===e.currentTarget;};
  const onMouseUp=e=>{const close=downOnOverlay.current&&e.target===e.currentTarget;downOnOverlay.current=false;if(close)fire();};
  useEffect(()=>{
    if (!open) return;
    // Когда guarded — keydown и блокировку скролла ведёт useModalClose (в теле).
    if (guarded) return;
    const h=e=>{if(e.key==='Escape')onClose();};
    document.addEventListener('keydown',h);
    const prev=document.body.style.overflow; document.body.style.overflow='hidden';
    return()=>{document.removeEventListener('keydown',h);document.body.style.overflow=prev;};
  },[open,onClose,guarded]);
  if (!open) return null;
  return <div onMouseDown={onMouseDown} onMouseUp={onMouseUp} style={{position:'fixed',inset:0,background:'rgba(15,23,42,0.42)',backdropFilter:'blur(2px)',zIndex:2000,display:'flex',alignItems:'flex-start',justifyContent:'center',padding:'28px 24px 48px',overflow:'auto'}}>
    {/* overflow:visible — чтобы выпадающие списки (напр. выбор родителя) не обрезались рамкой окна */}
    <div onClick={e=>e.stopPropagation()} style={{width:'100%',maxWidth:width,background:'white',borderRadius:14,boxShadow:'0 24px 60px rgba(15,23,42,0.28)',overflow:'visible',animation:'admModalIn .16s ease-out'}}>
      <div style={{padding:'16px 22px 14px',borderBottom:'1px solid '+T.hairline,display:'flex',alignItems:'flex-start',gap:14}}>
        <div style={{flex:1,minWidth:0}}>
          <div style={{fontSize:16,fontWeight:800,color:T.headingFg,letterSpacing:'-.2px'}}>{title}</div>
          {subtitle&&<div style={{fontSize:12,color:T.mutedFg,marginTop:3}}>{subtitle}</div>}
        </div>
        <button onClick={fire} style={{width:30,height:30,borderRadius:8,border:'1px solid '+T.cardBorder,background:'white',color:T.mutedFg,cursor:'pointer',fontSize:15,lineHeight:1,padding:0,flexShrink:0}}
          onMouseEnter={e=>{e.currentTarget.style.background='#f8fafc';}}
          onMouseLeave={e=>{e.currentTarget.style.background='white';}}>×</button>
      </div>
      <div>{children}</div>
    </div>
  </div>;
}
function DetailHeader({breadcrumb, title, subtitle, actions}) {
  return <div style={{padding:'20px 24px 16px',borderBottom:'1px solid '+T.hairline}}>
    <div style={{fontSize:11,color:T.dimFg,fontWeight:700,textTransform:'uppercase',letterSpacing:.5,marginBottom:6}}>{breadcrumb}</div>
    <div style={{display:'flex',alignItems:'flex-start',justifyContent:'space-between',gap:14}}>
      <div style={{flex:1,minWidth:0}}>
        <div style={{fontSize:20,fontWeight:800,color:T.headingFg,letterSpacing:'-.3px',lineHeight:1.2}}>{title}</div>
        {subtitle&&<div style={{fontSize:13,color:T.mutedFg,marginTop:4}}>{subtitle}</div>}
      </div>
      {actions&&<div style={{display:'flex',gap:8,flexShrink:0,flexWrap:'wrap',justifyContent:'flex-end'}}>{actions}</div>}
    </div>
  </div>;
}
function DetailSection({title, children}) {
  return <div style={{padding:'18px 24px',borderBottom:'1px solid '+T.hairline}}>
    {title&&<div style={{fontSize:11,color:T.dimFg,fontWeight:700,textTransform:'uppercase',letterSpacing:.5,marginBottom:10}}>{title}</div>}
    {children}
  </div>;
}

// ── TEAM COMBOBOX (for user grants) ─────────────────────────────────────────
// single — single-select mode (selectedIds holds 0 or 1 id, picking replaces and closes).
// excludeIds — Set of ids that cannot be picked (e.g. the team itself and its descendants).
function TeamCombobox({selectedIds, onChange, teams, placeholder, single, excludeIds}) {
  const [q, setQ] = useState('');
  const [open, setOpen] = useState(false);
  const [hi, setHi] = useState(0);
  const inputRef = useRef(), wrapRef = useRef();
  const selected = selectedIds.map(id=>teams.find(t=>t.id===id)).filter(Boolean);
  const ql = q.trim().toLowerCase();
  const ordered = useMemo(()=>flatHierOrder(teams).map(t=>({...t, depth:teamDepth(teams,t.id)})),[teams]);
  const pool = excludeIds ? ordered.filter(t=>!excludeIds.has(t.id)) : ordered;
  const selectable = new Set(), visible = new Set();
  pool.forEach(t=>{
    const matches = !ql||t.name.toLowerCase().includes(ql)||(TEAM_TYPE_LABEL[t.type]||'').toLowerCase().includes(ql);
    if (matches) {
      selectable.add(t.id);
      let p=t.parent_id; while(p!=null){visible.add(p);const pt=teams.find(x=>x.id===p);p=pt?pt.parent_id:null;}
      visible.add(t.id);
    }
  });
  if (!ql) pool.forEach(t=>{selectable.add(t.id);visible.add(t.id);});
  const list = pool.filter(t=>visible.has(t.id)).map(t=>{
    const isSelected = selectedIds.includes(t.id);
    const coveredByParent = !single && !isSelected && selectedIds.some(sid=>sid!==t.id&&descendantIds(teams,sid).has(t.id));
    return {...t, isSelected, coveredByParent, isSelectable:selectable.has(t.id)&&!isSelected&&!coveredByParent};
  });
  const interactable = list.filter(x=>x.isSelectable);
  useEffect(()=>{setHi(0);},[q,open]);
  useEffect(()=>{
    const h=e=>{if(wrapRef.current&&!wrapRef.current.contains(e.target))setOpen(false);};
    document.addEventListener('mousedown',h);
    return()=>document.removeEventListener('mousedown',h);
  },[]);
  const add = t=>{
    if (single) { onChange([t.id]); setQ(''); setOpen(false); return; }
    const desc = descendantIds(teams,t.id);
    onChange([...selectedIds.filter(id=>!desc.has(id)),t.id]);
    setQ(''); inputRef.current?.focus();
  };
  const remove = id=>onChange(selectedIds.filter(x=>x!==id));
  const onKey = e=>{
    if(e.key==='ArrowDown'){e.preventDefault();setOpen(true);setHi(h=>Math.min(interactable.length-1,h+1));}
    else if(e.key==='ArrowUp'){e.preventDefault();setHi(h=>Math.max(0,h-1));}
    else if(e.key==='Enter'){e.preventDefault();if(open&&interactable[hi])add(interactable[hi]);}
    else if(e.key==='Escape'){if(open){e.preventDefault();setOpen(false);}}
    else if(e.key==='Backspace'&&!q&&selected.length>0)remove(selected[selected.length-1].id);
  };
  return <div ref={wrapRef} style={{position:'relative'}}>
    <div onClick={()=>{setOpen(true);inputRef.current?.focus();}}
      style={{minHeight:42,padding:'5px 7px',border:`1.5px solid ${open?T.accent:T.cardBorder}`,borderRadius:9,background:'white',display:'flex',flexWrap:'wrap',gap:5,alignItems:'center',cursor:'text',transition:'border-color .12s'}}>
      {selected.map(t=>{
        const color=TEAM_TYPE_COLOR[t.type]||T.mutedFg;
        return <div key={t.id} style={{display:'inline-flex',alignItems:'center',gap:6,padding:'3px 4px 3px 8px',borderRadius:16,background:`${color}15`,border:`1px solid ${color}40`}}>
          <span style={{fontSize:9,fontWeight:700,color,textTransform:'uppercase',letterSpacing:.4}}>{TEAM_TYPE_LABEL[t.type]}</span>
          <span style={{fontSize:12,fontWeight:600,color:T.bodyFg}}>{t.name}</span>
          <button onClick={e=>{e.stopPropagation();remove(t.id);}} style={{background:'none',border:'none',cursor:'pointer',color:T.mutedFg,fontSize:14,lineHeight:1,padding:'0 4px',borderRadius:8}}
            onMouseEnter={e=>e.currentTarget.style.color=T.danger} onMouseLeave={e=>e.currentTarget.style.color=T.mutedFg}>×</button>
        </div>;
      })}
      <input ref={inputRef} value={q} onChange={e=>{setQ(e.target.value);setOpen(true);}} onFocus={()=>setOpen(true)} onKeyDown={onKey}
        placeholder={selected.length?(single?'Заменить…':'Ещё команду…'):(placeholder||'Начните вводить название')}
        style={{flex:1,minWidth:180,border:'none',outline:'none',fontSize:13,padding:'6px 4px',background:'transparent',fontFamily:'inherit',color:T.bodyFg}}/>
    </div>
    {open&&<div style={{position:'absolute',top:'calc(100% + 4px)',left:0,right:0,background:'white',borderRadius:9,boxShadow:'0 10px 30px rgba(0,0,0,0.15)',border:'1px solid '+T.cardBorder,maxHeight:300,overflow:'auto',zIndex:50}}>
      {list.length===0
        ? <div style={{padding:'14px 16px',fontSize:12,color:T.dimFg,textAlign:'center'}}>{ql?`Не найдено: «${q}»`:'Все команды добавлены'}</div>
        : (()=>{
            let hiIdx=0;
            return list.map(t=>{
              const color=TEAM_TYPE_COLOR[t.type]||T.mutedFg;
              const isHi=t.isSelectable&&hiIdx===hi; const myIdx=t.isSelectable?hiIdx:-1;
              if(t.isSelectable)hiIdx++;
              return <div key={t.id} onClick={t.isSelectable?()=>add(t):undefined} onMouseEnter={t.isSelectable?()=>setHi(myIdx):undefined}
                style={{padding:'6px 12px 6px 8px',display:'flex',alignItems:'center',gap:6,cursor:t.isSelectable?'pointer':'default',background:isHi?'#f3f4f6':'white',opacity:t.isSelectable?1:0.55,borderLeft:`3px solid ${isHi?color:'transparent'}`}}>
                <div style={{display:'flex',flexShrink:0}}>{Array.from({length:t.depth}).map((_,d)=><div key={d} style={{width:14,borderRight:'1px dashed #e5e7eb'}}/>)}</div>
                <div style={{width:3,height:22,borderRadius:2,background:color,flexShrink:0}}/>
                <span style={{fontSize:9,fontWeight:700,color,textTransform:'uppercase',letterSpacing:.4,padding:'2px 5px',background:`${color}12`,borderRadius:3,flexShrink:0}}>{TEAM_TYPE_LABEL[t.type]}</span>
                <span style={{fontSize:13,fontWeight:t.isSelectable?500:400,color:t.isSelectable?T.bodyFg:T.dimFg,flex:1,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{t.name}</span>
                {t.isSelected&&<span style={{fontSize:10,color:T.success,fontWeight:600}}>✓</span>}
                {t.coveredByParent&&<span style={{fontSize:10,color:T.info,fontWeight:600}}>покрыта</span>}
              </div>;
            });
          })()
      }
    </div>}
  </div>;
}

// Sidebar lives in the shared sidebar.js module (loaded before this script).

// ── SHELL ────────────────────────────────────────────────────────────────────
const ADMIN_SECTIONS = [
  {id:'periods',  label:'Периоды',     hint:'Квартальные окна',        icon:'📅'},
  {id:'teams',    label:'Команды',     hint:'Иерархия и руководители', icon:'👥'},
  {id:'users',    label:'Пользователи',hint:'Админы и доступ',         icon:'🔑'},
  {id:'settings', label:'Настройки',   hint:'Доступ и политики',       icon:'⚙'},
  {id:'health-checkin', label:'Health Check-in', hint:'Настройки проверок', icon:'⚡'},
  {id:'notifications', label:'Уведомления', hint:'Каналы доставки', icon:'🔔'},
];

function Shell({section, setSection, currentUser, children}) {
  const sections = ADMIN_SECTIONS;
  const cur = sections.find(s=>s.id===section);
  return <div style={{display:'flex',height:'100vh',overflow:'hidden'}}>
    <Sidebar user={currentUser} active={null} showSections={false}>
      <div className="sidebar__context">
        <div className="sidebar__section-label">Администрирование</div>
        {sections.map(s=>(
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
          <span style={{fontSize:11,color:T.dimFg,fontWeight:600,textTransform:'uppercase',letterSpacing:.5}}>Управление</span>
          <span style={{color:T.dimFg,fontSize:12}}>/</span>
          <span style={{fontSize:17,fontWeight:800,color:T.headingFg,letterSpacing:'-.2px'}}>{cur?.label}</span>
        </div>
      </div>
      <div style={{flex:1,overflow:'auto'}}>{children}</div>
    </div>
  </div>;
}

// ── PERIODS SECTION ──────────────────────────────────────────────────────────
const PERIOD_STATUS = {
  future:   {label:'Планируется', dot:'#3b82f6', bg:'#dbeafe', fg:'#1e40af'},
  active:   {label:'В работе',     dot:'#22c55e', bg:'#dcfce7', fg:'#166534'},
  closed:   {label:'Закрыто',      dot:'#9ca3af', bg:'#f3f4f6', fg:'#4b5563'},
  archived: {label:'Архив',        dot:'#9ca3af', bg:'#f3f4f6', fg:'#6b7280'},
};
function PeriodBadge({status}) {
  const s = PERIOD_STATUS[status] || PERIOD_STATUS.closed;
  return <span style={{display:'inline-flex',alignItems:'center',gap:6,padding:'2px 8px',borderRadius:999,background:s.bg,color:s.fg,fontSize:11,fontWeight:600}}>
    <span style={{width:7,height:7,borderRadius:999,background:s.dot}}/>{s.label}
  </span>;
}
// Парсит 'YYYY-MM-DD' в локальную дату без сдвига по таймзоне.
function parseYMD(s) {
  if (!s) return null;
  const [y,m,d] = String(s).slice(0,10).split('-').map(Number);
  if (!y) return null;
  return new Date(y, (m||1)-1, d||1);
}
// Короткий формат для списка: 01.01.26
function fmtDateShort(iso) {
  const d = parseYMD(iso);
  if (!d) return '';
  const p = n=>String(n).padStart(2,'0');
  return `${p(d.getDate())}.${p(d.getMonth()+1)}.${String(d.getFullYear()).slice(2)}`;
}
// Предпросмотр статуса по датам — та же логика, что и на сервере
// (domain/period_status.go): archived — ручной флаг, остальное считается по датам
// с включёнными границами.
function periodStatusPreview(start, end, archived) {
  if (archived) return 'archived';
  const s = parseYMD(start), e = parseYMD(end);
  if (!s || !e) return null;
  const now = new Date();
  const t = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  if (t < s) return 'future';
  if (t > e) return 'closed';
  return 'active';
}
// Иконочная кнопка-действие в строке (SVG + подсказка при наведении).
function IconBtn({onClick, title, danger, children}) {
  const [hover, setHover] = useState(false);
  const color = danger ? T.danger : T.mutedFg;
  return <button onClick={e=>{e.stopPropagation();onClick();}} title={title} aria-label={title}
    onMouseEnter={()=>setHover(true)} onMouseLeave={()=>setHover(false)}
    style={{display:'inline-flex',alignItems:'center',justifyContent:'center',width:30,height:30,
      background:hover?(danger?'#fdecec':'#f1f0fb'):'transparent',border:'none',borderRadius:8,
      cursor:'pointer',color,opacity:hover?1:0.75,transition:'background .12s,opacity .12s',padding:0}}>
    {children}
  </button>;
}

// Набор SVG-иконок 16×16 (наследуют currentColor).
const Icons = {
  gear: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>,
  nested: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9 4v10a2 2 0 0 0 2 2h9"/><path d="m16 12 4 4-4 4"/></svg>,
  pencil: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/></svg>,
  trash: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>,
  archiveIn: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8"/><path d="M10 12h4"/></svg>,
  archiveOut: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8"/><path d="M12 18v-6"/><path d="m9 14 3-3 3 3"/></svg>,
};

// Пояснение статус-флоу над таблицей (req: описание статусов в формате флоу).
function StatusFlowGuide() {
  const step = (k, note) => {
    const s = PERIOD_STATUS[k];
    return <div style={{display:'flex',flexDirection:'column',gap:6,alignItems:'flex-start'}}>
      <PeriodBadge status={k}/>
      <span style={{fontSize:11.5,color:T.mutedFg}}>{note}</span>
    </div>;
  };
  const arrow = <span style={{color:T.dimFg,fontSize:15,alignSelf:'flex-start',marginTop:2}}>→</span>;
  return <div style={{background:'white',border:'1px solid '+T.cardBorder,borderRadius:12,padding:'16px 20px',marginBottom:18,boxShadow:'0 1px 3px rgba(15,23,42,0.04)'}}>
    <div style={{fontSize:13,fontWeight:700,color:T.headingFg}}>
      Как работают статусы <span style={{fontWeight:500,color:T.dimFg}}>— вычисляются по датам, кроме «Архива»</span>
    </div>
    <div style={{display:'flex',gap:18,alignItems:'flex-start',marginTop:14,flexWrap:'wrap'}}>
      {step('future','до даты начала')}{arrow}
      {step('active','между началом и концом')}{arrow}
      {step('closed','после даты окончания')}
    </div>
    <div style={{borderTop:'1px dashed '+T.cardBorder,margin:'14px 0'}}/>
    <div style={{display:'flex',gap:10,alignItems:'flex-start'}}>
      <span style={{display:'inline-flex',alignItems:'center',padding:'2px 8px',borderRadius:999,background:'#efe9df',color:'#8a6d3b',fontSize:11,fontWeight:600,flexShrink:0}}>Архив</span>
      <span style={{fontSize:11.5,color:T.mutedFg,lineHeight:1.5}}>
        <span style={{color:T.dimFg}}>↳ </span>
        вручную из статуса «Закрыто» — прячет период из активных списков. Возврат обратно возможен.
      </span>
    </div>
  </div>;
}

// Метрики строки периода: X/Y с целями, прогресс-бар, %, бейдж ошибок весов.
function PeriodMetrics({stat}) {
  const total = stat.total_teams || 0;
  const withGoals = stat.teams_with_goals || 0;
  const pct = Math.max(0, Math.min(100, stat.avg_progress || 0));
  const err = stat.weight_error_count || 0;
  return <div style={{display:'flex',alignItems:'center',gap:10,marginTop:4,flexWrap:'wrap'}}>
    <span style={{fontSize:11.5,color:T.dimFg}}>{withGoals}/{total} с целями</span>
    <span style={{display:'inline-block',width:56,height:5,borderRadius:999,background:'#eceaf6',position:'relative'}}>
      <span style={{position:'absolute',left:0,top:0,bottom:0,width:pct+'%',borderRadius:999,background:T.accent}}/>
    </span>
    <span style={{fontSize:12,fontWeight:700,color:T.headingFg}}>{pct}%</span>
    {err > 0 && <span style={{fontSize:11,fontWeight:600,color:'#b91c1c',background:'#fdecec',borderRadius:999,padding:'1px 8px'}}>веса {err}</span>}
  </div>;
}

// Строка таблицы периодов.
function PeriodRow({p, stat, cols, first, onOpen, actions}) {
  const [hover, setHover] = useState(false);
  const s = PERIOD_STATUS[p.status] || PERIOD_STATUS.closed;
  const root = p.depth === 0;
  return <div onClick={onOpen}
    onMouseEnter={()=>setHover(true)} onMouseLeave={()=>setHover(false)}
    style={{display:'grid',gridTemplateColumns:cols,alignItems:'center',gap:12,padding:'13px 20px',borderTop:first?'none':'1px solid '+T.hairline,cursor:'pointer',background:hover?'#faf9ff':'white',transition:'background .12s'}}>
    <div style={{display:'flex',alignItems:'center',gap:10,minWidth:0,paddingLeft:p.depth*22}}>
      <span style={{width:8,height:8,borderRadius:999,background:s.dot,flexShrink:0}}/>
      <span style={{fontSize:root?14.5:13.5,fontWeight:root?700:500,color:T.headingFg,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{p.name}</span>
    </div>
    <div style={{minWidth:0}}>
      <div style={{fontSize:12.5,color:T.mutedFg,fontFamily:'ui-monospace,Menlo,monospace'}}>{fmtDateShort(p.start_date)} – {fmtDateShort(p.end_date)}</div>
      {stat ? <PeriodMetrics stat={stat}/> : <div style={{height:16}}/>}
    </div>
    <div><PeriodBadge status={p.status}/></div>
    <div style={{display:'flex',gap:4,alignItems:'center',justifyContent:'flex-end'}}>{actions}</div>
  </div>;
}

// Модалка «Управление периодом»: шапка + общий компонент обзора (period_overview_view.js).
function PeriodOverviewModal({period, onEdit, onDelete, reload}) {
  const [data, setData] = useState(null);
  const [busy, setBusy] = useState(false);

  const load = () => apiGet(`/api/v1/periods/${period.id}/overview?scope=org`).then(r => r && r.json()).then(setData).catch(()=>{});
  useEffect(() => { load(); }, [period.id]);

  async function onApply(ep) {
    if (busy) return;
    setBusy(true);
    try {
      const res = await apiPost(`/api/v1/admin/periods/${period.id}/teams/${ep}`, {});
      if (!res || !res.ok) { alert('Ошибка операции'); return; }
      await load();
      reload();
    } finally { setBusy(false); }
  }

  return <div>
    <div style={{padding:'18px 22px',borderBottom:'1px solid '+T.hairline,display:'flex',alignItems:'flex-start',gap:16}}>
      <div style={{flex:1}}>
        <div style={{fontSize:20,fontWeight:800,color:T.headingFg}}>{period.name}</div>
        <div style={{fontSize:12.5,color:T.mutedFg,marginTop:4,display:'flex',alignItems:'center',gap:10}}>
          <span style={{fontFamily:'ui-monospace,Menlo,monospace'}}>{fmtDateShort(period.start_date)} — {fmtDateShort(period.end_date)}</span>
          <PeriodBadge status={period.status}/>
        </div>
      </div>
      <Btn onClick={onEdit}>Редактировать</Btn>
      <Btn danger onClick={onDelete}>Удалить</Btn>
    </div>
    <PeriodOverviewContent data={data} busy={busy} onApply={onApply} isAdmin={true} scope="org"/>
  </div>;
}

function PeriodsSection({periods, reload}) {
  const [modal, setModal] = useState(null); // {mode:'new', parent} | {mode:'edit', period}
  const periodCloseRef = useRef(null);
  const [saving, setSaving] = useState(false);
  const [overview, setOverview] = useState(null); // period selected for the management modal
  const [stats, setStats] = useState({}); // periodID -> {total_teams, teams_with_goals, avg_progress, weight_error_count}

  useEffect(() => {
    let alive = true;
    apiGet('/api/v1/admin/periods/stats').then(r => r && r.json()).then(data => {
      if (!alive || !data || !data.items) return;
      const m = {};
      for (const it of data.items) m[it.period_id] = it;
      setStats(m);
    }).catch(()=>{});
    return () => { alive = false; };
  }, [periods]);

  const openNew = parent => setModal({mode:'new', parent: parent||null});
  const openEdit = period => setModal({mode:'edit', period});

  async function remove(p) {
    if (!confirm(`Удалить период «${p.name}»? Цели внутри останутся, но не будут отображаться.`)) return;
    const res = await apiDel(`/api/v1/admin/periods/${p.id}`);
    if (res && res.ok) { setModal(null); reload(); }
    else alert('Ошибка удаления периода');
  }
  async function toggleArchive(p) {
    const ep = p.status==='archived' ? 'unarchive' : 'archive';
    const res = await apiPost(`/api/v1/admin/periods/${p.id}/${ep}`, {});
    if (res && res.ok) reload();
    else if (res && res.status===409 && ep==='archive') alert('Архивировать можно только закрытый период.');
    else alert('Ошибка изменения статуса');
  }
  async function save(f) {
    setSaving(true);
    try {
      const body = {name: f.name.trim(), start_date: f.start_date, end_date: f.end_date};
      let res;
      if (f.id) res = await apiPatch(`/api/v1/admin/periods/${f.id}`, body);
      else        res = await apiPost('/api/v1/admin/periods', body);
      if (!res || !res.ok) { alert('Ошибка сохранения'); return; }
      setModal(null);
      reload();
    } finally { setSaving(false); }
  }

  // Последняя колонка — фиксированной ширины: набор действий у строк разный
  // (у «Закрыто» есть «В архив»), и `auto` заставлял бы колонки ДАТЫ/СТАТУС
  // разъезжаться между строками и относительно заголовка.
  const cols = 'minmax(0,1fr) 260px 150px 210px';
  const hdrCell = {fontSize:11,color:T.dimFg,fontWeight:700,textTransform:'uppercase',letterSpacing:.5};

  return <div style={{padding:'20px 24px 24px'}}>
    <div style={{display:'flex',alignItems:'flex-start',justifyContent:'space-between',gap:16,marginBottom:18}}>
      <div>
        <div style={{fontSize:24,fontWeight:800,color:T.headingFg,letterSpacing:'-.4px'}}>Периоды целеполагания</div>
        <div style={{fontSize:13,color:T.mutedFg,marginTop:5}}>Актуальные и будущие — выше. Вложенность: год → кварталы.</div>
      </div>
      <Btn variant="primary" onClick={()=>openNew(null)} style={{flexShrink:0}}>+ Период</Btn>
    </div>

    <StatusFlowGuide/>

    <div style={{background:'white',border:'1px solid '+T.cardBorder,borderRadius:12,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}>
      <div style={{display:'grid',gridTemplateColumns:cols,gap:12,padding:'11px 20px',background:'#f8fafc',borderBottom:'1px solid '+T.cardBorder}}>
        <span style={hdrCell}>Период</span>
        <span style={hdrCell}>Даты</span>
        <span style={hdrCell}>Статус</span>
        <span/>
      </div>
      {periods.length === 0
        ? <div style={{padding:'40px 20px',textAlign:'center',color:T.mutedFg,fontSize:13}}>
            Периодов пока нет. Создайте первый — кнопка «+ Период» справа вверху.
          </div>
        : periods.map((p, i) => {
            const actions = [];
            if (p.status === 'closed')   actions.push(<IconBtn key="arch" title="В архив — скрыть из активных списков" onClick={()=>toggleArchive(p)}>{Icons.archiveIn}</IconBtn>);
            if (p.status === 'archived') actions.push(<IconBtn key="unarch" title="Вернуть из архива" onClick={()=>toggleArchive(p)}>{Icons.archiveOut}</IconBtn>);
            actions.push(<IconBtn key="manage" title="Управление периодом" onClick={()=>setOverview(p)}>{Icons.gear}</IconBtn>);
            actions.push(<IconBtn key="add" title="Создать вложенный период" onClick={()=>openNew(p)}>{Icons.nested}</IconBtn>);
            actions.push(<IconBtn key="edit" title="Редактировать период" onClick={()=>openEdit(p)}>{Icons.pencil}</IconBtn>);
            actions.push(<IconBtn key="del" danger title="Удалить период" onClick={()=>remove(p)}>{Icons.trash}</IconBtn>);
            return <PeriodRow key={p.id} p={p} stat={stats[p.id]} cols={cols} first={i===0} onOpen={()=>openEdit(p)} actions={actions}/>;
          })}
    </div>

    <Modal open={!!modal}
      title={modal && modal.mode==='new' ? 'Новый период' : 'Редактировать период'}
      subtitle={modal && modal.mode==='new'
        ? 'Заполните название и даты. Статус вычисляется по датам.'
        : 'Измените название и даты. Статус вычисляется по датам.'}
      onClose={()=>setModal(null)} width={560} guarded closeRef={periodCloseRef}>
      {modal && <PeriodModalBody
        key={modal.mode==='edit' ? 'edit-'+modal.period.id : 'new-'+(modal.parent?modal.parent.id:'root')}
        modal={modal} saving={saving}
        onSave={save} onClose={()=>setModal(null)} closeRef={periodCloseRef}
        onDelete={modal.mode==='edit' ? ()=>remove(modal.period) : null}/>}
    </Modal>

    <Modal open={!!overview}
      title={overview ? `Управление периодом · ${overview.name}` : ''}
      subtitle="Статусы команд, ошибки весов и массовые операции"
      onClose={()=>setOverview(null)} width={820}>
      {overview && <PeriodOverviewModal
        period={overview}
        onEdit={()=>{ const p=overview; setOverview(null); openEdit(p); }}
        onDelete={()=>{ const p=overview; setOverview(null); remove(p); }}
        reload={reload}/>}
    </Modal>
  </div>;
}

function PeriodModalBody({modal, saving, onSave, onClose, onDelete, closeRef}) {
  const isNew = modal.mode === 'new';
  const period = modal.period;
  const parent = modal.parent;
  const initial = isNew
    ? {name:'', start_date: parent ? fmtDate(parent.start_date) : '', end_date: parent ? fmtDate(parent.end_date) : ''}
    : {id: period.id, name: period.name, start_date: fmtDate(period.start_date), end_date: fmtDate(period.end_date)};
  const isArchived = !isNew && period.status === 'archived';

  const [f, setF] = useState(initial);
  const canSave = f.name.trim() && f.start_date && f.end_date;
  const isDirty = JSON.stringify(f) !== JSON.stringify(initial);
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave: !!canSave && !saving, onSave: () => { if (canSave) onSave(f); }, onClose });
  useEffect(() => { if (closeRef) closeRef.current = requestClose; }, [closeRef, requestClose]);
  const preview = periodStatusPreview(f.start_date, f.end_date, isArchived);

  return <div>
    <div style={{padding:'18px 22px 4px'}}>
      <Field label="Название" required>
        <input value={f.name} onChange={e=>setF({...f,name:e.target.value})} placeholder="Y26 · Y26-Q1 · …" style={inpStyle} autoFocus/>
      </Field>
      <div style={{display:'flex',gap:9,padding:'10px 13px',background:'#f8fafc',border:'1px solid '+T.hairline,borderRadius:9,marginBottom:16}}>
        <span style={{color:T.info,fontSize:13,lineHeight:1.5,flexShrink:0}}>ⓘ</span>
        <span style={{fontSize:12,color:T.mutedFg,lineHeight:1.5}}>Вложенность определяется автоматически по датам: период попадёт внутрь любого, чей диапазон его охватывает.</span>
      </div>
      <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:16}}>
        <Field label="Начало" required><input type="date" value={f.start_date} onChange={e=>setF({...f,start_date:e.target.value})} style={inpStyle}/></Field>
        <Field label="Конец" required><input type="date" value={f.end_date} onChange={e=>setF({...f,end_date:e.target.value})} style={inpStyle}/></Field>
      </div>
      <Field label="Статус">
        <div style={{display:'flex',alignItems:'center',gap:10,padding:'9px 13px',background:'#f8fafc',border:'1px solid '+T.hairline,borderRadius:9}}>
          {preview ? <PeriodBadge status={preview}/> : <span style={{fontSize:13,color:T.dimFg}}>—</span>}
          <span style={{fontSize:12,color:T.dimFg}}>определяется автоматически по датам</span>
        </div>
      </Field>
    </div>
    <div style={{display:'flex',alignItems:'center',gap:8,padding:'14px 22px',borderTop:'1px solid '+T.hairline,background:'#fafbfc'}}>
      {onDelete && <Btn danger onClick={onDelete} disabled={saving}>Удалить</Btn>}
      <div style={{flex:1}}/>
      <Btn onClick={onClose} disabled={saving}>Отмена</Btn>
      <Btn variant="primary" onClick={()=>canSave&&onSave(f)} disabled={!canSave||saving}>{saving ? 'Сохранение…' : isNew ? 'Создать' : 'Сохранить'}</Btn>
    </div>
    {confirmEl}
  </div>;
}

// ── TEAMS SECTION ────────────────────────────────────────────────────────────
// Полноширинное дерево с drill-down: по умолчанию раскрыты только корневые узлы.
// Раскрытие хранится во множестве id (expanded) и в localStorage, чтобы
// сохраняться между перезагрузками данных и страницы. При поиске сворачивание
// игнорируется — показываются все совпадения с их путём в иерархии.
const TEAMS_EXPANDED_KEY = 'okr_admin_teams_expanded';
function readTeamsExpanded() {
  try { const a = JSON.parse(localStorage.getItem(TEAMS_EXPANDED_KEY)); return new Set(Array.isArray(a)?a:[]); }
  catch { return new Set(); }
}
function writeTeamsExpanded(set) {
  try { localStorage.setItem(TEAMS_EXPANDED_KEY, JSON.stringify([...set])); } catch {}
}
function TeamsSection({teams, reload}) {
  const [q, setQ] = useState('');
  const [expanded, setExpanded] = useState(readTeamsExpanded);
  useEffect(()=>{ writeTeamsExpanded(expanded); },[expanded]);
  const [modal, setModal] = useState(null); // {mode:'new', parentId} | {mode:'edit', team}
  const [saving, setSaving] = useState(false);
  const teamCloseRef = useRef(null);

  const activeTeams = teams.filter(t=>!t.deleted_at);
  const deletedTeams = teams.filter(t=>!!t.deleted_at);

  // Direct children grouped by parent, each group ordered by type then name.
  const childrenMap = useMemo(()=>{
    const m = {};
    for (const t of activeTeams) { const k = t.parent_id!=null ? t.parent_id : '__root__'; (m[k]=m[k]||[]).push(t); }
    for (const k in m) m[k].sort((a,b)=>(TEAM_TYPE_ORDER[a.type]-TEAM_TYPE_ORDER[b.type])||a.name.localeCompare(b.name));
    return m;
  },[activeTeams]);
  const parentIds = useMemo(()=>activeTeams.filter(t=>(childrenMap[t.id]||[]).length>0).map(t=>t.id),[activeTeams,childrenMap]);
  const allExpanded = parentIds.length>0 && parentIds.every(id=>expanded.has(id));

  const ql = q.trim().toLowerCase();
  const searchActive = !!ql;
  const filteredIds = useMemo(()=>{
    if (!ql) return null;
    const out = new Set();
    for (const t of activeTeams) { if (t.name.toLowerCase().includes(ql)) teamPath(activeTeams,t.id).forEach(n=>out.add(n.id)); }
    return out;
  },[ql, activeTeams]);

  // Flatten the visible tree into rows (depth + whether the node is open).
  const rows = [];
  (function walk(nodes, depth) {
    for (const t of nodes) {
      if (searchActive && !filteredIds.has(t.id)) continue;
      const kids = childrenMap[t.id] || [];
      const open = searchActive ? true : expanded.has(t.id);
      rows.push({t, depth, kidsCount:kids.length, open});
      if (kids.length && open) walk(kids, depth+1);
    }
  })(childrenMap['__root__'] || [], 0);

  const toggle = id => setExpanded(s=>{ const n = new Set(s); n.has(id) ? n.delete(id) : n.add(id); return n; });
  const toggleAll = () => setExpanded(allExpanded ? new Set() : new Set(parentIds));
  const leadHash = s => { let h=0; for (let i=0;i<(s||'').length;i++) h=(h*31+s.charCodeAt(i))|0; return h; };

  async function save(fv) {
    setSaving(true);
    try {
      const body = {name:fv.name.trim(), type:fv.type, parent_id:fv.parent_id||null, lead:fv.lead||'', lead_udid:fv.lead_udid||null, description:fv.description||''};
      const res = fv.id ? await apiPatch(`/api/v1/admin/teams/${fv.id}`, body) : await apiPost('/api/v1/admin/teams', body);
      if (!res || !res.ok) { alert('Ошибка сохранения'); return; }
      setModal(null); reload();
    } finally { setSaving(false); }
  }
  async function remove(id, name) {
    if (!confirm(`Деактивировать команду «${name}»?`)) return;
    const res = await apiDel(`/api/v1/admin/teams/${id}`);
    if (res && res.ok) { setModal(null); reload(); } else alert('Ошибка удаления');
  }
  async function restore(id) {
    const res = await apiPost(`/api/v1/admin/teams/${id}/restore`, {});
    if (res && res.ok) reload(); else alert('Ошибка восстановления');
  }
  async function hardDelete(id, name) {
    if (!confirm(`Удалить «${name}» безвозвратно вместе со всеми данными?`)) return;
    const res = await apiDel(`/api/v1/admin/teams/${id}/hard`);
    if (res && res.ok) reload(); else alert('Ошибка удаления');
  }

  const modalValue = modal ? (modal.mode==='edit' ? modal.team : {name:'',type:'team',parent_id:modal.parentId??null,lead:'',lead_udid:null,description:''}) : null;
  const modalTitle = modal ? (modal.mode==='edit' ? `Редактирование · ${modal.team.name}` : 'Новая команда') : '';
  const modalSubtitle = (()=>{
    if (!modal) return null;
    if (modal.mode==='edit') return teamPath(activeTeams,modal.team.id).slice(0,-1).map(t=>t.name).join(' › ') || 'Корневой узел';
    if (modal.parentId!=null) return 'Внутри: '+teamPath(activeTeams,modal.parentId).map(t=>t.name).join(' › ');
    return 'Новый корневой кластер';
  })();

  return <div style={{padding:'20px 24px 24px'}}>
    <div style={{background:'white',borderRadius:14,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}>
      <div style={{padding:'14px 16px',borderBottom:'1px solid '+T.hairline,display:'flex',gap:10,alignItems:'center'}}>
        <input value={q} onChange={e=>setQ(e.target.value)} placeholder="Поиск команды…" style={{...inpStyle,flex:1,padding:'9px 14px'}}/>
        <Btn onClick={toggleAll} disabled={parentIds.length===0}>{allExpanded?'Свернуть всё':'Раскрыть всё'}</Btn>
        <Btn variant="primary" onClick={()=>setModal({mode:'new',parentId:null})}>+ Новая команда</Btn>
      </div>
      <div style={{padding:'10px 16px',borderBottom:'1px solid '+T.hairline,fontSize:11,color:T.mutedFg,fontWeight:700,textTransform:'uppercase',letterSpacing:.5,background:'#fafbfc'}}>
        Иерархия · {activeTeams.length}
      </div>
      <div>
        {rows.length===0 && <div style={{padding:'40px 16px',textAlign:'center',color:T.dimFg,fontSize:13}}>{searchActive?`Не найдено: «${q}»`:'Команд пока нет'}</div>}
        {rows.map(({t,depth,kidsCount,open})=>{
          const color = TEAM_TYPE_COLOR[t.type]||T.mutedFg;
          return <div key={t.id} onClick={()=>setModal({mode:'edit',team:t})}
            style={{display:'flex',alignItems:'center',gap:10,padding:'9px 16px 9px '+(12+depth*24)+'px',borderBottom:'1px solid '+T.hairline,cursor:'pointer',background:'white'}}
            onMouseEnter={e=>e.currentTarget.style.background='#fafbfc'}
            onMouseLeave={e=>e.currentTarget.style.background='white'}>
            {kidsCount>0
              ? <button onClick={e=>{e.stopPropagation();if(!searchActive)toggle(t.id);}} title={open?'Свернуть':'Раскрыть'}
                  style={{width:18,height:18,flexShrink:0,border:'none',background:'none',cursor:searchActive?'default':'pointer',color:T.mutedFg,fontSize:10,padding:0,lineHeight:1,display:'inline-flex',alignItems:'center',justifyContent:'center'}}>{open?'▼':'▶'}</button>
              : <span style={{width:18,flexShrink:0}}/>}
            <span style={{width:10,height:10,borderRadius:3,background:color,flexShrink:0}}/>
            <div style={{flex:1,minWidth:0}}>
              <div style={{display:'flex',alignItems:'center',gap:7}}>
                <TypeBadge type={t.type}/>
                <span style={{fontSize:14,fontWeight:600,color:T.headingFg,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{t.name}</span>
                {kidsCount>0&&<span style={{fontSize:11,fontWeight:700,color:T.mutedFg,background:'#eef1f5',borderRadius:10,padding:'1px 8px',flexShrink:0}}>{kidsCount}</span>}
              </div>
              {t.lead&&<div style={{display:'flex',alignItems:'center',gap:6,marginTop:3}}>
                <span style={{width:18,height:18,borderRadius:'50%',background:avatarColor(leadHash(t.lead)),color:'white',fontSize:8.5,fontWeight:700,display:'inline-flex',alignItems:'center',justifyContent:'center',flexShrink:0,letterSpacing:'-.2px'}}>{avatarInitials(t.lead)}</span>
                <span style={{fontSize:12,color:T.mutedFg,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{t.lead}</span>
              </div>}
            </div>
            <div onClick={e=>e.stopPropagation()} style={{display:'flex',gap:6,flexShrink:0}}>
              <RowAction title="Добавить дочернюю команду" onClick={()=>setModal({mode:'new',parentId:t.id})}>+</RowAction>
              <RowAction title="Редактировать" onClick={()=>setModal({mode:'edit',team:t})}>✎</RowAction>
              <RowAction title="Удалить" danger onClick={()=>remove(t.id,t.name)}>×</RowAction>
            </div>
          </div>;
        })}
      </div>
      {deletedTeams.length>0&&<>
        <div style={{padding:'8px 16px',fontSize:10,color:T.danger,fontWeight:700,textTransform:'uppercase',letterSpacing:.5,background:'#fff5f5',borderTop:'1px solid '+T.hairline,borderBottom:'1px solid '+T.hairline}}>Деактивированные · {deletedTeams.length}</div>
        {deletedTeams.map(t=>(
          <div key={t.id} style={{display:'flex',alignItems:'center',gap:10,padding:'9px 16px',borderBottom:'1px solid '+T.hairline,background:'white'}}>
            <span style={{width:18,flexShrink:0}}/>
            <span style={{width:10,height:10,borderRadius:3,background:'#e5e7eb',flexShrink:0}}/>
            <div style={{flex:1,minWidth:0,opacity:.6,display:'flex',alignItems:'center',gap:7}}>
              <TypeBadge type={t.type}/>
              <span style={{fontSize:14,fontWeight:600,color:T.headingFg,textDecoration:'line-through'}}>{t.name}</span>
            </div>
            <div style={{display:'flex',gap:6,flexShrink:0}}>
              <RowAction title="Восстановить" onClick={()=>restore(t.id)}>↩</RowAction>
              <RowAction title="Удалить безвозвратно" danger onClick={()=>hardDelete(t.id,t.name)}>✕</RowAction>
            </div>
          </div>
        ))}
      </>}
    </div>
    <Modal open={!!modal} title={modalTitle} subtitle={modalSubtitle} onClose={()=>setModal(null)} width={680} guarded closeRef={teamCloseRef}>
      {modal&&<TeamEditor value={modalValue} teams={activeTeams} onSave={save} onClose={()=>setModal(null)} closeRef={teamCloseRef}
        onDelete={modal.mode==='edit'?()=>remove(modal.team.id,modal.team.name):null} saving={saving}/>}
    </Modal>
  </div>;
}

// ── USER SELECTOR ────────────────────────────────────────────────────────────
let _adminAllUsers = null;

async function _adminLoadUsers() {
  if (_adminAllUsers) return _adminAllUsers;
  try {
    const res = await apiGet('/api/v1/admin/users');
    const data = res ? await res.json() : [];
    _adminAllUsers = Array.isArray(data) ? data.map(u => ({
      id: u.ID,
      udid: u.UDID,
      display_name: u.DisplayName,
      avatar_url: u.AvatarURL,
      email: u.Email,
    })) : [];
    return _adminAllUsers;
  } catch { return []; }
}

function _adminFilterUsers(users, q) {
  if (!q) return users;
  const low = q.toLowerCase();
  return users.filter(u => u.display_name?.toLowerCase().includes(low) || u.email?.toLowerCase().includes(low));
}

function UserAvatar({user, size=24}) {
  if (user?.avatar_url) return <img src={user.avatar_url} width={size} height={size} alt="" style={{borderRadius:'50%',objectFit:'cover',flexShrink:0,display:'block'}}/>;
  return <span className="user-avatar__fallback" style={{width:size,height:size,fontSize:Math.round(size*0.45),lineHeight:1}}>{user?.display_name?.[0]?.toUpperCase()||'?'}</span>;
}

function UserSelector({value, onChange, placeholder='Поиск пользователя…'}) {
  const [allUsers, setAllUsers] = useState([]);
  const [inputVal, setInputVal] = useState(value||'');
  const [open, setOpen] = useState(false);
  const [hi, setHi] = useState(0);
  const inputRef = useRef(null);
  const wrapRef = useRef(null);

  useEffect(()=>{ _adminLoadUsers().then(setAllUsers); },[]);
  // Sync displayed text when external value changes (e.g. form reset)
  useEffect(()=>{ if(!open) setInputVal(value||''); },[value]);
  // Close dropdown on outside click, revert input to selected value
  useEffect(()=>{
    if(!open) return;
    const h = e=>{if(wrapRef.current&&!wrapRef.current.contains(e.target)){setOpen(false);setInputVal(value||'');}};
    document.addEventListener('mousedown',h);
    return ()=>document.removeEventListener('mousedown',h);
  },[open,value]);

  const users = _adminFilterUsers(allUsers, open ? inputVal : '');

  const handleInput = v => {
    setInputVal(v); setHi(0);
    if(!v.trim()) onChange('', null);
  };

  const selectedUser = value ? (allUsers.find(u=>u.display_name===value) || null) : null;

  const select = u=>{onChange(u.display_name, u.udid);setInputVal(u.display_name);setOpen(false);};
  const clear = e=>{e.stopPropagation();onChange('', null);setInputVal('');inputRef.current?.focus();};
  const onKey = e=>{
    if(e.key==='ArrowDown'){e.preventDefault();setOpen(true);setHi(h=>Math.min(users.length-1,h+1));}
    else if(e.key==='ArrowUp'){e.preventDefault();setHi(h=>Math.max(0,h-1));}
    else if(e.key==='Enter'){e.preventDefault();if(open&&users[hi])select(users[hi]);}
    else if(e.key==='Escape'){if(open){e.preventDefault();setOpen(false);setInputVal(value||'');}}
  };

  return (
    <div ref={wrapRef} className="user-selector">
      <div className={`user-selector__field${open?' user-selector__field--open':''}`}
        style={{flexWrap:'nowrap',gap:6}}>
        {selectedUser&&<UserAvatar user={selectedUser} size={22}/>}
        <input ref={inputRef}
          value={inputVal}
          onChange={e=>{handleInput(e.target.value);setOpen(true);}}
          onFocus={()=>{setInputVal('');setOpen(true);setHi(0);}}
          onKeyDown={onKey}
          placeholder={value||placeholder}
          className="user-selector__input"
          autoComplete="off"/>
        {value&&<button type="button" onClick={clear} className="user-chip__remove" style={{flexShrink:0,fontSize:18,padding:'0 2px'}}>×</button>}
      </div>
      {open&&(
        <div className="user-selector__dropdown">
          {users.length===0
            ? <div className="user-selector__empty">{inputVal?'Пользователи не найдены':'Список пуст'}</div>
            : users.slice(0,20).map((u,i)=>(
              <div key={u.udid} onMouseDown={e=>{e.preventDefault();select(u);}} onMouseEnter={()=>setHi(i)}
                className={`user-selector__option${i===hi?' user-selector__option--hi':''}`}>
                <UserAvatar user={u} size={26}/>
                <div className="user-selector__option-info">
                  <span className="user-selector__option-name">{u.display_name}</span>
                  {u.led_team&&<span className="user-selector__option-team">{u.led_team}</span>}
                </div>
              </div>
            ))
          }
        </div>
      )}
    </div>
  );
}

// Modal body for creating/editing a team. The surrounding <Modal> supplies the
// header (title + breadcrumb + close); this renders the form fields and footer.
function TeamEditor({value, teams, onSave, onClose, onDelete, saving, closeRef}) {
  const [f, setF] = useState({...value});
  useEffect(()=>{setF({...value});},[value.id]);
  const canSave = f.name.trim() && f.type;
  const isDirty = JSON.stringify(f) !== JSON.stringify(value);
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave: !!canSave && !saving, onSave: () => { if (canSave) onSave(f); }, onClose });
  useEffect(()=>{ if (closeRef) closeRef.current = requestClose; },[closeRef, requestClose]);
  const isNew = !value.id;
  const children = value.id ? teams.filter(t=>t.parent_id===value.id) : [];
  const sep = <div style={{height:1,background:T.hairline,margin:'2px 0 16px'}}/>;

  return <div>
    <div style={{padding:'18px 24px'}}>
      <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:16}}>
        <Field label="Название" required><input value={f.name} onChange={e=>setF({...f,name:e.target.value})} style={inpStyle} autoFocus/></Field>
        <Field label="Тип">
          <select value={f.type} onChange={e=>setF({...f,type:e.target.value})} style={{...inpStyle,cursor:'pointer'}}>
            {['department','cluster','unit','group','team','squad','employee'].map(k=><option key={k} value={k}>{TEAM_TYPE_LABEL[k]}</option>)}
          </select>
        </Field>
      </div>
      <Field label="Описание" hint="необязательно"><MarkdownEditor value={f.description} onChange={v=>setF({...f,description:v})} rows={2} textareaStyle={{...inpStyle,border:'none',borderRadius:0,resize:'vertical',lineHeight:1.5,minHeight:64}}/></Field>
      {sep}
      <Field label="Руководитель"><UserSelector value={f.lead||''} onChange={(name, udid)=>setF({...f,lead:name,lead_udid:udid||null})} placeholder="Поиск пользователя…"/></Field>
      {sep}
      <Field label="Расположение в иерархии">
        <TeamCombobox
          single
          teams={teams}
          selectedIds={f.parent_id!=null?[f.parent_id]:[]}
          excludeIds={value.id?descendantIds(teams,value.id):undefined}
          placeholder="Без родителя (корневой кластер)"
          onChange={ids=>setF({...f,parent_id:ids.length?ids[0]:null})}
        />
      </Field>
      {!isNew&&children.length>0&&<div style={{fontSize:12,color:T.mutedFg,marginTop:-6}}>Внутри: {children.map(c=>c.name).join(', ')}</div>}
    </div>
    <div style={{padding:'14px 24px',borderTop:'1px solid '+T.hairline,background:'#fafbfc',borderRadius:'0 0 14px 14px',display:'flex',alignItems:'center',gap:10}}>
      {!isNew&&onDelete&&<Btn danger onClick={onDelete} disabled={saving}>Удалить</Btn>}
      <div style={{flex:1}}/>
      <Btn onClick={onClose} disabled={saving}>Отмена</Btn>
      <Btn variant="primary" onClick={()=>canSave&&onSave(f)} disabled={!canSave||saving}>{saving?'Сохранение…':isNew?'Создать':'Сохранить'}</Btn>
    </div>
    {confirmEl}
  </div>;
}

// ── HEALTH CHECK-IN SETTINGS PANEL ───────────────────────────────────────────
function HealthCheckInSettingsPanel() {
  const [cfg, setCfg] = useState(null);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState('');

  useEffect(() => {
    apiGet('/api/v1/admin/settings/health-checkin')
      .then(r => r && r.json())
      .then(d => d && setCfg(d));
  }, []);

  if (!cfg) return <div style={{padding:24, color: T.mutedFg}}>Загрузка…</div>;

  const update = (key, val) => setCfg(prev => ({...prev, [key]: val}));
  const updateCounter = (k, val) => setCfg(prev => ({...prev, in_counter: {...prev.in_counter, [k]: val}}));

  const save = async () => {
    setSaving(true); setMsg('');
    const res = await apiPost('/api/v1/admin/settings/health-checkin', cfg);
    setSaving(false);
    setMsg(res && res.ok ? 'Сохранено' : 'Ошибка сохранения');
  };

  const catConfig = [
    {
      key: 'stale', icon: '🕐', label: 'Нет обновлений',
      hint: 'Цели и KR без обновления прогресса более N дней. Руководителю нужно напомнить команде обновить прогресс.',
      param: { field: 'stale_days', label: 'Порог (дней без обновления)', min: 1 },
    },
    {
      key: 'no_goals', icon: '○', label: 'Не заведены цели',
      hint: 'Команды без ни одной цели в периоде. Руководителю нужно инициировать заведение OKR.',
    },
    {
      key: 'awaiting_validation', icon: '○', label: 'Ожидают перевода в работу',
      hint: 'Команды со статусом «К валидации». Нужно перевести в «В работе».',
    },
    {
      key: 'formation_errors', icon: '⚠', label: 'Ошибки формирования',
      hint: 'Суммы весов ≠ 100%, отсутствие KR, нулевые диапазоны. Мешают корректному расчёту прогресса.',
      param: { field: 'weight_tolerance', label: 'Допуск по весам (%)', min: 0 },
    },
    {
      key: 'lagging', icon: '▼', label: 'Отстающие',
      hint: 'Цели ниже ожидаемого темпа периода. Информационная категория — показывает риски.',
      param: { field: 'behind_margin', label: 'Отставание (п.п.)', min: 1 },
    },
    {
      key: 'comments', icon: '💬', label: 'Комментарии',
      hint: 'Нерешённые комментарии к целям ваших команд и команд под ними. Тумблер «В счётчик» включает их в бейдж (по умолчанию выключено). «Мои решённые» показываются всегда, их непросмотренный счётчик считается локально.',
      param: { field: 'comment_depth', label: 'Глубина команд (уровней вниз)', min: 0 },
    },
  ];

  const fieldStyle = {background:'#f8fafc', border:'1px solid #e5e7eb', borderRadius:6, padding:'6px 10px', fontSize:13, width:70, fontFamily:'inherit'};
  const labelStyle = {fontSize:13, color: T.bodyFg, fontWeight:500};
  const hintStyle  = {fontSize:12, color: T.mutedFg, marginTop:4, lineHeight:1.5};

  return (
    <div style={{padding:'20px 24px 32px'}}>
      <div style={{background:'white', borderRadius:12, border:'1px solid '+T.cardBorder, padding:'20px 24px'}}>
        <div style={{fontSize:15, fontWeight:700, color: T.headingFg, marginBottom:4}}>⚡ Health Check-in — настройки</div>
        <div style={{fontSize:13, color: T.mutedFg, marginBottom:24}}>
          Определяют, какие проблемы попадают в счётчик кнопки ⚡ Health Check-in в трекере.
        </div>

        {catConfig.map(cat => (
          <div key={cat.key} style={{borderTop:'1px solid #f1f5f9', paddingTop:16, marginTop:16}}>
            <div style={{display:'flex', alignItems:'flex-start', gap:12}}>
              <div style={{flex:1}}>
                <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:4}}>
                  <span style={{fontSize:16}}>{cat.icon}</span>
                  <span style={{fontSize:14, fontWeight:600, color: T.headingFg}}>{cat.label}</span>
                </div>
                <div style={hintStyle}>{cat.hint}</div>
                {cat.param && (
                  <div style={{display:'flex', alignItems:'center', gap:8, marginTop:10}}>
                    <span style={labelStyle}>{cat.param.label}:</span>
                    <input
                      type="number" min={cat.param.min}
                      value={cfg[cat.param.field] ?? ''}
                      onChange={e => update(cat.param.field, Number(e.target.value))}
                      style={fieldStyle}
                    />
                  </div>
                )}
              </div>
              <div style={{display:'flex', flexDirection:'column', alignItems:'flex-end', gap:4, flexShrink:0}}>
                <label style={{display:'flex', alignItems:'center', gap:6, cursor:'pointer'}}>
                  <input
                    type="checkbox"
                    checked={cfg.in_counter?.[cat.key] ?? false}
                    onChange={e => updateCounter(cat.key, e.target.checked)}
                  />
                  <span style={{fontSize:12, color: T.mutedFg}}>В счётчик</span>
                </label>
              </div>
            </div>
          </div>
        ))}

        <div style={{borderTop:'1px solid #f1f5f9', paddingTop:16, marginTop:16}}>
          <div style={{fontSize:14, fontWeight:600, color: T.headingFg, marginBottom:8}}>Цвет прогресса</div>
          <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:6}}>
            <span style={labelStyle}>Порог «в плане» (%):</span>
            <input
              type="number" min={1} max={100}
              value={cfg.green_threshold ?? 80}
              onChange={e => update('green_threshold', Number(e.target.value))}
              style={fieldStyle}
            />
          </div>
          <div style={hintStyle}>Цель или команда с прогрессом не ниже порога считается «в плане» и подсвечивается зелёным (в сайдбаре и на странице целей), независимо от ожидаемого темпа периода.</div>
        </div>

        <div style={{borderTop:'1px solid #f1f5f9', paddingTop:16, marginTop:16}}>
          <div style={{fontSize:14, fontWeight:600, color: T.headingFg, marginBottom:8}}>💬 Мои решённые комментарии</div>
          <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:6}}>
            <span style={labelStyle}>Сколько показывать (K):</span>
            <input
              type="number" min={1}
              value={cfg.resolved_comments_limit ?? 5}
              onChange={e => update('resolved_comments_limit', Number(e.target.value))}
              style={fieldStyle}
            />
          </div>
          <div style={hintStyle}>Сколько ваших последних решённых (не вами) комментариев показывать в колокольчике.</div>
        </div>

        <div style={{borderTop:'1px solid #f1f5f9', paddingTop:16, marginTop:16}}>
          <div style={{fontSize:14, fontWeight:600, color: T.headingFg, marginBottom:8}}>Кеш</div>
          <div style={{display:'flex', alignItems:'center', gap:8, marginBottom:6}}>
            <span style={labelStyle}>Время жизни (мин):</span>
            <input
              type="number" min={1}
              value={cfg.cache_ttl_minutes ?? 5}
              onChange={e => update('cache_ttl_minutes', Number(e.target.value))}
              style={fieldStyle}
            />
          </div>
          <div style={hintStyle}>Интервал фонового пересчёта. Меньше — актуальнее данные, больше — меньше нагрузка на БД.</div>
        </div>

        <div style={{marginTop:24, display:'flex', alignItems:'center', gap:12}}>
          <button
            onClick={save}
            disabled={saving}
            style={{padding:'8px 20px', background: T.accent, color:'white', border:'none', borderRadius:8, fontSize:13, fontWeight:600, cursor:'pointer'}}>
            {saving ? 'Сохранение…' : 'Сохранить'}
          </button>
          {msg && <span style={{fontSize:13, color: msg.startsWith('Ош') ? T.danger : T.success}}>{msg}</span>}
        </div>
      </div>
    </div>
  );
}

// ── ACCESS SETTINGS PANEL ────────────────────────────────────────────────────
function AccessSettingsPanel({teams}) {
  const [policy, setPolicy] = useState('empty');
  const [nodeId, setNodeId] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const activeTeams = (teams||[]).filter(t=>!t.deleted_at);

  useEffect(()=>{
    apiGet('/api/v1/admin/settings/access').then(r=>r&&r.json()).then(data=>{
      if (!data) return;
      setPolicy(data.new_user_policy||'empty');
      setNodeId(data.default_hierarchy_node_id ? String(data.default_hierarchy_node_id) : '');
    });
  },[]);

  async function save() {
    setSaving(true); setSaved(false);
    const body = {new_user_policy: policy};
    if (policy==='default_node') body.default_hierarchy_node_id = nodeId ? Number(nodeId) : null;
    const res = await apiPost('/api/v1/admin/settings/access', body);
    setSaving(false);
    if (res && res.ok) { setSaved(true); setTimeout(()=>setSaved(false), 2500); }
    else alert('Ошибка сохранения настроек');
  }

  function teamOptions(nodes, depth=0) {
    const opts = [];
    (nodes||[]).forEach(t=>{
      opts.push(<option key={t.id} value={t.id}>{'\u00A0'.repeat(depth*2)}{TEAM_TYPE_LABEL[t.type]||t.type} · {t.name}</option>);
      if (t.children) opts.push(...teamOptions(t.children, depth+1));
    });
    return opts;
  }

  const treeTeams = (function buildTree(flat) {
    const byId = Object.fromEntries(flat.map(t=>([t.id, {...t, children:[]}])));
    const roots = [];
    flat.forEach(t=>{ if (t.parent_id && byId[t.parent_id]) byId[t.parent_id].children.push(byId[t.id]); else roots.push(byId[t.id]); });
    return roots;
  })(activeTeams);

  return <div>
    <DetailHeader breadcrumb="Настройки" title="Политика для новых пользователей"
      subtitle="Определяет, что видит пользователь при первом входе в систему"/>
    <DetailSection title="Доступ по умолчанию">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:16,lineHeight:1.6}}>
        Применяется однократно при первом логине. После этого доступ управляется вручную через карточку пользователя.
      </div>
      <div style={{display:'flex',flexDirection:'column',gap:10,marginBottom:20}}>
        <label style={{display:'flex',alignItems:'flex-start',gap:10,cursor:'pointer',padding:'12px 14px',borderRadius:9,border:'1.5px solid '+(policy==='empty'?T.accent:'#e5e7eb'),background:policy==='empty'?'#f5f3ff':'white',transition:'all .12s'}}>
          <input type="radio" name="np" value="empty" checked={policy==='empty'} onChange={()=>setPolicy('empty')} style={{marginTop:2,accentColor:T.accent}}/>
          <div>
            <div style={{fontSize:13,fontWeight:600,color:T.bodyFg}}>Нет доступа</div>
            <div style={{fontSize:11.5,color:T.mutedFg,marginTop:2}}>Новый пользователь видит пустую иерархию. Администратор выдаёт доступ вручную.</div>
          </div>
        </label>
        <label style={{display:'flex',alignItems:'flex-start',gap:10,cursor:'pointer',padding:'12px 14px',borderRadius:9,border:'1.5px solid '+(policy==='default_node'?T.accent:'#e5e7eb'),background:policy==='default_node'?'#f5f3ff':'white',transition:'all .12s'}}>
          <input type="radio" name="np" value="default_node" checked={policy==='default_node'} onChange={()=>setPolicy('default_node')} style={{marginTop:2,accentColor:T.accent}}/>
          <div style={{flex:1,minWidth:0}}>
            <div style={{fontSize:13,fontWeight:600,color:T.bodyFg}}>Доступ к узлу по умолчанию</div>
            <div style={{fontSize:11.5,color:T.mutedFg,marginTop:2}}>При первом входе пользователю автоматически выдаётся доступ к выбранному узлу и всем его дочерним командам.</div>
            {policy==='default_node'&&<div style={{marginTop:10}}>
              <select value={nodeId} onChange={e=>setNodeId(e.target.value)}
                style={{...inpStyle,fontSize:13,cursor:'pointer'}}>
                <option value="">— выберите узел иерархии —</option>
                {teamOptions(treeTeams)}
              </select>
            </div>}
          </div>
        </label>
      </div>
      <div style={{display:'flex',alignItems:'center',gap:10}}>
        <Btn variant="primary" onClick={save} disabled={saving||(policy==='default_node'&&!nodeId)}>
          {saving?'Сохранение…':'Сохранить'}
        </Btn>
        {saved&&<span style={{fontSize:12,color:'#059669',fontWeight:600}}>✓ Сохранено</span>}
      </div>
    </DetailSection>
  </div>;
}

function ActivityLogPanel() {
  const [depth, setDepth] = useState('quarter');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');
  const labels = {quarter:'старше квартала', year:'старше года', all:'все'};
  async function purge() {
    if (!confirm(`Удалить логи активности (${labels[depth]})? Действие необратимо.`)) return;
    setBusy(true); setMsg('');
    const res = await apiPost('/api/v1/admin/activity/purge', {older_than: depth});
    setBusy(false);
    if (res && res.ok) { const j = await res.json(); setMsg(`Удалено записей: ${j.deleted}`); }
    else if (res && res.status===422) setMsg('Неверная глубина очистки');
    else setMsg('Ошибка очистки');
  }
  return <div>
    <DetailHeader breadcrumb="Настройки" title="Лог активности" subtitle="Очистка накопленных событий журнала"/>
    <DetailSection title="Очистить лог активности">
      <div style={{display:'flex',alignItems:'center',gap:10,flexWrap:'wrap'}}>
        <select value={depth} onChange={e=>setDepth(e.target.value)} style={{...inpStyle,width:'auto',fontSize:13,cursor:'pointer'}}>
          <option value="quarter">Старше квартала</option>
          <option value="year">Старше года</option>
          <option value="all">Всё</option>
        </select>
        <Btn danger onClick={purge} disabled={busy}>{busy?'Очистка…':'Очистить'}</Btn>
        {msg && <span style={{fontSize:12,color:'#6b7280',fontWeight:600}}>{msg}</span>}
      </div>
    </DetailSection>
  </div>;
}

function GeneralSettingsPanel() {
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [emptyMsg, setEmptyMsg] = useState('');
  const [snapshotDays, setSnapshotDays] = useState(1);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(()=>{
    apiGet('/api/v1/admin/settings/general').then(r=>r&&r.json()).then(data=>{
      if (data) { setName(data.name||''); setUrl(data.documentation_url||''); setEmptyMsg(data.empty_hierarchy_message||''); if (data.progress_snapshot_interval_days >= 1) setSnapshotDays(data.progress_snapshot_interval_days); }
    });
  },[]);

  async function save() {
    if (!name.trim()) { alert('Укажите название пространства.'); return; }
    setSaving(true); setSaved(false);
    const res = await apiPost('/api/v1/admin/settings/general', {name: name.trim(), documentation_url: url.trim(), empty_hierarchy_message: emptyMsg, progress_snapshot_interval_days: Math.max(1, Number(snapshotDays)||1)});
    setSaving(false);
    if (res && res.ok) { setSaved(true); setTimeout(()=>setSaved(false), 2500); }
    else if (res && res.status===400) alert('Проверьте название пространства и ссылку на документацию.');
    else alert('Ошибка сохранения настроек');
  }

  return <div>
    <DetailHeader breadcrumb="Настройки" title="Общие настройки"
      subtitle="Название пространства, документация и сообщения"/>
    <DetailSection title="Название пространства">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:16,lineHeight:1.6}}>
        Отображается в переключателе пространств и заголовках. Slug пространства меняется только в системном разделе.
      </div>
      <input type="text" value={name} onChange={e=>setName(e.target.value)}
        placeholder="Название пространства"
        style={{...inpStyle,fontSize:13,marginBottom:16}}/>
    </DetailSection>
    <DetailSection title="Ссылка на документацию">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:16,lineHeight:1.6}}>
        Если ссылка указана, в меню пользователя появляется пункт «Документация». Оставьте поле пустым, чтобы скрыть пункт.
      </div>
      <input type="url" value={url} onChange={e=>setUrl(e.target.value)}
        placeholder="https://github.com/ShamanR/okrs/wiki"
        style={{...inpStyle,fontSize:13,marginBottom:16}}/>
    </DetailSection>
    <DetailSection title="Сообщение при отсутствии доступа к командам">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:16,lineHeight:1.6}}>
        Markdown. Показывается в трекере пользователю без доступных команд. Пусто → текст по умолчанию.
      </div>
      <div style={{marginBottom:16}}>
        <MarkdownEditor value={emptyMsg} onChange={setEmptyMsg} rows={4} textareaStyle={{...inpStyle,border:'none',borderRadius:0,resize:'vertical',lineHeight:1.5,minHeight:96}}/>
      </div>
    </DetailSection>
    <DetailSection title="График прогресса за период">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:16,lineHeight:1.6}}>
        Как часто фоновая задача фиксирует точку прогресса команд для графика за период. 1 — ежедневно.
      </div>
      <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:16}}>
        <span style={{fontSize:13,color:T.mutedFg}}>Частота снимков (дней):</span>
        <input type="number" min={1} value={snapshotDays}
          onChange={e=>setSnapshotDays(e.target.value)}
          onBlur={()=>setSnapshotDays(d=>Math.max(1, Math.floor(Number(d))||1))}
          style={{...inpStyle,fontSize:13,width:100}}/>
      </div>
      <div style={{display:'flex',alignItems:'center',gap:10}}>
        <Btn variant="primary" onClick={save} disabled={saving}>
          {saving?'Сохранение…':'Сохранить'}
        </Btn>
        {saved&&<span style={{fontSize:12,color:'#059669',fontWeight:600}}>✓ Сохранено</span>}
      </div>
    </DetailSection>
  </div>;
}

function FeedbackSettingsPanel() {
  const [url, setUrl] = useState('');
  const [popup, setPopup] = useState(false);
  const [menu, setMenu] = useState(false);
  const [freq, setFreq] = useState(30);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(()=>{
    apiGet('/api/v1/admin/settings/feedback').then(r=>r&&r.json()).then(data=>{
      if (!data) return;
      setUrl(data.feedback_url||'');
      setPopup(!!data.feedback_popup_enabled);
      setMenu(!!data.feedback_menu_link_enabled);
      setFreq(data.feedback_frequency_days||30);
    });
  },[]);

  async function save() {
    const n = parseInt(freq,10);
    if (!Number.isFinite(n) || n < 1) { alert('Частота должна быть целым числом ≥ 1.'); return; }
    setSaving(true); setSaved(false);
    const res = await apiPost('/api/v1/admin/settings/feedback', {
      feedback_url: url.trim(),
      feedback_popup_enabled: popup,
      feedback_menu_link_enabled: menu,
      feedback_frequency_days: n,
    });
    setSaving(false);
    if (res && res.ok) { setSaved(true); setTimeout(()=>setSaved(false), 2500); }
    else if (res && res.status===400) alert('Проверьте ссылку (http/https) и частоту (≥ 1).');
    else alert('Ошибка сохранения настроек');
  }

  const toggleRow = (checked, onChange, label, hint) => (
    <label style={{display:'flex',alignItems:'flex-start',gap:10,cursor:'pointer',marginBottom:14}}>
      <input type="checkbox" checked={checked} onChange={e=>onChange(e.target.checked)} style={{marginTop:3}}/>
      <span>
        <span style={{fontSize:13.5,fontWeight:600,color:T.bodyFg}}>{label}</span>
        <span style={{display:'block',fontSize:12,color:T.mutedFg,marginTop:2}}>{hint}</span>
      </span>
    </label>
  );

  return <div>
    <DetailHeader breadcrumb="Настройки" title="Обратная связь"
      subtitle="Сбор обратной связи по инструменту через внешний опрос"/>
    <DetailSection title="Ссылка на опрос">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:12,lineHeight:1.6}}>
        Ссылка на форму обратной связи. Пока поле пустое, пункт меню и всплывающее окно не показываются.
      </div>
      <input type="url" value={url} onChange={e=>setUrl(e.target.value)}
        placeholder="https://forms.gle/..."
        style={{...inpStyle,fontSize:13,marginBottom:4}}/>
    </DetailSection>
    <DetailSection title="Где показывать">
      {toggleRow(menu, setMenu, 'Пункт «Обратная связь» в меню', 'Постоянная ссылка в гамбургер-меню под «Документация».')}
      {toggleRow(popup, setPopup, 'Всплывающее окно с просьбой', 'Модальное окно появляется не ранее чем через 2 суток работы с инструментом.')}
    </DetailSection>
    <DetailSection title="Частота показа окна">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:10,lineHeight:1.6}}>
        Минимальный интервал между показами окна, в днях. Перерыв в работе дольше этого срока заново даёт пользователю 2 дня перед показом.
      </div>
      <input type="number" min="1" value={freq} onChange={e=>setFreq(e.target.value)}
        style={{...inpStyle,fontSize:13,width:140,marginBottom:16}}/>
      <div style={{display:'flex',alignItems:'center',gap:10}}>
        <Btn variant="primary" onClick={save} disabled={saving}>
          {saving?'Сохранение…':'Сохранить'}
        </Btn>
        {saved&&<span style={{fontSize:12,color:'#059669',fontWeight:600}}>✓ Сохранено</span>}
      </div>
    </DetailSection>
  </div>;
}

// ── USERS SECTION ────────────────────────────────────────────────────────────
function InviteLinksPanel() {
  const [links, setLinks] = useState([]);
  const [role, setRole] = useState('user');
  const [kind, setKind] = useState('once'); // once | unlimited | limited
  const [limit, setLimit] = useState(5);
  const [expires, setExpires] = useState(''); // yyyy-mm-dd or ''
  const [created, setCreated] = useState(null); // {url}
  const [busy, setBusy] = useState(false);

  const load = () => apiGet('/api/v1/admin/invitations').then(r=>r&&r.json()).then(d=>setLinks(Array.isArray(d)?d:[])).catch(()=>setLinks([]));
  useEffect(()=>{ load(); },[]);

  async function create() {
    setBusy(true);
    try {
      const body = {role};
      if (kind==='once') body.max_uses = 1;
      else if (kind==='limited') body.max_uses = Math.max(1, parseInt(limit,10)||1);
      // 'unlimited' → omit max_uses
      if (expires) body.expires_at = new Date(expires+'T23:59:59').toISOString();
      const r = await apiPost('/api/v1/admin/invitations', body);
      if (!r || !r.ok) { alert('Не удалось создать ссылку'); return; }
      const d = await r.json();
      setCreated({url:d.url});
      await load();
    } finally { setBusy(false); }
  }

  async function revoke(id) {
    const r = await apiPost(`/api/v1/admin/invitations/${id}/revoke`, {});
    if (r && r.ok) load(); else alert('Не удалось отозвать ссылку');
  }

  const usesLabel = l => l.max_uses==null ? `${l.use_count}/∞` : `${l.use_count}/${l.max_uses}`;

  return <div style={{background:'white',borderRadius:14,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden',marginBottom:16}}>
    <div style={{padding:'14px 16px',borderBottom:'1px solid '+T.hairline,fontSize:13,fontWeight:700,color:T.headingFg}}>Пригласительные ссылки</div>
    <div style={{padding:'14px 16px',display:'flex',gap:12,alignItems:'flex-end',flexWrap:'wrap',borderBottom:'1px solid '+T.hairline}}>
      <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,color:T.mutedFg,fontWeight:600}}>Роль
        <select value={role} onChange={e=>setRole(e.target.value)} style={{...inpStyle,padding:'8px 10px'}}>
          <option value="user">Пользователь</option>
          <option value="admin">Администратор</option>
        </select>
      </label>
      <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,color:T.mutedFg,fontWeight:600}}>Тип
        <select value={kind} onChange={e=>setKind(e.target.value)} style={{...inpStyle,padding:'8px 10px'}}>
          <option value="once">Одноразовая</option>
          <option value="unlimited">Многоразовая (без лимита)</option>
          <option value="limited">До N использований</option>
        </select>
      </label>
      {kind==='limited' && <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,color:T.mutedFg,fontWeight:600}}>N
        <input type="number" min="1" value={limit} onChange={e=>setLimit(e.target.value)} style={{...inpStyle,padding:'8px 10px',width:80}}/>
      </label>}
      <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,color:T.mutedFg,fontWeight:600}}>Срок (опц.)
        <input type="date" value={expires} onChange={e=>setExpires(e.target.value)} style={{...inpStyle,padding:'8px 10px'}}/>
      </label>
      <button onClick={create} disabled={busy} style={{padding:'9px 16px',border:'none',borderRadius:8,background:T.accent,color:'#fff',fontWeight:600,cursor:busy?'default':'pointer',fontFamily:'inherit',opacity:busy?0.6:1}}>Создать</button>
    </div>
    {created && <div style={{padding:'12px 16px',borderBottom:'1px solid '+T.hairline,display:'flex',gap:10,alignItems:'center',background:'#f5f3ff'}}>
      <input readOnly value={created.url} style={{...inpStyle,flex:1,fontFamily:'ui-monospace,Menlo,monospace',fontSize:12.5}} onFocus={e=>e.target.select()}/>
      <button onClick={()=>{ navigator.clipboard?.writeText(created.url); }} style={{padding:'9px 14px',border:'1.5px solid '+T.cardBorder,borderRadius:8,background:'#fff',color:T.accent,fontWeight:600,cursor:'pointer',fontFamily:'inherit'}}>Копировать</button>
    </div>}
    <div>
      {links.length===0 && <div style={{padding:'20px 16px',textAlign:'center',color:T.dimFg,fontSize:13}}>Активных ссылок нет</div>}
      {links.map(l=>(
        <div key={l.id} style={{display:'flex',alignItems:'center',gap:12,padding:'10px 16px',borderBottom:'1px solid '+T.hairline}}>
          <div style={{flex:1,minWidth:0}}>
            <div style={{fontSize:13.5,fontWeight:600,color:T.headingFg}}>{l.role==='admin'?'Администратор':'Пользователь'} · использовано {usesLabel(l)}</div>
            <div style={{fontSize:11.5,color:T.mutedFg,marginTop:2}}>{l.expires_at?`Действует до ${fmtDateTime(l.expires_at)}`:'Без срока'}</div>
          </div>
          <button onClick={()=>revoke(l.id)} style={{padding:'6px 12px',border:'1.5px solid '+T.cardBorder,borderRadius:7,background:'#fff',color:T.danger,fontWeight:600,cursor:'pointer',fontFamily:'inherit'}}>Отозвать</button>
        </div>
      ))}
    </div>
  </div>;
}

function UsersSection({users, teams, currentUser, reload}) {
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState('all'); // all | admins | noaccess
  const [modalId, setModalId] = useState(null);
  const userCloseRef = useRef(null);

  const activeTeams = useMemo(()=>teams.filter(t=>!t.deleted_at),[teams]);
  // Teams each user leads, keyed by lead UDID — used both for search and display.
  const ledByUdid = useMemo(()=>{
    const m = {};
    for (const t of activeTeams) { if (t.lead_udid) (m[t.lead_udid]=m[t.lead_udid]||[]).push(t); }
    return m;
  },[activeTeams]);
  const ledTeams = u => (u && u.UDID && ledByUdid[u.UDID]) || [];

  const isRequester = u => u.Status === 'requested';
  // Admin is tenant-scoped: the membership role in the active tenant, not a global flag.
  const isAdmin = u => u.Role === 'admin';
  const adminCount = users.filter(isAdmin).length;
  const requestCount = users.filter(isRequester).length;
  const noAccessCount = users.filter(u=>!isRequester(u) && !isAdmin(u) && (u.GrantedNodeCount||0)===0).length;

  const ql = q.trim().toLowerCase();
  const filtered = users.filter(u=>{
    if (filter==='admins' && !isAdmin(u)) return false;
    if (filter==='requests' && !isRequester(u)) return false;
    if (filter==='noaccess' && (isRequester(u) || isAdmin(u) || (u.GrantedNodeCount||0)>0)) return false;
    if (!ql) return true;
    const led = ledTeams(u).map(t=>t.name).join(' ');
    return [u.DisplayName, u.Email, u.Provider, led].some(s=>(s||'').toLowerCase().includes(ql));
  });

  const chips = [
    {id:'all', label:'Все пользователи', count:users.length},
    {id:'requests', label:'Заявки', count:requestCount},
    {id:'admins', label:'Админы', count:adminCount},
    {id:'noaccess', label:'Без доступов', count:noAccessCount},
  ];
  const activeChip = chips.find(c=>c.id===filter);
  const modalUser = users.find(u=>u.ID===modalId) || null;

  return <div style={{padding:'20px 24px 24px'}}>
    <InviteLinksPanel/>
    <div style={{background:'white',borderRadius:14,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}>
      <div style={{padding:'14px 16px',borderBottom:'1px solid '+T.hairline,display:'flex',gap:10,alignItems:'center',flexWrap:'wrap'}}>
        <input value={q} onChange={e=>setQ(e.target.value)} placeholder="Поиск по имени, роли, email, команде…" style={{...inpStyle,flex:1,minWidth:240,padding:'9px 14px'}}/>
        <div style={{display:'flex',gap:8,flexShrink:0}}>
          {chips.map(c=>{
            const on = c.id===filter;
            return <button key={c.id} onClick={()=>setFilter(c.id)}
              style={{display:'inline-flex',alignItems:'center',gap:7,padding:'8px 14px',borderRadius:20,border:'1.5px solid '+(on?T.accent:T.cardBorder),background:on?'#f5f3ff':'white',color:on?T.accent:T.bodyFg,fontSize:13,fontWeight:600,cursor:'pointer',fontFamily:'inherit'}}>
              {c.label}
              <span style={{fontSize:11,fontWeight:700,color:on?'white':T.mutedFg,background:on?T.accent:'#eef1f5',borderRadius:10,padding:'1px 8px',minWidth:18,textAlign:'center'}}>{c.count}</span>
            </button>;
          })}
        </div>
      </div>
      <div style={{padding:'10px 16px',borderBottom:'1px solid '+T.hairline,fontSize:11,color:T.mutedFg,fontWeight:700,textTransform:'uppercase',letterSpacing:.5,background:'#fafbfc'}}>
        {activeChip.label} · {filtered.length}
      </div>
      <div>
        {filtered.length===0 && <div style={{padding:'40px 16px',textAlign:'center',color:T.dimFg,fontSize:13}}>{ql?`Не найдено: «${q}»`:'Пользователей нет'}</div>}
        {filtered.map(u=>{
          const led = ledTeams(u);
          const nodes = u.GrantedNodeCount||0;
          const requester = isRequester(u);
          return <div key={u.ID} onClick={()=>{ if(!requester) setModalId(u.ID); }}
            style={{display:'flex',alignItems:'center',gap:12,padding:'10px 16px',borderBottom:'1px solid '+T.hairline,cursor:requester?'default':'pointer',background:'white'}}
            onMouseEnter={e=>e.currentTarget.style.background='#fafbfc'}
            onMouseLeave={e=>e.currentTarget.style.background='white'}>
            <Avatar user={u} size={36}/>
            <div style={{flex:1,minWidth:0}}>
              <div style={{display:'flex',alignItems:'center',gap:6}}>
                <span style={{fontSize:14,fontWeight:600,color:T.headingFg,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{u.DisplayName}</span>
                {u.ID===currentUser?.id&&<span style={{fontSize:9.5,color:T.mutedFg,background:'#f1f5f9',padding:'1px 6px',borderRadius:4,fontWeight:700,flexShrink:0}}>ВЫ</span>}
                {requester&&<span style={{fontSize:9.5,color:'#92400e',background:'#fef3c7',padding:'1px 6px',borderRadius:4,fontWeight:700,flexShrink:0}}>ЗАЯВКА</span>}
              </div>
              <div style={{fontSize:11.5,color:T.mutedFg,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap',marginTop:2}}>
                {u.Email}{led.length>0&&` · лид: ${led.map(t=>t.name).join(', ')}`}
              </div>
            </div>
            {requester
              ? <div onClick={e=>e.stopPropagation()} style={{display:'flex',alignItems:'center',gap:8,flexShrink:0}}>
                  <button onClick={async()=>{ const r=await apiPost(`/api/v1/admin/access-requests/${u.ID}/approve`,{}); if(r&&r.ok) reload(); else alert('Не удалось добавить пользователя'); }}
                    style={{padding:'6px 12px',border:'none',borderRadius:7,background:'#059669',color:'#fff',fontWeight:600,cursor:'pointer',fontFamily:'inherit'}}>Добавить</button>
                  <button onClick={async()=>{ const r=await apiPost(`/api/v1/admin/access-requests/${u.ID}/deny`,{}); if(r&&r.ok) reload(); else alert('Не удалось отклонить заявку'); }}
                    style={{padding:'6px 12px',border:'1.5px solid '+T.cardBorder,borderRadius:7,background:'#fff',color:T.danger,fontWeight:600,cursor:'pointer',fontFamily:'inherit'}}>Отклонить</button>
                </div>
              : <>
                  <div style={{display:'flex',flexDirection:'column',alignItems:'flex-end',gap:2,flexShrink:0}}>
                    {isAdmin(u)
                      ? <><Chip color="#92400e" bg="#fef3c7">Admin</Chip><span style={{fontSize:11,color:'#059669',fontWeight:600}}>полный доступ</span></>
                      : nodes>0
                        ? <span style={{fontSize:12.5,color:'#0891b2',fontWeight:600}}>{nodes} узл</span>
                        : <span style={{fontSize:12,color:'#dc2626',fontWeight:600}}>нет доступа</span>}
                  </div>
                  <div onClick={e=>e.stopPropagation()} style={{display:'flex',gap:6,flexShrink:0}}>
                    <RowAction title="Редактировать" onClick={()=>setModalId(u.ID)}>✎</RowAction>
                    {u.ID!==currentUser?.id &&
                      <RowAction title="Удалить из пространства" danger onClick={async()=>{
                        if(!confirm(`Удалить «${u.DisplayName}» из пространства? Пользователь потеряет членство и все доступы в этом пространстве.`)) return;
                        const res=await apiDel(`/api/v1/admin/members/${u.ID}`);
                        if(res&&res.ok) reload(); else alert('Не удалось удалить пользователя из пространства');
                      }}>🗑</RowAction>}
                  </div>
                </>}
          </div>;
        })}
      </div>
    </div>
    <Modal open={!!modalUser}
      title={modalUser&&<span style={{display:'inline-flex',alignItems:'center',gap:12}}><Avatar user={modalUser} size={36}/>{modalUser.DisplayName}{modalUser.ID===currentUser?.id&&<span style={{fontSize:10.5,color:T.mutedFg,background:'#f1f5f9',padding:'2px 7px',borderRadius:5,fontWeight:700}}>ВЫ</span>}</span>}
      subtitle={modalUser&&`${modalUser.Provider} · ${modalUser.Email}`}
      onClose={()=>setModalId(null)} width={760} guarded closeRef={userCloseRef}>
      {modalUser&&<UserModal user={modalUser} teams={teams} currentUser={currentUser} allUsers={users} ledTeams={ledTeams(modalUser)} onClose={()=>setModalId(null)} onSaved={()=>{setModalId(null);reload();}} closeRef={userCloseRef}/>}
    </Modal>
  </div>;
}

// Modal body for a user. The surrounding <Modal> supplies the header (avatar +
// name + close). Admin toggle and hierarchy grants are batched and applied on
// «Сохранить»; «Отмена» discards them.
function UserModal({user, teams, currentUser, allUsers, ledTeams, onClose, onSaved, closeRef}) {
  const [grants, setGrants] = useState(null);          // original grants loaded from API
  const [loading, setLoading] = useState(true);
  const [pendingGrantIds, setPendingGrantIds] = useState([]);
  const [pendingAdmin, setPendingAdmin] = useState(user.Role==='admin');
  const [saving, setSaving] = useState(false);

  useEffect(()=>{
    setLoading(true); setPendingAdmin(user.Role==='admin');
    apiGet(`/api/v1/admin/users/${user.ID}/grants`).then(r=>r&&r.json()).then(data=>{
      const arr = Array.isArray(data)?data:[];
      setGrants(arr); setPendingGrantIds(arr.map(g=>g.TeamID)); setLoading(false);
    }).catch(()=>setLoading(false));
  },[user.ID]);

  const isSelf = user.ID===currentUser?.id;
  const adminCount = allUsers.filter(u=>u.Role==='admin').length;
  const activeTeams = teams.filter(t=>!t.deleted_at);

  async function save() {
    setSaving(true);
    try {
      if (pendingAdmin !== (user.Role==='admin')) {
        if (!pendingAdmin && adminCount<=1) { alert('Нельзя снять admin-права с последнего администратора.'); return; }
        if (!pendingAdmin && isSelf && !confirm('Снять admin-права с себя? Вы потеряете доступ к этому разделу.')) return;
        const res = pendingAdmin ? await apiPost(`/api/v1/admin/users/${user.ID}/admin`, {}) : await apiDel(`/api/v1/admin/users/${user.ID}/admin`);
        if (!res || !res.ok) { alert('Ошибка изменения прав'); return; }
      }
      const orig = new Set((grants||[]).map(g=>g.TeamID));
      const next = new Set(pendingGrantIds);
      for (const id of next) if (!orig.has(id)) { const r = await apiPost(`/api/v1/admin/users/${user.ID}/grants`, {team_id:id}); if (!r||!r.ok) { alert('Ошибка выдачи доступа'); return; } }
      for (const id of orig) if (!next.has(id)) { const r = await apiDel(`/api/v1/admin/users/${user.ID}/grants/${id}`); if (!r||!r.ok) { alert('Ошибка отзыва доступа'); return; } }
      onSaved();
    } finally { setSaving(false); }
  }

  const grantsDirty = grants != null && (
    pendingGrantIds.length !== grants.length ||
    pendingGrantIds.some(id => !grants.some(g => g.TeamID === id))
  );
  const isDirty = (pendingAdmin !== (user.Role === 'admin')) || grantsDirty;
  const { requestClose, confirmEl } = useModalClose({ isDirty, canSave: !saving && !loading, onSave: save, onClose });
  useEffect(() => { if (closeRef) closeRef.current = requestClose; }, [closeRef, requestClose]);

  return <div>
    <DetailSection title="Учётная запись">
      <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:16}}>
        <Field label="Email"><input value={user.Email} readOnly style={{...inpStyle,background:'#f8fafc',color:T.mutedFg}}/></Field>
        <Field label="Последний вход"><input value={fmtDateTime(user.LastLoginAt)} readOnly style={{...inpStyle,background:'#f8fafc',color:T.mutedFg,fontFamily:'ui-monospace,Menlo,monospace',fontSize:13}}/></Field>
        <Field label="Провайдер"><input value={user.Provider} readOnly style={{...inpStyle,background:'#f8fafc',color:T.mutedFg,fontFamily:'ui-monospace,Menlo,monospace',fontSize:13}}/></Field>
        <Field label="ID пользователя"><input value={user.ID} readOnly style={{...inpStyle,background:'#f8fafc',color:T.mutedFg,fontFamily:'ui-monospace,Menlo,monospace',fontSize:13}}/></Field>
      </div>
    </DetailSection>
    <DetailSection title="Права администратора">
      <div onClick={()=>setPendingAdmin(v=>!v)} style={{display:'inline-flex',alignItems:'center',gap:13,cursor:'pointer'}}>
        <div style={{width:44,height:24,borderRadius:12,background:pendingAdmin?T.accent:'#cbd5e1',position:'relative',transition:'background .15s',flexShrink:0}}>
          <div style={{position:'absolute',top:2,left:pendingAdmin?22:2,width:20,height:20,borderRadius:'50%',background:'white',transition:'left .15s',boxShadow:'0 1px 3px rgba(0,0,0,0.25)'}}/>
        </div>
        <div>
          <div style={{fontSize:14,fontWeight:700,color:T.headingFg}}>{pendingAdmin?'Администратор':'Обычный пользователь'}</div>
          <div style={{fontSize:12,color:T.mutedFg,marginTop:1}}>{pendingAdmin?'Полный доступ к админ-разделу':'Нет доступа к админ-разделу'}</div>
        </div>
      </div>
    </DetailSection>
    <DetailSection title={`Является руководителем · ${ledTeams.length}`}>
      {ledTeams.length===0
        ? <div style={{fontSize:13,color:T.dimFg}}>Не является руководителем ни одной команды.</div>
        : ledTeams.map(t=>(
          <div key={t.id} style={{display:'flex',alignItems:'center',gap:10,padding:'10px 12px',border:'1px solid '+T.cardBorder,borderRadius:10,marginBottom:8}}>
            <span style={{width:10,height:10,borderRadius:3,background:TEAM_TYPE_COLOR[t.type]||T.mutedFg,flexShrink:0}}/>
            <TypeBadge type={t.type}/>
            <div style={{flex:1,minWidth:0}}>
              <div style={{fontSize:14,fontWeight:700,color:T.headingFg,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{t.name}</div>
              <div style={{fontSize:12,color:T.mutedFg,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{teamPath(teams,t.id).map(x=>x.name).join(' › ')}</div>
            </div>
            <a href={'/?team='+t.id} style={{fontSize:13,fontWeight:600,color:T.accent,textDecoration:'none',flexShrink:0}}>Открыть →</a>
          </div>
        ))}
    </DetailSection>
    <DetailSection title="Доступ к иерархии">
      <div style={{fontSize:12.5,color:T.mutedFg,marginBottom:12,lineHeight:1.5}}>
        Команды, к которым у пользователя есть доступ. При выборе узла он видит его и все дочерние. Если список пуст — у пользователя нет доступа ни к одной команде.
      </div>
      {loading
        ? <div style={{fontSize:13,color:T.dimFg}}>Загрузка…</div>
        : <>
          {pendingGrantIds.length===0&&(
            <div style={{padding:'12px 14px',background:'#fef2f2',border:'1px solid #fecaca',borderRadius:9,color:'#991b1b',fontSize:13,fontWeight:600,display:'flex',alignItems:'center',gap:8,marginBottom:12}}>
              <span style={{width:8,height:8,background:'#ef4444',borderRadius:'50%'}}/>
              Нет доступа — иерархия не видна
            </div>
          )}
          <Field label="Команды, к которым есть доступ" hint="выбор с поиском и иерархией">
            <TeamCombobox
              selectedIds={pendingGrantIds}
              teams={activeTeams}
              placeholder="Выберите команды из иерархии…"
              onChange={setPendingGrantIds}
            />
          </Field>
        </>}
    </DetailSection>
    <div style={{padding:'14px 24px',borderTop:'1px solid '+T.hairline,background:'#fafbfc',borderRadius:'0 0 14px 14px',display:'flex',alignItems:'center',justifyContent:'flex-end',gap:10}}>
      <Btn onClick={onClose} disabled={saving}>Отмена</Btn>
      <Btn variant="primary" onClick={save} disabled={saving||loading}>{saving?'Сохранение…':'Сохранить'}</Btn>
    </div>
    {confirmEl}
  </div>;
}

// ── NOTIFICATION CHANNELS ───────────────────────────────────────────────────────
// Каналы уведомлений пространства. Форма каждого канала строится по его же
// дескриптору (поля name/label/kind/required/hint с сервера): этот экран не знает
// ни одного канала по имени, поэтому канал из внешнего репозитория получает
// рабочую форму без единой правки здесь.
function NotificationsSection() {
  const [channels, setChannels] = useState([]);
  const [draft, setDraft] = useState({});   // name -> {enabled, values, secret}
  const [msg, setMsg] = useState({});       // name -> {text, ok}
  const [busy, setBusy] = useState('');
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    const r = await apiGet('/api/v1/admin/settings/notifications');
    setLoading(false);
    if (!r || !r.ok) return;
    const data = await r.json();
    const list = data.channels || [];
    setChannels(list);
    setDraft(prev => {
      const d = {};
      list.forEach(c => { d[c.name] = prev[c.name] || {enabled: c.enabled, values: {...(c.values||{})}, secret: ''}; });
      return d;
    });
  }, []);
  useEffect(() => { load(); }, [load]);

  const setField = (name, key, val) => setDraft(p => ({...p, [name]: {...p[name], values: {...p[name].values, [key]: val}}}));
  const setFlag  = (name, key, val) => setDraft(p => ({...p, [name]: {...p[name], [key]: val}}));

  const save = async (c) => {
    setBusy(c.name); setMsg(m => ({...m, [c.name]: null}));
    const d = draft[c.name];
    const res = await apiPut(`/api/v1/admin/settings/notifications/${encodeURIComponent(c.name)}`,
      {enabled: d.enabled, values: d.values, secret: d.secret});
    setBusy('');
    if (res && res.status === 204) {
      setMsg(m => ({...m, [c.name]: {text: 'Сохранено', ok: true}}));
      setDraft(p => ({...p, [c.name]: {...p[c.name], secret: ''}}));
      load();
    } else {
      const text = await readErr(res);
      setMsg(m => ({...m, [c.name]: {text, ok: false}}));
    }
  };

  const probe = async (c) => {
    setBusy(c.name); setMsg(m => ({...m, [c.name]: null}));
    const res = await apiPost(`/api/v1/admin/settings/notifications/${encodeURIComponent(c.name)}/test`, {});
    setBusy('');
    if (res && res.ok) {
      setMsg(m => ({...m, [c.name]: {text: 'Сообщение отправлено — проверьте мессенджер', ok: true}}));
    } else {
      const text = await readErr(res);
      setMsg(m => ({...m, [c.name]: {text, ok: false}}));
    }
  };

  if (loading) return <div style={{padding:24, color:T.mutedFg}}>Загрузка…</div>;

  const cardStyle = {background:'white', borderRadius:14, border:'1px solid '+T.cardBorder, boxShadow:'0 1px 3px rgba(15,23,42,0.04)', overflow:'hidden', marginBottom:16, padding:'16px 20px'};

  if (!channels.length) {
    return <div style={{padding:'20px 24px 32px'}}>
      <div style={cardStyle}>
        <div style={{fontSize:15, fontWeight:700, color:T.headingFg, marginBottom:4}}>🔔 Уведомления</div>
        <div style={{fontSize:13, color:T.mutedFg}}>
          Внутренние уведомления (колокольчик) работают всегда и настройки не требуют.
          Внешних каналов у этого пространства нет — их выдаёт администратор системы.
        </div>
      </div>
    </div>;
  }

  return <div style={{padding:'20px 24px 32px'}}>
    <div style={cardStyle}>
      <div style={{fontSize:15, fontWeight:700, color:T.headingFg, marginBottom:4}}>🔔 Каналы уведомлений</div>
      <div style={{fontSize:13, color:T.mutedFg}}>
        Внутренние уведомления работают всегда. Ниже — внешние каналы, выданные пространству.
      </div>
    </div>
    {channels.map(c => {
      const d = draft[c.name] || {values:{}};
      const m = msg[c.name];
      return <div key={c.name} style={cardStyle}>
        <div style={{display:'flex', alignItems:'center', gap:10, marginBottom:14}}>
          <strong style={{fontSize:14, color:T.headingFg}}>{c.title}</strong>
          <label style={{display:'flex', alignItems:'center', gap:6, fontSize:12.5, color:T.mutedFg, cursor:'pointer'}}>
            <input type="checkbox" checked={!!d.enabled} onChange={e => setFlag(c.name, 'enabled', e.target.checked)}/>
            включён
          </label>
        </div>
        {c.fields.map(f => (
          <Field key={f.key} label={f.label} hint={f.hint} required={f.required}>
            {f.kind === 'secret'
              ? <input type="password" autoComplete="new-password" style={inpStyle}
                  placeholder={c.secret_hint ? `сохранено: ${c.secret_hint} — оставьте пустым, чтобы не менять` : 'не задан'}
                  value={d.secret || ''} onChange={e => setFlag(c.name, 'secret', e.target.value)}/>
              : <input type={f.kind === 'url' ? 'url' : 'text'} style={inpStyle}
                  value={(d.values && d.values[f.key]) || ''} onChange={e => setField(c.name, f.key, e.target.value)}/>}
          </Field>
        ))}
        <div style={{display:'flex', gap:10, alignItems:'center', marginTop:6}}>
          <Btn variant="primary" onClick={() => save(c)} disabled={busy === c.name}>
            {busy === c.name ? 'Сохранение…' : 'Сохранить'}
          </Btn>
          <Btn onClick={() => probe(c)} disabled={busy === c.name || !c.configured}>
            Отправить проверку себе
          </Btn>
          {m && <span style={{fontSize:12.5, fontWeight:600, color: m.ok ? T.success : T.danger}}>{m.text}</span>}
        </div>
      </div>;
    })}
  </div>;
}

// ── APP ───────────────────────────────────────────────────────────────────────
const ADMIN_SECTION_IDS = ADMIN_SECTIONS.map(s=>s.id);
// Legacy server path routes (/admin/teams, …) fall back to their section.
const ADMIN_PATH_SECTION = {'/admin/teams':'teams','/admin/periods':'periods','/admin/access':'settings'};
function readSectionFromURL() {
  const q = new URLSearchParams(window.location.search).get('section');
  if (ADMIN_SECTION_IDS.includes(q)) return q;
  return ADMIN_PATH_SECTION[window.location.pathname] || null;
}

function App() {
  const [section, setSection] = useState(()=>readSectionFromURL()||localStorage.getItem('okr_admin_section')||'periods');
  const [me, setMe] = useState(null);
  const [periods, setPeriods] = useState([]);
  const [teams, setTeams] = useState([]);
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState(null);

  useEffect(()=>{localStorage.setItem('okr_admin_section',section);},[section]);

  // Заголовок вкладки: «Администрирование {раздел}».
  useEffect(()=>{
    const label = (ADMIN_SECTIONS.find(s=>s.id===section)||{}).label;
    document.title = label ? `Администрирование ${label}` : 'Администрирование';
  },[section]);

  // Reflect the current section in the URL (?section=) and support browser back/forward.
  useEffect(()=>{
    if (readSectionFromURL() !== section) {
      window.history.replaceState({section}, '', '/admin?section='+section);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  },[]);

  const navigate = useCallback((id)=>{
    setSection(id);
    window.history.pushState({section:id}, '', '/admin?section='+id);
  },[]);

  useEffect(()=>{
    const onPop = ()=> setSection(readSectionFromURL()||'periods');
    window.addEventListener('popstate', onPop);
    return ()=> window.removeEventListener('popstate', onPop);
  },[]);

  // silent=true refreshes data in place without showing the full-screen loader,
  // so mounted sections (and their UI state, e.g. the expanded team tree) survive.
  async function loadAll(silent) {
    if (!silent) setLoading(true);
    setErr(null);
    try {
      const [meR, periodsR, teamsR, usersR] = await Promise.all([
        apiGet('/api/v1/me'),
        apiGet('/api/v1/admin/periods'),
        apiGet('/api/v1/admin/teams'),
        apiGet('/api/v1/admin/users'),
      ]);
      if (!meR||!periodsR||!teamsR||!usersR) return;
      const [meD, periodsD, teamsD, usersD] = await Promise.all([meR.json(), periodsR.json(), teamsR.json(), usersR.json()]);
      setMe(meD);
      setPeriods(periodsD.items||[]);
      setTeams(teamsD.items||[]);
      setUsers(Array.isArray(usersD)?usersD:[]);
    } catch(e) { setErr(e.message); }
    finally { if (!silent) setLoading(false); }
  }

  useEffect(()=>{ loadAll(); },[]);
  const reload = useCallback(()=>loadAll(true),[]);

  if (loading) return <div style={{height:'100vh',display:'flex',alignItems:'center',justifyContent:'center',color:T.mutedFg,fontSize:14}}>Загрузка…</div>;
  if (err) return <div style={{height:'100vh',display:'flex',alignItems:'center',justifyContent:'center',color:T.danger,fontSize:14}}>Ошибка: {err}</div>;

  return <Shell section={section} setSection={navigate} currentUser={me}>
    {section==='periods' &&<PeriodsSection periods={periods} reload={reload}/>}
    {section==='teams'   &&<TeamsSection teams={teams} reload={reload}/>}
    {section==='users'   &&<UsersSection users={users} teams={teams} currentUser={me} reload={reload}/>}
    {section==='settings'&&<div style={{padding:'20px 24px 24px',display:'flex',flexDirection:'column',gap:20}}>
      <div style={{background:'white',borderRadius:12,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}><GeneralSettingsPanel/></div>
      <div style={{background:'white',borderRadius:12,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}><AccessSettingsPanel teams={teams}/></div>
      <div style={{background:'white',borderRadius:12,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}><FeedbackSettingsPanel/></div>
      <div style={{background:'white',borderRadius:12,border:'1px solid '+T.cardBorder,boxShadow:'0 1px 3px rgba(15,23,42,0.04)',overflow:'hidden'}}><ActivityLogPanel/></div>
    </div>}
    {section==='health-checkin'&&<HealthCheckInSettingsPanel/>}
    {section==='notifications'&&<NotificationsSection/>}
  </Shell>;
}

ReactDOM.createRoot(document.getElementById('root')).render(<App/>);
