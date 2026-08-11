/*
 * kisaf — user interface.
 *
 * No build step and no framework: one file of plain JavaScript. Every visible
 * string goes through t() from i18n.js, so adding a language means translating
 * one object rather than hunting through the markup.
 */
'use strict';

// ------------------------------------------------------------------ helpers

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

/** HTML escape. EVERY piece of user data that reaches a template goes through this. */
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => (
  { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
));

const fmtBytes = (n) => {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${units[i]}`;
};

const STEPS = [['year', 31536e6], ['month', 2592e6], ['day', 864e5], ['hour', 36e5], ['minute', 6e4]];

/** Relative time ("3 days ago"), formatted by the browser in the active language. */
function ago(iso) {
  if (!iso || iso.startsWith('0001')) return '—';
  const rtf = new Intl.RelativeTimeFormat(currentLang, { numeric: 'auto' });
  const diff = new Date(iso) - Date.now();
  for (const [unit, ms] of STEPS) {
    if (Math.abs(diff) >= ms) return rtf.format(Math.round(diff / ms), unit);
  }
  return rtf.format(0, 'minute');
}

const fmtDate = (iso) => (!iso || iso.startsWith('0001') ? '—'
  : new Date(iso).toLocaleString(currentLang, { dateStyle: 'medium', timeStyle: 'short' }));

/**
 * Shortens long paths through the middle: "D:\…\Projects\site".
 *
 * The trailing folder is the informative part, and clipping from the left in
 * CSS drags in bidirectional-text problems — so the shortening happens here.
 */
function shortPath(p) {
  const sep = p.includes('\\') ? '\\' : '/';
  const parts = p.split(/[\\/]/).filter(Boolean);
  if (parts.length <= 3) return p;
  const head = p.startsWith('/') ? '' : parts[0];
  return `${head}${sep}…${sep}${parts.slice(-2).join(sep)}`;
}

function toast(message, kind = '') {
  const node = document.createElement('div');
  node.className = `toast ${kind}`;
  node.textContent = message;
  $('#toasts').append(node);
  setTimeout(() => {
    node.style.opacity = '0';
    node.style.transition = 'opacity .3s';
    setTimeout(() => node.remove(), 300);
  }, kind === 'err' ? 6000 : 3000);
}

/** Shows a failure in the active language, falling back to the server's text. */
const fail = (err) => toast(tError(err), 'err');

function debounce(fn, ms) {
  let timer;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}

// ---------------------------------------------------------------------- API

/** ApiError carries the server's translation code and its arguments. */
class ApiError extends Error {
  constructor(data, status) {
    super(data.error || `HTTP ${status}`);
    this.code = data.code;
    this.args = data.args || [];
    this.status = status;
  }
}

async function api(path, { method = 'GET', body } = {}) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data = {};
  if (text) {
    try { data = JSON.parse(text); } catch { data = { error: text.slice(0, 300) }; }
  }
  if (!res.ok) throw new ApiError(data, res.status);
  return data;
}

// -------------------------------------------------------------------- state

const PRIORITY_KEYS = { high: 'priority.high', normal: 'priority.normal', low: 'priority.low' };

const S = {
  projects: [], settings: {}, tags: [], editors: [], server: {},
  summaries: {},                 // project id -> { branch, dirty, ahead, behind }
  selectedId: null,
  query: '', filter: 'all', activeTag: null,
  tab: 'todos',
  todoFilter: 'open',            // open | all | done
  tree: {}, treeOpen: new Set(),
};

function selected() {
  return S.projects.find((p) => p.id === S.selectedId) || null;
}

/**
 * Writes a server response over the matching row in the list.
 *
 * Finding it by id is essential: every call that refreshes state replaces the
 * S.projects array, so a reference held from before can go stale.
 */
function mergeProject(project) {
  if (!project) return;
  const idx = S.projects.findIndex((p) => p.id === project.id);
  if (idx >= 0) S.projects[idx] = { ...S.projects[idx], ...project };
}

const todoStats = (p) => {
  const todos = p.todos || [];
  return {
    total: todos.length,
    done: todos.filter((x) => x.done).length,
    high: todos.filter((x) => !x.done && x.priority === 'high').length,
  };
};

/** Projects that survive the search box and the active filters. */
function visibleProjects() {
  const q = S.query.trim().toLowerCase();
  return S.projects.filter((p) => {
    if (S.filter === 'archived') { if (!p.archived) return false; }
    else if (p.archived) return false;

    if (S.filter === 'favorite' && !p.favorite) return false;
    if (S.filter === 'dirty' && !S.summaries[p.id]?.dirty) return false;
    if (S.filter === 'todo') {
      const st = todoStats(p);
      if (st.total - st.done === 0) return false;
    }
    if (S.activeTag && !p.tags?.some((x) => x.toLowerCase() === S.activeTag.toLowerCase())) return false;

    if (!q) return true;
    const haystack = [
      p.name, p.path, p.description, p.notes,
      ...(p.tags || []),
      ...(p.todos || []).map((x) => x.text),
    ].join(' ').toLowerCase();
    return q.split(/\s+/).every((term) => haystack.includes(term));
  });
}

// ---------------------------------------------------------------- navigation

function showList() {
  S.selectedId = null;
  $('#view-list').hidden = false;
  $('#view-detail').hidden = true;
  renderList();
}

function showDetail(id) {
  S.selectedId = id;
  S.tab = 'todos';
  $('#view-list').hidden = true;
  $('#view-detail').hidden = false;
  renderDetail();
  $('#view-detail').scrollTop = 0;
}

// -------------------------------------------------------------- project list

function renderList() {
  const list = visibleProjects();
  const grid = $('#project-list');

  grid.classList.toggle('is-list', S.settings.view === 'list');
  $$('#view-toggle button').forEach((b) => {
    b.classList.toggle('is-active', b.dataset.view === (S.settings.view || 'grid'));
  });

  const nothingAtAll = S.projects.length === 0;
  $('#empty-state').hidden = !nothingAtAll;
  grid.hidden = nothingAtAll;

  grid.innerHTML = list.length
    ? list.map(cardHTML).join('')
    : `<p class="center-note">${esc(t('list.noMatch'))}</p>`;

  const total = S.projects.filter((p) => !p.archived).length;
  $('#result-count').textContent = nothingAtAll ? '' : t('list.count', list.length, total);

  const tagbar = $('#tagbar');
  tagbar.hidden = S.tags.length === 0;
  tagbar.innerHTML = S.tags.map((tag) => `
    <button class="chip ${S.activeTag === tag.name ? 'is-active' : ''}" data-tag="${esc(tag.name)}">
      ${esc(tag.name)} <span class="muted">${tag.count}</span>
    </button>`).join('');
}

function cardHTML(p) {
  const g = S.summaries[p.id];
  const st = todoStats(p);
  const badges = [];

  if (g?.isRepo) {
    badges.push(`<span class="badge badge-branch" title="${esc(g.branch || '')}">⑂ <span>${esc(g.branch || '—')}</span></span>`);
    badges.push(g.dirty
      ? `<span class="badge badge-dirty"><i class="dot"></i>${g.changes}</span>`
      : `<span class="badge badge-clean"><i class="dot"></i>${esc(t('card.clean'))}</span>`);
    if (g.ahead) badges.push(`<span class="badge badge-ahead">↑${g.ahead}</span>`);
    if (g.behind) badges.push(`<span class="badge badge-ahead">↓${g.behind}</span>`);
  } else if (g) {
    badges.push(`<span class="badge">${esc(t('card.noGit'))}</span>`);
  }
  if (st.high) badges.push(`<span class="badge badge-urgent">${esc(t('card.urgent', st.high))}</span>`);
  for (const tag of (p.tags || []).slice(0, 4)) {
    badges.push(`<span class="badge badge-tag">${esc(tag)}</span>`);
  }

  const pct = st.total ? Math.round((st.done / st.total) * 100) : 0;
  const todoRow = st.total ? `
    <div class="pcard-todo" title="${esc(t('card.tasksDone', st.done, st.total))}">
      <div class="bar"><span style="width:${pct}%"></span></div>
      <span>${esc(t('card.tasks', st.done, st.total))}</span>
    </div>` : '';

  return `
    <article class="pcard ${p.archived ? 'is-archived' : ''}" data-id="${esc(p.id)}" tabindex="0" role="button">
      <div class="pcard-top">
        <span class="pcard-name" title="${esc(p.name)}">${esc(p.name)}</span>
        <button class="pcard-star ${p.favorite ? 'on' : ''}" data-quick="favorite"
                title="${esc(t('card.favorite'))}" aria-label="${esc(t('card.favorite'))}"
        >${p.favorite ? '★' : '☆'}</button>
      </div>
      ${p.description ? `<p class="pcard-desc">${esc(p.description)}</p>` : ''}
      <div class="pcard-path" title="${esc(p.path)}">${esc(shortPath(p.path))}</div>
      ${badges.length ? `<div class="pcard-badges">${badges.join('')}</div>` : ''}
      ${todoRow}
      <div class="pcard-actions">
        <button class="btn btn-sm" data-quick="editor">${esc(t('card.editor'))}</button>
        <button class="btn btn-sm" data-quick="reveal">${esc(t('card.explorer'))}</button>
        <button class="btn btn-sm" data-quick="terminal">${esc(t('card.terminal'))}</button>
      </div>
    </article>`;
}

/** Fetches every badge in one call; far cheaper than one request per project. */
let lastSummaryAt = 0;
async function loadSummaries(force = true) {
  if (!force && Date.now() - lastSummaryAt < 10000) return;
  lastSummaryAt = Date.now();
  try {
    const { summaries } = await api('/api/git/summary');
    S.summaries = summaries || {};
    if (!S.selectedId) renderList();
  } catch { /* badges are not critical; fail quietly */ }
}

// -------------------------------------------------------------------- detail

function renderDetail() {
  const p = selected();
  if (!p) { showList(); return; }

  const st = todoStats(p);
  const open = st.total - st.done;

  const editorOptions = S.editors.map((e) => `
    <option value="${esc(e.id)}" ${e.id === (p.editor || S.settings.defaultEditor) ? 'selected' : ''}>
      ${esc(e.name)}
    </option>`).join('');

  $('#view-detail').innerHTML = `
    <div class="detail-head">
      <div class="detail-title">
        <button class="btn btn-sm" data-act="back">${esc(t('action.back'))}</button>
        <h1>${esc(p.name)}</h1>
        <button class="btn btn-sm" data-act="favorite">${p.favorite ? '★' : '☆'} ${esc(t('detail.favorite'))}</button>
        ${p.archived ? `<span class="badge">${esc(t('card.archived'))}</span>` : ''}
      </div>
      <div class="detail-path">
        <span>${esc(p.path)}</span>
        <button data-act="copy-path" title="${esc(t('detail.copyPath'))}"
                aria-label="${esc(t('detail.copyPath'))}">⧉</button>
      </div>

      <div class="actions">
        <button class="btn btn-primary" data-act="open-editor">${esc(t('detail.openInEditor'))}</button>
        <select class="btn" id="editor-select" title="${esc(t('detail.editorFor'))}" style="padding:7px 10px">
          ${editorOptions || `<option value="">${esc(t('detail.noEditor'))}</option>`}
        </select>
        <button class="btn" data-act="reveal">${esc(t('detail.reveal'))}</button>
        <button class="btn" data-act="folder">${esc(t('detail.openFolder'))}</button>
        <button class="btn" data-act="terminal">${esc(t('detail.terminal'))}</button>
      </div>

      <nav class="tabs">
        ${tabButton('todos', t('tab.tasks'), open ? String(open) : '')}
        ${tabButton('overview', t('tab.overview'))}
        ${tabButton('git', t('tab.git'))}
        ${tabButton('readme', t('tab.readme'))}
        ${tabButton('files', t('tab.files'))}
      </nav>
    </div>
    <div class="pane" id="pane"></div>`;

  renderPane();
}

const tabButton = (id, label, pill = '') =>
  `<button class="tab ${S.tab === id ? 'is-active' : ''}" data-tab="${id}">${esc(label)}${
    pill ? `<span class="pill">${esc(pill)}</span>` : ''}</button>`;

function renderPane() {
  const p = selected();
  if (!p) return;
  const pane = $('#pane');

  switch (S.tab) {
    case 'overview': return renderOverviewPane(pane, p);
    case 'git': return renderGitPane(pane, p);
    case 'readme': return renderReadmePane(pane, p);
    case 'files': return renderFilesPane(pane, p);
    default: return renderTodoPane(pane, p);
  }
}

// --------------------------------------------------------------------- tasks

function renderTodoPane(pane, p) {
  const todos = p.todos || [];
  const st = todoStats(p);
  const pct = st.total ? Math.round((st.done / st.total) * 100) : 0;

  const shown = todos.filter((x) => (
    S.todoFilter === 'all' || (S.todoFilter === 'done' ? x.done : !x.done)
  ));

  const priorityOptions = Object.entries(PRIORITY_KEYS)
    .map(([value, key]) => `<option value="${value}" ${value === 'normal' ? 'selected' : ''}>${esc(t(key))}</option>`)
    .join('');

  pane.innerHTML = `
    <form class="todo-add" id="todo-add">
      <input type="text" id="todo-text" placeholder="${esc(t('todo.placeholder'))}"
             autocomplete="off" maxlength="500">
      <select id="todo-priority" title="${esc(t('todo.priority'))}">${priorityOptions}</select>
      <button class="btn btn-primary" type="submit">${esc(t('todo.add'))}</button>
    </form>

    ${todos.length ? `
      <div class="todo-filters">
        ${todoFilterChip('open', t('todo.filterOpen', st.total - st.done))}
        ${todoFilterChip('done', t('todo.filterDone', st.done))}
        ${todoFilterChip('all', t('todo.filterAll', st.total))}
      </div>` : ''}

    <ul class="todos">${shown.map(todoHTML).join('') || `
      <li class="center-note">${esc(todos.length ? t('todo.noneInFilter') : t('todo.none'))}</li>`}
    </ul>

    ${todos.length ? `
      <div class="todo-foot">
        <div class="bar"><span style="width:${pct}%"></span></div>
        <span>${esc(t('todo.progress', st.done, st.total))}</span>
        ${st.done ? `<button class="btn btn-sm" data-act="clear-done">${esc(t('todo.clearDone'))}</button>` : ''}
      </div>` : ''}`;

  $('#todo-add').addEventListener('submit', addTodo);
}

const todoFilterChip = (value, label) =>
  `<button class="chip ${S.todoFilter === value ? 'is-active' : ''}" data-todo-filter="${value}">${esc(label)}</button>`;

function todoHTML(item) {
  const priorityLabel = t(PRIORITY_KEYS[item.priority] || 'priority.normal');
  return `
    <li class="todo ${item.done ? 'is-done' : ''}" data-todo="${esc(item.id)}">
      <input type="checkbox" ${item.done ? 'checked' : ''} data-todo-act="toggle"
             aria-label="${esc(t('todo.markDone'))}">
      <span class="todo-text" data-todo-act="edit" title="${esc(t('todo.edit'))}">${esc(item.text)}</span>
      <button class="prio prio-${esc(item.priority || 'normal')}" data-todo-act="cycle-priority"
              title="${esc(t('todo.changePriority'))}">${esc(priorityLabel)}</button>
      <span class="todo-tools">
        <button data-todo-act="up" title="${esc(t('todo.moveUp'))}" aria-label="${esc(t('todo.moveUp'))}">↑</button>
        <button data-todo-act="down" title="${esc(t('todo.moveDown'))}" aria-label="${esc(t('todo.moveDown'))}">↓</button>
        <button class="del" data-todo-act="delete" title="${esc(t('todo.delete'))}" aria-label="${esc(t('todo.delete'))}">✕</button>
      </span>
    </li>`;
}

async function addTodo(ev) {
  ev.preventDefault();
  const input = $('#todo-text');
  const text = input.value.trim();
  if (!text) return;

  try {
    const { project } = await api(`/api/projects/${S.selectedId}/todos`, {
      method: 'POST',
      body: { text, priority: $('#todo-priority').value },
    });
    mergeProject(project);
    input.value = '';
    renderDetail();
    // Adding several tasks in a row is common, so keep the focus in the box.
    $('#todo-text').focus();
  } catch (err) { fail(err); }
}

/** Priority cycles through: normal -> high -> low -> normal. */
const NEXT_PRIORITY = { normal: 'high', high: 'low', low: 'normal' };

async function onTodoAction(li, action) {
  const p = selected();
  const todoId = li.dataset.todo;
  const todo = (p.todos || []).find((x) => x.id === todoId);
  if (!todo) return;

  try {
    let project;
    switch (action) {
      case 'toggle':
        ({ project } = await api(`/api/projects/${p.id}/todos/${todoId}`, {
          method: 'PATCH', body: { done: !todo.done },
        }));
        break;

      case 'cycle-priority':
        ({ project } = await api(`/api/projects/${p.id}/todos/${todoId}`, {
          method: 'PATCH', body: { priority: NEXT_PRIORITY[todo.priority] || 'high' },
        }));
        break;

      case 'delete':
        ({ project } = await api(`/api/projects/${p.id}/todos/${todoId}`, { method: 'DELETE' }));
        break;

      case 'up':
      case 'down': {
        const ids = (p.todos || []).map((x) => x.id);
        const i = ids.indexOf(todoId);
        const j = action === 'up' ? i - 1 : i + 1;
        if (j < 0 || j >= ids.length) return;
        [ids[i], ids[j]] = [ids[j], ids[i]];
        ({ project } = await api(`/api/projects/${p.id}/todos/order`, { method: 'PUT', body: { ids } }));
        break;
      }

      case 'edit':
        return startTodoEdit(li, todo);

      default:
        return;
    }
    mergeProject(project);
    renderDetail();
  } catch (err) { fail(err); }
}

/** Edits task text in place: Enter saves, Escape cancels. */
function startTodoEdit(li, todo) {
  const span = $('.todo-text', li);
  if (!span) return;

  const input = document.createElement('input');
  input.type = 'text';
  input.className = 'todo-text-input';
  input.value = todo.text;
  input.maxLength = 500;
  span.replaceWith(input);
  input.focus();
  input.select();

  let settled = false;
  const finish = async (save) => {
    if (settled) return;          // blur and keydown can both fire
    settled = true;
    const text = input.value.trim();
    if (!save || !text || text === todo.text) { renderDetail(); return; }
    try {
      const { project } = await api(`/api/projects/${S.selectedId}/todos/${todo.id}`, {
        method: 'PATCH', body: { text },
      });
      mergeProject(project);
    } catch (err) { fail(err); }
    renderDetail();
  };

  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); finish(true); }
    if (e.key === 'Escape') { e.preventDefault(); finish(false); }
  });
  input.addEventListener('blur', () => finish(true));
}

async function clearDoneTodos() {
  try {
    const { project } = await api(`/api/projects/${S.selectedId}/todos/clear-done`, { method: 'POST' });
    mergeProject(project);
    renderDetail();
    toast(t('todo.cleared'), 'ok');
  } catch (err) { fail(err); }
}

// ------------------------------------------------------------------ overview

function renderOverviewPane(pane, p) {
  pane.innerHTML = `
    <div class="section">
      <h3>${esc(t('overview.description'))}</h3>
      <label class="field">
        <input type="text" id="f-description" value="${esc(p.description)}"
               placeholder="${esc(t('overview.descriptionPlaceholder'))}">
      </label>
    </div>

    <div class="section">
      <h3>${esc(t('overview.notes'))}</h3>
      <label class="field">
        <textarea id="f-notes" placeholder="${esc(t('overview.notesPlaceholder'))}">${esc(p.notes)}</textarea>
      </label>
    </div>

    <div class="section">
      <h3>${esc(t('overview.tags'))}</h3>
      <label class="field">
        <input type="text" id="f-tags" value="${esc((p.tags || []).join(', '))}"
               placeholder="${esc(t('overview.tagsPlaceholder'))}">
      </label>
    </div>

    <div class="section">
      <h3>${esc(t('overview.info'))}</h3>
      <dl class="kv">
        <dt>${esc(t('overview.added'))}</dt><dd>${fmtDate(p.createdAt)}</dd>
        <dt>${esc(t('overview.lastOpened'))}</dt>
        <dd>${p.lastOpenedAt ? `${fmtDate(p.lastOpenedAt)} (${ago(p.lastOpenedAt)})` : '—'}</dd>
        <dt>${esc(t('overview.openCount'))}</dt><dd>${p.openCount || 0}</dd>
        <dt>${esc(t('overview.size'))}</dt>
        <dd id="size-cell"><button class="btn btn-sm" data-act="size">${esc(t('overview.calculate'))}</button></dd>
      </dl>
    </div>

    <div class="section">
      <h3>${esc(t('overview.dangerZone'))}</h3>
      <div class="actions">
        <button class="btn" data-act="archive">${esc(p.archived ? t('overview.unarchive') : t('overview.archive'))}</button>
        <button class="btn btn-danger" data-act="delete">${esc(t('overview.remove'))}</button>
      </div>
      <p class="muted" style="font-size:12.5px;margin:8px 0 0">${esc(t('overview.removeNote'))}</p>
    </div>`;

  wireOverviewInputs(p);
}

/** Saves as you type; a separate "save" button would be pure friction here. */
function wireOverviewInputs(p) {
  const save = debounce(async (patch) => {
    try {
      const { project } = await api(`/api/projects/${p.id}`, { method: 'PATCH', body: patch });
      mergeProject(project);
    } catch (err) { fail(err); }
  }, 500);

  $('#f-description').addEventListener('input', (e) => save({ description: e.target.value }));
  $('#f-notes').addEventListener('input', (e) => save({ notes: e.target.value }));
  $('#f-tags').addEventListener('change', async (e) => {
    const tags = e.target.value.split(',').map((x) => x.trim()).filter(Boolean);
    try {
      await api(`/api/projects/${p.id}`, { method: 'PATCH', body: { tags } });
      await refreshState();   // tag counts changed too
    } catch (err) { fail(err); }
  });
}

// ----------------------------------------------------------------------- git

async function renderGitPane(pane, p) {
  pane.innerHTML = '<p class="center-note"><span class="spinner"></span></p>';
  let git;
  try {
    ({ git } = await api(`/api/projects/${p.id}/git?refresh=1`));
  } catch (err) {
    pane.innerHTML = `<p class="center-note">${esc(t('git.failed', tError(err)))}</p>`;
    return;
  }
  if (selected()?.id !== p.id) return; // the user moved on while we were loading

  if (!git.available) {
    pane.innerHTML = `<p class="center-note">${esc(git.error || t('settings.gitMissing'))}</p>`;
    return;
  }
  if (!git.isRepo) {
    pane.innerHTML = `<p class="center-note">${esc(t('git.notARepo'))}</p>`;
    return;
  }

  const head = [`<span class="badge badge-branch">⑂ <span>${esc(git.branch)}</span></span>`];
  if (git.upstream) head.push(`<span class="badge">→ ${esc(git.upstream)}</span>`);
  if (git.ahead) head.push(`<span class="badge badge-ahead">${esc(t('git.ahead', git.ahead))}</span>`);
  if (git.behind) head.push(`<span class="badge badge-ahead">${esc(t('git.behind', git.behind))}</span>`);
  head.push(git.dirty
    ? `<span class="badge badge-dirty"><i class="dot"></i> ${esc(t('git.changes', git.staged + git.unstaged + git.untracked))}</span>`
    : `<span class="badge badge-clean"><i class="dot"></i> ${esc(t('git.cleanTree'))}</span>`);

  const changes = git.changes?.length ? `
    <div class="section">
      <h3>${esc(t('git.workingTree'))}</h3>
      <ul class="changes">${git.changes.map((c) => `
        <li class="k-${esc(c.kind)}"><span class="st">${esc(c.status.trim() || '·')}</span>
        <span>${esc(c.path)}</span></li>`).join('')}</ul>
    </div>` : '';

  const commits = git.commits?.length ? git.commits.map((c) => `
    <div class="commit" data-hash="${esc(c.hash)}">
      <span class="commit-subject">${esc(c.subject)}</span>
      <span class="commit-hash">${esc(c.short)}</span>
      <span class="commit-meta">${esc(c.author)} · ${ago(c.date)} · ${fmtDate(c.date)}${
        c.refs ? ` · <span class="muted">${esc(c.refs)}</span>` : ''}</span>
      ${c.body ? `<pre class="commit-body" hidden>${esc(c.body)}</pre>` : ''}
    </div>`).join('') : `<p class="center-note">${esc(t('git.noCommits'))}</p>`;

  pane.innerHTML = `
    <div class="git-head">
      ${head.join('')}
      ${git.remoteUrl ? `<span class="badge" title="origin">${esc(git.remoteUrl)}</span>` : ''}
      <button class="btn btn-sm" data-act="git-refresh" style="margin-left:auto">${esc(t('git.refresh'))}</button>
    </div>
    ${changes}
    <div class="section">
      <h3>${esc(t('git.history'))}</h3>
      <div class="commits">${commits}</div>
    </div>`;
}

// -------------------------------------------------------------------- README

async function renderReadmePane(pane, p) {
  pane.innerHTML = '<p class="center-note"><span class="spinner"></span></p>';
  try {
    const data = await api(`/api/projects/${p.id}/readme`);
    if (selected()?.id !== p.id) return;
    if (!data.found) {
      pane.innerHTML = `<p class="center-note">${esc(t('readme.none'))}</p>`;
      return;
    }
    pane.innerHTML = `<article class="md">${markdown(data.content)}</article>${
      data.truncated ? `<p class="muted">${esc(t('readme.truncated'))}</p>` : ''}`;
  } catch (err) {
    pane.innerHTML = `<p class="center-note">${esc(tError(err))}</p>`;
  }
}

// --------------------------------------------------------------------- files

async function renderFilesPane(pane, p) {
  pane.innerHTML = '<div class="tree" id="tree"><p class="center-note"><span class="spinner"></span></p></div><div id="viewer"></div>';
  S.tree = {};
  S.treeOpen = new Set(['']);
  await loadTree(p, '');
  if (selected()?.id !== p.id) return;
  drawTree();
}

async function loadTree(p, rel) {
  if (S.tree[rel]) return;
  try {
    const data = await api(`/api/projects/${p.id}/tree?rel=${encodeURIComponent(rel)}`);
    S.tree[rel] = data.entries;
  } catch (err) {
    S.tree[rel] = [];
    fail(err);
  }
}

function drawTree() {
  const render = (rel) => {
    const entries = S.tree[rel] || [];
    if (!entries.length) return `<ul><li class="muted">${esc(t('files.empty'))}</li></ul>`;
    return `<ul>${entries.map((e) => {
      const open = S.treeOpen.has(e.rel);
      return `<li>
        <button data-rel="${esc(e.rel)}" data-dir="${e.isDir}">
          <span class="ico ${e.isDir ? 'ico-dir' : ''}">${e.isDir ? (open ? '▾' : '▸') : '·'}</span>
          <span>${esc(e.name)}</span>
          ${e.isDir ? '' : `<span class="size">${fmtBytes(e.size)}</span>`}
        </button>
        ${e.isDir && open ? render(e.rel) : ''}
      </li>`;
    }).join('')}</ul>`;
  };
  $('#tree').innerHTML = render('');
}

async function onTreeClick(btn) {
  const p = selected();
  const rel = btn.dataset.rel;
  if (btn.dataset.dir === 'true') {
    if (S.treeOpen.has(rel)) S.treeOpen.delete(rel);
    else { S.treeOpen.add(rel); await loadTree(p, rel); }
    drawTree();
    return;
  }
  try {
    const data = await api(`/api/projects/${p.id}/file?rel=${encodeURIComponent(rel)}`);
    $('#viewer').innerHTML = `
      <div class="viewer">
        <header><span>${esc(rel)}</span><span class="muted" style="margin-left:auto">${fmtBytes(data.size)}</span></header>
        <pre>${esc(data.content)}</pre>
      </div>`;
  } catch (err) {
    $('#viewer').innerHTML = `<p class="center-note">${esc(tError(err))}</p>`;
  }
}

// ------------------------------------------------------------------- actions

async function openProject(projectId, action, editorId) {
  const p = S.projects.find((x) => x.id === projectId);
  if (!p) return;
  try {
    await api(`/api/projects/${p.id}/open`, {
      method: 'POST',
      body: { action, editor: editorId || p.editor || S.settings.defaultEditor || '' },
    });
    if (action === 'editor') {
      toast(t('toast.openingEditor', p.name), 'ok');
      await refreshState();
    }
  } catch (err) { fail(err); }
}

async function patchProject(id, patch) {
  try {
    const { project } = await api(`/api/projects/${id}`, { method: 'PATCH', body: patch });
    mergeProject(project);
    if (S.selectedId === id) renderDetail(); else renderList();
  } catch (err) { fail(err); }
}

async function refreshState() {
  const state = await api('/api/state');
  S.projects = state.projects || [];
  S.settings = state.settings || {};
  S.tags = state.tags || [];
  if (S.selectedId && selected()) renderDetail(); else showList();
}

async function copyPath() {
  try {
    await navigator.clipboard.writeText(selected().path);
    toast(t('detail.pathCopied'), 'ok');
  } catch {
    toast(t('detail.copyFailed'), 'err');
  }
}

async function computeSize() {
  const p = selected();
  const cell = $('#size-cell');
  cell.innerHTML = '<span class="spinner"></span>';
  try {
    const data = await api(`/api/projects/${p.id}/size`);
    const key = data.partial ? 'overview.sizePartial' : 'overview.sizeResult';
    cell.textContent = t(key, fmtBytes(data.bytes), data.files);
  } catch (err) {
    cell.textContent = tError(err);
  }
}

async function deleteSelected() {
  const p = selected();
  if (!confirm(t('overview.confirmDelete', p.name))) return;
  try {
    await api(`/api/projects/${p.id}`, { method: 'DELETE' });
    S.selectedId = null;
    await bootstrap();
    toast(t('overview.removed'), 'ok');
  } catch (err) { fail(err); }
}

// ------------------------------------------------------------- folder picker

/** The directory browser shared by the add and scan dialogs. */
function createPicker(root, onPick) {
  const listEl = $('.picker-list', root);
  const crumbEl = $('.picker-crumbs', root);
  const rootsEl = $('.picker-roots', root);
  const pathEl = $('.picker-path', root);

  async function go(path) {
    listEl.innerHTML = '<p class="picker-empty"><span class="spinner"></span></p>';
    let data;
    try {
      data = await api(`/api/fs?path=${encodeURIComponent(path || '')}`);
    } catch (err) {
      listEl.innerHTML = `<p class="picker-empty">${esc(tError(err))}</p>`;
      return;
    }
    pathEl.value = data.path;
    onPick?.(data.path);

    rootsEl.innerHTML = (data.roots || []).map((r) =>
      `<button type="button" class="chip" data-goto="${esc(r.path)}">${esc(r.name)}</button>`).join('');

    crumbEl.innerHTML = data.path
      ? `${data.parent ? `<button type="button" data-goto="${esc(data.parent)}">${esc(t('picker.parent'))}</button>` : ''}
         <span style="margin-left:6px">${esc(data.path)}</span>`
      : '';

    listEl.innerHTML = data.entries.length
      ? data.entries.map((e) => `
          <button type="button" data-goto="${esc(e.path)}">
            <span class="ico-dir">📁</span><span>${esc(e.name)}</span>
            ${e.isGit ? '<span class="git-mark">git</span>' : ''}
          </button>`).join('')
      : `<p class="picker-empty">${esc(t('picker.noSubfolders'))}</p>`;
  }

  root.addEventListener('click', (ev) => {
    const btn = ev.target.closest('[data-goto]');
    if (!btn) return;
    ev.preventDefault();
    go(btn.dataset.goto);
  });
  pathEl.addEventListener('change', () => {
    onPick?.(pathEl.value);
    go(pathEl.value);
  });

  return { go, value: () => pathEl.value.trim(), reset: () => go('') };
}

// --------------------------------------------------------------- add project

let addPicker;

function openAddDialog() {
  $('#add-name').value = '';
  $('#add-tags').value = '';
  $('#add-desc').value = '';
  $('#add-error').hidden = true;
  addPicker ??= createPicker($('[data-picker="add"]'));
  addPicker.reset();
  $('#dlg-add').showModal();
}

async function submitAdd(ev) {
  ev.preventDefault();
  const path = addPicker.value();
  if (!path) { showAddError(t('add.pickFolder')); return; }

  try {
    const { project } = await api('/api/projects', {
      method: 'POST',
      body: {
        path,
        name: $('#add-name').value.trim(),
        description: $('#add-desc').value.trim(),
        tags: $('#add-tags').value.split(',').map((x) => x.trim()).filter(Boolean),
      },
    });
    $('#dlg-add').close();
    await bootstrap();
    showDetail(project.id);
    loadSummaries();
    toast(t('add.added', project.name), 'ok');
  } catch (err) {
    showAddError(tError(err));
  }
}

function showAddError(msg) {
  const el = $('#add-error');
  el.textContent = msg;
  el.hidden = false;
}

// --------------------------------------------------------------- scan folder

let scanPicker;
let scanResults = [];

function openScanDialog() {
  $('#scan-results').innerHTML = '';
  $('#scan-note').textContent = '';
  $('#scan-import').disabled = true;
  $('#scan-depth').value = S.settings.scanDepth || 3;
  scanPicker ??= createPicker($('[data-picker="scan"]'));
  scanPicker.go((S.settings.scanRoots || [])[0] || '');
  $('#dlg-scan').showModal();
}

async function runScan() {
  const root = scanPicker.value();
  if (!root) { toast(t('scan.pickRoot'), 'err'); return; }

  const results = $('#scan-results');
  results.innerHTML = `<p class="center-note"><span class="spinner"></span> ${esc(t('scan.running'))}</p>`;
  try {
    const data = await api('/api/scan', {
      method: 'POST',
      body: {
        root,
        depth: Number($('#scan-depth').value) || 3,
        includeNonGit: $('#scan-nongit').checked,
      },
    });
    scanResults = data.results || [];
    drawScanResults();
    // Remember this root so the next scan opens straight into it.
    const roots = [root, ...(S.settings.scanRoots || []).filter((r) => r !== root)].slice(0, 5);
    await saveSettings({ ...S.settings, scanRoots: roots });
  } catch (err) {
    results.innerHTML = `<p class="center-note">${esc(tError(err))}</p>`;
  }
}

function drawScanResults() {
  const box = $('#scan-results');
  if (!scanResults.length) {
    box.innerHTML = `<p class="center-note">${esc(t('scan.nothing'))}</p>`;
    $('#scan-import').disabled = true;
    return;
  }
  const fresh = scanResults.filter((r) => !r.known).length;
  box.innerHTML = `
    <div class="scan-head">
      <label class="check"><input type="checkbox" id="scan-all" ${fresh ? 'checked' : ''}>
        <span>${esc(t('scan.selectAll'))}</span></label>
      <span class="muted">${esc(t('scan.found', scanResults.length, fresh))}</span>
    </div>
    ${scanResults.map((r, i) => `
      <div class="scan-row ${r.known ? 'is-known' : ''}">
        <input type="checkbox" data-i="${i}" ${!r.known ? 'checked' : ''} ${r.known ? 'disabled' : ''}>
        <div class="col">
          <div class="n">${esc(r.name)} ${r.known ? `<span class="muted">· ${esc(t('scan.alreadyAdded'))}</span>` : ''}</div>
          <div class="p" title="${esc(r.path)}">${esc(r.path)}</div>
        </div>
        <div style="display:flex;gap:5px;flex-wrap:wrap">
          ${r.isGit ? '<span class="badge">git</span>' : ''}
          ${(r.kinds || []).map((k) => `<span class="badge">${esc(k)}</span>`).join('')}
        </div>
        <span class="muted" style="font-size:11.5px;white-space:nowrap">${ago(r.modified)}</span>
      </div>`).join('')}`;

  $('#scan-import').disabled = fresh === 0;
  $('#scan-note').textContent = t('scan.canAdd', fresh);
}

async function importScanned() {
  const paths = $$('#scan-results input[data-i]:checked')
    .map((cb) => scanResults[Number(cb.dataset.i)].path);
  if (!paths.length) return;

  try {
    const data = await api('/api/import', { method: 'POST', body: { paths } });
    $('#dlg-scan').close();
    await bootstrap();
    loadSummaries();
    toast(data.skipped?.length
      ? t('scan.importedSkipped', data.addedCount, data.skipped.length)
      : t('scan.imported', data.addedCount), 'ok');
  } catch (err) {
    fail(err);
  }
}

// ------------------------------------------------------------------ settings

function openSettingsDialog() {
  const s = S.settings;
  const srv = S.server;

  const editorOpts = [`<option value="">${esc(t('settings.autoFirst'))}</option>`]
    .concat(S.editors.map((e) => `<option value="${esc(e.id)}" ${e.id === s.defaultEditor ? 'selected' : ''}>${esc(e.name)}</option>`))
    .join('');

  const langOpts = [`<option value="auto" ${!s.language || s.language === 'auto' ? 'selected' : ''}>${esc(t('settings.languageAuto'))}</option>`]
    .concat(Object.entries(LANGUAGES).map(([code, name]) =>
      `<option value="${code}" ${s.language === code ? 'selected' : ''}>${esc(name)}</option>`))
    .join('');

  $('#settings-body').innerHTML = `
    <div class="section">
      <h3>${esc(t('settings.editor'))}</h3>
      <label class="field">
        <span>${esc(t('settings.defaultEditor'))}</span>
        <select id="set-editor">${editorOpts}</select>
      </label>
      <div class="actions">
        <button type="button" class="btn btn-sm" id="set-rescan">${esc(t('settings.rescan'))}</button>
        <span class="muted" style="align-self:center;font-size:12.5px">${esc(t('settings.editorsFound', S.editors.length))}</span>
      </div>
      <details style="margin-top:10px">
        <summary class="muted" style="cursor:pointer;font-size:12.5px">${esc(t('settings.detectedEditors'))}</summary>
        <dl class="kv" style="margin-top:8px">
          ${S.editors.map((e) => `<dt>${esc(e.name)}</dt><dd>${esc(e.exec)}</dd>`).join('')
            || `<dt>—</dt><dd>${esc(t('settings.none'))}</dd>`}
        </dl>
      </details>
    </div>

    <div class="section">
      <h3>${esc(t('settings.customEditors'))}</h3>
      <p class="muted" style="font-size:12.5px;margin-top:-4px">${t('settings.customHint')}</p>
      <div id="custom-editors">${(s.customEditors || []).map(customEditorRow).join('')}</div>
      <button type="button" class="btn btn-sm" id="add-custom">${esc(t('settings.addRow'))}</button>
    </div>

    <div class="section">
      <h3>${esc(t('settings.appearance'))}</h3>
      <div class="grid-2">
        <label class="field"><span>${esc(t('settings.language'))}</span>
          <select id="set-language">${langOpts}</select>
        </label>
        <label class="field"><span>${esc(t('settings.theme'))}</span>
          <select id="set-theme">
            <option value="dark" ${s.theme === 'dark' ? 'selected' : ''}>${esc(t('settings.themeDark'))}</option>
            <option value="light" ${s.theme === 'light' ? 'selected' : ''}>${esc(t('settings.themeLight'))}</option>
            <option value="system" ${s.theme === 'system' ? 'selected' : ''}>${esc(t('settings.themeSystem'))}</option>
          </select>
        </label>
        <label class="field"><span>${esc(t('settings.listView'))}</span>
          <select id="set-view">
            <option value="grid" ${s.view !== 'list' ? 'selected' : ''}>${esc(t('view.grid'))}</option>
            <option value="list" ${s.view === 'list' ? 'selected' : ''}>${esc(t('view.list'))}</option>
          </select>
        </label>
        <label class="field"><span>${esc(t('settings.sort'))}</span>
          <select id="set-sort">
            <option value="recent" ${s.sort === 'recent' ? 'selected' : ''}>${esc(t('settings.sortRecent'))}</option>
            <option value="name" ${s.sort === 'name' ? 'selected' : ''}>${esc(t('settings.sortName'))}</option>
            <option value="created" ${s.sort === 'created' ? 'selected' : ''}>${esc(t('settings.sortCreated'))}</option>
          </select>
        </label>
        <label class="field"><span>${esc(t('settings.commitCount'))}</span>
          <input type="number" id="set-commits" min="5" max="200" value="${s.commitCount || 25}">
        </label>
        <label class="field"><span>${esc(t('settings.scanDepth'))}</span>
          <input type="number" id="set-depth" min="1" max="8" value="${s.scanDepth || 3}">
        </label>
      </div>
    </div>

    <div class="section">
      <h3>${esc(t('settings.addresses'))}</h3>
      <dl class="kv">
        <dt>${esc(t('settings.thisComputer'))}</dt><dd>${esc(srv.localUrl || '')}</dd>
        ${srv.mdnsEnabled ? `<dt>${esc(t('settings.viaMdns'))}</dt><dd>${esc(srv.mdnsUrl || '')}</dd>` : ''}
        ${(srv.lanUrls || []).map((u) => `<dt>${esc(t('settings.viaIp'))}</dt><dd>${esc(u)}</dd>`).join('')}
        <dt>${esc(t('settings.remoteAccess'))}</dt>
        <dd>${esc(srv.remoteOpen ? t('settings.remoteOn') : t('settings.remoteOff'))}</dd>
      </dl>
      <p class="muted" style="font-size:12.5px">${t('settings.remoteHint', esc(srv.configFile || 'config.json'))}</p>
    </div>

    <div class="section">
      <h3>${esc(t('settings.files'))}</h3>
      <dl class="kv">
        <dt>${esc(t('settings.dataFile'))}</dt><dd>${esc(srv.dataFile || '')}</dd>
        <dt>${esc(t('settings.configFile'))}</dt><dd>${esc(srv.configFile || '')}</dd>
        <dt>${esc(t('settings.version'))}</dt><dd>${esc(srv.version || '')} · ${esc(srv.os || '')}</dd>
        <dt>git</dt><dd>${esc(srv.gitOk ? t('settings.gitFound') : t('settings.gitMissing'))}</dd>
      </dl>
    </div>`;

  $('#set-rescan').addEventListener('click', async () => {
    const { editors } = await api('/api/editors?refresh=1');
    S.editors = editors;
    toast(t('settings.editorsFound', editors.length), 'ok');
    openSettingsDialog();
  });
  $('#add-custom').addEventListener('click', () => {
    $('#custom-editors').insertAdjacentHTML('beforeend', customEditorRow({ id: '', name: '', exec: '', args: '' }));
  });

  $('#dlg-settings').showModal();
}

const customEditorRow = (e) => `
  <div class="grid-2 custom-editor" style="gap:0 8px">
    <label class="field"><span>${esc(t('settings.editorName'))}</span>
      <input type="text" data-k="name" value="${esc(e.name)}" placeholder="Notepad++"></label>
    <label class="field"><span>${esc(t('settings.editorExec'))}</span>
      <input type="text" data-k="exec" value="${esc(e.exec)}" placeholder="C:\\Program Files\\...\\app.exe"></label>
    <input type="hidden" data-k="id" value="${esc(e.id)}">
  </div>`;

async function saveSettings(next) {
  const { settings } = await api('/api/settings', { method: 'PUT', body: next });
  S.settings = settings;
  applyPreferences();
  return settings;
}

async function submitSettings(ev) {
  ev.preventDefault();
  const customEditors = $$('#custom-editors .custom-editor').map((row) => ({
    id: $('[data-k="id"]', row).value,
    name: $('[data-k="name"]', row).value.trim(),
    exec: $('[data-k="exec"]', row).value.trim(),
    args: '',
  })).filter((e) => e.name && e.exec);

  try {
    await saveSettings({
      ...S.settings,
      defaultEditor: $('#set-editor').value,
      language: $('#set-language').value,
      theme: $('#set-theme').value,
      view: $('#set-view').value,
      sort: $('#set-sort').value,
      commitCount: Number($('#set-commits').value) || 25,
      scanDepth: Number($('#set-depth').value) || 3,
      customEditors,
    });
    const { editors } = await api('/api/editors');
    S.editors = editors;
    $('#dlg-settings').close();
    await bootstrap();
    toast(t('settings.saved'), 'ok');
  } catch (err) {
    fail(err);
  }
}

/** Applies the stored language and theme to the document. */
function applyPreferences() {
  applyLanguage(S.settings.language);
  translateDocument();

  const theme = S.settings.theme || 'dark';
  const resolved = theme === 'system'
    ? (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark')
    : theme;
  document.documentElement.dataset.theme = resolved;
}

// -------------------------------------------------------------- mini markdown

/**
 * Just enough markdown to display a README. Everything is escaped first and
 * only the tags produced here are added back, so HTML inside a README is never
 * executed.
 */
function markdown(src) {
  const blocks = [];
  let text = esc(src).replace(/\r\n/g, '\n');

  // Fenced code is lifted out first so its contents are never interpreted.
  text = text.replace(/```([\w-]*)\n([\s\S]*?)```/g, (_, lang, code) => {
    blocks.push(`<pre><code data-lang="${lang}">${code.replace(/\n$/, '')}</code></pre>`);
    return `\u0000${blocks.length - 1}\u0000`;
  });

  text = text
    .replace(/^###### (.*)$/gm, '<h6>$1</h6>')
    .replace(/^##### (.*)$/gm, '<h5>$1</h5>')
    .replace(/^#### (.*)$/gm, '<h4>$1</h4>')
    .replace(/^### (.*)$/gm, '<h3>$1</h3>')
    .replace(/^## (.*)$/gm, '<h2>$1</h2>')
    .replace(/^# (.*)$/gm, '<h1>$1</h1>')
    .replace(/^\s*([-*_])(?:\s*\1){2,}\s*$/gm, '<hr>')
    .replace(/^&gt; ?(.*)$/gm, '<blockquote>$1</blockquote>')
    .replace(/`([^`\n]+)`/g, '<code>$1</code>')
    .replace(/!\[([^\]]*)\]\(([^)\s]+)[^)]*\)/g, (_, alt, src2) => `<img alt="${alt}" src="${safeURL(src2)}">`)
    .replace(/\[([^\]]+)\]\(([^)\s]+)[^)]*\)/g, (_, label, href) =>
      `<a href="${safeURL(href)}" target="_blank" rel="noopener noreferrer">${label}</a>`)
    .replace(/(^|[^*])\*\*([^*\n]+)\*\*/g, '$1<strong>$2</strong>')
    .replace(/(^|[^*\w])\*([^*\n]+)\*/g, '$1<em>$2</em>');

  // Consecutive bullet lines collapse into a single list.
  // "- [x] item" task lists are drawn as real checkboxes.
  text = text.replace(/(?:^[ \t]*[-*+] .*(?:\n|$))+/gm, (chunk) =>
    `<ul>${chunk.trim().split('\n').map((line) => {
      const item = line.replace(/^[ \t]*[-*+] /, '');
      const task = item.match(/^\[([ xX])\]\s*(.*)$/);
      if (!task) return `<li>${item}</li>`;
      const done = task[1].toLowerCase() === 'x';
      return `<li class="task"><input type="checkbox" disabled ${done ? 'checked' : ''}>${task[2]}</li>`;
    }).join('')}</ul>\n`);
  text = text.replace(/(?:^[ \t]*\d+\. .*(?:\n|$))+/gm, (chunk) =>
    `<ol>${chunk.trim().split('\n').map((l) => `<li>${l.replace(/^[ \t]*\d+\. /, '')}</li>`).join('')}</ol>\n`);

  // What is left, separated by blank lines, becomes paragraphs.
  text = text.split(/\n{2,}/).map((part) => {
    const trimmed = part.trim();
    if (!trimmed) return '';
    if (/^<(h[1-6]|ul|ol|pre|blockquote|hr|table|img)/.test(trimmed)) return trimmed;
    if (/^\u0000\d+\u0000$/.test(trimmed)) return trimmed;
    return `<p>${trimmed.replace(/\n/g, '<br>')}</p>`;
  }).join('\n');

  return text.replace(/\u0000(\d+)\u0000/g, (_, i) => blocks[Number(i)]);
}

