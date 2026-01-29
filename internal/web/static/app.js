(() => {
  const state = {
    hierarchy: null,
    teamOKR: null,
  };

  const jsonHeaders = { 'Content-Type': 'application/json; charset=utf-8' };
  const goalPriorityOptions = ['P0', 'P1', 'P2', 'P3'];
  const goalWorkOptions = ['Discovery', 'Delivery'];
  const goalFocusOptions = ['PROFITABILITY', 'STABILITY', 'SPEED_EFFICIENCY', 'TECH_INDEPENDENCE'];
  const lockedQuarterStatuses = ['validated', 'closed'];

  const isQuarterLocked = () => lockedQuarterStatuses.includes(state.teamOKR?.quarter_status);

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

  const renderTeamsList = (data, tbody, year, quarter) => {
    tbody.innerHTML = '';
    if (!data.items || data.items.length === 0) {
      const row = document.createElement('tr');
      const cell = document.createElement('td');
      cell.colSpan = 5;
      cell.className = 'text-muted';
      cell.textContent = 'Нет данных';
      row.appendChild(cell);
      tbody.appendChild(row);
      return;
    }
    data.items.forEach((team) => {
      const row = document.createElement('tr');
      row.appendChild(renderTeamCell(team, year, quarter));
      row.appendChild(renderQuarterProgressCell(team));
      row.appendChild(renderGoalsCell(team));
      row.appendChild(renderStatusCell(team));
      row.appendChild(renderActionsCell(team));
      tbody.appendChild(row);
    });
  };

  const renderOKRPage = (data, summaryEl, goalsEl, actionsEl) => {
    state.teamOKR = data;
    renderSummary(data, summaryEl);
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
  };

  const renderGoalCard = (goal) => {
    const card = document.createElement('div');
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
    if (isQuarterLocked()) {
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

    const krWrap = renderKRTable(goal);

    const actions = document.createElement('div');
    actions.className = 'mt-3';
    if (!isQuarterLocked()) {
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

  const renderKRTable = (goal) => {
    const wrapper = document.createElement('div');
    if (!goal.key_results || goal.key_results.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'text-muted';
      empty.textContent = 'Ключевые результаты не заданы.';
      wrapper.appendChild(empty);
      return wrapper;
    }

    const table = document.createElement('table');
    table.className = 'table table-sm align-middle mb-0';

    const head = document.createElement('thead');
    head.innerHTML = `
      <tr>
        <th>Вес</th>
        <th>Название</th>
        <th>Факт (%)</th>
        <th class="text-end">Действия</th>
      </tr>`;
    table.appendChild(head);

    const body = document.createElement('tbody');
    goal.key_results.forEach((kr) => {
      const { row, detailRow } = renderKRRow(kr);
      body.appendChild(row);
      body.appendChild(detailRow);
    });
    table.appendChild(body);
    wrapper.appendChild(table);
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
    const titleWrap = document.createElement('div');
    titleWrap.className = 'd-flex flex-column align-items-start';
    if (isQuarterLocked()) {
      const title = document.createElement('span');
      title.className = 'fw-semibold';
      title.textContent = kr.title;
      titleWrap.appendChild(title);
    } else {
      const title = document.createElement('button');
      title.type = 'button';
      title.className = 'btn btn-link p-0 fw-semibold';
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
    commentLabel.textContent = 'Комментарий';
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
      const input = document.createElement('input');
      input.type = 'number';
      input.step = 'any';
      input.className = 'form-control';
      const meta = kr.measure.percent || kr.measure.linear;
      input.value = meta?.current_value ?? 0;
      const label = document.createElement('label');
      label.className = 'form-label';
      label.textContent = 'Текущее значение';
      label.appendChild(input);
      form.appendChild(label);

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
            body: JSON.stringify({ current_value: parseFloat(input.value) }),
          });
          const comment = commentInput.value.trim();
          if (comment) {
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
          const comment = commentInput.value.trim();
          if (comment) {
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
          const comment = commentInput.value.trim();
          if (comment) {
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

  const renderSummary = (data, summaryEl) => {
    summaryEl.innerHTML = '';
    const title = document.createElement('h2');
    title.className = 'h5';
    title.textContent = 'Сводка квартала';

    const progressRow = document.createElement('div');
    progressRow.className = 'mb-3';

    const progressHeader = document.createElement('div');
    progressHeader.className = 'd-flex justify-content-between';
    const progressLabel = document.createElement('span');
    progressLabel.className = 'text-muted';
    progressLabel.textContent = 'Прогресс';
    const progressValue = document.createElement('strong');
    progressValue.textContent = `${data.quarter_progress}%`;
    progressHeader.append(progressLabel, progressValue);

    const progressBar = document.createElement('div');
    progressBar.className = 'progress';
    progressBar.setAttribute('role', 'progressbar');
    progressBar.setAttribute('aria-valuenow', data.quarter_progress);
    progressBar.setAttribute('aria-valuemin', '0');
    progressBar.setAttribute('aria-valuemax', '100');
    const progressFill = document.createElement('div');
    progressFill.className = 'progress-bar';
    progressFill.style.width = `${data.quarter_progress}%`;
    progressBar.appendChild(progressFill);

    progressRow.append(progressHeader, progressBar);

    const counts = document.createElement('div');
    counts.className = 'd-flex justify-content-between';
    counts.innerHTML = `<span class="text-muted">Целей</span><span class="fw-semibold">${data.goals_count}</span>`;

    const weight = document.createElement('div');
    weight.className = 'd-flex justify-content-between';
    weight.innerHTML = `<span class="text-muted">Суммарный вес</span><span class="fw-semibold">${data.goals_weight}</span>`;

    const status = document.createElement('div');
    status.className = 'mt-3';
    const statusLabel = document.createElement('h3');
    statusLabel.className = 'h6 mb-2';
    statusLabel.textContent = 'Статус квартала';

    const statusSelect = document.createElement('select');
    statusSelect.className = 'form-select';
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
      if (option.value === data.quarter_status) {
        opt.selected = true;
      }
      statusSelect.appendChild(opt);
    });
    statusSelect.addEventListener('change', () => updateQuarterStatus(statusSelect.value));

    status.append(statusLabel, statusSelect);

    summaryEl.append(title, progressRow, counts, weight, status);
  };

  const renderOKRActions = (data, actionsEl) => {
    actionsEl.innerHTML = '';
    const wrapper = document.createElement('div');
    wrapper.className = 'd-flex flex-wrap gap-2';

    if (!isQuarterLocked()) {
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

    if (!isQuarterLocked()) {
      menu.appendChild(buildMenuButton('Редактировать', () => openGoalModal(goal)));
      menu.appendChild(buildMenuButton('Шарить', () => openShareGoalModal(goal)));
    }
    menu.appendChild(buildMenuButton('Переместить вверх', () => moveGoal(goal.id, 'move-up')));
    menu.appendChild(buildMenuButton('Переместить вниз', () => moveGoal(goal.id, 'move-down')));
    if (!isQuarterLocked()) {
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

    if (!isQuarterLocked()) {
      menu.appendChild(buildMenuButton('Редактировать', () => openKRModal(kr)));
    }
    menu.appendChild(buildMenuButton('Переместить вверх', () => moveKeyResult(kr.id, 'move-up')));
    menu.appendChild(buildMenuButton('Переместить вниз', () => moveKeyResult(kr.id, 'move-down')));
    if (!isQuarterLocked()) {
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
    return `/teams/${state.teamOKR.team.id}/okr?year=${state.teamOKR.year}&quarter=${state.teamOKR.quarter}`;
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
    title.textContent = 'Комментарии';
    const list = document.createElement('ul');
    list.className = 'list-unstyled mb-0';
    const item = document.createElement('li');
    item.className = 'small';
    item.textContent = latestComment.text;
    list.appendChild(item);
    container.append(title, list);
    return container;
  };

  const sumKRWeights = (krs) => krs.reduce((sum, kr) => sum + (kr.weight || 0), 0);

  function getLatestComment(kr) {
    if (!kr.comments || kr.comments.length === 0) return null;
    return kr.comments[kr.comments.length - 1];
  }

  const renderSharedGoalBadge = (goal) => {
    const wrapper = document.createElement('div');
    wrapper.className = 'd-flex align-items-center gap-1';
    const badge = document.createElement('span');
    badge.className = 'badge text-bg-info share-goal-badge';
    badge.textContent = 'Общая';
    wrapper.appendChild(badge);
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
          `<li><a class="text-decoration-none" href="/teams/${team.id}/okr?year=${state.teamOKR?.year ?? ''}&quarter=${state.teamOKR?.quarter ?? ''}"><span class="text-muted">${team.type_label}</span> ${escapeHTML(team.name)}</a></li>`,
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

  const updateQuarterStatus = async (status) => {
    if (!state.teamOKR) return;
    try {
      await postFormData(`/api/v1/teams/${state.teamOKR.team.id}/status`, {
        year: state.teamOKR.year,
        quarter: state.teamOKR.quarter,
        status,
      });
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
    const includeQuarter = options.includeQuarter === true;
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
        ${includeQuarter ? `<input type="hidden" name="year" value="${goal.year ?? ''}" />` : ''}
        ${includeQuarter ? `<input type="hidden" name="quarter" value="${goal.quarter ?? ''}" />` : ''}
        <button class="btn btn-primary" type="submit">${submitLabel}</button>
      </form>`;
    const modalEl = openModal(titleText, body);
    const form = modalEl.querySelector('[data-goal-edit-form]');
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      await submitFormXHR(form);
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
      year: data.year,
      quarter: data.quarter,
    };
    openGoalModal(emptyGoal, {
      action: `/teams/${data.team.id}/okr`,
      titleText: 'Добавить цель',
      submitLabel: 'Создать',
      includeQuarter: true,
    });
  };

  const openShareGoalModal = async (goal) => {
    const hierarchy = state.hierarchy || (await fetchJSON('/api/v1/hierarchy')).items || [];
    state.hierarchy = hierarchy;
    const options = flattenHierarchyOptions(hierarchy);
    const buildRow = () => `
      <div class="row g-2 align-items-end" data-share-row>
        <div class="col-md-7">
          <label class="form-label">Команда</label>
          <select class="form-select" name="team_id">
            ${options}
          </select>
        </div>
        <div class="col-md-3">
          <label class="form-label">Вес</label>
          <input class="form-control" name="weight" type="number" value="0" />
        </div>
      </div>`;
    const body = `
      <form class="vstack gap-3" data-share-goal-form>
        <div data-share-list class="vstack gap-2">
          ${buildRow()}
        </div>
        <button class="btn btn-outline-secondary btn-sm" type="button" data-add-share>Добавить команду</button>
        <button class="btn btn-primary" type="submit">Сохранить</button>
      </form>`;
    const modalEl = openModal('Шарить цель', body);
    const form = modalEl.querySelector('[data-share-goal-form]');
    const addButton = modalEl.querySelector('[data-add-share]');
    addButton.addEventListener('click', () => {
      const list = modalEl.querySelector('[data-share-list]');
      list.insertAdjacentHTML('beforeend', buildRow());
    });
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      const rows = Array.from(form.querySelectorAll('[data-share-row]'));
      const targets = rows
        .map((row) => {
          const teamId = Number(row.querySelector('select[name="team_id"]').value);
          const weight = Number(row.querySelector('input[name="weight"]').value);
          return { team_id: teamId, weight };
        })
        .filter((target) => target.team_id);
      if (targets.length === 0) return;
      await fetchJSON(`/api/v1/goals/${goal.id}/share`, {
        method: 'POST',
        headers: jsonHeaders,
        body: JSON.stringify({ targets }),
      });
      await reloadTeamOKR();
      bootstrap.Modal.getInstance(modalEl)?.hide();
    });
  };

  const openKRModalWithAction = (kr, action, titleText) => {
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

  const renderTeamCell = (team, year, quarter) => {
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
    link.href = `/teams/${team.id}/okr?year=${year}&quarter=${quarter}`;
    link.textContent = team.name;

    wrapper.append(badge, link);
    cell.appendChild(wrapper);
    return cell;
  };

  const renderQuarterProgressCell = (team) => {
    const cell = document.createElement('td');
    cell.className = 'teams-col-progress';
    const wrapper = document.createElement('div');
    wrapper.className = 'd-flex align-items-center gap-2';
    const progressBar = document.createElement('div');
    progressBar.className = 'progress flex-grow-1';
    progressBar.setAttribute('role', 'progressbar');
    progressBar.setAttribute('aria-valuenow', team.quarter_progress);
    progressBar.setAttribute('aria-valuemin', '0');
    progressBar.setAttribute('aria-valuemax', '100');
    const fill = document.createElement('div');
    fill.className = 'progress-bar';
    fill.style.width = `${team.quarter_progress}%`;
    progressBar.appendChild(fill);
    const value = document.createElement('span');
    value.className = 'fw-semibold';
    value.textContent = `${team.quarter_progress}%`;
    wrapper.append(progressBar, value);
    cell.appendChild(wrapper);
    return cell;
  };

  const renderGoalsCell = (team) => {
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
    table.className = 'table table-sm align-middle mb-0 table-transparent teams-goals-table';
    const tbody = document.createElement('tbody');

    team.goals.forEach((goal) => {
      const row = document.createElement('tr');
      const weight = document.createElement('td');
      weight.className = 'teams-goals-col-weight';
      const weightBadge = document.createElement('span');
      weightBadge.className = 'badge text-bg-light border';
      weightBadge.textContent = goal.weight;
      weight.appendChild(weightBadge);

      const title = document.createElement('td');
      title.className = 'teams-goals-col-title text-break';
      const titleWrapper = document.createElement('div');
      titleWrapper.className = 'd-flex align-items-center gap-2';
      const titleText = document.createElement('span');
      titleText.textContent = goal.title;
      titleWrapper.appendChild(titleText);
      if (goal.share_teams && goal.share_teams.length > 1) {
        const share = document.createElement('span');
        share.className = 'text-muted small';
        share.textContent = `\u2194 ${goal.share_teams.map((team) => team.name).join(', ')}`;
        titleWrapper.appendChild(share);
      }
      title.appendChild(titleWrapper);

      const progress = document.createElement('td');
      progress.className = 'teams-goals-col-progress';
      const progressBadge = document.createElement('span');
      progressBadge.className = 'badge text-bg-light border';
      progressBadge.textContent = `${goal.progress}%`;
      progress.appendChild(progressBadge);

      row.append(weight, title, progress);
      tbody.appendChild(row);
    });

    table.appendChild(tbody);
    cell.appendChild(table);

    const weightSummary = document.createElement('div');
    weightSummary.className = 'mt-2';
    const weightBadge = document.createElement('span');
    weightBadge.className = `badge ${team.goals_weight !== 100 ? 'text-bg-danger' : 'text-bg-light border'}`;
    weightBadge.textContent = `Сумма целей ${team.goals_weight}`;
    weightSummary.appendChild(weightBadge);
    cell.appendChild(weightSummary);

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

  const renderActionsCell = (team) => {
    const cell = document.createElement('td');
    cell.className = 'teams-col-actions text-end';
    const wrapper = document.createElement('div');
    wrapper.className = 'd-inline-flex gap-2';

    const edit = document.createElement('a');
    edit.className = 'btn btn-outline-secondary btn-sm';
    edit.href = `/teams/${team.id}/edit`;
    edit.textContent = 'Редактировать';

    const form = document.createElement('form');
    form.method = 'post';
    form.action = `/teams/${team.id}/delete`;
    form.className = 'm-0';
    const button = document.createElement('button');
    button.type = 'submit';
    button.className = 'btn btn-outline-danger btn-sm';
    button.textContent = 'Удалить';
    form.appendChild(button);

    wrapper.append(edit, form);
    cell.appendChild(wrapper);
    return cell;
  };

  const createOption = (value, label) => {
    const option = document.createElement('option');
    option.value = value;
    option.textContent = label;
    return option;
  };

  let reloadTeamOKR = async () => {};

  const initTeamsPage = () => {
    const page = document.querySelector('[data-page="teams"]');
    if (!page) return;
    const filtersForm = document.querySelector('[data-teams-filters]');
    const quarterSelect = filtersForm.querySelector('[data-quarter-select]');
    const hierarchySelect = filtersForm.querySelector('[data-hierarchy-select]');
    const tbody = document.querySelector('[data-teams-body]');

    const loadHierarchy = async () => {
      if (state.hierarchy) return state.hierarchy;
      const payload = await fetchJSON('/api/v1/hierarchy');
      state.hierarchy = payload.items || [];
      return state.hierarchy;
    };

    const loadTeams = async () => {
      const orgId = hierarchySelect.value !== 'ALL' ? hierarchySelect.value : '';
      const url = new URL('/api/v1/teams', window.location.origin);
      url.searchParams.set('quarter', quarterSelect.value);
      if (orgId) {
        url.searchParams.set('org_id', orgId);
      }
      const payload = await fetchJSON(url.toString());
      const [year, quarter] = quarterSelect.value.split('-');
      renderTeamsList(payload, tbody, year, quarter);
    };

    const selectedTeam = page.dataset.selectedTeam || 'ALL';

    loadHierarchy()
      .then((tree) => {
        renderHierarchySelect(tree, hierarchySelect, selectedTeam);
        return loadTeams();
      })
      .catch((error) => {
        tbody.innerHTML = `<tr><td colspan="5" class="text-danger">${error.message}</td></tr>`;
      });

    filtersForm.addEventListener('submit', (event) => {
      event.preventDefault();
      loadTeams();
    });
  };

  const initTeamOKRPage = () => {
    const page = document.querySelector('[data-page="team-okr"]');
    if (!page) return;
    const summaryEl = page.querySelector('[data-okr-summary]');
    const actionsEl = page.querySelector('[data-okr-actions]');
    const goalsEl = page.querySelector('[data-okr-goals]');
    const teamID = page.dataset.teamId;
    const year = page.dataset.year;
    const quarter = page.dataset.quarter;

    const load = async () => {
      const url = new URL(`/api/v1/teams/${teamID}/okrs`, window.location.origin);
      url.searchParams.set('quarter', `${year}-${quarter}`);
      const payload = await fetchJSON(url.toString());
      renderOKRPage(payload, summaryEl, goalsEl, actionsEl);
    };

    reloadTeamOKR = load;

    load().catch((error) => {
      summaryEl.innerHTML = `<p class="text-danger mb-0">${error.message}</p>`;
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
      bootstrap.Popover.getOrCreateInstance(el, {
        trigger: el.getAttribute('data-popover-trigger') === 'hoverable' ? 'hover' : 'click',
        content: contentEl.innerHTML,
        html: true,
        placement: 'bottom',
        customClass: 'okr-popover',
      });
    });
  };
})();
