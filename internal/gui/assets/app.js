"use strict";

const TOKEN = document.querySelector('meta[name="csolve-token"]').content;
const API = {
  spec: '/api/spec',
  validate: '/api/validate',
  jobs: '/api/jobs',
  quit: '/api/quit',
  fs: '/api/fs',
  upload: '/api/upload',
};

// ---------- i18n ----------
const I18N = {
  en: {
    'presets': 'Presets', 'input': 'Input', 'output': 'Output',
    'params': 'Parameters', 'run': 'Run', 'results': 'Results',
    'show_adv': 'Show advanced', 'mode_demo': 'Demo data',
    'mode_json': 'Single retrieval (JSON)', 'mode_batch': 'Batch profile (TXT)',
    'data_json': 'JSON file with measurements',
    'data_batch': 'TXT batch profile',
    'tables_builtin': 'Built-in tables', 'tables_external': 'External tables directory',
    'browse': 'Browse…', 'upload': 'Upload / drag & drop',
    'plot_none': 'No plots', 'plot_png': 'PNG', 'plot_pdf': 'PDF', 'plot_svg': 'SVG',
    'plot_engine': 'Render engine', 'engine_internal': 'Built-in', 'engine_gnuplot': 'gnuplot',
    'report_off': 'No report', 'report_on': 'Generate report',
    'run_btn': 'Run', 'check_btn': 'Check', 'copy': 'Copy command',
    'cancel': 'Cancel', 'cmd_preview': 'Command preview',
    'invalid': 'Invalid parameters', 'running': 'Running…',
    'done': 'Done', 'failed': 'Failed', 'cancelled': 'Cancelled',
    'duration': 'Duration', 'exit': 'Exit code', 'files': 'Output files',
    'summary': 'Summary', 'channels': 'Channels', 'modes': 'Modes',
    'solutions': 'Best solutions', 'batch': 'Batch summary',
    'grp_moments': 'Moments', 'grp_refractive': 'Refractive index',
    'grp_errors': 'Channel errors', 'grp_delpol': 'Depolarization',
    'grp_lidar_ratios': 'Lidar ratio',
    'download': 'Download', 'err': 'No results yet', 'language': 'Language',
    'by_height': 'Files', 'spectra': 'Spectra', 'true_label': 'True m',
    'result_json': 'JSON result', 'spec_none': 'Check a preset or fill parameters',
    'not_run': 'Configuration is invalid — check highlighted fields.',
    'reuse': 'Rerun', 'edit': 'Edit parameters',
  },
  ru: {
    'presets': 'Пресеты', 'input': 'Вход', 'output': 'Вывод',
    'params': 'Параметры', 'run': 'Запуск', 'results': 'Результаты',
    'show_adv': 'Показать все параметры', 'mode_demo': 'Демо-данные',
    'mode_json': 'Одиночное восстановление (JSON)',
    'mode_batch': 'Пакетный профиль (TXT)',
    'data_json': 'JSON-файл с измерениями',
    'data_batch': 'TXT-файл пакетного профиля',
    'tables_builtin': 'Встроенные таблицы', 'tables_external': 'Внешний каталог таблиц',
    'browse': 'Обзор…', 'upload': 'Загрузить / перетащить',
    'plot_none': 'Без графиков', 'plot_png': 'PNG', 'plot_pdf': 'PDF', 'plot_svg': 'SVG',
    'plot_engine': 'Движок отрисовки', 'engine_internal': 'Встроенный', 'engine_gnuplot': 'gnuplot',
    'report_off': 'Без отчёта', 'report_on': 'Создать отчёт',
    'run_btn': 'Запустить', 'check_btn': 'Проверить', 'copy': 'Копировать команду',
    'cancel': 'Прервать', 'cmd_preview': 'Эквивалентная команда',
    'invalid': 'Параметры неверны', 'running': 'Выполняется…',
    'done': 'Готово', 'failed': 'Ошибка', 'cancelled': 'Отменено',
    'duration': 'Длительность', 'exit': 'Код выхода', 'files': 'Выходные файлы',
    'summary': 'Сводка', 'channels': 'Каналы', 'modes': 'Моды',
    'solutions': 'Лучшие решения', 'batch': 'Сводка по высотам',
    'grp_moments': 'Моменты', 'grp_refractive': 'Показатель преломления',
    'grp_errors': 'Ошибки по каналам', 'grp_delpol': 'Деполяризация',
    'grp_lidar_ratios': 'Лидарное отношение',
    'download': 'Скачать', 'err': 'Результатов пока нет', 'language': 'Язык',
    'by_height': 'Файлы', 'spectra': 'Спектры', 'true_label': 'Истинный m',
    'result_json': 'JSON-результат', 'spec_none': 'Выберите пресет или заполните параметры',
    'not_run': 'Конфигурация неверна — проверьте поля.',
    'reuse': 'Повторить', 'edit': 'Изменить параметры',
  },
};
let L = 'en'; // default; switch if navigator prefers ru
function t(key) { return (I18N[L] && I18N[L][key]) || I18N.en[key] || key; }
function F(labelEN, labelRU) { return L === 'ru' ? labelRU : labelEN; }

