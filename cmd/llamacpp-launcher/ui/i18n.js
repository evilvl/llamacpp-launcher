// ---- i18n ---------------------------------------------------------------
// Declarative localization. A string is translated by putting a
// `data-i18n="key"` (or `data-i18n-placeholder` / `data-i18n-html` /
// `data-i18n-title`) attribute on the element; `applyI18n()` then walks the
// document and fills every translatable node. Extensible: add a new language
// with an `I18N.<code>` object. Missing keys fall back to English, then to
// the key itself.
const I18N = {
  en: {
    search: "Search models",
    empty_notice: "No models available. Place a .gguf file in the model directory.",
    no_models: "No models found",
    l_presets: "Presets",
    badge_config: "config",
    badge_none: "no config",
    l_host: "Bind address",
    l_port: "HTTP port",
    l_api_key: "API key",
    l_gpu_layers: "GPU layers",
    l_wait_timeout: "Health wait (s)",
    l_context_size: "Context (tokens)",
    l_parallel: "Parallel sequences",
    l_batch_size: "Batch size",
    l_ubatch_size: "Unpadded batch",
    l_fit: "Fit to VRAM",
    l_fit_target: "VRAM margin (MiB)",
    l_fit_ctx: "Fit context (tokens)",
    l_cache_k: "KV cache type (K)",
    l_cache_v: "KV cache type (V)",
    l_flash_attn: "Flash attention",
    l_numa: "NUMA strategy",
    l_cpu_threads: "CPU threads (-t)",
    l_cpu_threads_batch: "CPU batch threads (-tb)",
    btn_generate: "▶ Generate & Start",
    btn_save: "💾 Save config",
    btn_start: "🚀 Start",
    btn_logs: "⇩ Logs",
    btn_infer: "✓ Inference test",
    btn_restart: "⟳ Restart",
    btn_stop: "■ Stop",
    h_service: "Service logs",
    l_server_settings: "Server settings",
    l_server_url: "URL",
    l_web_host: "Web host",
    l_web_port: "Web port",
    btn_save_settings: "Save &amp; restart",
    settings_menu: "Settings",
    settings_hint: "Web server host and port",
    h_flags: "Flags",
    s_restarting: "Restarting…",
    s_running: "Running",
    s_stopped: "Stopped",
    s_failed: "Failed",
    s_unknown: "Unknown",
  },
  ru: {
    search: "Поиск моделей",
    empty_notice: "Нет доступных моделей. Положите .gguf в модельный каталог.",
    no_models: "Модели не найдены",
    l_presets: "Пресеты",
    badge_config: "config",
    badge_none: "no config",
    l_host: "Адрес привязки",
    l_port: "Порт HTTP-сервера",
    l_api_key: "API-ключ",
    l_gpu_layers: "Слои GPU",
    l_wait_timeout: "Ожидание /health, сек",
    l_context_size: "Контекст, токены",
    l_parallel: "Параллельные последовательности",
    l_batch_size: "Batch size",
    l_ubatch_size: "Unpadded batch",
    l_fit: "Fit в VRAM",
    l_fit_target: "Запас VRAM, MiB",
    l_fit_ctx: "Контекст fit, токены",
    l_cache_k: "Кэш ключей",
    l_cache_v: "Кэш значений",
    l_flash_attn: "Flash attention",
    l_numa: "NUMA стратегия",
    l_cpu_threads: "Потоки CPU (-t)",
    l_cpu_threads_batch: "Потоки CPU для батча (-tb)",
    btn_generate: "▶ Generate & Start",
    btn_save: "💾 Save config",
    btn_start: "🚀 Start",
    btn_logs: "⇩ Логи",
    btn_infer: "✓ Inference test",
    btn_restart: "⟳ Restart",
    btn_stop: "■ Stop",
    h_service: "Логи службы",
    l_server_settings: "Настройки сервера",
    l_server_url: "Адрес",
    l_web_host: "Адрес веб-интерфейса",
    l_web_port: "Порт веб-интерфейса",
    btn_save_settings: "Сохранить и restart",
    settings_menu: "Настройки",
    settings_hint: "Адрес и порт веб-интерфейса",
    h_flags: "Флаги",
    s_restarting: "Перезапуск…",
    s_running: "Running",
    s_stopped: "Stopped",
    s_failed: "Failed",
    s_unknown: "Unknown",
  },
};
const LANGS = ["en", "ru"];
let curLang = "en";

function loadLang() {
  const saved = localStorage.getItem("lang");
  const nav = (navigator.language || "").slice(0, 2).toLowerCase();
  if (saved && LANGS.includes(saved)) curLang = saved;
  else if (nav && LANGS.includes(nav)) curLang = nav;
  else curLang = "en";
}
function setLang(l) {
  if (!LANGS.includes(l)) l = "en";
  curLang = l;
  localStorage.setItem("lang", l);
  applyI18n();
}
function t(k) {
  return (I18N[curLang] && I18N[curLang][k]) || I18N.en[k] || k;
}
function applyI18n() {
  document.documentElement.lang = curLang;
  const sel = document.getElementById("lang");
  if (sel) sel.value = curLang;
  document.querySelectorAll("[data-i18n]").forEach(el => { el.textContent = t(el.getAttribute('data-i18n')); });
  document.querySelectorAll("[data-i18n-html]").forEach(el => { el.innerHTML = t(el.getAttribute('data-i18n-html')); });
  document.querySelectorAll("[data-i18n-placeholder]").forEach(el => { el.placeholder = t(el.getAttribute('data-i18n-placeholder')); });
  document.querySelectorAll("[data-i18n-title]").forEach(el => { el.title = t(el.getAttribute('data-i18n-title')); });
  renderModels();
  renderStatus();
}
// -------------------------------------------------------------------------
