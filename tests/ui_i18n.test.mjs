// UI / i18n tests for cmd/llamacpp-launcher/ui/index.html
// Runs the page's <script> in a sandbox with a mocked DOM/localStorage/navigator/fetch,
// then exercises the i18n logic (default EN, language dropdown, fallback, extensibility)
// and the dynamic-UI logic (all --help flags rendered by kind, live language switch,
// the top-right settings menu, and config save / preset application).
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const UI = join(ROOT, 'cmd', 'llamacpp-launcher', 'ui');
const html = readFileSync(join(UI, 'index.html'), 'utf8');
const i18nJS = readFileSync(join(UI, 'i18n.js'), 'utf8');
const appJS = readFileSync(join(UI, 'app.js'), 'utf8');

let failures = 0;
function check(name, cond) {
  if (cond) console.log('  ok  - ' + name);
  else { console.error('  FAIL - ' + name); failures++; }
}
function group(name) { console.log('\n# ' + name); }

// ---- static checks on the source ----
group('static HTML assertions');
check('default lang="en"', /<html lang="en">/.test(html));
check('has language dropdown', /<select[^>]*id="lang"/.test(html));
check('EN option present', /<option value="en">EN<\/option>/.test(html));
check('RU option present', /<option value="ru">RU<\/option>/.test(html));
check('no inline <style>', !/<style/i.test(html));
const scriptTags = html.match(/<script([^>]*)>/gi) || [];
check('no inline <script>', scriptTags.length > 0 && scriptTags.every(t => /src=/.test(t)));
check('links styles.css', /<link[^>]*href="styles\.css"/.test(html));
check('loads i18n.js', /<script[^>]*src="i18n\.js"/.test(html));
check('loads app.js', /<script[^>]*src="app\.js"/.test(html));
check('uses declarative data-i18n', /data-i18n(-[a-z]+)?=/.test(html));
check('settings gear button present', /id="btn-settings"/.test(html));
check('settings pop-in panel present', /id="settings-pop"/.test(html));
check('no hardcoded PARAMS array', !/\bPARAMS\s*=/.test(html));

