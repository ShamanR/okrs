// Чистый layout-движок дерева целей. Никакого React/DOM — только вычисления.
// Слоёная (Sugiyama-подобная) раскладка:
//  - Y: горизонтальные ряды = бэнд периода (годовые/квартальные) + под-уровень по глубине
//    связей ВНУТРИ бэнда; все карточки одного ряда — на одной линии.
//  - X: tidy-раскладка (родитель по центру над детьми), плотно, без перекрытий; порядок —
//    по хронологии периода (раньше → левее).
//  - Цели БЕЗ связей — отдельным нижним бэндом, СЕТКОЙ в несколько рядов (не одна линия).
// Возвращает { bands, nodes, edges, groups, width, height } — контракт для goal_tree.js.

// Ортогональный путь со скруглёнными углами от (x1,y1) снизу родителя к (x2,y2) сверху ребёнка.
// turnY — Y горизонтального поворота (по умолчанию середина); уводим его в межслойный зазор.
function orthEdgePath(x1, y1, x2, y2, r, turnY) {
  const midY = (turnY == null) ? (y1 + y2) / 2 : turnY;
  const dir = x2 >= x1 ? 1 : -1;
  const rr = Math.min(r, Math.abs(x2 - x1) / 2, Math.abs(midY - y1), Math.abs(y2 - midY));
  if (rr < 1 || Math.abs(x2 - x1) < 1) {
    return `M ${x1} ${y1} L ${x1} ${midY} L ${x2} ${midY} L ${x2} ${y2}`;
  }
  return [
    `M ${x1} ${y1}`,
    `L ${x1} ${midY - rr}`,
    `Q ${x1} ${midY} ${x1 + dir * rr} ${midY}`,
    `L ${x2 - dir * rr} ${midY}`,
    `Q ${x2} ${midY} ${x2} ${midY + rr}`,
    `L ${x2} ${y2}`,
  ].join(' ');
}

