// Shared user picker — the same searchable, avatar dropdown used for goal drivers/leads.
// Loaded as text/babel BEFORE app scripts, so it exports the globals UserAvatar / UserSelector.
// Uses React.useState/useRef (not top-level destructuring) to avoid colliding with app scripts.
// Styles: components.css (.user-selector*, .user-chip*, .user-avatar__fallback). Depends on: React.
//
// Single-select mode keys on UDID (value = udid, onChange(udid, display_name)); multi-select mode
// keeps an array of UDIDs (used for goal owners). A user-ref cache resolves a stored UDID to its
// display name/avatar so a chip renders correctly even before a fresh search.

const _userByUdid = new Map();
const _userByName = new Map();

function _cacheUserRef(ref) {
  if (!ref) return;
  if (ref.udid) {
    const prev = _userByUdid.get(ref.udid);
    _userByUdid.set(ref.udid, prev ? { ...prev, ...ref } : ref);
  }
  if (ref.display_name) {
    const prev = _userByName.get(ref.display_name);
    _userByName.set(ref.display_name, prev ? { ...prev, ...ref } : ref);
  }
}

function _cachedUsersList() {
  return Array.from(_userByName.values());
}

function UserAvatar({ user, size = 24 }) {
  if (user && user.avatar_url) {
    return <img src={user.avatar_url} width={size} height={size} alt=""
      style={{ borderRadius: '50%', objectFit: 'cover', flexShrink: 0, display: 'block' }} />;
  }
  return (
    <span className="user-avatar__fallback" style={{ width: size, height: size, fontSize: Math.round(size * 0.45) }}>
      {user && user.display_name ? user.display_name[0].toUpperCase() : '?'}
    </span>
  );
}

function UserSelector({ value, onChange, multiple = false, placeholder = 'Поиск пользователя…', fetchFn }) {
  const [q, setQ] = React.useState('');
  const [open, setOpen] = React.useState(false);
  const [hi, setHi] = React.useState(0);
  const [fetchedUsers, setFetchedUsers] = React.useState(null);
  const fetchTimer = React.useRef(null);
  const inputRef = React.useRef(null);
  const wrapRef = React.useRef(null);

  React.useEffect(() => { setHi(0); }, [q]);
  React.useEffect(() => {
    const h = e => { if (wrapRef.current && !wrapRef.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);

  React.useEffect(() => {
    if (!fetchFn || !open) return;
    clearTimeout(fetchTimer.current);
    fetchTimer.current = setTimeout(() => {
      fetchFn(q).then(data => {
        if (Array.isArray(data)) {
          data.forEach(u => _cacheUserRef(u));
          setFetchedUsers(data);
        }
      }).catch(() => { });
    }, q ? 200 : 0);
    return () => clearTimeout(fetchTimer.current);
  }, [q, open, fetchFn]);

  const qLow = q.toLowerCase();
  const users = fetchFn
    ? (fetchedUsers || [])
    : (qLow ? _cachedUsersList().filter(u => u.display_name?.toLowerCase().includes(qLow)) : _cachedUsersList());

  const handleQueryChange = newQ => { setQ(newQ); if (fetchFn) setFetchedUsers(null); };

  const values = multiple ? (Array.isArray(value) ? value : []) : (value ? [value] : []);
  const findUserByValue = v => _userByUdid.get(v) || users.find(u => u.udid === v);
  const available = multiple ? users.filter(u => !values.includes(u.udid)) : users;

  const select = u => {
    if (multiple) { if (!values.includes(u.udid)) onChange([...values, u.udid]); }
    else { onChange(u.udid, u.display_name); setOpen(false); }
    setQ(''); inputRef.current?.focus();
  };
  const remove = udid => { if (multiple) onChange(values.filter(v => v !== udid)); else onChange('', ''); };
  const onKey = e => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setOpen(true); setHi(h => Math.min(available.length - 1, h + 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)); }
    else if (e.key === 'Enter') { e.preventDefault(); if (open && available[hi]) select(available[hi]); }
    else if (e.key === 'Escape') setOpen(false);
    else if (e.key === 'Backspace' && !q && multiple && values.length > 0) remove(values[values.length - 1]);
  };

  return (
    <div ref={wrapRef} className="user-selector">
      <div onClick={() => { setOpen(true); inputRef.current?.focus(); }}
        className={`user-selector__field${open ? ' user-selector__field--open' : ''}`}>
        {values.map(v => {
          const u = findUserByValue(v);
          return (
            <span key={v} className="user-chip">
              <UserAvatar user={u} size={18} />
              <span className="user-chip__name">{u?.display_name || v}</span>
              <button type="button" onClick={e => { e.stopPropagation(); remove(v); }} className="user-chip__remove">×</button>
            </span>
          );
        })}
        {(multiple || values.length === 0) && (
          <input ref={inputRef} value={q} onChange={e => { handleQueryChange(e.target.value); setOpen(true); }}
            onFocus={() => setOpen(true)} onKeyDown={onKey}
            placeholder={values.length === 0 ? placeholder : 'Ещё…'}
            className="user-selector__input" />
        )}
      </div>
      {open && (
        <div className="user-selector__dropdown">
          {available.length === 0
            ? <div className="user-selector__empty">{q ? 'Пользователи не найдены' : 'Список пуст'}</div>
            : available.slice(0, 20).map((u, i) => (
              <div key={u.udid} onClick={() => select(u)} onMouseEnter={() => setHi(i)}
                className={`user-selector__option${i === hi ? ' user-selector__option--hi' : ''}`}>
                <UserAvatar user={u} size={26} />
                <div className="user-selector__option-info">
                  <span className="user-selector__option-name">{u.display_name}</span>
                  {u.led_team && <span className="user-selector__option-team">{u.led_team}</span>}
                </div>
              </div>
            ))
          }
        </div>
      )}
    </div>
  );
}
