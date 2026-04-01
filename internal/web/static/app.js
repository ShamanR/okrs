(() => {
  const state = {
    hierarchy: null,
    hierarchyByPeriod: {},
    teamOKR: null,
    teamsSummaryByPeriod: {},
  };

  const jsonHeaders = { 'Content-Type': 'application/json; charset=utf-8' };
  const goalPriorityOptions = ['P0', 'P1', 'P2', 'P3'];
  const goalWorkOptions = ['Discovery', 'Delivery'];
  const goalFocusOptions = ['PROFITABILITY', 'STABILITY', 'SPEED_EFFICIENCY', 'TECH_INDEPENDENCE'];
  const lockedPeriodStatuses = ['validated', 'closed'];

  const isPeriodLocked = () => lockedPeriodStatuses.includes(state.teamOKR?.period_status);

  const pluralize = (count, forms) => {
    const mod10 = count % 10;
    const mod100 = count % 100;
    if (mod10 === 1 && mod100 !== 11) return forms[0];
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return forms[1];
    return forms[2];
  };

  const formatRelativeUpdate = (value) => {
    if (!value) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    const now = new Date();
    const diffMs = now - date;
    const dayMs = 1000 * 60 * 60 * 24;
    const days = Math.floor(diffMs / dayMs);
    if (days <= 0) return 'сегодня';
    if (days < 7) return `${days} ${pluralize(days, ['день', 'дня', 'дней'])} назад`;
    if (days < 30) {
      const weeks = Math.max(1, Math.floor(days / 7));
      return `${weeks} ${pluralize(weeks, ['неделю', 'недели', 'недель'])} назад`;
    }
    const months = Math.max(1, Math.floor(days / 30));
    return `${months} ${pluralize(months, ['месяц', 'месяца', 'месяцев'])} назад`;
  };

  const formatAbsoluteDate = (value) => {
    if (!value) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return date.toLocaleDateString('ru-RU', { year: 'numeric', month: 'long', day: 'numeric' });
  };

  const renderWorkTypeIcon = (workType) => {
    const normalized = String(workType || '').trim().toLowerCase();
    const icon = document.createElement('i');
    icon.classList.add('bi');
    if (normalized === 'discovery') {
      icon.classList.add('bi-compass');
      icon.title = 'Discovery';
      icon.setAttribute('aria-label', 'Discovery');
      return icon;
    }
    if (normalized === 'delivery') {
      icon.classList.add('bi-wrench-adjustable-circle');
      icon.title = 'Delivery';
      icon.setAttribute('aria-label', 'Delivery');
      return icon;
    }
    return document.createTextNode('—');
  };

  const fetchJSON = async (url, options = {}) => {
    const response = await fetch(url, options);
    const payload = await response.json();
    if (!response.ok) {
      const message = payload?.error?.message || 'Request failed';
      const error = new Error(message);
      error.details = payload?.error;
      throw error;
    }
    return payload;
  };

  const periodsCacheKey = 'okr_periods_cache_v1';
  const periodsCacheTTL = 1000 * 60 * 60 * 6;

  const readPeriodsCache = () => {
    try {
      const raw = window.localStorage.getItem(periodsCacheKey);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      if (!parsed?.items || !Array.isArray(parsed.items)) return null;
      if (!parsed.savedAt || Date.now() - parsed.savedAt > periodsCacheTTL) return null;
      return parsed.items;
    } catch (error) {
      return null;
    }
  };

  const writePeriodsCache = (items) => {
    try {
      window.localStorage.setItem(periodsCacheKey, JSON.stringify({ savedAt: Date.now(), items }));
    } catch (error) {
      // ignore cache errors
    }
  };

  const loadPeriods = async () => {
    const cached = readPeriodsCache();
    try {
      const payload = await fetchJSON('/api/v1/periods');
      const items = payload.items || [];
      writePeriodsCache(items);
      return items;
    } catch (error) {
      if (cached) return cached;
      throw error;
    }
  };

  const loadHierarchyForPeriod = async (periodID) => {
    const cacheKey = String(periodID || '');
    if (state.hierarchyByPeriod[cacheKey]) return state.hierarchyByPeriod[cacheKey];
    const url = new URL('/api/v1/hierarchy', window.location.origin);
    if (periodID) {
      url.searchParams.set('period_id', periodID);
    }
    const payload = await fetchJSON(url.toString());
    state.hierarchy = payload.items || [];
    state.hierarchyByPeriod[cacheKey] = state.hierarchy;
    return state.hierarchyByPeriod[cacheKey];
  };

  const renderPeriodSelect = (select, periods, selectedID) => {
    select.innerHTML = '';
    if (!periods.length) {
      select.appendChild(createOption('', 'Нет периодов'));
      select.disabled = true;
      return;
    }
    select.disabled = false;
    periods.forEach((period) => {
      const option = createOption(String(period.id), period.name);
      if (selectedID && String(period.id) === String(selectedID)) {
        option.selected = true;
      }
      select.appendChild(option);
    });
  };

  const renderHierarchySelect = (tree, select, selected) => {
    const options = [createOption('ALL', 'Все команды')];
    const walk = (nodes, level) => {
      nodes.forEach((node) => {
        const prefix = '\u00A0'.repeat(level * 2);
        options.push(createOption(String(node.id), `${prefix}${node.type_label} ${node.name}`));
        if (node.children && node.children.length) {
          walk(node.children, level + 1);
        }
      });
    };
    walk(tree, 0);
    select.innerHTML = '';
    options.forEach((option) => {
      if (selected && option.value === String(selected)) {
        option.selected = true;
      }
      select.appendChild(option);
    });
  };

  const renderTeamsList = (data, tbody, periodID) => {
    tbody.innerHTML = '';
    if (!data.items || data.items.length === 0) {
      const row = document.createElement('tr');
      const cell = document.createElement('td');
      cell.colSpan = 4;
      cell.className = 'text-muted';
      cell.textContent = 'Нет данных';
      row.appendChild(cell);
      tbody.appendChild(row);
      return;
    }
    data.items.forEach((team) => {
      const row = document.createElement('tr');
      row.appendChild(renderTeamCell(team, periodID));
      row.appendChild(renderPeriodProgressCell(team));
      row.appendChild(renderGoalsCell(team, periodID));
      row.appendChild(renderStatusCell(team));
      tbody.appendChild(row);
    });
  };

  const renderOKRPage = (data, goalsEl, actionsEl) => {
    state.teamOKR = data;
    if (actionsEl) {
      renderOKRActions(data, actionsEl);
    }
    goalsEl.innerHTML = '';
    if (!data.goals || data.goals.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'text-muted';
      empty.textContent = 'Нет целей';
      goalsEl.appendChild(empty);
      return;
    }
    data.goals.forEach((goal) => {
      goalsEl.appendChild(renderGoalCard(goal));
    });
    initPopovers();
    if (window.location.hash) {
      window.setTimeout(() => {
        const target = document.querySelector(window.location.hash);
        if (target) {
          target.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }
      }, 0);
    }
  };

  const renderGoalCard = (goal) => {
    const card = document.createElement('div');
    if (goal.id) {
      card.id = `goal-${goal.id}`;
    }
    const krWeightSum = sumKRWeights(goal.key_results || []);
    card.className = `card ${krWeightSum !== 100 ? 'border-danger' : ''}`;
    const body = document.createElement('div');
    body.className = 'card-body';

    const header = document.createElement('div');
    header.className = 'd-flex flex-wrap align-items-center gap-2 mb-2';

    const priority = document.createElement('span');
    priority.className = `badge ${priorityBadgeClass(goal.priority)}`;
    priority.textContent = goal.priority;

    const weight = document.createElement('span');
    weight.className = 'badge text-bg-light border';
    weight.textContent = `Вес ${goal.weight}%`;

    const krWeightBadge = document.createElement('span');
    krWeightBadge.className = `badge ${krWeightSum !== 100 ? 'text-bg-danger' : 'text-bg-light border'}`;
    krWeightBadge.textContent = `Σ KR ${krWeightSum}`;

    const titleWrap = document.createElement('div');
    titleWrap.className = 'd-flex flex-column';
    if (isPeriodLocked()) {
      const title = document.createElement('span');
      title.className = 'fw-semibold';
      title.textContent = goal.title;
      titleWrap.appendChild(title);
    } else {
      const title = document.createElement('button');
      title.type = 'button';
      title.className = 'btn btn-link p-0 fw-semibold';
      title.textContent = goal.title;
      title.addEventListener('click', () => openGoalModal(goal));
      titleWrap.appendChild(title);
    }

    const menu = renderGoalMenu(goal);

    header.append(priority, weight, krWeightBadge);
    if (goal.share_teams && goal.share_teams.length > 1) {
      header.appendChild(renderSharedGoalBadge(goal));
    }
    header.append(titleWrap, menu);

    const description = document.createElement('p');
    description.className = 'text-muted mb-2';
    description.textContent = goal.description || '';

    const progressWrap = document.createElement('div');
    progressWrap.className = 'd-flex flex-wrap align-items-center gap-3 mb-3';
    const progressBar = document.createElement('div');
    progressBar.className = 'progress flex-grow-1';
    progressBar.setAttribute('role', 'progressbar');
    progressBar.setAttribute('aria-valuenow', goal.progress);
    progressBar.setAttribute('aria-valuemin', '0');
    progressBar.setAttribute('aria-valuemax', '100');

    const progressFill = document.createElement('div');
    progressFill.className = 'progress-bar';
    progressFill.style.width = `${goal.progress}%`;
    progressBar.appendChild(progressFill);

    const progressValue = document.createElement('span');
    progressValue.className = 'fw-semibold';
    progressValue.textContent = `${goal.progress}%`;

    progressWrap.append(progressBar, progressValue);

    const meta = document.createElement('div');
    meta.className = 'd-flex flex-wrap align-items-center gap-2 mb-3';

    const workBadge = document.createElement('span');
    workBadge.className = 'badge text-bg-light border';
    workBadge.textContent = goal.work_type;

    const focusBadge = document.createElement('span');
    focusBadge.className = 'badge text-bg-light border';
    focusBadge.textContent = goal.focus_type;

    const owner = document.createElement('span');
    owner.innerHTML = `Владелец: <span class="text-decoration-underline">${goal.owner_text}</span>`;

    meta.append(workBadge, focusBadge, owner);

    const isGoalFromHash = window.location.hash === `#goal-${goal.id}`;
    const krWrap = renderKRTable(goal, { expanded: isGoalFromHash });

    const actions = document.createElement('div');
    actions.className = 'mt-3';
    if (!isPeriodLocked()) {
      const addKRButton = document.createElement('button');
      addKRButton.type = 'button';
      addKRButton.className = 'btn btn-outline-primary btn-sm align-self-start';
      addKRButton.textContent = 'Добавить KR';
      addKRButton.addEventListener('click', () => openKRCreateModal(goal));
      actions.appendChild(addKRButton);
    }

    body.append(header, description, progressWrap, meta, krWrap, actions);
    card.appendChild(body);
    return card;
  };

  const renderKRTable = (goal, options = {}) => {
    const isExpanded = options.expanded === true;
    const wrapper = document.createElement('div');
    const collapseID = `goal-${goal.id || 'new'}-krs`;
    const toggle = document.createElement('button');
    toggle.type = 'button';
    toggle.className = 'btn btn-outline-secondary btn-sm mb-2';
    toggle.setAttribute('data-bs-toggle', 'collapse');
    toggle.setAttribute('data-bs-target', `#${collapseID}`);
    toggle.setAttribute('aria-expanded', isExpanded ? 'true' : 'false');
    toggle.setAttribute('aria-controls', collapseID);
    toggle.textContent = 'Key Results';
    wrapper.appendChild(toggle);

    const collapse = document.createElement('div');
    collapse.id = collapseID;
    collapse.className = `collapse${isExpanded ? ' show' : ''}`;

    if (!goal.key_results || goal.key_results.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'text-muted';
      empty.textContent = 'Ключевые результаты не заданы.';
      collapse.appendChild(empty);
      wrapper.appendChild(collapse);
      return wrapper;
    }

    const table = document.createElement('table');
    table.className = 'table table-sm align-middle mb-0';

    const head = document.createElement('thead');
    head.innerHTML = `
      <tr>
        <th>Вес</th>
        <th class="okr-kr-title-col">Название</th>
        <th>Факт (%)</th>
        <th class="text-end"></th>
      </tr>`;
    table.appendChild(head);

    const body = document.createElement('tbody');
    goal.key_results.forEach((kr) => {
      const enrichedKR = {
        ...kr,
        goal_priority: goal.priority,
        goal_work_type: goal.work_type,
        goal_focus_type: goal.focus_type,
      };
      const { row, detailRow } = renderKRRow(enrichedKR);
      body.appendChild(row);
      body.appendChild(detailRow);
    });
    table.appendChild(body);
    collapse.appendChild(table);
    wrapper.appendChild(collapse);
    return wrapper;
  };

  const renderKRRow = (kr) => {
    const row = document.createElement('tr');

    const weightCell = document.createElement('td');
    const weight = document.createElement('span');
    weight.className = 'badge text-bg-light border';
    weight.textContent = kr.weight;
    weightCell.appendChild(weight);

    const titleCell = document.createElement('td');
    titleCell.className = 'okr-kr-title-col';
    const titleWrap = document.createElement('div');
    titleWrap.className = 'd-flex flex-column align-items-start';
    if (isPeriodLocked()) {
      const title = document.createElement('span');
      title.className = 'fw-semibold';
      title.textContent = kr.title;
      titleWrap.appendChild(title);
    } else {
      const title = document.createElement('button');
      title.type = 'button';
      title.className = 'btn btn-link p-0 fw-semibold okr-kr-title-button';
      title.textContent = kr.title;
      title.addEventListener('click', () => openKRModal(kr));
      titleWrap.appendChild(title);
    }
    const updatedText = formatRelativeUpdate(kr.updated_at);
    if (updatedText) {
      const updatedAt = document.createElement('span');
      updatedAt.className = 'text-muted small';
      updatedAt.textContent = updatedText;
      updatedAt.title = formatAbsoluteDate(kr.updated_at);
      titleWrap.appendChild(updatedAt);
    }
    titleCell.appendChild(titleWrap);

    const progressCell = document.createElement('td');
    const progress = document.createElement('span');
    progress.className = 'badge text-bg-light border';
    progress.textContent = `${kr.progress}%`;
    progressCell.appendChild(progress);

    const actionsCell = document.createElement('td');
    actionsCell.className = 'text-end';
    const actions = document.createElement('div');
    actions.className = 'd-flex justify-content-end gap-2';

    const menu = renderKRMenu(kr);

    const updateButton = document.createElement('button');
    updateButton.type = 'button';
    updateButton.className = 'btn btn-outline-primary btn-sm';
    updateButton.textContent = 'Обновить прогресс';

    actions.append(updateButton, menu);
    actionsCell.appendChild(actions);

    row.append(weightCell, titleCell, progressCell, actionsCell);

    const detailRow = document.createElement('tr');
    const detailCell = document.createElement('td');
    detailCell.colSpan = 4;
    const panel = renderMeasurePanel(kr);
    panel.classList.add('mt-2');
    panel.hidden = true;
    const comments = renderKRComments(kr);
    detailCell.append(panel, comments);
    detailRow.appendChild(detailCell);

    updateButton.addEventListener('click', () => {
      panel.hidden = !panel.hidden;
    });

    return { row, detailRow };
  };

  const renderMeasurePanel = (kr) => {
    const panel = document.createElement('div');
    panel.className = 'border rounded p-3 bg-light';

    const description = document.createElement('div');
    description.className = 'text-muted small mb-2';
    description.textContent = kr.description || 'Описание не указано.';
    panel.appendChild(description);

    const form = document.createElement('form');
    form.className = 'd-flex flex-column gap-2';

    const status = document.createElement('div');
    status.className = 'text-muted small';

    const commentLabel = document.createElement('label');
    commentLabel.className = 'form-label';
    commentLabel.textContent = 'Заметки';
    const commentInput = document.createElement('textarea');
    commentInput.className = 'form-control';
    commentInput.rows = 2;
    commentInput.value = getLatestComment(kr)?.text ?? '';
    commentLabel.appendChild(commentInput);
    form.appendChild(commentLabel);

    if (!kr.measure || !kr.measure.kind) {
      status.textContent = 'Нет метрик для обновления.';
      panel.appendChild(status);
      return panel;
    }

    if (kr.measure.kind === 'PERCENT' || kr.measure.kind === 'LINEAR') {
      const meta = kr.measure.percent || kr.measure.linear;
      const row = document.createElement('div');
      row.className = 'row g-2';

      const currentCol = document.createElement('div');
      currentCol.className = 'col-5';
      const currentLabel = document.createElement('label');
      currentLabel.className = 'form-label';
      currentLabel.textContent = 'Текущее значение';
      const currentInput = document.createElement('input');
      currentInput.type = 'number';
      currentInput.step = 'any';
      currentInput.className = 'form-control';
      currentInput.value = meta?.current_value ?? 0;
      currentLabel.appendChild(currentInput);
      currentCol.appendChild(currentLabel);

      const targetCol = document.createElement('div');
      targetCol.className = 'col-7';
      const targetLabel = document.createElement('label');
      targetLabel.className = 'form-label';
      targetLabel.textContent = 'Целевое значение';
      const targetInput = document.createElement('input');
      targetInput.type = 'number';
      targetInput.step = 'any';
      targetInput.className = 'form-control';
      targetInput.value = meta?.target_value ?? 0;
      targetInput.disabled = true;
      targetLabel.appendChild(targetInput);
      targetCol.appendChild(targetLabel);

      row.append(currentCol, targetCol);
      form.appendChild(row);

      const button = document.createElement('button');
      button.type = 'submit';
      button.className = 'btn btn-primary btn-sm align-self-start';
      button.textContent = 'Сохранить';
      form.appendChild(button);

      form.addEventListener('submit', async (event) => {
        event.preventDefault();
        button.disabled = true;
        status.textContent = 'Сохранение...';
        try {
          await fetchJSON(`/api/v1/krs/${kr.id}/progress/percent`, {
            method: 'POST',
            headers: jsonHeaders,
            body: JSON.stringify({ current_value: parseFloat(currentInput.value) }),
          });
          const { normalized: comment, trimmed } = prepareCommentForSave(commentInput.value);
          if (trimmed && comment !== (kr.comments[0]?.text ?? '')) {
            await fetchJSON(`/api/v1/krs/${kr.id}/comments`, {
              method: 'POST',
              headers: jsonHeaders,
              body: JSON.stringify({ text: comment }),
            });
          }
          status.textContent = 'Сохранено.';
          await reloadTeamOKR();
        } catch (error) {
          status.textContent = error.message;
        } finally {
          button.disabled = false;
        }
      });
    }

    if (kr.measure.kind === 'BOOLEAN') {
      const checkbox = document.createElement('input');
      checkbox.type = 'checkbox';
      checkbox.className = 'form-check-input';
      checkbox.checked = kr.measure.boolean?.is_done ?? false;

      const label = document.createElement('label');
      label.className = 'form-check-label';
      label.textContent = 'Выполнено';

      const wrapper = document.createElement('div');
      wrapper.className = 'form-check';
      wrapper.append(checkbox, label);
      form.appendChild(wrapper);

      const button = document.createElement('button');
      button.type = 'submit';
      button.className = 'btn btn-primary btn-sm align-self-start';
      button.textContent = 'Сохранить';
      form.appendChild(button);

      form.addEventListener('submit', async (event) => {
        event.preventDefault();
        button.disabled = true;
        status.textContent = 'Сохранение...';
        try {
          await fetchJSON(`/api/v1/krs/${kr.id}/progress/boolean`, {
            method: 'POST',
            headers: jsonHeaders,
            body: JSON.stringify({ done: checkbox.checked }),
          });
          const { normalized: comment, trimmed } = prepareCommentForSave(commentInput.value);
          if (trimmed && comment !== (kr.comments[0]?.text ?? '')) {
            await fetchJSON(`/api/v1/krs/${kr.id}/comments`, {
              method: 'POST',
              headers: jsonHeaders,
              body: JSON.stringify({ text: comment }),
            });
          }
          status.textContent = 'Сохранено.';
          await reloadTeamOKR();
        } catch (error) {
          status.textContent = error.message;
        } finally {
          button.disabled = false;
        }
      });
    }

    if (kr.measure.kind === 'PROJECT') {
      const stages = kr.measure.project?.stages || [];
      stages.forEach((stage) => {
        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.className = 'form-check-input';
        checkbox.checked = stage.is_done;
        checkbox.dataset.stageId = stage.id;

        const label = document.createElement('label');
        label.className = 'form-check-label';
        label.textContent = `${stage.title} (${stage.weight}%)`;

        const wrapper = document.createElement('div');
        wrapper.className = 'form-check';
        wrapper.append(checkbox, label);
        form.appendChild(wrapper);
      });

      const button = document.createElement('button');
      button.type = 'submit';
      button.className = 'btn btn-primary btn-sm align-self-start';
      button.textContent = 'Сохранить';
      form.appendChild(button);

      form.addEventListener('submit', async (event) => {
        event.preventDefault();
        button.disabled = true;
        status.textContent = 'Сохранение...';
        try {
          const stagesPayload = Array.from(form.querySelectorAll('input[data-stage-id]')).map((input) => ({
            id: Number(input.dataset.stageId),
            done: input.checked,
          }));
          await fetchJSON(`/api/v1/krs/${kr.id}/progress/project`, {
            method: 'POST',
            headers: jsonHeaders,
            body: JSON.stringify({ stages: stagesPayload }),
          });
          const { normalized: comment, trimmed } = prepareCommentForSave(commentInput.value);
          if (trimmed && comment !== (kr.comments[0]?.text ?? '')) {
            await fetchJSON(`/api/v1/krs/${kr.id}/comments`, {
              method: 'POST',
              headers: jsonHeaders,
              body: JSON.stringify({ text: comment }),
            });
          }
          status.textContent = 'Сохранено.';
          await reloadTeamOKR();
        } catch (error) {
          status.textContent = error.message;
        } finally {
          button.disabled = false;
        }
      });
    }

    form.appendChild(status);
    panel.appendChild(form);
    return panel;
  };

  const renderOKRActions = (data, actionsEl) => {
    actionsEl.innerHTML = '';
    const wrapper = document.createElement('div');
    wrapper.className = 'd-flex flex-wrap gap-2';

    if (!isPeriodLocked()) {
      const addGoalButton = document.createElement('button');
      addGoalButton.type = 'button';
      addGoalButton.className = 'btn btn-primary';
      addGoalButton.textContent = 'Добавить цель';
      addGoalButton.addEventListener('click', () => openGoalCreateModal(data));
      wrapper.appendChild(addGoalButton);
    }
    actionsEl.appendChild(wrapper);
  };

  const renderGoalMenu = (goal) => {
    const wrapper = document.createElement('div');
    wrapper.className = 'dropdown ms-auto';
    const button = document.createElement('button');
    button.className = 'btn btn-outline-secondary btn-sm dropdown-toggle';
    button.type = 'button';
    button.dataset.bsToggle = 'dropdown';
    button.setAttribute('aria-expanded', 'false');
    button.textContent = '⋯';

    const menu = document.createElement('ul');
    menu.className = 'dropdown-menu dropdown-menu-end';

    if (!isPeriodLocked()) {
      menu.appendChild(buildMenuButton('Редактировать', () => openGoalModal(goal)));
      menu.appendChild(buildMenuButton('Шарить', () => openShareGoalModal(goal)));
    }
    menu.appendChild(buildMenuButton('Переместить вверх', () => moveGoal(goal.id, 'move-up')));
    menu.appendChild(buildMenuButton('Переместить вниз', () => moveGoal(goal.id, 'move-down')));
    if (!isPeriodLocked()) {
      menu.appendChild(buildMenuForm(`/goals/${goal.id}/delete`, buildReturnFields(), true));
    }

    wrapper.append(button, menu);
    return wrapper;
  };

  const renderKRMenu = (kr) => {
    const wrapper = document.createElement('div');
    wrapper.className = 'dropdown';
    const button = document.createElement('button');
    button.className = 'btn btn-outline-secondary btn-sm dropdown-toggle';
    button.type = 'button';
    button.dataset.bsToggle = 'dropdown';
    button.setAttribute('aria-expanded', 'false');
    button.textContent = '⋯';

    const menu = document.createElement('ul');
    menu.className = 'dropdown-menu dropdown-menu-end';

    if (!isPeriodLocked()) {
      menu.appendChild(buildMenuButton('Редактировать', () => openKRModal(kr)));
    }
    menu.appendChild(buildMenuButton('Переместить вверх', () => moveKeyResult(kr.id, 'move-up')));
    menu.appendChild(buildMenuButton('Переместить вниз', () => moveKeyResult(kr.id, 'move-down')));
    if (!isPeriodLocked()) {
      menu.appendChild(buildMenuForm(`/key-results/${kr.id}/delete`, buildReturnFields(), true));
    }

    wrapper.append(button, menu);
    return wrapper;
  };

  const buildMenuButton = (label, onClick) => {
    const item = document.createElement('li');
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'dropdown-item';
    button.textContent = label;
    button.addEventListener('click', onClick);
    item.appendChild(button);
    return item;
  };

  const buildMenuForm = (action, hiddenFields, confirmDelete = false) => {
    const item = document.createElement('li');
    const form = document.createElement('form');
    form.method = 'post';
    form.action = action;
    form.className = 'm-0';
    form.enctype = 'multipart/form-data';
    if (confirmDelete) {
      form.addEventListener('submit', (event) => {
        if (!window.confirm('Удалить запись?')) {
          event.preventDefault();
        }
      });
    }
    hiddenFields.forEach(({ name, value }) => {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = name;
      input.value = value;
      form.appendChild(input);
    });
    const button = document.createElement('button');
    button.type = 'submit';
    button.className = `dropdown-item ${confirmDelete ? 'text-danger' : ''}`;
    button.textContent = confirmDelete ? 'Удалить' : action.includes('move-up') ? 'Переместить вверх' : 'Переместить вниз';
    form.appendChild(button);
    item.appendChild(form);
    return item;
  };

  const buildReturnURL = () => {
    if (!state.teamOKR) return '';
    return `/teams/${state.teamOKR.team.id}/okr?period_id=${state.teamOKR.period.id}`;
  };

  const buildReturnFields = () => {
    if (!state.teamOKR) return [];
    return [
      { name: 'return', value: buildReturnURL() },
      { name: 'team_id', value: state.teamOKR.team.id },
    ];
  };
  const renderKRComments = (kr) => {
    const container = document.createElement('div');
    container.className = 'mt-2';
    const latestComment = getLatestComment(kr);
    if (!latestComment) {
      return container;
    }
    const title = document.createElement('div');
    title.className = 'small text-muted';
    title.textContent = 'Заметки';
    const list = document.createElement('ul');
    list.className = 'list-unstyled mb-0';
    const item = document.createElement('li');
    item.className = 'small okr-kr-comment is-clamped';
    item.textContent = latestComment.text;
    list.appendChild(item);
    container.append(title, list);
    return container;
  };

  const normalizeCommentText = (value) => value.replace(/\r\n/g, '\n');

  const prepareCommentForSave = (value) => {
    const normalized = normalizeCommentText(value);
    return {
      normalized,
      trimmed: normalized.trim(),
    };
  };

  const sumKRWeights = (krs) => krs.reduce((sum, kr) => sum + (kr.weight || 0), 0);

  function getLatestComment(kr) {
    if (!kr.comments || kr.comments.length === 0) return null;
    return kr.comments[0];
  }

  const renderSharedGoalBadge = (goal, options = {}) => {
    const wrapper = document.createElement('div');
    wrapper.className = 'd-flex align-items-center gap-1';
    const sharePeriodID = options.period_id ?? state.teamOKR?.period?.id ?? '';
    const showBadge = options.showBadge !== false;
    if (showBadge) {
      const badge = document.createElement('span');
      badge.className = 'badge text-bg-info share-goal-badge';
      badge.textContent = 'Общая';
      wrapper.appendChild(badge);
    }
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'btn btn-link p-0 share-goal-button';
    button.setAttribute('data-popover-content', `#share-goal-${goal.id}`);
    button.setAttribute('data-popover-trigger', 'hoverable');
    button.setAttribute('aria-label', 'Shared goal');
    button.innerHTML = '<i class="bi bi-share"></i>';
    wrapper.appendChild(button);

    const popover = document.createElement('div');
    popover.id = `share-goal-${goal.id}`;
    popover.className = 'd-none';
    const list = goal.share_teams
      .map(
        (team) =>
          `<li><a class="text-decoration-none" href="/teams/${team.id}/okr?period_id=${sharePeriodID}"><span class="text-muted">${team.type_label}</span> ${escapeHTML(team.name)}</a></li>`,
      )
      .join('');
    popover.innerHTML = `<div class="small fw-semibold mb-1">Команды с целью</div><ul class="list-unstyled mb-0">${list}</ul>`;
    wrapper.appendChild(popover);
    return wrapper;
  };

  const postForm = async (url, data) => {
    const body = new URLSearchParams(data);
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded; charset=utf-8' },
      body,
    });
    if (!response.ok) {
      throw new Error('Request failed');
    }
  };

  const postFormData = async (url, data) => {
    const body = new FormData();
    Object.entries(data).forEach(([key, value]) => {
      body.append(key, value);
    });
    const response = await fetch(url, { method: 'POST', body });
    if (!response.ok) {
      throw new Error('Request failed');
    }
  };

  const moveGoal = async (goalID, direction) => {
    try {
      await postFormData(`/api/v1/goals/${goalID}/${direction}`, {});
      await reloadTeamOKR();
    } catch (error) {
      // noop
    }
  };

  const moveKeyResult = async (krID, direction) => {
    try {
      await postFormData(`/api/v1/krs/${krID}/${direction}`, {});
      await reloadTeamOKR();
    } catch (error) {
      // noop
    }
  };

  const submitFormXHR = async (form) => {
    const response = await fetch(form.action, { method: 'POST', body: new FormData(form) });
    if (!response.ok) {
      throw new Error('Request failed');
    }
    return response;
  };

  const updateTeamPeriodStatus = async (teamID, periodID, status) => {
    await postFormData(`/api/v1/teams/${teamID}/status`, {
      period_id: periodID,
      status,
    });
  };

  const updatePeriodStatus = async (status) => {
    if (!state.teamOKR) return;
    try {
      await updateTeamPeriodStatus(state.teamOKR.team.id, state.teamOKR.period.id, status);
      await reloadTeamOKR();
    } catch (error) {
      // noop
    }
  };

  const priorityBadgeClass = (priority) => {
    switch (priority) {
      case 'P0':
        return 'text-bg-danger';
      case 'P1':
      case 'P2':
        return 'text-bg-success';
      case 'P3':
        return 'text-bg-secondary';
      default:
        return 'text-bg-secondary';
    }
  };

  const ensureModal = () => {
    let modalEl = document.getElementById('okr-modal');
    if (!modalEl) {
      modalEl = document.createElement('div');
      modalEl.className = 'modal fade';
      modalEl.id = 'okr-modal';
      modalEl.tabIndex = -1;
      modalEl.innerHTML = `
        <div class="modal-dialog modal-lg">
          <div class="modal-content">
            <div class="modal-header">
              <h5 class="modal-title"></h5>
              <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Закрыть"></button>
            </div>
            <div class="modal-body"></div>
          </div>
        </div>`;
      document.body.appendChild(modalEl);
    }
    return modalEl;
  };

  const openModal = (title, bodyHTML) => {
    const modalEl = ensureModal();
    modalEl.querySelector('.modal-title').textContent = title;
    modalEl.querySelector('.modal-body').innerHTML = bodyHTML;
    const modal = bootstrap.Modal.getOrCreateInstance(modalEl);
    modal.show();
    return modalEl;
  };

  const openGoalModal = (goal, options = {}) => {
    const action = options.action || `/api/v1/goals/${goal.id}`;
    const titleText = options.titleText || 'Редактировать цель';
    const submitLabel = options.submitLabel || 'Сохранить';
    const includePeriod = options.includePeriod === true;
    const isSharedGoal = goal.share_teams && goal.share_teams.length > 1;
    const body = `
      <form method="post" action="${action}" class="vstack gap-3" data-goal-edit-form>
        <div>
          <label class="form-label">Название</label>
          <input class="form-control" name="title" value="${escapeHTML(goal.title)}" />
        </div>
        <div>
          <label class="form-label">Описание</label>
          <textarea class="form-control" name="description">${escapeHTML(goal.description || '')}</textarea>
        </div>
        <div class="row g-3">
          <div class="col-md-4">
            <label class="form-label">Приоритет</label>
            ${buildSelect('priority', goalPriorityOptions, goal.priority)}
          </div>
          <div class="col-md-4">
            <label class="form-label">Вес</label>
            <input class="form-control" name="weight" type="number" value="${goal.weight}" />
          </div>
          <div class="col-md-4">
            <label class="form-label">Работа</label>
            ${buildSelect('work_type', goalWorkOptions, goal.work_type)}
          </div>
        </div>
        <div class="row g-3">
          <div class="col-md-6">
            <label class="form-label">Фокус</label>
            ${buildSelect('focus_type', goalFocusOptions, goal.focus_type)}
          </div>
          <div class="col-md-6">
            <label class="form-label">Владелец</label>
            <input class="form-control" name="owner_text" value="${escapeHTML(goal.owner_text || '')}" />
          </div>
        </div>
        ${includePeriod ? `<input type="hidden" name="period_id" value="${goal.period_id ?? ''}" />` : ''}
        ${isSharedGoal && state.teamOKR ? `<input type="hidden" name="team_id" value="${state.teamOKR.team.id}" />` : ''}
        <button class="btn btn-primary" type="submit">${submitLabel}</button>
      </form>`;
    const modalEl = openModal(titleText, body);
    const form = modalEl.querySelector('[data-goal-edit-form]');
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      const response = await submitFormXHR(form);
      const responseURL = response?.url ? new URL(response.url, window.location.origin) : null;
      if (responseURL?.hash) {
        window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}${responseURL.hash}`);
      }
      await reloadTeamOKR();
      bootstrap.Modal.getInstance(modalEl)?.hide();
    });
  };

  const openGoalCreateModal = (data) => {
    const emptyGoal = {
      id: 0,
      title: '',
      description: '',
      priority: goalPriorityOptions[2] || goalPriorityOptions[0],
      weight: 0,
      work_type: goalWorkOptions[0],
      focus_type: goalFocusOptions[0],
      owner_text: '',
      period_id: data.period?.id,
    };
    openGoalModal(emptyGoal, {
      action: `/teams/${data.team.id}/okr`,
      titleText: 'Добавить цель',
      submitLabel: 'Создать',
      includePeriod: true,
    });
  };

  const openShareGoalModal = async (goal) => {
    const hierarchy = state.hierarchy || (await fetchJSON('/api/v1/hierarchy')).items || [];
    state.hierarchy = hierarchy;
    const teamsById = new Map();
    const collectTeams = (tree) => {
      tree.forEach((node) => {
        teamsById.set(node.id, node);
        if (node.children && node.children.length) {
          collectTeams(node.children);
        }
      });
    };
    collectTeams(hierarchy);
    (goal.share_teams || []).forEach((team) => {
      teamsById.set(team.id, team);
    });

    const selectedTeamIds = (goal.share_teams || []).map((team) => team.id);
    const buildOptions = (currentTeamId, tree = hierarchy, level = 0) => {
      let html = '';
      if (level === 0) {
        const currentTeam = teamsById.get(currentTeamId);
        if (currentTeam) {
          html += `<option value="${currentTeam.id}" selected hidden>${currentTeam.type_label} ${escapeHTML(currentTeam.name)}</option>`;
        }
      }
      tree.forEach((node) => {
        if (!selectedTeamIds.includes(node.id)) {
          const prefix = '&nbsp;'.repeat(level * 2);
          html += `<option value="${node.id}">${prefix}${node.type_label} ${escapeHTML(node.name)}</option>`;
        }
        if (node.children && node.children.length) {
          html += buildOptions(currentTeamId, node.children, level + 1);
        }
      });
      return html;
    };
    const buildRow = (teamId, index, canRemove) => {
      const team = teamsById.get(teamId);
      const teamLabel = team ? `${team.type_label} ${escapeHTML(team.name)}` : 'Команда';
      return `
        <div class="row g-2 align-items-end" data-share-row data-share-index="${index}">
          <div class="col-md-8">
            <label class="form-label">Команда</label>
            <select class="form-select" name="team_id" aria-label="${teamLabel}">
              ${buildOptions(teamId)}
            </select>
          </div>
          <div class="col-auto">
            <button class="btn btn-outline-danger px-3" type="button" data-remove-share${canRemove ? '' : ' disabled'}>&times;</button>
          </div>
        </div>`;
    };
    const body = `
      <form class="vstack gap-3" data-share-goal-form>
        <div class="small text-muted">Команды, владеющие целью</div>
        <div data-share-list class="vstack gap-2"></div>
        <button class="btn btn-outline-secondary btn-sm align-self-start" type="button" data-add-share>Добавить команду</button>
        <button class="btn btn-primary" type="submit">Сохранить</button>
      </form>`;
    const modalEl = openModal('Шарить цель', body);
    const form = modalEl.querySelector('[data-share-goal-form]');
    const list = modalEl.querySelector('[data-share-list]');
    const addButton = modalEl.querySelector('[data-add-share]');

    const renderRows = () => {
      list.innerHTML = selectedTeamIds.map((teamId, index) => buildRow(teamId, index, selectedTeamIds.length > 1)).join('');
      addButton.disabled = teamsById.size === selectedTeamIds.length;
    };

    addButton.addEventListener('click', () => {
      const nextTeam = Array.from(teamsById.keys()).find((teamId) => !selectedTeamIds.includes(teamId));
      if (!nextTeam) return;
      selectedTeamIds.push(nextTeam);
      renderRows();
    });

    list.addEventListener('click', (event) => {
      const removeButton = event.target.closest('[data-remove-share]');
      if (!removeButton || selectedTeamIds.length <= 1) return;
      const row = removeButton.closest('[data-share-row]');
      const index = Number(row?.dataset.shareIndex);
      if (Number.isNaN(index)) return;
      selectedTeamIds.splice(index, 1);
      renderRows();
    });

    list.addEventListener('change', (event) => {
      const select = event.target.closest('select[name="team_id"]');
      if (!select) return;
      const row = select.closest('[data-share-row]');
      const index = Number(row?.dataset.shareIndex);
      const teamId = Number(select.value);
      if (Number.isNaN(index) || !teamId) return;
      selectedTeamIds[index] = teamId;
      renderRows();
    });

    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      if (selectedTeamIds.length === 0) return;
      const payload = new FormData();
      selectedTeamIds.forEach((teamId) => {
        payload.append('team_ids', String(teamId));
      });
      payload.append('return', `${window.location.pathname}${window.location.search}`);

      const response = await fetch(`/goals/${goal.id}/share`, {
        method: 'POST',
        body: payload,
      });
      if (!response.ok) {
        throw new Error('Не удалось обновить список команд для цели');
      }
      await reloadTeamOKR();
      bootstrap.Modal.getInstance(modalEl)?.hide();
    });

    renderRows();
  };

  const openKRModalWithAction = (kr, action, titleText) => {
    const goalMetaSource = state.teamOKR?.goals?.find((goal) => goal.id === kr.goal_id);
    const goalPriority = kr.goal_priority || goalMetaSource?.priority || '';
    const goalWorkType = kr.goal_work_type || goalMetaSource?.work_type || '';
    const goalFocusType = kr.goal_focus_type || goalMetaSource?.focus_type || '';
    const goalMeta = `
      <div class="row g-3">
        <div class="col-md-4">
          <label class="form-label">Приоритет цели</label>
          <input class="form-control" value="${escapeHTML(goalPriority)}" disabled />
        </div>
        <div class="col-md-4">
          <label class="form-label">Тип работы</label>
          <input class="form-control" value="${escapeHTML(goalWorkType)}" disabled />
        </div>
        <div class="col-md-4">
          <label class="form-label">Фокус</label>
          <input class="form-control" value="${escapeHTML(goalFocusType)}" disabled />
        </div>
      </div>`;
    const kindOptions = ['PERCENT', 'LINEAR', 'BOOLEAN', 'PROJECT'];
    const normalizedKind = (kr.kind || kr.measure?.kind || 'PERCENT').toUpperCase();
    const selectedKind = kindOptions.includes(normalizedKind) ? normalizedKind : 'PERCENT';
    const percentSection = `
      <div data-kind-section="PERCENT" class="vstack gap-2">
        <div class="row g-3">
          <div class="col-md-4">
            <label class="form-label">Start</label>
            <input class="form-control" name="percent_start" type="number" step="any" value="${kr.measure?.percent?.start_value ?? 0}" />
          </div>
          <div class="col-md-4">
            <label class="form-label">Target</label>
            <input class="form-control" name="percent_target" type="number" step="any" value="${kr.measure?.percent?.target_value ?? 0}" />
          </div>
          <div class="col-md-4">
            <label class="form-label">Current</label>
            <input class="form-control" name="percent_current" type="number" step="any" value="${kr.measure?.percent?.current_value ?? 0}" />
          </div>
        </div>
      </div>`;
    const linearSection = `
      <div data-kind-section="LINEAR" class="vstack gap-2">
        <div class="row g-3">
          <div class="col-md-4">
            <label class="form-label">Start</label>
            <input class="form-control" name="linear_start" type="number" step="any" value="${kr.measure?.linear?.start_value ?? 0}" />
          </div>
          <div class="col-md-4">
            <label class="form-label">Target</label>
            <input class="form-control" name="linear_target" type="number" step="any" value="${kr.measure?.linear?.target_value ?? 0}" />
          </div>
          <div class="col-md-4">
            <label class="form-label">Current</label>
            <input class="form-control" name="linear_current" type="number" step="any" value="${kr.measure?.linear?.current_value ?? 0}" />
          </div>
        </div>
      </div>`;
    const booleanSection = `
      <div data-kind-section="BOOLEAN" class="form-check">
        <input class="form-check-input" type="checkbox" name="boolean_done" value="true" ${kr.measure?.boolean?.is_done ? 'checked' : ''} />
        <label class="form-check-label">Выполнено</label>
      </div>`;
    const projectStages = (kr.measure?.project?.stages || [])
      .map(
        (stage) => `
          <div class="row g-2 align-items-end" data-stage-row>
            <div class="col-md-6">
              <label class="form-label">Шаг</label>
              <input class="form-control" name="step_title[]" value="${escapeHTML(stage.title)}" />
            </div>
            <div class="col-md-3">
              <label class="form-label">Вес</label>
              <input class="form-control" name="step_weight[]" type="number" value="${stage.weight}" />
            </div>
            <div class="col-md-3 form-check">
              <input class="form-check-input" type="checkbox" name="step_done[]" value="true" ${stage.is_done ? 'checked' : ''} />
              <label class="form-check-label">Готово</label>
            </div>
          </div>`,
      )
      .join('');

    const projectSection = `
      <div data-kind-section="PROJECT" class="vstack gap-2">
        <div data-stage-list class="vstack gap-2">
          ${projectStages || ''}
        </div>
        <button type="button" class="btn btn-outline-secondary btn-sm" data-add-stage>Добавить шаг</button>
      </div>`;

    const body = `
      <form method="post" action="${action}" class="vstack gap-3" data-kr-edit-form>
        <div>
          <label class="form-label">Название</label>
          <input class="form-control" name="title" value="${escapeHTML(kr.title)}" />
        </div>
        <div>
          <label class="form-label">Описание</label>
          <textarea class="form-control" name="description">${escapeHTML(kr.description || '')}</textarea>
        </div>
        <div class="row g-3">
          <div class="col-md-6">
            <label class="form-label">Вес</label>
            <input class="form-control" name="weight" type="number" value="${kr.weight}" />
          </div>
          <div class="col-md-6">
            <label class="form-label">Тип</label>
            ${buildSelect('kind', kindOptions, selectedKind)}
          </div>
        </div>
        ${goalMeta}
        ${percentSection}
        ${linearSection}
        ${booleanSection}
        ${projectSection}
        <input type="hidden" name="return" value="${buildReturnURL()}" />
        <button class="btn btn-primary" type="submit">Сохранить</button>
      </form>`;
    const modalEl = openModal(titleText, body);
    const form = modalEl.querySelector('[data-kr-edit-form]');
    const kindSelect = form.querySelector('select[name="kind"]');
    const sections = Array.from(form.querySelectorAll('[data-kind-section]'));
    const updateSections = () => {
      sections.forEach((section) => {
        section.hidden = section.dataset.kindSection !== kindSelect.value;
      });
    };
    kindSelect.value = selectedKind;
    updateSections();
    kindSelect.addEventListener('change', updateSections);
    const addStageButton = form.querySelector('[data-add-stage]');
    if (addStageButton) {
      addStageButton.addEventListener('click', () => {
        const list = form.querySelector('[data-stage-list]');
        if (!list) return;
        const row = document.createElement('div');
        row.className = 'row g-2 align-items-end';
        row.innerHTML = `
          <div class="col-md-6">
            <label class="form-label">Шаг</label>
            <input class="form-control" name="step_title[]" />
          </div>
          <div class="col-md-3">
            <label class="form-label">Вес</label>
            <input class="form-control" name="step_weight[]" type="number" value="0" />
          </div>
          <div class="col-md-3 form-check">
            <input class="form-check-input" type="checkbox" name="step_done[]" value="true" />
            <label class="form-check-label">Готово</label>
          </div>`;
        list.appendChild(row);
      });
    }
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      await submitFormXHR(form);
      await reloadTeamOKR();
      bootstrap.Modal.getInstance(modalEl)?.hide();
    });
  };

  const openKRModal = (kr) => {
    openKRModalWithAction(kr, `/api/v1/krs/${kr.id}`, 'Редактировать KR');
  };

  const openKRCreateModal = (goal) => {
    const emptyKR = {
      id: 0,
      title: '',
      description: '',
      weight: 0,
      kind: 'PERCENT',
      measure: {
        percent: { start_value: 0, target_value: 100, current_value: 0 },
        linear: { start_value: 0, target_value: 100, current_value: 0 },
        boolean: { is_done: false },
        project: { stages: [] },
      },
    };
    openKRModalWithAction(emptyKR, `/api/v1/goals/${goal.id}/key-results`, 'Добавить KR');
  };

  const buildSelect = (name, options, selected) => {
    const select = document.createElement('select');
    select.className = 'form-select';
    select.name = name;
    options.forEach((option) => {
      const opt = document.createElement('option');
      opt.value = option;
      opt.textContent = option;
      if (option === selected) {
        opt.selected = true;
      }
      select.appendChild(opt);
    });
    return select.outerHTML;
  };

  const flattenHierarchyOptions = (tree, level = 0) => {
    let html = '';
    tree.forEach((node) => {
      const prefix = '&nbsp;'.repeat(level * 2);
      html += `<option value="${node.id}">${prefix}${node.type_label} ${escapeHTML(node.name)}</option>`;
      if (node.children && node.children.length) {
        html += flattenHierarchyOptions(node.children, level + 1);
      }
    });
    return html;
  };

  const escapeHTML = (value) => {
    const div = document.createElement('div');
    div.textContent = value ?? '';
    return div.innerHTML;
  };

  const renderTeamCell = (team, periodID) => {
    const cell = document.createElement('td');
    cell.className = 'teams-col-team';
    const wrapper = document.createElement('div');
    wrapper.className = 'd-flex align-items-center gap-2';
    wrapper.style.paddingLeft = `${team.indent}px`;

    const badge = document.createElement('span');
    badge.className = 'badge text-bg-secondary';
    badge.textContent = team.type_label;

    const link = document.createElement('a');
    link.className = 'link-primary';
    link.href = `/teams/${team.id}/okr?period_id=${periodID}`;
    link.textContent = team.name;

    wrapper.append(badge, link);
    cell.appendChild(wrapper);
    return cell;
  };

  const renderPeriodProgressCell = (team) => {
    const cell = document.createElement('td');
    cell.className = 'teams-col-progress';
    const wrapper = document.createElement('div');
    wrapper.className = 'd-flex align-items-center gap-2';
    const progressBar = document.createElement('div');
    progressBar.className = 'progress flex-grow-1';
    progressBar.setAttribute('role', 'progressbar');
    progressBar.setAttribute('aria-valuenow', team.period_progress);
    progressBar.setAttribute('aria-valuemin', '0');
    progressBar.setAttribute('aria-valuemax', '100');
    const fill = document.createElement('div');
    fill.className = 'progress-bar';
    fill.style.width = `${team.period_progress}%`;
    progressBar.appendChild(fill);
    const value = document.createElement('span');
    value.className = 'fw-semibold';
    value.textContent = `${team.period_progress}%`;
    wrapper.append(progressBar, value);
    cell.appendChild(wrapper);
    return cell;
  };

  const renderGoalsCell = (team, periodID) => {
    const cell = document.createElement('td');
    cell.className = 'teams-col-goals';
    if (!team.goals || team.goals.length === 0) {
      const empty = document.createElement('span');
      empty.className = 'text-muted';
      empty.textContent = 'Нет целей';
      cell.appendChild(empty);
      return cell;
    }

    const table = document.createElement('table');
    table.className = 'table table-hover';
    table.style.fontSize = 'small';
    const thead = document.createElement('thead');
    const theadTR = document.createElement('tr');

    const theadTRW = document.createElement('td')
    theadTRW.textContent = "Вес"
    theadTRW.className = 'teams-goals-col-weight';
    theadTR.append(theadTRW);

    const theadTRP = document.createElement('td')
    theadTRP.textContent = "Prio"
    theadTRP.className = 'teams-goals-col-priority';
    theadTR.append(theadTRP)

    const theadTRT = document.createElement('td')
    theadTRT.textContent = ""
    theadTRT.className = 'text-center';
    theadTR.append(theadTRT)

    const theadTRN = document.createElement('td')
    theadTRN.textContent = "Название"
    theadTRN.className = 'teams-goals-col-title';
    theadTR.append(theadTRN)

    const theadTRPR = document.createElement('td')
    theadTRPR.textContent = "Прогресс"
    theadTRPR.className = 'teams-goals-col-progress';
    theadTR.append(theadTRPR)

    thead.appendChild(theadTR);
    table.appendChild(thead);
    const tbody = document.createElement('tbody');

    team.goals.forEach((goal) => {
      const row = document.createElement('tr');
      const weight = document.createElement('td');
      weight.className = 'teams-goals-col-weight';
      const weightBadge = document.createElement('span');
      weightBadge.className = 'badge text-bg-light border';
      weightBadge.textContent = goal.weight;
      weight.appendChild(weightBadge);

      const priority = document.createElement('td');
      priority.className = 'teams-goals-col-priority';
      const priorityBadge = document.createElement('span');
      priorityBadge.className = `badge ${priorityBadgeClass(goal.priority)}`;
      priorityBadge.textContent = goal.priority;
      priority.appendChild(priorityBadge);

      const shared = document.createElement('td');
      shared.className = 'text-center';
      if (goal.share_teams && goal.share_teams.length > 1) {
        shared.appendChild(renderSharedGoalBadge(goal, { period_id: periodID, showBadge: false }));
      }

      const title = document.createElement('td');
      title.className = 'teams-goals-col-title text-break';
      const titleWrapper = document.createElement('div');
      titleWrapper.className = 'd-flex align-items-center gap-2';
      const titleText = document.createElement('span');
      titleText.textContent = goal.title;
      titleWrapper.appendChild(titleText);
      title.appendChild(titleWrapper);

      const progress = document.createElement('td');
      progress.className = 'teams-goals-col-progress';
      const progressBadge = document.createElement('span');
      progressBadge.className = 'badge text-bg-light border';
      progressBadge.textContent = `${goal.progress}%`;
      progress.appendChild(progressBadge);

      row.append(weight, priority, shared, title, progress);
      tbody.appendChild(row);
    });

    table.appendChild(tbody);
    cell.appendChild(table);

    if (team.goals_weight !== 100) {
      const weightSummary = document.createElement('div');
      weightSummary.className = 'mt-2';
      const weightBadge = document.createElement('span');
      weightBadge.className = 'badge text-bg-danger';
      weightBadge.textContent = `Сумма целей ${team.goals_weight}`;
      weightSummary.appendChild(weightBadge);
      cell.appendChild(weightSummary);
    }

    return cell;
  };

  const renderStatusCell = (team) => {
    const cell = document.createElement('td');
    cell.className = 'teams-col-status';
    const badge = document.createElement('span');
    badge.className = 'badge text-bg-light border';
    badge.textContent = team.status_label;
    cell.appendChild(badge);
    return cell;
  };

  const createOption = (value, label) => {
    const option = document.createElement('option');
    option.value = value;
    option.textContent = label;
    return option;
  };

  let reloadTeamOKR = async () => { };

  const findCurrentTeamNode = (nodes) => {
    for (const node of nodes || []) {
      if (node.current) return node;
      const child = findCurrentTeamNode(node.children || []);
      if (child) return child;
    }
    return null;
  };

  const flattenHierarchyNodes = (nodes, map = new Map(), parentByID = new Map(), parentID = null) => {
    (nodes || []).forEach((node) => {
      map.set(String(node.id), node);
      if (parentID !== null) {
        parentByID.set(String(node.id), String(parentID));
      }
      flattenHierarchyNodes(node.children || [], map, parentByID, node.id);
    });
    return { map, parentByID };
  };

  const collectPathToTeam = (teamID, parentByID) => {
    const path = new Set();
    let cursor = String(teamID);
    while (cursor && parentByID.has(cursor)) {
      const parent = parentByID.get(cursor);
      path.add(parent);
      cursor = parent;
    }
    return path;
  };

  const renderGoalsTable = (goals, periodID, currentTeamID) => {
    const wrapper = document.createElement('div');
    wrapper.className = 'team-goals-table-wrapper';

    const table = document.createElement('table');
    table.className = 'table table-hover';
    table.style.fontSize = 'small';
    table.innerHTML = `
      <thead>
        <tr>
          <td class="teams-goals-col-weight">Вес</td>
          <td class="teams-goals-col-priority">Prio</td>
          <td class="text-center"></td>
          <td class="teams-goals-col-title">Название</td>
          <td>Владелец</td>
          <td>Тип</td>
          <td class="teams-goals-col-progress">Прогресс</td>
          <td>Обновлено</td>
        </tr>
      </thead>`;
    const tbody = document.createElement('tbody');
    (goals || []).forEach((goal) => {
      const row = document.createElement('tr');
      const weight = document.createElement('td');
      weight.className = 'teams-goals-col-weight';
      weight.innerHTML = `<span class="badge text-bg-light border">${goal.weight}</span>`;

      const priority = document.createElement('td');
      priority.className = 'teams-goals-col-priority';
      priority.innerHTML = `<span class="badge ${priorityBadgeClass(goal.priority)}">${escapeHTML(goal.priority)}</span>`;

      const shared = document.createElement('td');
      shared.className = 'text-center';
      if (goal.share_teams && goal.share_teams.length > 1) {
        shared.appendChild(renderSharedGoalBadge(goal, { period_id: periodID, showBadge: false }));
      }

      const title = document.createElement('td');
      title.className = 'teams-goals-col-title text-break';
      const titleWrap = document.createElement('div');
      titleWrap.className = 'd-flex align-items-center gap-2';
      const titleLink = document.createElement('a');
      titleLink.className = 'link-primary';
      const targetTeamID = currentTeamID || goal.team_id;
      titleLink.href = `/teams/${targetTeamID}/okr?period_id=${periodID}#goal-${goal.id}`;
      titleLink.textContent = goal.title;
      titleWrap.appendChild(titleLink);
      title.appendChild(titleWrap);

      const owner = document.createElement('td');
      const ownerValue = document.createElement('span');
      ownerValue.className = 'text-decoration-underline';
      ownerValue.textContent = goal.owner_text || '—';
      owner.appendChild(ownerValue);

      const work = document.createElement('td');
      work.className = 'text-center';
      const workIcon = renderWorkTypeIcon(goal.work_type);
      work.appendChild(workIcon);

      const progress = document.createElement('td');
      progress.className = 'teams-goals-col-progress';
      progress.innerHTML = `<span class="badge text-bg-light border">${goal.progress}%</span>`;

      const updated = document.createElement('td');
      const goalUpdateMeta = getLastGoalUpdateMeta([goal]);
      if (!goalUpdateMeta.hasDate) {
        updated.textContent = '—';
      } else {
        const freshness = document.createElement('span');
        freshness.className = goalUpdateMeta.isStale ? 'text-danger' : 'text-success';
        const emoji = goalUpdateMeta.isStale ? '⏰' : '✅';
        freshness.textContent = `${emoji} ${goalUpdateMeta.relativeText}`;
        freshness.title = goalUpdateMeta.absoluteText;
        updated.appendChild(freshness);
      }

      row.append(weight, priority, shared, title, owner, work, progress, updated);
      tbody.appendChild(row);
    });
    table.appendChild(tbody);
    wrapper.appendChild(table);
    return wrapper;
  };

  const renderForecastProgress = (progressMeta) => {
    const wrapper = document.createElement('div');
    wrapper.className = 'd-flex flex-column gap-1';

    const row = document.createElement('div');
    row.className = 'd-flex align-items-center gap-2';

    const track = document.createElement('div');
    track.className = 'progress flex-grow-1 position-relative';
    track.style.height = '12px';

    const actual = Number(progressMeta?.actual || 0);
    const forecast = Number(progressMeta?.forecast || 0);

    const actualFill = document.createElement('div');
    actualFill.className = 'progress-bar';
    actualFill.style.width = `${Math.max(0, Math.min(100, actual))}%`;
    track.appendChild(actualFill);

    const marker = document.createElement('span');
    marker.style.position = 'absolute';
    marker.style.left = `${Math.max(0, Math.min(100, forecast))}%`;
    marker.style.top = '-4px';
    marker.style.bottom = '-4px';
    marker.style.width = '4px';
    marker.style.transform = 'translateX(-2px)';
    marker.style.borderRadius = '999px';
    marker.style.backgroundColor = '#ff1744';
    marker.style.boxShadow = '0 0 0 1px rgba(255,255,255,0.65), 0 0 6px rgba(255,23,68,0.8)';
    marker.title = `Forecast ${forecast}%`;
    track.appendChild(marker);

    const value = document.createElement('span');
    value.className = 'small fw-semibold';
    value.textContent = `${actual}%`;

    const hoverText = `Forecast: ${forecast}% | Actual: ${actual}%`;
    track.title = hoverText;
    value.title = hoverText;

    row.append(track, value);

    const chip = document.createElement('span');
    chip.className = 'badge align-self-start';
    const status = progressMeta?.status || 'on_track';
    if (status === 'above') {
      chip.classList.add('text-bg-success');
      chip.textContent = '▲ above';
    } else if (status === 'below') {
      chip.classList.add('text-bg-danger');
      chip.textContent = '▼ below';
    } else {
      chip.classList.add('text-bg-secondary');
      chip.textContent = '● on track';
    }

    wrapper.append(row, chip);
    return wrapper;
  };

  const getLastGoalUpdateMeta = (goals = []) => {
    const lastUpdated = goals
      .map((goal) => goal.updated_at)
      .filter(Boolean)
      .map((value) => new Date(value))
      .filter((date) => !Number.isNaN(date.getTime()))
      .reduce((max, date) => (max && max > date ? max : date), null);

    if (!lastUpdated) {
      return { hasDate: false, text: '—', relativeText: '—', absoluteText: '', iso: '', isStale: false };
    }

    const now = new Date();
    const diffMs = now - lastUpdated;
    const staleThresholdMs = 14 * 24 * 60 * 60 * 1000;
    const isStale = diffMs > staleThresholdMs;
    const iso = lastUpdated.toISOString();
    const absolute = formatAbsoluteDate(iso);
    const relative = formatRelativeUpdate(iso);
    return {
      hasDate: true,
      isStale,
      iso,
      absoluteText: absolute,
      relativeText: relative || 'сегодня',
      text: `${absolute}${relative ? ` (${relative})` : ''}`,
    };
  };

  const renderChildrenOverview = (container, overviewData) => {
    const overview = document.createElement('div');
    overview.className = 'card mb-3';
    const body = document.createElement('div');
    body.className = 'card-body';

    const title = document.createElement('h4');
    title.className = 'h6 mb-3';
    title.textContent = 'Overview по дочерним командам';
    body.appendChild(title);

    const avgProgress = Number(overviewData?.average_progress || 0);

    const progressWrap = document.createElement('div');
    progressWrap.className = 'mb-3';
    progressWrap.innerHTML = `<div class="small text-muted mb-1">Совокупный прогресс (средний по командам с целями)</div>`;
    progressWrap.appendChild(renderForecastProgress(overviewData?.progress_meta || {
      actual: avgProgress,
      forecast: avgProgress,
      status: 'on_track',
    }));
    body.appendChild(progressWrap);

    const priorities = {
      P0: Number(overviewData?.priorities?.p0 || 0),
      P1: Number(overviewData?.priorities?.p1 || 0),
      P2: Number(overviewData?.priorities?.p2 || 0),
      P3: Number(overviewData?.priorities?.p3 || 0),
    };
    const workBalance = {
      discovery: Number(overviewData?.work_balance?.discovery || 0),
      delivery: Number(overviewData?.work_balance?.delivery || 0),
    };

    const chartSection = document.createElement('div');
    chartSection.className = 'd-flex flex-wrap align-items-start gap-4';

    const renderPie = (titleText, data, colors, labels) => {
      const wrap = document.createElement('div');
      wrap.className = 'd-flex align-items-center gap-3';

      const chartWrap = document.createElement('div');
      const chartTitle = document.createElement('div');
      chartTitle.className = 'small text-muted mb-1';
      chartTitle.textContent = titleText;
      const chart = document.createElement('div');
      chart.style.width = '120px';
      chart.style.height = '120px';
      chart.style.borderRadius = '50%';

      const total = Object.values(data).reduce((sum, val) => sum + val, 0);
      if (total > 0) {
        let current = 0;
        const segments = Object.entries(data).map(([key, value]) => {
          const angle = (value / total) * 360;
          const start = current;
          const end = current + angle;
          current = end;
          return `${colors[key]} ${start}deg ${end}deg`;
        });
        chart.style.background = `conic-gradient(${segments.join(', ')})`;
      } else {
        chart.style.background = '#e9ecef';
      }
      chartWrap.append(chartTitle, chart);

      const legend = document.createElement('div');
      legend.className = 'small';
      legend.innerHTML = Object.entries(data)
        .map(([key, value]) => `<div class="mb-1"><span class="badge" style="background:${colors[key]}">${labels[key]}</span> — ${value}</div>`)
        .join('');

      wrap.append(chartWrap, legend);
      return wrap;
    };

    chartSection.appendChild(renderPie('Приоритеты целей', priorities, {
      P0: '#dc3545',
      P1: '#fd7e14',
      P2: '#198754',
      P3: '#6c757d',
    }, {
      P0: 'P0',
      P1: 'P1',
      P2: 'P2',
      P3: 'P3',
    }));

    chartSection.appendChild(renderPie('Баланс Discovery / Delivery', workBalance, {
      discovery: '#0d6efd',
      delivery: '#20c997',
    }, {
      discovery: 'Discovery',
      delivery: 'Delivery',
    }));

    body.appendChild(chartSection);
    overview.appendChild(body);
    container.appendChild(overview);
  };

  const renderTeamGoalSection = (data, container, options = {}) => {
    const section = document.createElement('section');
    section.className = 'team-section card';
    const body = document.createElement('div');
    body.className = 'card-body';

    const title = document.createElement(options.headingTag || 'h3');
    title.className = options.titleClass || 'h5 mb-2';
    const titleLink = document.createElement('a');
    titleLink.className = 'link-primary text-decoration-none';
    titleLink.href = `/teams/${data.team.id}/okr?period_id=${data.period?.id || ''}`;
    titleLink.textContent = options.title || `${data.team.type_label} ${data.team.name}`;
    title.appendChild(titleLink);
    body.appendChild(title);

    const hasGoals = Array.isArray(data.goals) && data.goals.length > 0;

    if (hasGoals && typeof data.period_progress === 'number') {
      body.appendChild(renderForecastProgress(data.progress_meta || {
        actual: data.period_progress,
        forecast: data.period_progress,
        status: 'on_track',
      }));
    }

    if (options.showStatusControls) {
      const controls = document.createElement('div');
      controls.className = 'd-flex flex-wrap align-items-end gap-2 mb-2';

      const statusWrap = document.createElement('div');
      statusWrap.className = 'flex-grow-1';
      const statusLabel = document.createElement('label');
      statusLabel.className = 'form-label mb-1 small';
      statusLabel.textContent = 'Статус периода';
      const statusSelect = document.createElement('select');
      statusSelect.className = 'form-select form-select-sm';
      const statusOptions = [
        { value: 'no_goals', label: 'Нет целей' },
        { value: 'forming', label: 'Черновик целей' },
        { value: 'in_progress', label: 'Готовы к валидации' },
        { value: 'validated', label: 'Провалидировано' },
        { value: 'closed', label: 'Цели закрыты' },
      ];
      statusOptions.forEach((option) => {
        const opt = document.createElement('option');
        opt.value = option.value;
        opt.textContent = option.label;
        if (option.value === data.period_status) {
          opt.selected = true;
        }
        statusSelect.appendChild(opt);
      });
      statusSelect.addEventListener('change', async () => {
        statusSelect.disabled = true;
        try {
          await updateTeamPeriodStatus(data.team.id, data.period.id, statusSelect.value);
          if (typeof options.onStatusUpdated === "function") {
            await options.onStatusUpdated();
          }
        } finally {
          statusSelect.disabled = false;
        }
      });
      statusWrap.append(statusLabel, statusSelect);
      controls.appendChild(statusWrap);
      body.appendChild(controls);

      if (hasGoals) {
        const lastUpdateMeta = getLastGoalUpdateMeta(data.goals || []);
        const updatedText = document.createElement('div');
        updatedText.className = 'small text-muted mb-2';
        updatedText.textContent = `Последнее обновление целей: ${lastUpdateMeta.text}`;
        body.appendChild(updatedText);
      }
    }

    if (options.subtitle) {
      const subtitle = document.createElement('p');
      subtitle.className = 'text-muted mb-3';
      subtitle.textContent = options.subtitle;
      body.appendChild(subtitle);
    }

    if (!data.goals || data.goals.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'text-muted mb-2';
      empty.textContent = 'У команды нет OKR на выбранный период';
      body.appendChild(empty);

      const createGoalsBtn = document.createElement('a');
      createGoalsBtn.className = 'btn btn-primary btn-sm';
      createGoalsBtn.href = `/teams/${data.team.id}/okr?period_id=${data.period?.id || ''}`;
      createGoalsBtn.textContent = 'Создать цели';
      body.appendChild(createGoalsBtn);
    } else {
      body.appendChild(renderGoalsTable(data.goals, data.period?.id, data.team?.id));
      if (options.showWeightSummary) {
        const goalsWeight = data.goals.reduce((sum, goal) => sum + (Number(goal.weight) || 0), 0);
        const summaryWrap = document.createElement('div');
        summaryWrap.className = 'mt-2';
        const summaryBadge = document.createElement('span');
        summaryBadge.className = `badge ${goalsWeight === 100 ? 'text-bg-success' : 'text-bg-danger'}`;
        summaryBadge.textContent = `Сумма целей ${goalsWeight}`;
        summaryWrap.appendChild(summaryBadge);
        body.appendChild(summaryWrap);
      }
    }

    section.appendChild(body);
    container.appendChild(section);
  };

  const filterHierarchyByVisibleTeams = (nodes, visibleIDs) => {
    const normalized = new Set(Array.from(visibleIDs || []).map((id) => String(id)));
    const walk = (tree) => {
      const result = [];
      (tree || []).forEach((node) => {
        const children = walk(node.children || []);
        const isVisible = normalized.has(String(node.id));
        if (isVisible) {
          result.push({ ...node, children });
        } else {
          result.push(...children);
        }
      });
      return result;
    };
    return walk(nodes || []);
  };

  const initTeamsPage = () => {
    const page = document.querySelector('[data-page="team-okrs"]');
    if (!page) return;

    const periodSelect = document.querySelector('[data-period-select]');
    const treeEl = page.querySelector('[data-team-hierarchy-tree]');
    const mainEl = page.querySelector('[data-team-main-content]');
    const initialTeam = page.dataset.selectedTeam || '';
    const expandedNodes = new Set();

    const loadHierarchy = async () => loadHierarchyForPeriod(periodSelect?.value || page.dataset.periodId || '');

    const getVisibleHierarchy = (teamSummaryMap = new Map()) => {
      const visibleIDs = new Set(Array.from(teamSummaryMap.keys()));
      return filterHierarchyByVisibleTeams(state.hierarchy || [], visibleIDs);
    };
    const loadTeamsSummaryMap = async () => {
      const periodID = periodSelect?.value || page.dataset.periodId;
      if (!periodID) return new Map();
      const cacheKey = String(periodID);
      if (state.teamsSummaryByPeriod[cacheKey]) {
        return state.teamsSummaryByPeriod[cacheKey];
      }
      const url = new URL('/api/v1/teams', window.location.origin);
      url.searchParams.set('period_id', periodID);
      const payload = await fetchJSON(url.toString());
      const map = new Map((payload.items || []).map((team) => [String(team.id), team]));
      state.teamsSummaryByPeriod[cacheKey] = map;
      return map;
    };

    const resolveSelectedTeamID = (tree) => {
      const url = new URL(window.location.href);
      const fromURL = url.searchParams.get('team');
      const { map, parentByID } = flattenHierarchyNodes(tree);
      let selected = fromURL || initialTeam;
      if (!selected || selected === 'ALL' || !map.has(String(selected))) {
        const currentNode = findCurrentTeamNode(tree);
        selected = currentNode ? String(currentNode.id) : (tree[0] ? String(tree[0].id) : '');
      }
      collectPathToTeam(selected, parentByID).forEach((id) => expandedNodes.add(id));
      return { selected, map, parentByID };
    };

    const updateURL = (teamID, periodID, replace = false) => {
      const url = new URL(window.location.href);
      if (teamID) {
        url.searchParams.set('team', teamID);
      }
      if (periodID) {
        url.searchParams.set('period_id', periodID);
      }
      if (replace) {
        window.history.replaceState({}, '', url);
      } else {
        window.history.pushState({}, '', url);
      }
    };

    const renderTree = (nodes, selectedTeamID, teamSummaryMap = new Map()) => {
      const renderNode = (node) => {
        const hasChildren = Array.isArray(node.children) && node.children.length > 0;
        const nodeWrap = document.createElement('div');
        nodeWrap.className = 'vstack gap-1';

        const row = document.createElement('div');
        row.className = `team-node-card ${String(node.id) === String(selectedTeamID) ? 'is-selected' : ''}`;

        const header = document.createElement('div');
        header.className = 'd-flex justify-content-between align-items-start gap-2';
        const title = document.createElement('div');
        title.className = 'fw-semibold text-break small';
        title.textContent = node.name;

        const controls = document.createElement('div');
        controls.className = 'd-flex align-items-center gap-1';

        if (hasChildren) {
          const toggle = document.createElement('button');
          toggle.type = 'button';
          toggle.className = 'btn btn-sm btn-outline-secondary px-2 py-0';
          const isExpanded = expandedNodes.has(String(node.id));
          toggle.innerHTML = isExpanded ? '<i class="bi bi-dash"></i>' : '<i class="bi bi-plus"></i>';
          toggle.addEventListener('click', (event) => {
            event.stopPropagation();
            if (expandedNodes.has(String(node.id))) {
              expandedNodes.delete(String(node.id));
            } else {
              expandedNodes.add(String(node.id));
            }
            renderTree(getVisibleHierarchy(teamSummaryMap), selectedTeamID, teamSummaryMap);
          });
          controls.appendChild(toggle);
        }

        header.append(title, controls);
        row.appendChild(header);

        const teamSummary = teamSummaryMap.get(String(node.id));

        const lead = document.createElement('div');
        lead.className = 'small text-muted text-truncate';
        lead.textContent = node.lead || node.lead_name || '—';
        row.appendChild(lead);

        const hasGoals = (typeof node.hasGoals === 'boolean')
          ? node.hasGoals
          : ((teamSummary?.goals_count || 0) > 0);
        const nodeProgress = (typeof node.progress === 'number')
          ? node.progress
          : (typeof teamSummary?.period_progress === 'number' ? teamSummary.period_progress : null);

        if (hasGoals && typeof nodeProgress === 'number') {
          const progressWrap = document.createElement('div');
          progressWrap.className = 'd-flex align-items-center gap-2 mt-1';
          const progress = document.createElement('div');
          progress.className = 'progress flex-grow-1 team-node-progress';
          progress.innerHTML = `<div class="progress-bar" style="width:${Math.max(0, Math.min(100, nodeProgress))}%"></div>`;
          const pct = document.createElement('span');
          pct.className = 'small text-muted';
          pct.textContent = `${nodeProgress}%`;
          progressWrap.append(progress, pct);
          row.appendChild(progressWrap);
        } else {
          const noGoals = document.createElement('div');
          noGoals.className = 'small text-muted mt-1';
          noGoals.textContent = 'No goals';
          row.appendChild(noGoals);
        }

        row.addEventListener('click', async () => {
          const periodID = periodSelect?.value || page.dataset.periodId;
          updateURL(String(node.id), periodID);
          await renderContentForSelection(String(node.id));
          renderTree(getVisibleHierarchy(teamSummaryMap), String(node.id), teamSummaryMap);
        });

        nodeWrap.appendChild(row);

        if (hasChildren && expandedNodes.has(String(node.id))) {
          const childrenWrap = document.createElement('div');
          childrenWrap.className = 'team-node-children vstack gap-1';
          node.children.forEach((child) => {
            childrenWrap.appendChild(renderNode(child));
          });
          nodeWrap.appendChild(childrenWrap);
        }
        return nodeWrap;
      };

      treeEl.innerHTML = '';
      if (!nodes.length) {
        treeEl.innerHTML = '<div class="text-muted">Нет доступных команд.</div>';
        return;
      }
      nodes.forEach((node) => treeEl.appendChild(renderNode(node)));
    };

    const loadTeamOKR = async (teamID) => {
      const periodID = periodSelect?.value || page.dataset.periodId;
      const url = new URL(`/api/v1/teams/${teamID}/okrs`, window.location.origin);
      url.searchParams.set('period_id', periodID);
      return fetchJSON(url.toString());
    };
    const loadTeamOverview = async (teamID) => {
      const periodID = periodSelect?.value || page.dataset.periodId;
      const url = new URL(`/api/v1/teams/${teamID}/overview`, window.location.origin);
      url.searchParams.set('period_id', periodID);
      return fetchJSON(url.toString());
    };

    const renderContentForSelection = async (teamID) => {
      mainEl.innerHTML = '<div class="card"><div class="card-body text-muted">Загрузка данных команды…</div></div>';
      try {
        const hierarchy = getVisibleHierarchy(await loadTeamsSummaryMap());
        const { map } = flattenHierarchyNodes(hierarchy);
        const selectedNode = map.get(String(teamID));
        if (!selectedNode) {
          mainEl.innerHTML = '<div class="card"><div class="card-body text-muted">Команда не найдена в иерархии.</div></div>';
          return;
        }

        const selectedData = await loadTeamOKR(teamID);
        mainEl.innerHTML = '';
        renderTeamGoalSection(selectedData, mainEl, {
          title: `${selectedData.team.type_label} ${selectedData.team.name}`,
          headingTag: 'h2',
          titleClass: 'h4 mb-2',
          showWeightSummary: true,
          showStatusControls: true,
          onStatusUpdated: () => renderContentForSelection(String(teamID)),
        });

        const childNodes = Array.isArray(selectedNode.children) ? selectedNode.children : [];
        if (!childNodes.length) {
          initPopovers();
          return;
        }

        const childrenTitle = document.createElement('h3');
        childrenTitle.className = 'h5 mt-3 mb-2';
        childrenTitle.textContent = 'Дочерние команды';
        mainEl.appendChild(childrenTitle);

        const childContainer = document.createElement('div');
        childContainer.className = 'vstack gap-2';
        mainEl.appendChild(childContainer);

        const overviewData = await loadTeamOverview(teamID);
        renderChildrenOverview(childContainer, overviewData);

        const childResults = await Promise.all(
          childNodes.map(async (child) => {
            try {
              return await loadTeamOKR(child.id);
            } catch (error) {
              return {
                team: { id: child.id, name: child.name, type_label: child.type_label || '' },
                period: selectedData.period,
                goals: [],
                period_progress: 0,
                status_label: '—',
                period_status: '',
              };
            }
          }),
        );

        const childTableWrap = document.createElement('div');
        childTableWrap.className = 'table-responsive';
        const childTable = document.createElement('table');
        childTable.className = 'table table-hover';
        childTable.innerHTML = `
          <thead class="table-light">
            <tr>
              <th>Статус</th>
              <th>Команда</th>
              <th>Прогресс</th>
              <th>Руководитель</th>
              <th>Обновлено</th>
            </tr>
          </thead>`;
        const childTBody = document.createElement('tbody');

        childResults.forEach((childData) => {
          const row = document.createElement('tr');
          const childNodeMeta = childNodes.find((node) => String(node.id) === String(childData.team.id));
          const leadText = childNodeMeta?.lead || childNodeMeta?.lead_name || '—';
          const lastUpdateMeta = getLastGoalUpdateMeta(childData.goals || []);

          const nameCell = document.createElement('td');
          const nameLink = document.createElement('a');
          nameLink.className = 'link-primary';
          nameLink.href = `/teamOkrs?period_id=${childData.period?.id || ''}&team=${childData.team.id}`;
          nameLink.textContent = `${childData.team.type_label} ${childData.team.name}`;
          nameCell.appendChild(nameLink);

          const leadCell = document.createElement('td');
          const leadValue = document.createElement('span');
          leadValue.className = 'text-decoration-underline';
          leadValue.textContent = leadText;
          leadCell.appendChild(leadValue);

          const statusCell = document.createElement('td');
          const statusBadge = document.createElement('span');
          statusBadge.className = 'badge text-bg-light border';
          statusBadge.textContent = childData.status_label || childData.period_status || '—';
          statusCell.appendChild(statusBadge);

          const progressCell = document.createElement('td');
          if (!childData.goals || childData.goals.length === 0) {
            progressCell.textContent = '—';
          } else {
            progressCell.appendChild(renderForecastProgress(childData.progress_meta || {
              actual: childData.period_progress || 0,
              forecast: childData.period_progress || 0,
              status: 'on_track',
            }));
          }

          const updatedCell = document.createElement('td');
          if (!lastUpdateMeta.hasDate) {
            updatedCell.textContent = '—';
          } else {
            const freshness = document.createElement('span');
            freshness.className = lastUpdateMeta.isStale ? 'text-danger' : 'text-success';
            const emoji = lastUpdateMeta.isStale ? '⏰' : '✅';
            freshness.textContent = `${emoji} ${lastUpdateMeta.relativeText}`;
            freshness.title = lastUpdateMeta.absoluteText;
            updatedCell.appendChild(freshness);
          }

          row.append(statusCell, nameCell, progressCell, leadCell, updatedCell);
          childTBody.appendChild(row);
        });

        childTable.appendChild(childTBody);
        childTableWrap.appendChild(childTable);
        childContainer.appendChild(childTableWrap);
        initPopovers();
      } catch (error) {
        mainEl.innerHTML = `<div class="card"><div class="card-body text-danger">${error.message}</div></div>`;
      }
    };

    const bootstrapPage = async () => {
      try {
        const [, periods] = await Promise.all([loadHierarchy(), loadPeriods()]);
        const teamSummaryMap = await loadTeamsSummaryMap();
        const tree = getVisibleHierarchy(teamSummaryMap);
        renderPeriodSelect(periodSelect, periods, page.dataset.periodId || periodSelect?.value);
        const { selected } = resolveSelectedTeamID(tree);
        if (selected) {
          updateURL(selected, periodSelect?.value || page.dataset.periodId, true);
        }
        renderTree(tree, selected, teamSummaryMap);
        await renderContentForSelection(selected);
      } catch (error) {
        treeEl.innerHTML = `<div class="text-danger">${error.message}</div>`;
        mainEl.innerHTML = `<div class="card"><div class="card-body text-danger">${error.message}</div></div>`;
      }
    };

    periodSelect?.addEventListener('change', () => {
      const url = new URL(window.location.href);
      url.searchParams.set('period_id', periodSelect.value);
      window.location.assign(url.toString());
    });

    window.addEventListener('popstate', () => {
      const url = new URL(window.location.href);
      const teamID = url.searchParams.get('team');
      if (!teamID) return;
      loadTeamsSummaryMap().then((teamSummaryMap) => {
        renderTree(getVisibleHierarchy(teamSummaryMap), teamID, teamSummaryMap);
      });
      renderContentForSelection(teamID);
    });

    bootstrapPage();
  };

  const initTeamOKRPage = () => {
    const page = document.querySelector('[data-page="team-okr"]');
    if (!page) return;
    const actionsEl = page.querySelector('[data-okr-actions]');
    const goalsEl = page.querySelector('[data-okr-goals]');
    const breadcrumbsEl = page.querySelector('[data-okr-breadcrumbs]');
    const teamID = page.dataset.teamId;
    const periodID = page.dataset.periodId;

    const loadTeamAncestry = async (selectedTeamID) => {
      const chain = [];
      const seen = new Set();
      let cursor = selectedTeamID;
      while (cursor && !seen.has(String(cursor))) {
        seen.add(String(cursor));
        const team = await fetchJSON(`/api/v1/teams/${cursor}`);
        chain.unshift(team);
        cursor = team.parent_id;
      }
      return chain;
    };

    const renderTeamBreadcrumbs = async () => {
      if (!breadcrumbsEl) return;
      const chain = await loadTeamAncestry(teamID);
      const fragment = document.createDocumentFragment();
      const rootCrumb = document.createElement('li');
      rootCrumb.className = 'breadcrumb-item';
      const rootLink = document.createElement('a');
      rootLink.href = '/teamOkrs';
      rootLink.textContent = 'OKR команд';
      rootCrumb.appendChild(rootLink);
      fragment.appendChild(rootCrumb);

      chain.forEach((node, index) => {
        const item = document.createElement('li');
        const isLast = index === chain.length - 1;
        item.className = `breadcrumb-item${isLast ? ' active' : ''}`;
        if (isLast) {
          item.setAttribute('aria-current', 'page');
          item.textContent = `${node.type_label} ${node.name}`;
        } else {
          const link = document.createElement('a');
          link.href = `/teamOkrs?period_id=${periodID}&team=${node.id}`;
          link.textContent = `${node.type_label} ${node.name}`;
          item.appendChild(link);
        }
        fragment.appendChild(item);
      });
      breadcrumbsEl.replaceChildren(fragment);
    };

    const load = async () => {
      const url = new URL(`/api/v1/teams/${teamID}/okrs`, window.location.origin);
      url.searchParams.set('period_id', periodID);
      const payload = await fetchJSON(url.toString());
      renderOKRPage(payload, goalsEl, actionsEl);
      await renderTeamBreadcrumbs();
    };

    reloadTeamOKR = load;

    load().catch((error) => {
      goalsEl.innerHTML = `<div class="text-danger">${error.message}</div>`;
    });
  };

  document.addEventListener('DOMContentLoaded', () => {
    initTeamsPage();
    initTeamOKRPage();
  });

  const initPopovers = () => {
    if (!window.bootstrap) return;
    document.querySelectorAll('[data-popover-content]').forEach((el) => {
      const targetSelector = el.getAttribute('data-popover-content');
      if (!targetSelector) return;
      const contentEl = document.querySelector(targetSelector);
      if (!contentEl) return;
      const isHoverable = el.getAttribute('data-popover-trigger') === 'hoverable';
      const popover = bootstrap.Popover.getOrCreateInstance(el, {
        trigger: isHoverable ? 'manual' : 'click',
        content: contentEl.innerHTML,
        html: true,
        placement: 'bottom',
        customClass: 'okr-popover',
        container: 'body',
      });
      if (!isHoverable) return;
      if (el.dataset.hoverablePopoverBound === 'true') return;
      el.dataset.hoverablePopoverBound = 'true';

      let hideTimeout;
      let isPointerOnTrigger = false;
      let isPointerOnPopover = false;
      let isFocusOnTrigger = false;
      let isFocusOnPopover = false;

      const hideIfNotHovered = () => {
        const tip = popover.getTipElement();
        const isTipHovered = Boolean(tip && tip.matches(':hover'));
        const active = document.activeElement;
        const isTipFocused = Boolean(tip && active && tip.contains(active));
        if (isPointerOnTrigger || isPointerOnPopover || isTipHovered || isFocusOnTrigger || isFocusOnPopover || isTipFocused) return;
        popover.hide();
      };

      const scheduleHide = () => {
        cancelHide();
        hideTimeout = window.setTimeout(hideIfNotHovered, 220);
      };
      const cancelHide = () => {
        if (hideTimeout) {
          window.clearTimeout(hideTimeout);
          hideTimeout = null;
        }
      };

      el.addEventListener('mouseenter', () => {
        isPointerOnTrigger = true;
        cancelHide();
        popover.show();
      });
      el.addEventListener('mouseleave', () => {
        isPointerOnTrigger = false;
        scheduleHide();
      });
      el.addEventListener('focusin', () => {
        isFocusOnTrigger = true;
        cancelHide();
        popover.show();
      });
      el.addEventListener('focusout', (event) => {
        const next = event.relatedTarget;
        if (next && el.contains(next)) return;
        isFocusOnTrigger = false;
        scheduleHide();
      });

      el.addEventListener('shown.bs.popover', () => {
        const tip = popover.getTipElement();
        if (!tip) return;
        if (tip.dataset.hoverableBound === 'true') return;
        tip.dataset.hoverableBound = 'true';

        tip.addEventListener('mouseenter', () => {
          isPointerOnPopover = true;
          cancelHide();
        });
        tip.addEventListener('mouseleave', () => {
          isPointerOnPopover = false;
          scheduleHide();
        });
        tip.addEventListener('focusin', () => {
          isFocusOnPopover = true;
          cancelHide();
        });
        tip.addEventListener('focusout', (event) => {
          const next = event.relatedTarget;
          if (next && tip.contains(next)) return;
          isFocusOnPopover = false;
          scheduleHide();
        });
      });

      el.addEventListener('hidden.bs.popover', () => {
        isPointerOnPopover = false;
        isFocusOnTrigger = false;
        isFocusOnPopover = false;
      });
    });
  };
})();