// ---------- state ----------
const state = {
  mode: 'demo',
  data_path: '',
  batch_path: '',
  tables_kind: 'builtin',
  builtin_type: '',
  tables_dir: '',
  options: {},
  output: { plot_format: 'png', plot_engine: 'internal', plot_dpi: 150, plot_font_size: 12, spectra: true, report: false, report_format: 'docx' },
  spec: null,
  currentJob: null,
  runAbort: null,
};

function optionsClone(defs) { const o = {}; for (const k in defs) o[k] = defs[k]; return o; }

// ---------- dom helpers ----------
function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

// ---------- boot ----------
async function boot() {
  const resp = await fetch(API.spec, { headers: { 'X-CSolve-Token': TOKEN } });
  const spec = await resp.json();
  state.spec = spec;
  if (navigator.language && navigator.language.toLowerCase().startsWith('ru')) L = 'ru';
  state.builtin_type = spec.default_type;
  state.options = optionsClone(defaultsOf(spec));
  renderAll();
  bindLang();
}

function defaultsOf(spec) {
  const d = {};
  for (const f of spec.fields) d[f.key] = f.default;
  return d;
}

// ---------- render ----------
function renderAll() {
  applyLang();
  renderPresets();
  renderInput();
  renderTables();
  renderOutput();
  renderGroups();
  runValidate();
}

function applyLang() {
  document.title = 'csolve GUI';
  setText('side-preset-title', t('presets'));
  setText('side-input-title', t('input'));
  setText('side-output-title', t('output'));
  setText('params-title', t('params'));
  setText('adv-label', t('show_adv'));
  setText('run-btn', t('run_btn'));
  setText('validate-btn', t('check_btn'));
  setText('copy-cmd', t('copy'));
  setText('run-title', t('run'));
  setText('result-title', t('results'));
}
function setText(id, s) { const e = document.getElementById(id); if (e) e.textContent = s; }

function renderPresets() {
  const c = document.getElementById('presets');
  c.innerHTML = '';
  for (const p of state.spec.presets) {
    const b = el('button', 'btn preset');
    const title = el('div', null, F(p.label_en, p.label_ru));
    const desc = el('div', 'pd', F(p.desc_en, p.desc_ru));
    b.append(title, desc);
    b.addEventListener('click', () => { applyPreset(p); });
    c.appendChild(b);
  }
}

function applyPreset(p) {
  for (const k in p.values) {
    if (k in state) { state[k] = p.values[k]; }
    else { state.options[k] = p.values[k]; }
  }
  renderInput(); renderTables(); renderGroups(); runValidate();
}

function renderInput() {
  const c = document.getElementById('input-mode');
  c.innerHTML = '';
  const modes = [['demo', 'mode_demo'], ['json', 'mode_json'], ['batch', 'mode_batch']];
  const field = el('div', 'field full');
  field.appendChild(el('label', null, t('mode_demo')));
  const sel = el('select');
  for (const [val, key] of modes) {
    const o = document.createElement('option');
    o.value = val; o.textContent = t(key);
    if (state.mode === val) o.selected = true;
    sel.appendChild(o);
  }
  sel.addEventListener('change', () => {
    state.mode = sel.value;
    renderInput(); renderTables(); renderGroups(); runValidate();
  });
  field.appendChild(sel);
  c.appendChild(field);

  const fc = document.getElementById('input-file');
  fc.innerHTML = '';
  if (state.mode === 'json' || state.mode === 'batch') {
    const keyPath = state.mode === 'json' ? 'data_path' : 'batch_path';
    const label = state.mode === 'json' ? t('data_json') : t('data_batch');
    const row = el('div', 'field full');
    row.appendChild(el('label', null, label));
    const pr = el('div', 'path-row');
    const inp = el('input');
    inp.type = 'text'; inp.value = state[keyPath]; inp.placeholder = '/path/to/file';
    inp.addEventListener('change', () => { state[keyPath] = inp.value; runValidate(); });
    const browse = el('button', 'btn', t('browse'));
    browse.addEventListener('click', () => openFSModal((path) => { state[keyPath] = path; renderInput(); runValidate(); }));
    pr.append(inp, browse);
    row.appendChild(pr);
    const drop = el('button', 'btn ghost', t('upload'));
    drop.addEventListener('click', () => pickUpload(state.mode === 'json' ? 'json,txt' : 'txt', (p) => { state[keyPath] = p; renderInput(); runValidate(); }));
    row.appendChild(drop);
    fc.appendChild(row);
  }
}

