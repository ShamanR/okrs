# Domain Model

## Сущности

### Team

**Поля:**

- id
- name
- type: cluster | unit | team
- parent_id
- lead
- description

**Инварианты:**

- команда может иметь родителя;
- дерево не должно содержать циклов;
- удаление команды каскадно удаляет зависимые goal/status records только если это явно допускается продуктом. - Сейчас база уже использует FK/cascade в нескольких местах, это надо учитывать при расширении.

### Period

**Поля:**

- id
- name
- start_date
- end_date
- sort_order

**Инварианты:**

- период имеет уникальное имя;
- start_date <= end_date;
- сортировка периодов управляется явно через sort_order.

### Goal

**Поля:**

- id
- team_id
- period_id
- title
- description
- priority
- weight
- work_type
- focus_type
- owner_text

**Инварианты:**

- goal всегда принадлежит owner team и одному периоду;
- weight в диапазоне 0..100;
- порядок goals внутри (team_id, period_id) управляется sort_order;
- shared goal не меняет identity goal, а лишь добавляет видимость/вес для других команд.

### KeyResult

**Поля:**

- id
- goal_id
- title
- description
- weight
- kind
- sort_order

**Типы:**

- PROJECT
- PERCENT
- LINEAR
- BOOLEAN

**Инварианты:**

- KR принадлежит ровно одной goal;
- weight в диапазоне 0..100;
- порядок KR управляется отдельно внутри goal.

### GoalShare

**Поля:**

- goal_id
- team_id
- weight
- sort_order

**Инварианты:**

- одна и та же goal может быть расшарена на много команд;
- для каждой shared team хранится собственный weight;
- pair (goal_id, team_id) уникален.

### TeamPeriodStatus

**Значения:**

- no_goals
- forming
- in_progress
- validated
- closed

### Производные вычисления

- Goal.progress = взвешенное среднее прогресса KR.
- Period.progress = взвешенное среднее прогресса goals.
- PROJECT KR.progress = сумма весов завершённых этапов, clamp 0..100.
- BOOLEAN KR.progress = 100 или 0.
- PERCENT KR.progress = линейно, либо по checkpoint interpolation.
- LINEAR KR.progress = линейный clamp 0..100.

### Обязательные тест-кейсы на домен

- goal без KR даёт 0%.
- goal с KR суммарного веса 0 даёт 0%.
- project KR с completed stages >100 clamp’ится до 100.
- percent KR без checkpoints считает линейно.
- percent KR с checkpoints интерполирует между соседними точками.
- shared goal показывает разный weight для разных команд без дублирования goal identity.