group('static JS assertions');
check('i18n exposes t()', /function t\(/.test(i18nJS));
check('i18n exposes setLang()', /function setLang\(/.test(i18nJS));
check('i18n exposes applyI18n()', /function applyI18n\(/.test(i18nJS));
check('i18n exposes I18N table', /I18N\s*=/.test(i18nJS));
check('app loads flags from /api/flags', /function loadFlags\(/.test(appJS) && /\/api\/flags/.test(appJS));

// ---- a compact real DOM tree so querySelectorAll / dataset / checked work ----
class El {
  constructor(tag) {
    this.tagName = (tag || 'div').toUpperCase();
    this.children = [];
    this.parentNode = null;
    this.attributes = {};
    this.dataset = {};
    this._classes = [];
    this.value = '';
    this.checked = false;
    this.selected = false;
    this.textContent = '';
    this.innerHTML = '';
    this.title = '';
    this.style = {};
    this._listeners = {};
  }
  get classList() {
    const self = this;
    const list = {
      add: (...cs) => cs.forEach(c => { if (!self._classes.includes(c)) self._classes.push(c); }),
      remove: (...cs) => { self._classes = self._classes.filter(c => !cs.includes(c)); },
      contains: (c) => self._classes.includes(c),
      toggle: (c, on) => {
        const want = on === undefined ? !self._classes.includes(c) : !!on;
        if (want) list.add(c); else list.remove(c);
      },
    };
    return list;
  }
  get className() { return this._classes.join(' '); }
  set className(v) { this._classes = String(v).split(/\s+/).filter(Boolean); }
  getAttribute(n) { return n in this.attributes ? this.attributes[n] : null; }
  setAttribute(n, v) { this.attributes[n] = String(v); if (String(n).startsWith('data-')) this.dataset[n.slice(5)] = String(v); }
  hasAttribute(n) { return n in this.attributes; }
  removeAttribute(n) { delete this.attributes[n]; }
  get innerHTML() { return this._html; }
  set innerHTML(v) {
    this._html = String(v);
    this.children = [];
  }
  appendChild(c) { c.parentNode = this; this.children.push(c); return c; }
  contains(node) {
    if (node === this) return true;
    return this.children.some(c => c.contains && c.contains(node));
  }
  match(sel) {
    sel = (sel || '').trim();
    let m = sel.match(/^\[([a-zA-Z0-9_-]+)\]$/);
    if (m) return this.hasAttribute(m[1]);
    m = sel.match(/^\[([a-zA-Z-]+)="([^"]*)"\]$/);
    if (m) return this.getAttribute(m[1]) === m[2];
    if (sel.startsWith('.')) return this.classList.contains(sel.slice(1));
    if (sel === '*') return true;
    if (sel) return this.tagName === sel.toUpperCase();
    return true;
  }
  querySelectorAll(sel) {
    const out = [];
    const rec = (el) => { el.children.forEach(c => { if (c.match && c.match(sel)) out.push(c); rec(c); }) };
    rec(this);
    return out;
  }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  addEventListener(ev, fn) { (this._listeners[ev] = this._listeners[ev] || []).push(fn); }
  dispatchEvent(ev) { (this._listeners[ev.type] || []).forEach(fn => fn(ev)); }
}

function staticElementAttrs(html) {
  const map = new Map();
  const body = html.replace(/\n/g, ' ');
  const tagRe = /<([a-zA-Z0-9]+)\b([^>]*)>/g;
  let tm;
  while ((tm = tagRe.exec(body))) {
    if (tm[1] === 'script' || tm[1] === 'style') continue;
    const attrStr = tm[2];
    const idm = attrStr.match(/\bid="([^"]*)"/);
    if (!idm) continue;
    const attrs = {};
    const ar = /([a-zA-Z0-9_:.-]*)\s*=\s*"([^"]*)"/g;
    let am;
    while ((am = ar.exec(attrStr))) attrs[am[1]] = am[2];
    map.set(idm[1], { tag: tm[1].toLowerCase(), attrs });
  }
  return map;
}
function makeRegistry(html) {
  const statics = staticElementAttrs(html);
  const map = new Map();
  statics.forEach((s, id) => {
    const el = new El(s ? s.tag : 'div');
    if (s.attrs.class) el._classes.push(...s.attrs.class.split(/\s+/).filter(Boolean));
    for (const [k, v] of Object.entries(s.attrs)) if (k !== 'class') el.setAttribute(k, v);
    map.set(id, el);
  });
  const all = (sel) => [...map.values()].filter(el => el.match && el.match(sel));
  return {
    documentElement: new El('html'),
    getElementById: (id) => map.get(id) || new El('div'),
    createElement: (tag) => new El(tag),
    addEventListener: () => {},
    querySelectorAll: (sel) => all(sel),
    querySelector: (sel) => all(sel)[0] || null,
    _map: map,
  };
}

const FLAGS_MOCK = [
  { name: '--threads', kind: 'value', default: '-1', desc: 'number of CPU threads' },
  { name: '--flash-attn', kind: 'enum', choices: ['on', 'off', 'auto'], default: 'auto', desc: 'Flash Attention' },
  { name: '--perf', kind: 'toggle', default: 'false', desc: 'whether to enable perf' },
];
const PRESETS_MOCK = [
  { name: 'fast', title: 'Fast', desc: 'quick', flags: { '--threads': '64', '--flash-attn': 'on', '--perf': 'true' } },
];

function runApp(navigatorLang) {
  const document = makeRegistry(html);
  const store = {};
  const state = { lastPost: null, models: [] };
  const fetchImpl = async (url, opts) => {
    const post = opts && opts.method === 'POST';
    if (post || String(url).endsWith('/api/config')) {
      let body = {};
      if (opts && opts.body) { try { body = JSON.parse(opts.body); } catch (e) {} }
      state.lastPost = body;
      return { status: 200, text: async () => JSON.stringify({ ok: true, path: '/x.conf' }), json: async () => ({ ok: true }) };
    }
    const j = () => ({ status: 200, text: async () => '{}', json: async () => ({}) });
    if (String(url).includes('/api/flags')) return { status: 200, text: async () => JSON.stringify(FLAGS_MOCK), json: async () => FLAGS_MOCK };
    if (String(url).includes('/api/settings')) return { status: 200, text: async () => JSON.stringify({ webHost: '127.0.0.1', webPort: '8080', boundAddr: 'http://127.0.0.1:8080' }), json: async () => ({ webHost: '127.0.0.1', webPort: '8080', boundAddr: 'http://127.0.0.1:8080' }) };
    if (String(url).includes('/api/models')) return { status: 200, text: async () => JSON.stringify({ models: state.models || [] }), json: async () => ({ models: state.models || [] }) };
    if (String(url).includes('/api/status')) return { status: 200, text: async () => JSON.stringify({ ActiveState: 'inactive', SubState: 'inactive' }), json: async () => ({ ActiveState: 'inactive', SubState: 'inactive' }) };
    if (String(url).includes('/api/presets')) return { status: 200, text: async () => JSON.stringify(PRESETS_MOCK), json: async () => PRESETS_MOCK };
    return j();
  };
  const sandbox = {
    document,
    localStorage: {
      getItem: (k) => (k in store ? store[k] : null),
      setItem: (k, v) => { store[k] = String(v); },
      removeItem: (k) => { delete store[k]; },
    },
    navigator: navigatorLang === undefined ? undefined : { language: navigatorLang },
    fetch: fetchImpl,
    console,
    setTimeout,
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  const code = i18nJS + '\n' + appJS;
  vm.runInContext(code + '\n;globalThis.__api={t:t,setLang:setLang,loadLang:loadLang,applyI18n:applyI18n,I18N:I18N,LANGS:LANGS,loadFlags:loadFlags,buildForm:buildForm,makeControl:makeControl,onPreset:onPreset,saveCfg:saveCfg,controlValue:controlValue,refreshStatus:refreshStatus,renderStatus:renderStatus,setStatus:(s)=>{STATUS=s},setCurrent:(c)=>{CURRENT=c},setConfig:(c)=>{CONFIG=c},toggleSettings:toggleSettings,closeSettings:closeSettings,renderModels:renderModels,selectModel:selectModel,loadModels:loadModels,loadPresets:loadPresets,getFlags:()=>FLAGS};', sandbox);
  return { sandbox, store, api: sandbox.__api, state, document };
}

// ---- original i18n groups (kept unchanged in behaviour) ----
group('i18n runtime (default EN)');
{
  const { sandbox } = runApp('en');
  const t = sandbox.__api.t;
  check('default language is EN', sandbox.__api.LANGS.includes('en'));
  check("t('search') = 'Search models'", t('search') === 'Search models');
  check("t('h_service') = 'Service logs'", t('h_service') === 'Service logs');
  check("t('l_numa') = 'NUMA strategy'", t('l_numa') === 'NUMA strategy');
  check('document lang set to en', sandbox.document.documentElement.lang === 'en');
  check('search placeholder translated', sandbox.document.getElementById('search').placeholder === 'Search models');
  check('empty notice translated', sandbox.document.getElementById('empty').textContent === 'No models available. Place a .gguf file in the model directory.');
  check('lang select value = en', sandbox.document.getElementById('lang').value === 'en');
}

group('i18n runtime (switch to RU)');
{
  const { sandbox, store } = runApp('en');
  sandbox.__api.setLang('ru');
  const t = sandbox.__api.t;
  check("t('search') switches to RU", t('search') === 'Поиск моделей');
  check("t('h_service') switches to RU", t('h_service') === 'Логи службы');
  check("t('l_numa') switches to RU", t('l_numa') === 'NUMA стратегия');
  check('language persisted to localStorage', store['lang'] === 'ru');
  check('lang select reflects RU', sandbox.document.getElementById('lang').value === 'ru');
  check('search placeholder updated on switch', sandbox.document.getElementById('search').placeholder === 'Поиск моделей');
  check('service heading updated on switch', sandbox.document.getElementById('h-service-logs').textContent === 'Логи службы');
}

group('i18n runtime (fallbacks & extensibility)');
{
  const { sandbox } = runApp('en');
  const t = sandbox.__api.t;
  check('unknown key falls back to the key itself', t('definitely_not_a_real_key') === 'definitely_not_a_real_key');
  check('I18N extensible (can add new language entries)', sandbox.__api.I18N.en && sandbox.__api.I18N.ru);
  check('invalid language code falls back to EN', (() => { sandbox.__api.setLang('fr'); return t('search') === 'Search models'; })());
}

group('i18n runtime (navigator-based default)');
{
  const { sandbox } = runApp('ru');
  check('navigator lang=ru becomes default', sandbox.__api.t('search') === 'Поиск моделей');
}

// ---- new dynamic-UI groups ----
group('flags loaded from /api/flags');
{
  const { api, document } = runApp('en');
  const loaded = await api.loadFlags();
  check('loadFlags returns the flag list', Array.isArray(loaded) && loaded.length === FLAGS_MOCK.length);
  check('getFlags mirrors the module state', api.getFlags().length === FLAGS_MOCK.length);
  check('flag shape has name/kind/default', loaded.every(f => f.name && f.kind && ('default' in f)));
}

group('buildForm renders one control per flag by kind');
{
  const { api, document } = runApp('en');
  await api.loadFlags();
  const cfg = { model: '/m.gguf', flags: { '--threads': '8', '--flash-attn': 'off', '--perf': 'on' } };
  await api.buildForm(cfg);
  const box = document.getElementById("form");
  const controls = box.querySelectorAll('[data-key]');
  check('one control per flag', controls.length === FLAGS_MOCK.length);
  const byKey = {};
  controls.forEach(c => { byKey[c.getAttribute('data-key')] = c; });
  check('toggle -> checkbox', byKey['--perf'].tagName === 'INPUT' && byKey['--perf'].type === 'checkbox');
  check('toggle checked when "on"', byKey['--perf'].checked === true);
  check('enum -> select', byKey['--flash-attn'].tagName === 'SELECT');
  const opts = byKey['--flash-attn'].children.map(o => o.value);
  check('enum options include all choices', opts.includes('on') && opts.includes('off') && opts.includes('auto'));
  check('enum selected value follows config', byKey['--flash-attn'].value === 'off');
  check('value -> text input', byKey['--threads'].tagName === 'INPUT' && byKey['--threads'].value === '8');
}

group('buildForm falls back to flag default when not in config');
{
  const { api, document } = runApp('en');
  await api.loadFlags();
  await api.buildForm({ model: '/m.gguf', flags: {} });
  const box = document.getElementById("form");
  const controls = box.querySelectorAll('[data-key]');
  const byKey = {};
  controls.forEach(c => { byKey[c.getAttribute('data-key')] = c; });
  check('value default applied', byKey['--threads'].value === '-1');
  check('enum default applied', byKey['--flash-attn'].value === 'auto');
  check('toggle default false -> unchecked', byKey['--perf'].checked === false);
}

group('saveCfg collects every control type and converts numbers');
{
  const { api, state, document } = runApp('en');
  api.setCurrent({ path: '/m.gguf' });
  await api.loadFlags();
  const cfg = { model: '/m.gguf', flags: { '--threads': '8', '--flash-attn': 'off', '--perf': 'on' } };
  await api.buildForm(cfg);
  const box = document.getElementById("form");
  const byKey = {};
  box.querySelectorAll('[data-key]').forEach(c => { byKey[c.getAttribute('data-key')] = c; });
  byKey['--threads'].value = '16';
  byKey['--flash-attn'].value = 'auto';
  byKey['--perf'].checked = false;
  await api.saveCfg();
  check('saveCfg sent the model', state.lastPost && state.lastPost.model === '/m.gguf');
  check('numeric flag converted to number', state.lastPost.flags['--threads'] === 16);
  check('enum value kept as string', state.lastPost.flags['--flash-attn'] === 'auto');
  check('toggle off sent as "off"', state.lastPost.flags['--perf'] === 'off');
}

group('onPreset applies preset values into the form');
{
  const { api, document } = runApp('en');
  await api.loadFlags();
  await api.loadPresets();
  const cfg = { model: '/m.gguf', flags: {} };
  await api.buildForm(cfg);
  const sel = document.getElementById('presets');
  sel.value = 'fast';
  await api.onPreset();
  const box = document.getElementById("form");
  const byKey = {};
  box.querySelectorAll('[data-key]').forEach(c => { byKey[c.getAttribute('data-key')] = c; });
  check('onPreset sets threads', byKey['--threads'].value === '64');
  check('onPreset sets enum', byKey['--flash-attn'].value === 'on');
  check('onPreset sets toggle (true)', byKey['--perf'].checked === true);
}

group('live language switch re-renders dynamic content');
{
  const { api, state, document } = runApp('en');
  state.models = []; // force the "no models" branch
  await api.loadModels();
  const box = document.getElementById('models');
  check('no_models shown in EN', box.innerHTML.includes('No models found'));
  api.setLang('ru');
  check('no_models re-rendered in RU on switch', box.innerHTML.includes('Модели не найдены'));
  check('flags heading translated to RU', document.getElementById('h_flags').textContent === 'Флаги');
  check('presets label translated to RU', document.getElementById('presets-label').textContent === 'Пресеты');
  check('settings heading translated to RU', document.getElementById('h-settings').textContent === 'Настройки');
  check('settings hint translated to RU', document.getElementById('settings-hint').textContent === 'Адрес и порт веб-интерфейса');
}

group('status label updates on language switch');
{
  const { api, document } = runApp('en');
  api.setLang('en');
  check('status label EN before refresh', document.getElementById('state').textContent === 'Unknown');
  api.setLang('ru');
  check('status label RU after refresh', document.getElementById('state').textContent === 'Unknown');
  api.setStatus({ ActiveState: 'active', SubState: 'running', MainPID: '5' });
  api.renderStatus();
  check('status label EN running', document.getElementById('state').textContent === 'Running');
  api.setLang('ru');
  api.renderStatus();
  check('status label RU running', document.getElementById('state').textContent === 'Running');
}

group('settings menu toggle (top-right gear)');
{
  const { api, document } = runApp('en');
  const pop = document.getElementById('settings-pop');
  check('panel starts hidden', pop.classList.contains('hidden'));
  api.toggleSettings(null);
  check('panel opens on first click', !pop.classList.contains('hidden'));
  api.toggleSettings(null);
  check('panel closes on second click', pop.classList.contains('hidden'));
  api.toggleSettings(null);
  const outside = document.createElement('div');
  api.closeSettings({ target: outside });
  check('panel closes on outside click', pop.classList.contains('hidden'));
  api.toggleSettings(null);
  const btn = document.getElementById('btn-settings');
  api.closeSettings({ target: btn });
  check('panel stays open when clicking the gear', !pop.classList.contains('hidden'));
}

check('PARAMS no longer defined in page source', !/const PARAMS|let PARAMS|var PARAMS/.test(html));

console.log('\n' + (failures === 0 ? 'ALL UI/I18N TESTS PASSED' : failures + ' TEST(S) FAILED'));
process.exit(failures === 0 ? 0 : 1);