function renderTables() {
  const c = document.getElementById('input-tables');
  c.innerHTML = '';
  const field = el('div', 'field full');
  field.appendChild(el('label', null, t('tables_builtin')));
  const sel = el('select');
  for (const kind of ['builtin', 'external']) {
    const o = document.createElement('option');
    o.value = kind;
    o.textContent = kind === 'builtin' ? t('tables_builtin') : t('tables_external');
    if (state.tables_kind === kind) o.selected = true;
    sel.appendChild(o);
  }
  sel.addEventListener('change', () => { state.tables_kind = sel.value; renderTables(); runValidate(); });
  field.appendChild(sel);
  c.appendChild(field);

  if (state.tables_kind === 'builtin') {
    const f2 = el('div', 'field full');
    f2.appendChild(el('label', null, '--type'));
    const tsel = el('select');
    for (const tt of state.spec.table_types) {
      const o = document.createElement('option');
      o.value = tt; o.textContent = tt;
      if (state.builtin_type === tt) o.selected = true;
      tsel.appendChild(o);
    }
    tsel.addEventListener('change', () => { state.builtin_type = tsel.value; });
    f2.appendChild(tsel);
    c.appendChild(f2);
  } else {
    const row = el('div', 'field full');
    row.appendChild(el('label', null, t('tables_external')));
    const pr = el('div', 'path-row');
    const inp = el('input');
    inp.type = 'text'; inp.value = state.tables_dir;
    inp.addEventListener('change', () => { state.tables_dir = inp.value; runValidate(); });
    const browse = el('button', 'btn', t('browse'));
    browse.addEventListener('click', () => openFSModal((p) => { state.tables_dir = p; renderTables(); runValidate(); }));
    pr.append(inp, browse);
    row.appendChild(pr);
    c.appendChild(row);
  }
}

function renderOutput() {
  const c = document.getElementById('output-opts');
  c.innerHTML = '';
  const o = state.output;

  // render engine: built-in renderer or external gnuplot
  if (o.plot_engine !== 'gnuplot') o.plot_engine = 'internal';
  if (o.plot_engine !== 'gnuplot' && o.plot_format === 'svg') o.plot_format = 'png';
  const fe = el('div', 'field full');
  fe.appendChild(el('label', null, t('plot_engine')));
  const esel = el('select');
  for (const [val, key] of [['internal', 'engine_internal'], ['gnuplot', 'engine_gnuplot']]) {
    const opt = document.createElement('option');
    opt.value = val; opt.textContent = t(key);
    if (o.plot_engine === val) opt.selected = true;
    esel.appendChild(opt);
  }
  esel.addEventListener('change', () => {
    o.plot_engine = esel.value;
    if (o.plot_engine !== 'gnuplot' && o.plot_format === 'svg') o.plot_format = 'png';
    renderOutput(); runValidate();
  });
  fe.appendChild(esel);
  c.appendChild(fe);

  // plot format
  const f = el('div', 'field full');
  f.appendChild(el('label', null, 'Plot'));
  const sel = el('select');
  const formats = [['none', 'plot_none'], ['png', 'plot_png'], ['pdf', 'plot_pdf']];
  if (o.plot_engine === 'gnuplot') formats.push(['svg', 'plot_svg']);
  for (const [val, key] of formats) {
    const opt = document.createElement('option');
    opt.value = val; opt.textContent = t(key);
    if (o.plot_format === val) opt.selected = true;
    sel.appendChild(opt);
  }
  sel.addEventListener('change', () => { o.plot_format = sel.value; renderOutput(); runValidate(); });
  f.appendChild(sel);
  c.appendChild(f);

  if (o.plot_format !== 'none') {
    const d = el('div', 'field');
    d.appendChild(el('label', null, 'DPI'));
    const dpi = el('input'); dpi.type = 'number'; dpi.min = 72; dpi.max = 600; dpi.value = o.plot_dpi;
    dpi.addEventListener('change', () => { o.plot_dpi = +dpi.value; });
    d.appendChild(dpi); c.appendChild(d);

    const fs = el('div', 'field');
    fs.appendChild(el('label', null, 'Font size'));
    const fsz = el('input'); fsz.type = 'number'; fsz.min = 4; fsz.max = 72; fsz.value = o.plot_font_size;
    fsz.addEventListener('change', () => { o.plot_font_size = +fsz.value; });
    fs.appendChild(fsz); c.appendChild(fs);
  }

  const sp = el('div', 'field full');
  const spectra = el('label', 'chk');
  const cb = el('input'); cb.type = 'checkbox'; cb.checked = o.spectra;
  cb.addEventListener('change', () => { o.spectra = cb.checked; });
  spectra.appendChild(cb);
  spectra.appendChild(document.createTextNode(' ' + 'Spectra'));
  sp.appendChild(spectra);
  c.appendChild(sp);

  const rl = el('label', 'chk');
  const rcb = el('input'); rcb.type = 'checkbox'; rcb.checked = o.report;
  rcb.addEventListener('change', () => { o.report = rcb.checked; });
  rl.appendChild(rcb);
  rl.appendChild(document.createTextNode(' ' + 'Report'));
  c.appendChild(rl);
}

