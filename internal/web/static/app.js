(() => {
  const state = {
    hierarchy: null,
    teamOKR: null,
  };

  const jsonHeaders = { 'Content-Type': 'application/json; charset=utf-8' };

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

  const renderOKRPage = (data, summaryEl, goalsEl) => {
    state.teamOKR = data;
    renderSummary(data, summaryEl);
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
  };

  const renderGoalCard = (goal) => {
    const card = document.createElement('div');
    card.className = 'card';
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

    const title = document.createElement('button');
    title.type = 'button';
    title.className = 'btn btn-link p-0 fw-semibold';
    title.textContent = goal.title;
    title.addEventListener('click', () => openGoalModal(goal));

    const menu = renderGoalMenu(goal);

    header.append(priority, weight, title, menu);

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

    const krWrap = document.createElement('div');
    krWrap.className = 'vstack gap-2';

    if (goal.key_results && goal.key_results.length) {
      goal.key_results.forEach((kr) => {
        krWrap.appendChild(renderKRRow(kr));
      });
    } else {
      const empty = document.createElement('div');
      empty.className = 'text-muted';
      empty.textContent = 'Ключевые результаты не заданы.';
      krWrap.appendChild(empty);
    }

    const addKRButton = document.createElement('button');
    addKRButton.type = 'button';
    addKRButton.className = 'btn btn-outline-primary btn-sm align-self-start';
    addKRButton.textContent = 'Добавить KR';
    addKRButton.addEventListener('click', () => openKRCreateModal(goal));

    const actions = document.createElement('div');
    actions.className = 'mt-3';
    actions.appendChild(addKRButton);

    body.append(header, description, progressWrap, meta, krWrap, actions);
    card.appendChild(body);
    return card;
  };

  const renderKRRow = (kr) => {
    const wrapper = document.createElement('div');
    wrapper.className = 'border rounded p-3';

    const header = document.createElement('div');
    header.className = 'd-flex flex-wrap align-items-center gap-2';

    const title = document.createElement('button');
    title.type = 'button';
    title.className = 'btn btn-link p-0 fw-semibold';
    title.textContent = kr.title;
    title.addEventListener('click', () => openKRModal(kr));

    const weight = document.createElement('span');
    weight.className = 'badge text-bg-light border';
    weight.textContent = `Вес ${kr.weight}%`;

    const progress = document.createElement('span');
    progress.className = 'badge text-bg-light border';
    progress.textContent = `${kr.progress}%`;

    const menu = renderKRMenu(kr);

    const updateButton = document.createElement('button');
    updateButton.type = 'button';
    updateButton.className = 'btn btn-outline-primary btn-sm ms-auto';
    updateButton.textContent = 'Обновить прогресс';

    header.append(title, weight, progress, menu, updateButton);

    const panel = renderMeasurePanel(kr);
    panel.classList.add('mt-3');
    panel.hidden = true;

    updateButton.addEventListener('click', () => {
      panel.hidden = !panel.hidden;
    });

    const comments = renderKRComments(kr);

    wrapper.append(header, panel, comments);
    return wrapper;
  };

  const renderMeasurePanel = (kr) => {
    const panel = document.createElement('div');
    panel.className = 'border rounded p-3 bg-light';

    const form = document.createElement('form');
    form.className = 'd-flex flex-column gap-2';

    const status = document.createElement('div');
    status.className = 'text-muted small';

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
    status.innerHTML = `<h3 class="h6 mb-2">Статус квартала</h3><span class="badge text-bg-light border">${data.status_label}</span>`;

    summaryEl.append(title, progressRow, counts, weight, status);
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

    menu.appendChild(buildMenuButton('Редактировать', () => openGoalModal(goal)));
    menu.appendChild(buildMenuForm(`/goals/${goal.id}/move-up`, buildReturnFields()));
    menu.appendChild(buildMenuForm(`/goals/${goal.id}/move-down`, buildReturnFields()));
    menu.appendChild(buildMenuForm(`/goals/${goal.id}/delete`, buildReturnFields(), true));

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

    menu.appendChild(buildMenuButton('Редактировать', () => openKRModal(kr)));
    menu.appendChild(buildMenuForm(`/key-results/${kr.id}/move-up`, buildReturnFields()));
    menu.appendChild(buildMenuForm(`/key-results/${kr.id}/move-down`, buildReturnFields()));
    menu.appendChild(buildMenuForm(`/key-results/${kr.id}/delete`, buildReturnFields(), true));

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

  const buildReturnFields = () => {
    if (!state.teamOKR) return [];
    return [
      { name: 'return', value: `/teams/${state.teamOKR.team.id}/okr?year=${state.teamOKR.year}&quarter=${state.teamOKR.quarter}` },
      { name: 'team_id', value: state.teamOKR.team.id },
    ];
  };

  const renderKRComments = (kr) => {
    const container = document.createElement('div');
    container.className = 'mt-2';
    if (!kr.comments || kr.comments.length === 0) {
      return container;
    }
    const title = document.createElement('div');
    title.className = 'small text-muted';
    title.textContent = 'Комментарии';
    const list = document.createElement('ul');
    list.className = 'list-unstyled mb-0';
    kr.comments.forEach((comment) => {
      const item = document.createElement('li');
      item.className = 'small';
      item.textContent = comment.text;
      list.appendChild(item);
    });
    container.append(title, list);
    return container;
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
  };

  const openGoalModal = (goal) => {
    const body = `
      <form method="post" action="/goals/${goal.id}/update" class="vstack gap-3">
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
            ${buildSelect('priority', ['P0', 'P1', 'P2', 'P3'], goal.priority)}
          </div>
          <div class="col-md-4">
            <label class="form-label">Вес</label>
            <input class="form-control" name="weight" type="number" value="${goal.weight}" />
          </div>
          <div class="col-md-4">
            <label class="form-label">Работа</label>
            ${buildSelect('work_type', ['Discovery', 'Delivery'], goal.work_type)}
          </div>
        </div>
        <div class="row g-3">
          <div class="col-md-6">
            <label class="form-label">Фокус</label>
            ${buildSelect('focus_type', ['PROFITABILITY', 'STABILITY', 'SPEED_EFFICIENCY', 'TECH_INDEPENDENCE'], goal.focus_type)}
          </div>
          <div class="col-md-6">
            <label class="form-label">Владелец</label>
            <input class="form-control" name="owner_text" value="${escapeHTML(goal.owner_text || '')}" />
          </div>
        </div>
        <button class="btn btn-primary" type="submit">Сохранить</button>
      </form>`;
    openModal(`Редактировать цель`, body);
  };

  const openKRModal = (kr) => {
    const body = `
      <form method="post" action="/key-results/${kr.id}/update" class="vstack gap-3">
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
            ${buildSelect('kind', ['PERCENT', 'LINEAR', 'BOOLEAN', 'PROJECT'], kr.kind)}
          </div>
        </div>
        <button class="btn btn-primary" type="submit">Сохранить</button>
      </form>`;
    openModal(`Редактировать KR`, body);
  };

  const openKRCreateModal = (goal) => {
    const body = `
      <form method="post" action="/goals/${goal.id}/key-results" class="vstack gap-3">
        <div>
          <label class="form-label">Название</label>
          <input class="form-control" name="title" />
        </div>
        <div>
          <label class="form-label">Описание</label>
          <textarea class="form-control" name="description"></textarea>
        </div>
        <div class="row g-3">
          <div class="col-md-6">
            <label class="form-label">Вес</label>
            <input class="form-control" name="weight" type="number" value="0" />
          </div>
          <div class="col-md-6">
            <label class="form-label">Тип</label>
            ${buildSelect('kind', ['PERCENT', 'LINEAR', 'BOOLEAN', 'PROJECT'], 'PERCENT')}
          </div>
        </div>
        <button class="btn btn-primary" type="submit">Добавить</button>
      </form>`;
    openModal(`Добавить KR`, body);
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
    const goalsEl = page.querySelector('[data-okr-goals]');
    const teamID = page.dataset.teamId;
    const year = page.dataset.year;
    const quarter = page.dataset.quarter;

    const load = async () => {
      const url = new URL(`/api/v1/teams/${teamID}/okrs`, window.location.origin);
      url.searchParams.set('quarter', `${year}-${quarter}`);
      const payload = await fetchJSON(url.toString());
      renderOKRPage(payload, summaryEl, goalsEl);
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
})();
