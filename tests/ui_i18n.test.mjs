// UI / i18n tests for cmd/llamacpp-launcher/ui/index.html
// Runs the page's <script> in a sandbox with a mocked DOM/localStorage/navigator/fetch,
// then exercises the i18n logic (default EN, language dropdown, fallback, extensibility).
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');
const html = readFileSync(join(ROOT, 'cmd', 'llamacpp-launcher', 'ui', 'index.html'), 'utf8');

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
check('script exposes t()', /function t\(/.test(html));
check('script exposes setLang()', /function setLang\(/.test(html));
check('script exposes I18N table', /I18N\s*=/.test(html));

// ---- dynamic checks in a sandbox ----
function makeEl(id) {
  return {
    id, value: '', textContent: '', innerHTML: '', placeholder: '', lang: '', style: {},
    classList: { add() {}, remove() {} },
    appendChild() {}, setAttribute() {}, addEventListener() {},
  };
}

function runI18n(navigatorLang) {
  const els = new Map();
  const el = (id) => { if (!els.has(id)) els.set(id, makeEl(id)); return els.get(id); };
  const store = {};
  const sandbox = {
    document: {
      documentElement: { lang: '' },
      getElementById: (id) => el(id),
      querySelector: (sel) => (sel === 'main h2' ? el('__h2') : null),
    },
    localStorage: {
      getItem: (k) => (k in store ? store[k] : null),
      setItem: (k, v) => { store[k] = String(v); },
      removeItem: (k) => { delete store[k]; },
    },
    navigator: navigatorLang === undefined ? undefined : { language: navigatorLang },
    // resolve fast so the async init() completes without network
    fetch: async () => ({ status: 200, text: async () => '{"models":[]', json: async () => ({}) }),
    console,
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  const code = html.split('<script>')[1].split('</script>')[0];
  vm.runInContext(code + '\n;globalThis.__api={t:t,setLang:setLang,loadLang:loadLang,renderI18n:renderI18n,I18N:I18N,LANGS:LANGS};', sandbox);
  return { sandbox, store };
}

group('i18n runtime (default EN)');
{
  const { sandbox } = runI18n('en');
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
  const { sandbox, store } = runI18n('en');
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
  const { sandbox } = runI18n('en');
  const t = sandbox.__api.t;
  check('unknown key falls back to the key itself', t('definitely_not_a_real_key') === 'definitely_not_a_real_key');
  check('I18N extensible (can add new language entries)', sandbox.__api.I18N.en && sandbox.__api.I18N.ru);
  check('invalid language code falls back to EN', (() => { sandbox.__api.setLang('fr'); return t('search') === 'Search models'; })());
}

group('i18n runtime (navigator-based default)');
{
  const { sandbox } = runI18n('ru');
  check('navigator lang=ru becomes default', sandbox.__api.t('search') === 'Поиск моделей');
}

console.log('\n' + (failures === 0 ? 'ALL UI/I18N TESTS PASSED' : failures + ' TEST(S) FAILED'));
process.exit(failures === 0 ? 0 : 1);