// ---------- options form ----------
let visibleNeed = {};
function renderGroups() {
  const c = document.getElementById('groups');
  c.innerHTML = '';
  const showAll = document.getElementById('show-advanced').checked;
  const groups = state.spec.groups.slice().sort((a, b) => a.order - b.order);
  for (const g of groups) {
    const fields = state.spec.fields.filter((f) => f.group === g.id);
    if (fields.length === 0) continue;
    const box = el('div', 'group');
    const head = el('div', 'group-head');
    const h = el('h3', null, F(g.label_en, g.label_ru));
    const chev = el('span', null, '');
    chev.textContent = '▾';
    let collapsed = false;
    head.append(h, chev);
    head.addEventListener('click', () => {
      collapsed = !collapsed;
      body.classList.toggle('collapsed', collapsed);
      chev.textContent = collapsed ? '▸' : '▾';
    });
    box.appendChild(head);
    const body = el('div', 'group-body');
    for (const f of fields) {
      body.appendChild(makeField(f));
    }
    // скрываем группы, полностью невидимые
    body.dataset.group = g.id;
    box.appendChild(body);
    c.appendChild(box);
  }
  applyVisibility();
}

function isVisible(f) {
  if (!f.visible) return true;
  for (const k in f.visible) {
    const wants = f.visible[k];
    let got = null;
    if (k === 'mode') got = state.mode;
    else if (k === 'tables_kind') got = state.tables_kind;
    else got = String(state.options[k]);
    if (!wants.includes(got)) return false;
  }
  return true;
}

function applyVisibility() {
  for (const box of document.querySelectorAll('.group')) {
    const body = box.querySelector('.group-body');
    const key = body.dataset.group;
    const show = showGroup(key);
    box.style.display = show ? '' : 'none';
  }
  for (const f of state.spec.fields) {
    const wrap = document.querySelector(`[data-field="${f.key}"]`);
    if (wrap) wrap.style.display = isVisible(f) ? '' : 'none';
  }
}

function showGroup(gid) {
  const any = state.spec.fields.some((f) => f.group === gid && isVisible(f));
  return any;
}

function makeField(f) {
  const wrap = el('div', 'field' + (f.type === 'optional' ? ' full' : ''));
  wrap.dataset.field = f.key;
  const cur = state.options[f.key];

  if (f.type === 'bool') {
    const lab = el('label', 'chk');
    const span = el('span', null, (L === 'ru' ? f.label_ru : f.label_en) + '  <' + f.key + '>');
    const input = el('input'); input.type = 'checkbox'; input.checked = !!cur;
    input.addEventListener('change', () => { state.options[f.key] = input.checked; applyVisibility(); runValidate(); });
    lab.appendChild(span); lab.appendChild(input);
    wrap.appendChild(lab);
    if (f.help_en && f.help_ru) wrap.appendChild(el('div', 'help', F(f.help_en, f.help_ru)));
    return wrap;
  }

  const label = el('label', null, (L === 'ru' ? f.label_ru : f.label_en) + '  <' + f.key + '>');
  wrap.appendChild(label);
  let input;
  if (f.type === 'enum') {
    input = el('select');
    for (const opt of f.options) {
      const o = document.createElement('option');
      o.value = opt; o.textContent = opt;
      if (String(cur) === opt) o.selected = true;
      input.appendChild(o);
    }
    input.addEventListener('change', () => { state.options[f.key] = input.value; applyVisibility(); runValidate(); });
  } else if (f.type === 'int') {
    input = el('input'); input.type = 'number'; input.step = f.step || 1;
    if (f.min !== undefined) input.min = f.min;
    if (f.max !== undefined) input.max = f.max;
    input.value = cur;
    input.addEventListener('change', () => { state.options[f.key] = input.value === '' ? '' : Number(input.value); runValidate(); });
  } else if (f.type === 'float') {
    input = el('input'); input.type = 'number'; input.step = f.step || 'any';
    if (f.min !== undefined) input.min = f.min;
    if (f.max !== undefined) input.max = f.max;
    input.value = cur;
    input.addEventListener('change', () => { state.options[f.key] = input.value === '' ? '' : Number(input.value); runValidate(); });
  } else { // optional
    input = el('input'); input.type = 'text'; input.value = cur || '';
    input.placeholder = f.default || '';
    input.addEventListener('change', () => { state.options[f.key] = input.value.trim(); runValidate(); });
  }
  wrap.appendChild(input);
  if (f.help_en && f.help_ru) wrap.appendChild(el('div', 'help', F(f.help_en, f.help_ru)));
  return wrap;
}

// ---------- request ----------
function collectReq() {
  return {
    mode: state.mode,
    data_path: state.data_path,
    batch_path: state.batch_path,
    tables_kind: state.tables_kind,
    builtin_type: state.builtin_type,
    tables_dir: state.tables_dir,
    options: state.options,
    output: state.output,
  };
}

// ---------- validate ----------
let valTimer = null;
function runValidate() {
  if (!state.spec) return;
  clearTimeout(valTimer);
  valTimer = setTimeout(async () => {
    const req = collectReq();
    const resp = await postJSON(API.validate, req);
    const runBtn = document.getElementById('run-btn');
    const prev = document.getElementById('command-preview');
    if (resp.ok) {
      runBtn.disabled = false;
      prev.classList.remove('hidden');
      prev.textContent = t('cmd_preview') + ':\n' + resp.command;
      window.__lastCmd = resp.command;
      renderInput();
    } else {
      runBtn.disabled = true;
      prev.classList.remove('hidden');
      prev.textContent = t('invalid') + ': ' + resp.error;
      window.__lastError = resp.error;
    }
    applyVisibility();
  }, 250);
}