function computeGoalTreeLayout(data, opts) {
  const cardW = opts.cardW, cardH = opts.cardH;
  const hGap = opts.hGap, vGap = opts.vGap, bandGap = opts.bandGap;
  const colPitch = cardW + hGap;
  const rowPitch = cardH + vGap;
  const padTop = 70, padBottom = 16, margin = 40;

  const periodById = new Map(data.periods.map(p => [p.id, p]));
  const goalById = new Map(data.goals.map(g => [g.id, g]));
  const teamOrder = new Map(data.teams.map((t, i) => [t.id, i]));
  const teamName = new Map(data.teams.map(t => [t.id, t.name]));
  const periodRank = (opts.periodRank instanceof Map) ? opts.periodRank : null;
  const pr = g => (periodRank ? (periodRank.get(g.period_id) ?? 0) : 0);
  const cmp = (a, b) => pr(a) - pr(b) || (teamOrder.get(a.team_id) ?? 1e9) - (teamOrder.get(b.team_id) ?? 1e9) || a.id - b.id;

  const hasLink = g => (g.parent_goal_ids.length + g.child_goal_ids.length) > 0;
  const periodDepth = g => { const p = periodById.get(g.period_id); return p ? p.depth : 0; };

  const linkedGoals = data.goals.filter(hasLink);
  const unlinkedGoals = data.goals.filter(g => !hasLink(g)).sort(cmp);

  // --- связные бэнды по глубине периода (сверху вниз) ---
  const linkedDepths = Array.from(new Set(linkedGoals.map(periodDepth))).sort((a, b) => a - b);
  const linkedByDepth = new Map(linkedDepths.map(d => [d, []]));
  for (const g of linkedGoals) linkedByDepth.get(periodDepth(g)).push(g);

  // под-уровень внутри бэнда: 0 если нет родителя того же depth; иначе max(род.)+1
  const sublevel = new Map();
  const sameBandParents = g => g.parent_goal_ids.filter(pid => {
    const p = goalById.get(pid);
    return p && hasLink(p) && periodDepth(p) === periodDepth(g);
  });
  const computeSub = (g, stack) => {
    if (sublevel.has(g.id)) return sublevel.get(g.id);
    let m = 0;
    for (const pid of sameBandParents(g)) {
      if (stack.has(pid)) continue;
      stack.add(pid);
      m = Math.max(m, computeSub(goalById.get(pid), stack) + 1);
      stack.delete(pid);
    }
    sublevel.set(g.id, m);
    return m;
  };
  for (const g of linkedGoals) computeSub(g, new Set([g.id]));

  // ряды связных бэндов
  const rowY = new Map(); // `${depth}|${sub}` -> y
  const bands = [];
  let runningY = 0;
  for (const d of linkedDepths) {
    const gs = linkedByDepth.get(d);
    let maxSub = 0;
    for (const g of gs) maxSub = Math.max(maxSub, sublevel.get(g.id));
    const bandTop = runningY;
    const firstRowY = bandTop + padTop;
    for (let s = 0; s <= maxSub; s++) rowY.set(`${d}|${s}`, firstRowY + s * rowPitch);
    const bandHeight = (firstRowY + maxSub * rowPitch + cardH + padBottom) - bandTop;
    const name = d === 0 ? 'Годовые цели' : d === 1 ? 'Квартальные цели' : `Уровень ${d}`;
    bands.push({ depth: d, name, count: gs.length, label: `${name} · ${gs.length}`, y: bandTop, height: bandHeight });
    runningY = bandTop + bandHeight + bandGap;
  }

  // --- узлы связной части ---
  const rowKeyOf = g => `${periodDepth(g)}|${sublevel.get(g.id)}`;
  const nodeById = new Map();
  const nodes = [];
  for (const g of linkedGoals) {
    const n = { id: g.id, goal: g, x: 0, y: rowY.get(rowKeyOf(g)), isRoot: g.parent_goal_ids.length === 0, teamId: g.team_id, rowKey: rowKeyOf(g) };
    nodes.push(n);
    nodeById.set(g.id, n);
  }

  // --- X связной части: tidy (Reingold–Tilford-подобная), порядок по хронологии периода ---
  const treeChildren = new Map();
  const inForest = new Set();
  const roots = linkedGoals.filter(g => g.parent_goal_ids.length === 0).sort(cmp);
  const buildForest = id => {
    if (inForest.has(id)) return;
    inForest.add(id);
    const kids = (goalById.get(id).child_goal_ids || []).slice()
      .filter(c => nodeById.has(c) && !inForest.has(c))
      .sort((a, b) => cmp(goalById.get(a), goalById.get(b)));
    treeChildren.set(id, kids);
    for (const c of kids) buildForest(c);
  };
  for (const r of roots) buildForest(r.id);
  for (const g of linkedGoals) if (!inForest.has(g.id)) { buildForest(g.id); roots.push(g); }

  const rowCursor = new Map();
  const nextX = rk => (rowCursor.has(rk) ? rowCursor.get(rk) : 0);
  const bump = (rk, x) => rowCursor.set(rk, x + colPitch);
  const shiftSubtree = (id, dx, vis) => {
    if (vis.has(id)) return;
    vis.add(id);
    const n = nodeById.get(id);
    n.x += dx;
    bump(n.rowKey, n.x);
    for (const c of (treeChildren.get(id) || [])) shiftSubtree(c, dx, vis);
  };
  const place = id => {
    const n = nodeById.get(id);
    const kids = treeChildren.get(id) || [];
    if (kids.length === 0) { n.x = nextX(n.rowKey); bump(n.rowKey, n.x); return; }
    for (const c of kids) place(c);
    const xs = kids.map(c => nodeById.get(c).x);
    let cx = (Math.min(...xs) + Math.max(...xs)) / 2;
    const minHere = nextX(n.rowKey);
    if (cx < minHere) { shiftSubtree(id, minHere - cx, new Set()); cx = minHere; }
    n.x = cx;
    bump(n.rowKey, n.x);
  };
  for (const r of roots) place(r.id);

  // нормализация X связной части к левому краю
  let minX = Infinity;
  for (const n of nodes) if (n.x < minX) minX = n.x;
  if (!isFinite(minX)) minX = 0;
  for (const n of nodes) n.x = n.x - minX + margin;
  const linkedRight = nodes.length ? Math.max(...nodes.map(n => n.x + cardW)) : margin;

  // --- цели БЕЗ связей: сетка снизу (несколько рядов) ---
  if (unlinkedGoals.length) {
    let cols;
    if (nodes.length) cols = Math.max(1, Math.round((linkedRight - margin) / colPitch));
    else cols = Math.max(1, Math.round(Math.sqrt(unlinkedGoals.length)));
    cols = Math.min(cols, unlinkedGoals.length);
    const rows = Math.ceil(unlinkedGoals.length / cols);
    const bandTop = nodes.length ? runningY : 0;
    const firstRowY = bandTop + padTop;
    unlinkedGoals.forEach((g, i) => {
      const n = { id: g.id, goal: g, x: margin + (i % cols) * colPitch, y: firstRowY + Math.floor(i / cols) * rowPitch, isRoot: false, teamId: g.team_id, rowKey: 'unlinked' };
      nodes.push(n);
      nodeById.set(g.id, n);
    });
    const bandHeight = (firstRowY + (rows - 1) * rowPitch + cardH + padBottom) - bandTop;
    bands.push({ depth: null, name: 'Без связей', count: unlinkedGoals.length, label: `Без связей · ${unlinkedGoals.length}`, y: bandTop, height: bandHeight });
    runningY = bandTop + bandHeight + bandGap;
  }

  // --- рёбра (только связные) ---
  const edges = [];
  for (const g of linkedGoals) {
    const child = nodeById.get(g.id);
    if (!child) continue;
    for (const pid of g.parent_goal_ids) {
      const parent = nodeById.get(pid);
      if (!parent) continue;
      const x1 = parent.x + cardW / 2, y1 = parent.y + cardH;
      const x2 = child.x + cardW / 2, y2 = child.y;
      const turnY = Math.min(y1 + 22, y2 - 30); // поворот в зазоре, выше шапки группы ребёнка
      edges.push({ from: pid, to: g.id, path: orthEdgePath(x1, y1, x2, y2, 12, turnY) });
    }
  }

  // --- группы: ≥2 цели ОДНОЙ команды в одном ряду под общим РЕАЛЬНЫМ родителем ---
  const byRowTeamParent = new Map();
  for (const g of linkedGoals) {
    const n = nodeById.get(g.id);
    if (!n || g.parent_goal_ids.length === 0) continue;
    for (const pid of g.parent_goal_ids) {
      const k = `${n.rowKey}|${g.team_id}|${pid}`;
      if (!byRowTeamParent.has(k)) byRowTeamParent.set(k, []);
      byRowTeamParent.get(k).push(n);
    }
  }
  const groups = [];
  // Инсеты бокса группы. groupPadX ≤ hGap/2 (иначе бокс наползёт на соседнюю карточку —
  // hGap задаётся ≥ 2*groupPadX в CARD). Верх чуть больше под лейбл, бока/низ равны.
  const groupHeader = 20, groupPadX = 16, groupPadBottom = 16;
  for (const [key, ns] of byRowTeamParent) {
    if (ns.length < 2) continue;
    const minGX = Math.min(...ns.map(n => n.x)) - groupPadX;
    const maxGX = Math.max(...ns.map(n => n.x + cardW)) + groupPadX;
    const teamId = ns[0].teamId;
    const parts = key.split('|');
    const parentGoalId = Number(parts[parts.length - 1]); // pid — последний сегмент ключа
    groups.push({ teamId, teamName: teamName.get(teamId) || '', parentGoalId, x: minGX, y: ns[0].y - groupHeader, w: maxGX - minGX, h: groupHeader + cardH + groupPadBottom, nodeIds: ns.map(n => n.id) });
  }

  const width = Math.max(margin, ...nodes.map(n => n.x + cardW)) + margin;
  const height = (bands.length ? bands[bands.length - 1].y + bands[bands.length - 1].height : 0) + 40;
  return { bands, nodes, edges, groups, width, height };
}