/** Drops schemes like javascript:; only safe links survive. */
function safeURL(url) {
  const clean = url.trim();
  if (/^(https?:|mailto:|#|\/|\.)/i.test(clean)) return clean;
  return '#';
}

// -------------------------------------------------------------------- events

function wireEvents() {
  $('#search').addEventListener('input', debounce((e) => {
    S.query = e.target.value;
    if (S.selectedId) showList();
    else renderList();
  }, 120));

  $('#filters').addEventListener('click', (e) => {
    const btn = e.target.closest('[data-filter]');
    if (!btn) return;
    S.filter = btn.dataset.filter;
    $$('#filters .chip').forEach((c) => c.classList.toggle('is-active', c === btn));
    if (S.selectedId) showList(); else renderList();
  });

  $('#tagbar').addEventListener('click', (e) => {
    const btn = e.target.closest('[data-tag]');
    if (!btn) return;
    S.activeTag = S.activeTag === btn.dataset.tag ? null : btn.dataset.tag;
    renderList();
  });

  $('#view-toggle').addEventListener('click', async (e) => {
    const btn = e.target.closest('[data-view]');
    if (!btn || btn.dataset.view === S.settings.view) return;
    S.settings.view = btn.dataset.view;
    renderList();
    try { await saveSettings({ ...S.settings }); } catch { /* view preference is not critical */ }
  });

  // Card clicks: the quick-action buttons must not also open the card.
  $('#project-list').addEventListener('click', (e) => {
    const card = e.target.closest('.pcard');
    if (!card) return;
    const quick = e.target.closest('[data-quick]');
    if (quick) {
      e.stopPropagation();
      const id = card.dataset.id;
      if (quick.dataset.quick === 'favorite') {
        const p = S.projects.find((x) => x.id === id);
        patchProject(id, { favorite: !p.favorite });
      } else {
        openProject(id, quick.dataset.quick);
      }
      return;
    }
    showDetail(card.dataset.id);
  });

  $('#project-list').addEventListener('keydown', (e) => {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    const card = e.target.closest('.pcard');
    if (!card || e.target.closest('[data-quick]')) return;
    e.preventDefault();
    showDetail(card.dataset.id);
  });

  $('#btn-add').addEventListener('click', openAddDialog);
  $('#btn-scan').addEventListener('click', openScanDialog);
  $('#btn-settings').addEventListener('click', openSettingsDialog);
  $('#empty-state').addEventListener('click', (e) => {
    const act = e.target.closest('[data-action]')?.dataset.action;
    if (act === 'add') openAddDialog();
    if (act === 'scan') openScanDialog();
  });

  $('#add-submit').addEventListener('click', submitAdd);
  $('#settings-save').addEventListener('click', submitSettings);
  $('#scan-run').addEventListener('click', (e) => { e.preventDefault(); runScan(); });
  $('#scan-import').addEventListener('click', (e) => { e.preventDefault(); importScanned(); });
  $('#scan-results').addEventListener('change', (e) => {
    if (e.target.id === 'scan-all') {
      $$('#scan-results input[data-i]:not(:disabled)').forEach((cb) => { cb.checked = e.target.checked; });
    }
    const n = $$('#scan-results input[data-i]:checked').length;
    $('#scan-import').disabled = n === 0;
    $('#scan-note').textContent = t('scan.selected', n);
  });

  // The detail pane is redrawn constantly, so it is driven by one listener.
  $('#view-detail').addEventListener('click', (e) => {
    const todoLi = e.target.closest('[data-todo]');
    const todoAct = e.target.closest('[data-todo-act]')?.dataset.todoAct;
    if (todoLi && todoAct) { onTodoAction(todoLi, todoAct); return; }

    const todoFilter = e.target.closest('[data-todo-filter]')?.dataset.todoFilter;
    if (todoFilter) { S.todoFilter = todoFilter; renderPane(); return; }

    const tab = e.target.closest('[data-tab]')?.dataset.tab;
    if (tab) { S.tab = tab; renderDetail(); return; }

    const act = e.target.closest('[data-act]')?.dataset.act;
    switch (act) {
      case 'back': return showList();
      case 'open-editor': return openProject(S.selectedId, 'editor', $('#editor-select')?.value);
      case 'reveal': return openProject(S.selectedId, 'reveal');
      case 'folder': return openProject(S.selectedId, 'folder');
      case 'terminal': return openProject(S.selectedId, 'terminal');
      case 'favorite': return patchProject(S.selectedId, { favorite: !selected().favorite });
      case 'archive': return patchProject(S.selectedId, { archived: !selected().archived });
      case 'git-refresh': return renderPane();
      case 'copy-path': return copyPath();
      case 'size': return computeSize();
      case 'delete': return deleteSelected();
      case 'clear-done': return clearDoneTodos();
      default: break;
    }

    const commit = e.target.closest('.commit');
    if (commit) {
      const body = $('.commit-body', commit);
      if (body) body.hidden = !body.hidden;
      return;
    }
    const treeBtn = e.target.closest('#tree [data-rel]');
    if (treeBtn) onTreeClick(treeBtn);
  });

  $('#view-detail').addEventListener('change', (e) => {
    if (e.target.id === 'editor-select') patchProject(S.selectedId, { editor: e.target.value });
  });

  document.addEventListener('keydown', (e) => {
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName);

    if (e.key === '/' && !typing) {
      e.preventDefault();
      $('#search').focus();
      $('#search').select();
      return;
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'n') {
      e.preventDefault();
      openAddDialog();
      return;
    }
    if (e.key === 'Escape') {
      if (e.target === $('#search')) {
        $('#search').value = '';
        S.query = '';
        renderList();
      } else if (!typing && S.selectedId && !$$('dialog[open]').length) {
        showList();
      }
      return;
    }
    if (e.key === 'Enter' && e.target === $('#search')) {
      const first = visibleProjects()[0];
      if (first) showDetail(first.id);
    }
  });

  matchMedia('(prefers-color-scheme: light)').addEventListener('change', applyPreferences);

  // Coming back from the editor, commits may have been made; refresh the badges.
  window.addEventListener('focus', () => loadSummaries(false));
}

// ------------------------------------------------------------------- startup

async function bootstrap() {
  const state = await api('/api/state');
  S.projects = state.projects || [];
  S.settings = state.settings || {};
  S.tags = state.tags || [];
  S.editors = state.editors || [];
  S.server = state.server || {};
  applyPreferences();
  if (S.selectedId && selected()) renderDetail(); else showList();
}

async function main() {
  // Translate the static markup before the first request, so a slow or failed
  // connection still shows a labelled interface rather than empty buttons.
  applyLanguage(null);
  translateDocument();

  wireEvents();
  try {
    await bootstrap();
    loadSummaries();
  } catch (err) {
    toast(t('toast.connectFailed', tError(err)), 'err');
  }
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch(() => { /* PWA is optional */ });
  }
}

main();