// ---------- run / cancel ----------
async function doRun() {
  const req = collectReq();
  const resp = await postJSON(API.jobs, req);
  if (!resp.id) {
    alert(resp.error || t('invalid'));
    return;
  }
  state.currentJob = resp.id;
  showRunView();
  openLogEvents(resp.id);
}

function showRunView() {
  document.getElementById('run-card').classList.remove('hidden');
  document.getElementById('result-card').classList.add('hidden');
  const log = document.getElementById('log');
  log.innerHTML = '<span class="spin"></span>' + t('running');
  document.getElementById('cancel-btn').classList.remove('hidden');
}

function openLogEvents(id) {
  const es = new EventSource('/api/jobs/' + id + '/events');
  const log = document.getElementById('log');
  log.innerHTML = '';
  es.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.type === 'log') {
      const div = el('div', msg.stream === 'err' ? 'err' : '', msg.text);
      log.appendChild(div);
      log.scrollTop = log.scrollHeight;
    } else if (msg.type === 'status') {
      es.close();
      onStatus(msg);
    }
  };
  es.onerror = () => { /* reconnect handled by EventSource */ };
}

function onStatus(msg) {
  document.getElementById('cancel-btn').classList.add('hidden');
  const log = document.getElementById('log');
  const p = el('div');
  p.innerHTML = '--- ' + t(msg.status) + (msg.exit_code ? ' (' + t('exit') + ' ' + msg.exit_code + ')' : '') +
    ' · ' + t('duration') + ' ' + (msg.duration_ms / 1000).toFixed(1) + 's ---';
  log.appendChild(p);
  if (msg.status === 'done') {
    loadResult(state.currentJob);
  } else if (msg.error) {
    const e = el('div', 'err', msg.error);
    log.appendChild(e);
  }
}

async function loadResult(id) {
  const resCard = document.getElementById('result-card');
  resCard.classList.remove('hidden');
  const res = document.getElementById('result');
  res.innerHTML = '<span class="spin"></span>' + t('running');
  const filesResp = await fetch('/api/jobs/' + id + '/files', { headers: { 'X-CSolve-Token': TOKEN } });
  const filesData = await filesResp.json();
  let result = null;
  if (state.mode === 'batch') {
    const rr = await fetch('/api/jobs/' + id + '/result', { headers: { 'X-CSolve-Token': TOKEN } });
    result = await rr.json();
  } else {
    const rr = await fetch('/api/jobs/' + id + '/result', { headers: { 'X-CSolve-Token': TOKEN } });
    result = await rr.json();
  }
  renderResult(result, filesData.files);
}

function renderResult(result, files) {
  const res = document.getElementById('result');
  res.innerHTML = '';
  // summary cards (single)
  const imgs = files.filter((f) => f.kind === 'image');
  const others = files.filter((f) => f.kind !== 'image');
  const charts = imgs.sort((a, b) => a.name < b.name ? -1 : 1);

  if (state.mode === 'batch' && result.tables) {
    const sec = el('div', 'two');
    const pan = el('div', 'pan');
    pan.appendChild(el('h3', null, t('batch')));
    pan.appendChild(tableOfBatch(result));
    sec.appendChild(pan);
    res.appendChild(sec);
  } else if (result.m_avg) {
    res.appendChild(cardsOf(result));
  }

  // images gallery
  if (charts.length) {
    const gal = el('div', 'gallery');
    for (const f of charts) {
      const fig = el('figure', null);
      const img = document.createElement('img');
      img.src = f.url; img.alt = f.name; img.loading = 'lazy';
      fig.appendChild(img);
      fig.appendChild(el('figcaption', null, f.name));
      gal.appendChild(fig);
    }
    res.appendChild(gal);
  }

  // tables
  if (state.mode !== 'batch') {
    if (result.m_avg) res.appendChild(tablesOfSingle(result));
  }

  // files / downloads
  if (others.length) {
    const sec = el('div', 'dl');
    sec.appendChild(el('h3', null, t('files')));
    const ul = el('ul', null);
    for (const f of others) {
      const li = el('li', null);
      const a = el('a', null, f.name);
      a.href = f.url; a.download = f.name;
      li.appendChild(a);
      ul.appendChild(li);
    }
    sec.appendChild(ul);
    res.appendChild(sec);
  }

  return res;
}

