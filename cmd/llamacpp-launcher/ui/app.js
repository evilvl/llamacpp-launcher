let MODELS=[], CURRENT=null, CONFIG=null, STATUS={}, FLAGS=[];

async function api(url,opts){const r=await fetch(url,opts);const t=await r.text();try{return JSON.parse(t)}catch(e){return{text:t}}}
async function jget(url){const r=await api(url);return r}
async function jpost(url,body){return api(url,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body||{})})}

async function loadFlags(){ FLAGS = (await jget("/api/flags")) || []; return FLAGS; }

function isTrueish(v) {
  switch (String(v).toLowerCase()) {
    case "1": case "true": case "on": case "yes": return true;
    default: return false;
  }
}

function makeControl(flag, v) {
  const name = flag.name;
  const title = flag.desc ? (name + ": " + flag.desc) : name;
  if (flag.kind === "toggle") {
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.setAttribute("data-key", name);
    cb.checked = isTrueish(v);
    cb.title = title;
    return cb;
  }
  if (flag.kind === "enum") {
    const sel = document.createElement("select");
    sel.setAttribute("data-key", name);
    sel.title = title;
    const choices = (flag.choices && flag.choices.length) ? flag.choices : [String(v)];
    const uniq = [...new Set(choices.concat([v]).filter(x => x !== undefined && x !== null && x !== ""))];
    uniq.forEach(o => {
      const op = document.createElement("option");
      op.value = o; op.textContent = o;
      sel.appendChild(op);
    });
    sel.value = choices.includes(String(v)) ? String(v) : (uniq[0] ?? "");
    return sel;
  }
  const inp = document.createElement("input");
  inp.type = "text";
  inp.setAttribute("data-key", name);
  inp.value = (v === undefined || v === null) ? "" : String(v);
  inp.title = title;
  return inp;
}

async function buildForm(cfg){
  const box=document.getElementById("form");box.innerHTML="";
  FLAGS.forEach(flag=>{
    const key=flag.name;
    let v;
    if (cfg && cfg.flags && Object.prototype.hasOwnProperty.call(cfg.flags, key)) {
      v = cfg.flags[key];
    } else if (flag.default !== undefined && flag.default !== null && flag.default !== "") {
      v = flag.default;
    } else {
      v = "";
    }
    const f=document.createElement("div");f.className="field";
    const desc = flag.desc ? ` <span class="notice">${esc(flag.desc)}</span>` : "";
    f.innerHTML=`<label>${esc(key)}${desc}</label>`;
    const ctrl = makeControl(flag, v);
    box.appendChild(f);
    f.appendChild(ctrl);
  });
}