function cardsOf(r) {
  const row = el('div', 'metric-row');
  const m = r.moments;
  const mk = (key, label, fmt) => {
    const c = el('div', 'metric');
    c.appendChild(el('div', 'mv', fmt == null ? String(label) : fmt));
    c.appendChild(el('div', 'mk', key));
    return c;
  };
  let mstr = r.m_avg ? (r.m_avg.real + (r.m_avg.imag >= 0 ? '+' : '') + r.m_avg.imag + 'j') : '';
  row.appendChild(mk('m', mstr));
  if (m) {
    row.appendChild(mk('N, cm⁻³', m['N_cm-3'] == null ? '' : (+m['N_cm-3']).toExponential(3)));
    row.appendChild(mk('V, µm³/cm³', m.vol_conc_um3_per_cm3 == null ? '' : (+m.vol_conc_um3_per_cm3).toFixed(3)));
    row.appendChild(mk('r_eff, µm', m.r_eff_um == null ? '' : (+m.r_eff_um).toFixed(4)));
    row.appendChild(mk('cov, µm', (m.r_cov_min_um == null || m.r_cov_max_um == null) ? '' : m.r_cov_min_um + '–' + m.r_cov_max_um));
  }
  return row;
}

function tablesOfSingle(r) {
  const wrap = el('div', 'two');
  // channels
  if (r.channels && r.channels.length) {
    const pan = el('div', 'pan');
    pan.appendChild(el('h3', null, t('channels')));
    const table = el('table', null);
    const thead = el('tr', null);
    for (const h of ['λ, nm', 'kind', 'g1', 'g_fit']) thead.appendChild(el('th', null, h));
    table.appendChild(thead);
    for (const ch of r.channels) {
      const tr = el('tr', null);
      tr.appendChild(el('td', null, ch.wl_nm));
      tr.appendChild(el('td', null, ch.kind));
      tr.appendChild(el('td', null, fmtNum(ch.g1)));
      tr.appendChild(el('td', null, fmtNum(ch.g_fit)));
      table.appendChild(tr);
    }
    pan.appendChild(table);
    wrap.appendChild(pan);
  }
  // modes
  if (r.modes) {
    const pan = el('div', 'pan');
    pan.appendChild(el('h3', null, t('modes')));
    const table = el('table', null);
    const trh = el('tr', null);
    for (const h of ['mode', 'V', 'rm, µm', 'σ']) trh.appendChild(el('th', null, h));
    table.appendChild(trh);
    const names = [['fine', 'fine'], ['coarse', 'coarse'], ['overall', 'overall']];
    for (const [key, lbl] of names) {
      const mo = r.modes[key]; if (!mo) continue;
      const tr = el('tr', null);
      tr.appendChild(el('td', null, lbl));
      tr.appendChild(el('td', null, mo.V_um3_per_cm3 == null ? '' : (+mo.V_um3_per_cm3).toFixed(3)));
      tr.appendChild(el('td', null, mo.rm_um == null ? '' : (+mo.rm_um).toFixed(4)));
      tr.appendChild(el('td', null, mo.sigma == null ? '' : (+mo.sigma).toFixed(3)));
      table.appendChild(tr);
    }
    pan.appendChild(table);
    wrap.appendChild(pan);
  }
  // solutions (top 50)
  if (r.best_solutions && r.best_solutions.length) {
    const pan = el('div', 'pan');
    pan.appendChild(el('h3', null, t('solutions') + ' (top 50)'));
    const table = el('table', null);
    const thead = el('tr', null);
    for (const h of ['#', 'm', 'r0', 'r1', 'γ', 'Err']) thead.appendChild(el('th', null, h));
    table.appendChild(thead);
    for (const s of r.best_solutions.slice(0, 50)) {
      const tr = el('tr', null);
      tr.appendChild(el('td', null, s.rank));
      tr.appendChild(el('td', null, s.m_real + '+' + s.m_imag + 'j'));
      tr.appendChild(el('td', null, fmtNum(s.r0)));
      tr.appendChild(el('td', null, fmtNum(s.r1)));
      tr.appendChild(el('td', null, fmtNum(s.gamma)));
      tr.appendChild(el('td', null, fmtNum(s.err)));
      table.appendChild(tr);
    }
    pan.appendChild(table);
    wrap.appendChild(pan);
  }
  return wrap;
}

// Пояснения к столбцам сводки по высотам: [en, ru] — подпись, единица и
// полное описание (показывается во всплывающей подсказке).
const BATCH_COLINFO = {
  'Vf': { label: ['V, fine', 'V, мелк.'], unit: ['µm³/cm³', 'мкм³/см³'], desc: ['Volume concentration, fine mode', 'Объёмная концентрация, мелкая фракция'] },
  'Vc': { label: ['V, coarse', 'V, крупн.'], unit: ['µm³/cm³', 'мкм³/см³'], desc: ['Volume concentration, coarse mode', 'Объёмная концентрация, крупная фракция'] },
  'V': { label: ['V, total', 'V, общ.'], unit: ['µm³/cm³', 'мкм³/см³'], desc: ['Total volume concentration', 'Полная объёмная концентрация'] },
  'rmf': { label: ['r_m, fine', 'r_m, мелк.'], unit: ['µm', 'мкм'], desc: ['Mean radius, fine mode', 'Средний радиус, мелкая фракция'] },
  'rmc': { label: ['r_m, coarse', 'r_m, крупн.'], unit: ['µm', 'мкм'], desc: ['Mean radius, coarse mode', 'Средний радиус, крупная фракция'] },
  'rm': { label: ['r_m, total', 'r_m, общ.'], unit: ['µm', 'мкм'], desc: ['Mean radius, overall', 'Средний радиус, общий'] },
  'reff': { label: ['r_eff', 'r_эфф'], unit: ['µm', 'мкм'], desc: ['Effective radius', 'Эффективный радиус'] },
  'N': { label: ['N', 'N'], unit: ['cm⁻³', 'см⁻³'], desc: ['Number concentration', 'Счётная концентрация'] },
  'sigma_f': { label: ['σ, fine', 'σ, мелк.'], unit: ['', ''], desc: ['Distribution width (geometric σ), fine mode', 'Ширина распределения (геометрическая σ), мелкая фракция'] },
  'sigma_c': { label: ['σ, coarse', 'σ, крупн.'], unit: ['', ''], desc: ['Distribution width (geometric σ), coarse mode', 'Ширина распределения (геометрическая σ), крупная фракция'] },
  'sigma': { label: ['σ, total', 'σ, общ.'], unit: ['', ''], desc: ['Distribution width (geometric σ), overall', 'Ширина распределения (геометрическая σ), общая'] },
  'Re(m)': { label: ['Re m', 'Re m'], unit: ['', ''], desc: ['Real part of the refractive index', 'Действительная часть показателя преломления'] },
  'Im(m)': { label: ['Im m', 'Im m'], unit: ['', ''], desc: ['Imaginary part of the refractive index', 'Мнимая часть показателя преломления'] },
  'Err': { label: ['Err', 'Err'], unit: ['', ''], desc: ['Mean relative error over channels', 'Средняя относительная ошибка по каналам'] },
};

function batchColHeader(k, raw) {
  const c = BATCH_COLINFO[raw];
  if (c) {
    const u = F(c.unit[0], c.unit[1]);
    return { label: F(c.label[0], c.label[1]),
             title: F(c.desc[0], c.desc[1]) + (u ? ' [' + u + ']' : '') };
  }
  let m = /^delta_l_meas_(.+)$/.exec(raw);
  if (m) return { label: F('δ_l meas @ ' + m[1] + ' nm', 'δ_l изм. @ ' + m[1] + ' нм'),
                  title: F('Measured linear depolarization at 180°, %', 'Измеренная линейная деполяризация при 180°, %') };
  m = /^delta_l_(.+)$/.exec(raw);
  if (m) return { label: F('δ_l @ ' + m[1] + ' nm', 'δ_l @ ' + m[1] + ' нм'),
                  title: F('Linear depolarization at 180°, %', 'Линейная деполяризация при 180°, %') };
  m = /^S_(.+)$/.exec(raw);
  if (m) return { label: F('S @ ' + m[1] + ' nm', 'S @ ' + m[1] + ' нм'),
                  title: F('Lidar ratio S = α/β', 'Лидарное отношение S = α/β') };
  if (k === 'errors') return { label: raw, title: F('Relative error |g_fit-g1|/g1 for channel ' + raw, 'Относительная ошибка |g_fit-g1|/g1 для канала ' + raw) };
  return { label: raw, title: k + ': ' + raw };
}

function tableOfBatch(bs) {
  const wrap = el('div', 'batch-summary');
  const table = el('table', null);
  const tables = bs.tables || {};
  const keys = Object.keys(tables);
  const alts = bs.alt || [];
  if (!keys.length) {
    const tr = el('tr', null);
    tr.appendChild(el('td', null, t('err')));
    table.appendChild(tr);
    wrap.appendChild(table);
    return wrap;
  }

  // число столбцов данных в каждом файле
  const counts = {};
  for (const k of keys) {
    const col = tables[k] || [];
    counts[k] = Math.max(1, Math.round(col.length / Math.max(1, alts.length)));
  }

  // полоса групп: к какому блоку относится столбец
  const grp = el('tr', 'grp-row');
  const h0 = el('th', null, F('H, m', 'H, м'));
  h0.rowSpan = 2;
  grp.appendChild(h0);
  for (const k of keys) {
    const th = el('th', null, t('grp_' + k));
    th.colSpan = counts[k];
    grp.appendChild(th);
  }

  // имена столбцов из шапок файлов
  const hdrRow = el('tr', null);
  for (const k of keys) {
    const cols = (bs.columns && bs.columns[k]) || [];
    for (let j = 0; j < counts[k]; j++) {
      const raw = cols[j] != null ? cols[j] : (k + ' #' + (j + 1));
      const info = batchColHeader(k, raw);
      const th = el('th', null, info.label);
      th.title = info.title;
      hdrRow.appendChild(th);
    }
  }

  const thead = el('thead');
  thead.appendChild(grp);
  thead.appendChild(hdrRow);
  table.appendChild(thead);

  for (let i = 0; i < alts.length; i++) {
    const tr = el('tr', null);
    tr.appendChild(el('td', null, fmtNum(alts[i])));
    for (const k of keys) {
      const col = tables[k];
      const perH = counts[k];
      for (let j = 0; j < perH; j++) {
        const val = col[i * perH + j];
        tr.appendChild(el('td', null, val === undefined ? '' : fmtNum(val)));
      }
    }
    table.appendChild(tr);
  }
  wrap.appendChild(table);

  // легенда с единицами измерения из примечаний решателя
  for (const k of keys) {
    const note = bs.notes && bs.notes[k];
    if (!note) continue;
    wrap.appendChild(el('div', 'batch-note', t('grp_' + k) + ': ' + note));
  }
  return wrap;
}