let PRESETS=[];
async function loadPresets(){
  const r=await jget("/api/presets");
  PRESETS=Array.isArray(r)?r:(r.presets||[]);
  const sel=document.getElementById("presets");
  sel.innerHTML='<option value="">—</option>';
  PRESETS.forEach(p=>{
    const o=document.createElement("option");
    o.value=p.name; o.textContent=p.title; o.title=p.desc;
    sel.appendChild(o);
  });
}
function onPreset(){
  const sel=document.getElementById("presets");
  const p=PRESETS.find(x=>x.name===sel.value);
  if(!p)return;
  const form=document.getElementById("form");
  form.querySelectorAll("[data-key]").forEach(el=>{
    const k = el.getAttribute("data-key") || el.dataset.key;
    if (!p.flags || !(k in p.flags)) return;
    const v = p.flags[k];
    if (el.tagName === "INPUT" && el.type === "checkbox") el.checked = isTrueish(v);
    else el.value = v;
  });
}
function controlValue(el){
  if (el.tagName === "INPUT" && el.type === "checkbox") return el.checked ? "on" : "off";
  return el.value;
}
async function saveCfg(){
  if(!CURRENT)return;
  const flags={};
  const form=document.getElementById("form");
  form.querySelectorAll("[data-key]").forEach(el=>{
    const k = el.getAttribute("data-key") || el.dataset.key;
    const v = controlValue(el);
    flags[k] = /^-?\d+$/.test(v) ? parseInt(v,10) : v;
  });
  const r=await jpost("/api/config",{model:CURRENT.path, flags:flags});
  flash(JSON.stringify(r).slice(0,200));
  loadModels();
}
async function doStart(){
  if(!CURRENT)return;
  await saveCfg();
  const r=await jpost("/api/start?model="+encodeURIComponent(CURRENT.path));
  flash(JSON.stringify(r).slice(0,400));
  refreshStatus();
}
async function doAction(action){
  const q=CURRENT?("?model="+encodeURIComponent(CURRENT.path)):"";
  const r=await jpost("/api/"+action+q);
  flash(JSON.stringify(r).slice(0,300));
  refreshStatus();
}
async function refreshLogs(){
  const r=await jget("/api/logs?lines=200");
  document.getElementById("logs").textContent=r.logs||"";
}
async function healthTest(){
  if(!CURRENT)return;
  const r=await jpost("/api/health-test?model="+encodeURIComponent(CURRENT.path));
  flash(JSON.stringify(r).slice(0,500));
}
function flash(msg){document.getElementById("result").textContent=msg;}
function human(n){const u=1024;if(n<u)return n+" B";const e=Math.floor(Math.log(n)/Math.log(u));return (n/Math.pow(u,e)).toFixed(1)+" "+["KB","MB","GB","TB"][e-1];}
function esc(s){return String(s).replace(/[&<>]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;"}[c]));}
function escAttr(s){return esc(String(s)).replace(/"/g,"&quot;");}

async function refreshStatus(){
  const s=await jget("/api/status");STATUS=s;renderStatus();
}
function renderStatus(){
  const dot=document.getElementById("dot"),st=document.getElementById("state");
  const sub=document.getElementById("sub");
  const a=(STATUS.ActiveState||"").toLowerCase();
  dot.classList.remove("running","stopped","unknown");
  if(a==="active"){dot.classList.add("running");st.textContent=t("s_running");}
  else if(a==="inactive"){dot.classList.add("stopped");st.textContent=t("s_stopped");}
  else if(a==="failed"){dot.classList.add("stopped");st.textContent=t("s_failed");}
  else{dot.classList.add("unknown");st.textContent=t("s_unknown");}
  sub.textContent=(STATUS.SubState||"")+(STATUS.MainPID?(" pid "+STATUS.MainPID):"");
}

async function loadModels(){
  const m=await jget("/api/models");MODELS=m.models||[];renderModels();
}
function renderModels(){
  const q=(document.getElementById("search").value||"").toLowerCase();
  const box=document.getElementById("models");box.innerHTML="";
  const list=MODELS.filter(x=>x.name.toLowerCase().includes(q));
  if(!list.length){box.innerHTML='<div class="notice" style="padding:14px">'+t("no_models")+'</div>';return;}
  list.forEach(m=>{
    const el=document.createElement("div");
    el.className="model"+(CURRENT&&CURRENT.path===m.path?" active":"");
    el.innerHTML=`<div class="name">${esc(m.name)}</div>
      <div class="meta"><span>${human(m.size)}</span>
      <span class="badge ${m.has_config?"cfg":""}">${m.has_config?t("badge_config"):t("badge_none")}</span></div>`;
    el.onclick=()=>selectModel(m);
    box.appendChild(el);
  });
}
function selectModel(m){
  CURRENT=m;
  document.getElementById("empty").classList.add("hidden");
  document.getElementById("editor").classList.remove("hidden");
  document.getElementById("sel-path").textContent="Model: "+m.path;
  jget("/api/config?model="+encodeURIComponent(m.path)).then(cfg=>{CONFIG=cfg;buildForm(cfg);});
  renderModels();
  applyI18n();
}

// ---- settings menu ------------------------------------------------------
function toggleSettings(e){
  if (e && e.stopPropagation) e.stopPropagation();
  document.getElementById("settings-pop").classList.toggle("hidden");
}
function closeSettings(e){
  const pop=document.getElementById("settings-pop");
  const btn=document.getElementById("btn-settings");
  if (!pop || !btn) return;
  if (!pop.contains(e.target) && !btn.contains(e.target)) {
    pop.classList.add("hidden");
  }
}
async function loadSettings(){
  const s=await jget("/api/settings");
  document.getElementById("set-host").value=s.webHost||"127.0.0.1";
  document.getElementById("set-port").value=s.webPort||8080;
  document.getElementById("set-model-dir").value=s.modelDir||"";
  document.getElementById("set-llama-server").value=s.llamaServer||"";
  const url=s.boundAddr||("http://"+(s.webHost||"127.0.0.1")+":"+(s.webPort||8080));
  document.getElementById("server-url").textContent=url;
  const lang=(s.lang==="en"||s.lang==="ru")?s.lang:"en";
  document.getElementById("lang").value=lang;
  document.getElementById("set-lang").value=lang;
  setLang(lang);
}
async function saveAppSettings(){
  const dir=document.getElementById("set-model-dir").value.trim();
  if(!dir){flash(t("model_dir_empty"));return;}
  const bin=document.getElementById("set-llama-server").value.trim();
  if(!bin){flash(t("llama_server_empty"));return;}
  let ok=true;
  const r=await jpost("/api/app/model-dir",{modelDir:dir});
  if(r.error){flash(r.error);ok=false;}
  const r2=await jpost("/api/app/llama-server",{llamaServer:bin});
  if(r2.error){flash(r2.error);ok=false;}
  if(!ok){return;}
  flash(t("saved"));
  // Re-read the model list from the new directory without a restart.
  CURRENT=null;
  document.getElementById("empty").classList.remove("hidden");
  document.getElementById("editor").classList.add("hidden");
  await loadModels();
}
async function saveSettings(){
  const host=document.getElementById("set-host").value.trim()||"127.0.0.1";
  const port=document.getElementById("set-port").value.trim()||"8080";
  const lang=document.getElementById("set-lang").value;
  const r=await jpost("/api/settings",{webHost:host,webPort:port,lang:lang});
  if(r.error){flash(r.error);return;}
  const newLang=r.lang||lang;
  document.getElementById("lang").value=newLang;
  document.getElementById("set-lang").value=newLang;
  setLang(newLang);
  const newHost=r.webHost||host;
  const newPort=r.webPort||port;
  document.getElementById("server-url").textContent="http://"+newHost+":"+newPort;
  if(r.restarted){
    flash(t("s_restarting"));
    const url="http://"+newHost+":"+newPort;
    setTimeout(()=>{location.href=url;},1200);
  } else {
    flash(t("saved"));
  }
}

async function setAppLang(v){
  document.getElementById("lang").value=v;
  document.getElementById("set-lang").value=v;
  setLang(v);
  const host=document.getElementById("set-host").value.trim()||"127.0.0.1";
  const port=document.getElementById("set-port").value.trim()||"8080";
  const r=await jpost("/api/settings",{webHost:host,webPort:port,lang:v});
  if(r.error){flash(r.error);return;}
  if(r.restarted){
    const url="http://"+(r.webHost||host)+":"+(r.webPort||port);
    document.getElementById("server-url").textContent=url;
    flash(t("s_restarting"));
    setTimeout(()=>{location.href=url;},1200);
  } else {
    flash(t("language_saved"));
  }
}

async function init(){
  if (typeof document.addEventListener === "function") document.addEventListener("click", closeSettings);
  loadLang();
  applyI18n();
  await Promise.all([loadFlags(), loadSettings(), refreshStatus(), loadModels(), loadPresets()]);
  if (CURRENT) buildForm(CONFIG);
}
init();