// ---------- fs modal ----------
function openFSModal(cb) {
  const modal = document.getElementById('fs-modal');
  if (!modal) { buildFSModal(); }
  window.__fsCb = cb;
  const m = document.getElementById('fs-modal');
  m.classList.add('open');
  loadFS('');
}
function buildFSModal() {
  const modal = el('div', 'fs-modal');
  modal.id = 'fs-modal';
  modal.innerHTML = `
    <div class="fs-box">
      <div class="fs-bar">
        <input type="text" id="fs-path" placeholder="/path">
        <button id="fs-up" class="btn">↑</button>
        <button id="fs-select" class="btn primary">Select</button>
        <button id="fs-close" class="btn">✕</button>
      </div>
      <div class="fs-list" id="fs-list"></div>
    </div>`;
  document.body.appendChild(modal);
  modal.querySelector('#fs-close').addEventListener('click', () => { modal.classList.remove('open'); });
  modal.querySelector('#fs-up').addEventListener('click', () => {
    const p = document.getElementById('fs-path').value || '';
    loadFS(dirname(p));
  });
  modal.querySelector('#fs-select').addEventListener('click', () => {
    const p = document.getElementById('fs-path').value;
    modal.classList.remove('open');
    window.__fsCb(p);
  });
  modal.querySelector('#fs-path').addEventListener('change', () => loadFS(modal.querySelector('#fs-path').value));
}
async function loadFS(path) {
  const list = document.getElementById('fs-list');
  const resp = await fetch('/api/fs?path=' + encodeURIComponent(path), { headers: { 'X-CSolve-Token': TOKEN } });
  const data = await resp.json();
  document.getElementById('fs-path').value = data.path || path;
  list.innerHTML = '';
  if (data.error) { list.appendChild(el('div', 'err', data.error)); return; }
  for (const e of data.entries) {
    const row = el('div', 'fs-row');
    const name = el('span', null, (e.dir ? '📁 ' : '📄 ') + e.name);
    const sz = el('span', 'pd', e.dir ? '' : sizeOf(e.size));
    row.append(name, sz);
    row.addEventListener('click', () => {
      if (e.dir) { loadFS(e.path); }
      else { window.__fsCb(e.path); document.getElementById('fs-modal').classList.remove('open'); }
    });
    list.appendChild(row);
  }
}
function dirname(p) {
  p = (p || '').replace(/[\\/]+$/, '');
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'));
  return i < 0 ? '' : p.slice(0, i);
}
function sizeOf(n) {
  if (n >= 1048576) return (n / 1048576).toFixed(1) + 'M';
  if (n >= 1024) return (n / 1024).toFixed(0) + 'K';
  return n + 'B';
}

// ---------- upload ----------
function pickUpload(accept, cb) {
  const inp = document.createElement('input');
  inp.type = 'file';
  inp.accept = accept;
  inp.onchange = async () => {
    if (!inp.files.length) return;
    const fd = new FormData();
    fd.append('files', inp.files[0]);
    const resp = await fetch(API.upload, { method: 'POST', body: fd, headers: { 'X-CSolve-Token': TOKEN } });
    const data = await resp.json();
    if (data.paths && data.paths.length) cb(data.paths[0]);
    else alert('upload failed');
  };
  inp.click();
}

// ---------- helpers ----------
async function postJSON(url, body) {
  const resp = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-CSolve-Token': TOKEN },
    body: JSON.stringify(body),
  });
  return resp.json();
}
function fmtNum(v) {
  if (v == null) return '';
  if (typeof v === 'number' && isNaN(v)) return '';
  if (Math.abs(v) >= 1e3 || (Math.abs(v) < 1e-3 && v !== 0)) return (+v).toExponential(3);
  return (+v).toFixed(4);
}

function bindLang() {
  document.getElementById('lang-btn').addEventListener('click', () => {
    L = L === 'en' ? 'ru' : 'en';
    renderAll();
  });
  document.getElementById('show-advanced').addEventListener('change', renderGroups);
  document.getElementById('run-btn').addEventListener('click', doRun);
  document.getElementById('validate-btn').addEventListener('click', () => { window.clearTimeout(valTimer); runValidate(); });
  document.getElementById('cancel-btn').addEventListener('click', () => {
    if (state.currentJob) postJSON('/api/jobs/' + state.currentJob + '/cancel', {});
  });
  document.getElementById('copy-cmd').addEventListener('click', () => {
    if (window.__lastCmd) navigator.clipboard.writeText(window.__lastCmd);
  });
}

boot();