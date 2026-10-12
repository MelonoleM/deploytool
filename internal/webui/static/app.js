"use strict";

const $ = (sel, root = document) => root.querySelector(sel);

const state = {
  boot: null,
  services: [],
  config: null,
  view: "maintenance",
  tab: "installed",
  busy: false,
  progressOpen: false,
  mode: { native: false, portal: "" },
  autoOpenPortal: true,
  update: { has_update: false, current_version: "", baseline_version: "", dismissed: false },
};

const statusMeta = {
  healthy: { label: "运行中", pill: "healthy", dot: "healthy" },
  unhealthy: { label: "异常", pill: "unhealthy", dot: "unhealthy" },
  stopped: { label: "已停止", pill: "stopped", dot: "stopped" },
  skipped: { label: "已禁用", pill: "skipped", dot: "skipped" },
  missing: { label: "未安装", pill: "missing", dot: "stopped" },
};

function classify(svc) {
  const st = svc.status || {};
  if (st.skipped) return "skipped";
  if (st.healthy) return "healthy";
  if (st.running) return "unhealthy";
  if (st.installed) return "stopped";
  return "missing";
}

function el(tag, className, text) {
  const n = document.createElement(tag);
  if (className) n.className = className;
  if (text !== undefined) n.textContent = text;
  return n;
}

async function jget(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || r.statusText);
  return r.json();
}

async function jpost(url, body) {
  const r = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body || {}),
  });
  if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || r.statusText);
  return r.json();
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function toast(msg, kind = "info") {
  const box = el("div", "toast " + kind, msg);
  $("#toasts").appendChild(box);
  setTimeout(() => box.remove(), 4200);
}

function setBusy(busy) {
  state.busy = busy;
}

function serviceByName(name) {
  return state.services.find((s) => s.name === name);
}

// --- dependency-ordered service names (Kahn) ---
function orderServices(services, reverse = false) {
  const by = Object.fromEntries(services.map((s) => [s.name, s]));
  const degree = {};
  const next = {};
  services.forEach((s) => { degree[s.name] = 0; });
  services.forEach((s) => (s.depends_on || []).forEach((d) => {
    if (by[d]) { degree[s.name]++; (next[d] = next[d] || []).push(s.name); }
  }));
  let ready = services.filter((s) => degree[s.name] === 0).map((s) => s.name);
  const out = [];
  while (ready.length) {
    ready.sort();
    const batch = reverse ? ready.slice().reverse() : ready.slice();
    out.push(...batch);
    const nxt = [];
    batch.forEach((n) => (next[n] || []).forEach((v) => { if (--degree[v] === 0) nxt.push(v); }));
    ready = nxt;
  }
  services.forEach((s) => { if (!out.includes(s.name)) out.push(s.name); });
  return out;
}

// --- init & refresh ---
async function init() {
  state.autoOpenPortal = localStorage.getItem("dt.auto_open_portal") !== "0";
  try {
    state.boot = await jget("/api/bootstrap");
    $("#project-sub").textContent = `${state.boot.project.name} · ${state.boot.project.version} · ${state.boot.install_root}`;
  } catch (e) {
    toast("无法连接服务：" + e.message, "error");
    return;
  }
  try {
    state.mode = await jget("/api/mode");
  } catch (e) {
    state.mode = { native: false, portal: "" };
  }
  state.view = state.boot.provisioned ? "maintenance" : "onboarding";
  await refresh();
  showView();
  if (!state.boot.provisioned) {
    showWelcome();
    return;
  }
  const upd = await checkUpdate();
  if (upd.has_update && !upd.dismissed) {
    showUpdatePrompt();
  }
}

async function openPortal() {
  const url = state.mode && state.mode.portal ? state.mode.portal : (state.boot && state.boot.project ? state.boot.project.portal : "");
  if (!url) return toast("未配置项目门户网址", "info");
  if (state.mode && state.mode.native) {
    try {
      await jpost("/api/open", { url });
    } catch (e) {
      toast("打开主页失败：" + e.message, "error");
    }
    return;
  }
  window.open(url, "_blank", "noopener,noreferrer");
}

// --- update check & prompt ---
async function checkUpdate() {
  try {
    state.update = await jget("/api/update/check");
  } catch (e) {
    state.update = { has_update: false, current_version: "", baseline_version: "", dismissed: false };
  }
  renderUpdateDot();
  return state.update;
}

function renderUpdateDot() {
  const show = !!(state.update && state.update.has_update);
  $("#settings-dot").classList.toggle("hidden", !show);
  $("#about-dot").classList.toggle("hidden", !show);
}

function showUpdatePrompt() {
  $("#update-prompt-msg").textContent =
    `当前版本 ${state.update.baseline_version || "—"} · 最新版本 ${state.update.current_version}`;
  $("#update-dialog").showModal();
}

async function doDismissUpdate() {
  $("#update-dialog").close();
  try {
    await jpost("/api/update/dismiss", {});
    state.update.dismissed = true;
    renderUpdateDot();
    toast("好的，稍后可在设置中查看更新", "info");
  } catch (e) {
    toast("操作失败：" + e.message, "error");
  }
}

async function doApplyUpdate() {
  setBusy(true);
  try {
    await jpost("/api/update/apply", {});
    $("#update-dialog").close();
    $("#update-details-dialog").close();
    await refresh();
    await checkUpdate();
    const active = document.querySelector(".settings-nav-item.active");
    if ($("#settings-dialog").open && active && active.dataset.settingsTab === "about") {
      await renderAboutPanel();
    }
    toast("更新完成", "success");
  } catch (e) {
    toast("更新失败：" + e.message, "error");
  } finally {
    setBusy(false);
  }
}

async function openUpdateDetails() {
  const body = $("#update-details-body");
  body.replaceChildren(el("div", "muted", "加载中…"));
  $("#update-details-dialog").showModal();
  try {
    const data = await jget("/api/update/diff");
    renderUpdateDetails(data.changes || []);
  } catch (e) {
    body.replaceChildren(el("div", "muted", "加载失败：" + e.message));
  }
}

function renderUpdateDetails(changes) {
  const body = $("#update-details-body");
  body.replaceChildren();
  if (!changes.length) {
    body.appendChild(el("div", "muted", "没有可显示的差异"));
    return;
  }
  const labels = { added: "新增", removed: "删除", changed: "修改" };
  const list = el("div", "change-list");
  changes.forEach((ch) => {
    const row = el("div", "change-item");
    const head = el("div", "change-head");
    head.append(
      el("span", "change-kind " + (ch.kind || "changed"), labels[ch.kind] || "修改"),
      el("code", "change-path", ch.path)
    );
    row.appendChild(head);
    if (ch.kind === "changed") {
      row.appendChild(el("div", "change-old", "- " + (ch.old || "（空）")));
      row.appendChild(el("div", "change-new", "+ " + (ch.new || "（空）")));
    } else if (ch.kind === "added") {
      row.appendChild(el("div", "change-new", "+ " + (ch.new || "（空）")));
    } else if (ch.kind === "removed") {
      row.appendChild(el("div", "change-old", "- " + (ch.old || "（空）")));
    }
    list.appendChild(row);
  });
  body.appendChild(list);
}

async function refresh() {
  try {
    const ov = await jget("/api/overview");
    state.services = ov.services;
  } catch (e) {
    toast("刷新失败：" + e.message, "error");
    return;
  }
  try {
    state.config = await jget("/api/config");
  } catch (e) {
    state.config = { yaml: "", modified: false, path: "" };
  }
  renderView();
}

function showView() {
  $("#view-onboarding").classList.toggle("hidden", state.view !== "onboarding");
  $("#view-maintenance").classList.toggle("hidden", state.view !== "maintenance");
}

function renderView() {
  if (state.view === "onboarding") renderOnboarding();
  else renderMaintenance();
  renderConfigModifiedBadge();
}

// --- welcome ---
function showWelcome() {
  const w = state.boot.welcome;
  $("#welcome-title").textContent = w.title;
  $("#welcome-desc").textContent = w.description;
  $("#welcome-guide").textContent = w.guide;
  const list = $("#welcome-features");
  list.replaceChildren();
  (w.features || []).forEach((f) => list.appendChild(el("li", "", f)));
  document.body.classList.add("welcome-mode");
  $("#welcome-dialog").showModal();
}

// --- onboarding ---
function renderOnboarding() {
  mountConfigForm($("#onboard-config"));
}

async function toggleEnabled(name, enabled, input) {
  input.disabled = true;
  try {
    await jpost("/api/config/service", { name, enabled });
    await refresh();
    toast(`${name} 已${enabled ? "启用" : "停用"}`, "success");
  } catch (e) {
    input.checked = !enabled;
    toast(e.message, "error");
  } finally {
    input.disabled = false;
  }
}

// --- config form editor ---
const SERVICE_TYPES = ["exe", "python", "java", "nodejs", "go", "nginx"];
const HEALTH_TYPES = [["", "无"], ["http", "HTTP"], ["tcp", "TCP"], ["process", "进程"], ["script", "脚本"]];
const START_MODES = [["background", "后台"], ["console", "前台控制台（独立窗口）"]];

function setAttr(node, k, v) { node.setAttribute(k, v); return node; }
function textInput(value) { const i = el("input"); i.type = "text"; i.value = value ?? ""; return i; }
function numInput(value) { const i = el("input"); i.type = "number"; i.min = "0"; i.value = value ?? ""; return i; }
function areaInput(value, rows) { const t = el("textarea"); t.rows = rows || 3; t.value = value ?? ""; return t; }
function selectInput(options, value) {
  const s = el("select");
  options.forEach(([v, label]) => { const o = el("option", "", label); o.value = v; s.appendChild(o); });
  s.value = value ?? "";
  return s;
}
function field(label, input) {
  const wrap = el("label", "form-field");
  wrap.append(el("span", "form-label", label), input);
  return wrap;
}
function switchField(label, checked, key) {
  const input = el("input");
  input.type = "checkbox";
  input.checked = !!checked;
  input.dataset.form = key;
  const sw = el("label", "switch");
  sw.append(input, el("span", "slider"));
  const wrap = el("div", "form-switch");
  wrap.append(el("span", "form-label", label), sw);
  return wrap;
}

let currentForm = null;

async function mountConfigForm(container) {
  container.replaceChildren();
  const loading = el("div", "muted", "加载配置…");
  container.appendChild(loading);
  let data;
  try {
    data = await jget("/api/config/form");
  } catch (e) {
    loading.textContent = "加载配置失败：" + e.message;
    return;
  }
  currentForm = data.form;
  const wrap = el("div", "config-form");
  wrap.appendChild(buildConfigForm(currentForm));
  const actions = el("div", "form-actions");
  const reset = el("button", "btn btn-ghost", "重置为初始配置");
  reset.addEventListener("click", () => resetConfig());
  actions.append(reset);
  wrap.appendChild(actions);
  container.replaceChildren(wrap);
}

// The onboarding wizard only exposes the install directory plus the service list;
// project/welcome/global flags stay in YAML or move to the maintenance settings.
function buildConfigForm(form) {
  const wrap = el("div", "config-form");

  const root = el("div", "install-root-row");
  root.append(
    el("span", "install-root-label", "安装目录"),
    setAttr(textInput(form.install_root), "data-form", "install_root")
  );

  const services = el("div", "services-block");
  const runtime = {};
  state.services.forEach((s) => { runtime[s.name] = s.status || {}; });
  form.services.forEach((svc) => services.appendChild(buildServiceForm(svc, runtime[svc.name])));

  wrap.append(root, services);
  return wrap;
}

function buildServiceForm(svc, runtime) {
  const isInstalled = !!(runtime && runtime.installed);
  const isRunning = !!(runtime && runtime.running);

  const d = el("details", "service-form");
  const summary = el("summary");
  summary.append(el("span", "service-form-name", svc.name), el("span", "type-badge", svc.type));
  if (!isInstalled) {
    const en = el("input");
    en.type = "checkbox";
    en.checked = svc.enabled;
    en.dataset.svc = "enabled";
    const sw = el("label", "switch");
    sw.append(en, el("span", "slider"));
    summary.append(sw);
  }
  d.append(summary);

  if (isInstalled || isRunning) {
    const hint = el("div", "form-hint");
    hint.textContent = isRunning
      ? "该服务正在运行：安装期字段及启动/停止/健康检查需先停止后才能修改。"
      : "该服务已安装：安装步骤等安装期字段不可修改，停止后可调整启动与健康检查。";
    d.append(hint);
  }

  const nameInput = setAttr(textInput(svc.name), "data-svc", "name");
  const typeInput = setAttr(selectInput(SERVICE_TYPES.map((t) => [t, t]), svc.type), "data-svc", "type");
  const workdirInput = setAttr(textInput(svc.workdir), "data-svc", "workdir");
  const depsInput = setAttr(textInput((svc.depends_on || []).join(", ")), "data-svc", "depends_on");
  const startInput = setAttr(textInput(svc.start), "data-svc", "start");
  const modeInput = setAttr(selectInput(START_MODES, svc.start_mode), "data-svc", "start_mode");
  const stopInput = setAttr(textInput(svc.stop), "data-svc", "stop");
  const htypeInput = setAttr(selectInput(HEALTH_TYPES, svc.health_type), "data-svc", "health_type");
  const hurlInput = setAttr(textInput(svc.health_url), "data-svc", "health_url");
  const haddrInput = setAttr(textInput(svc.health_address), "data-svc", "health_address");
  const hintervalInput = setAttr(textInput(svc.health_interval), "data-svc", "health_interval");
  const htimeoutInput = setAttr(textInput(svc.health_timeout), "data-svc", "health_timeout");
  const hretriesInput = setAttr(numInput(svc.health_retries), "data-svc", "health_retries");
  const envInput = setAttr(areaInput(svc.env || "", 4), "data-svc", "env");
  const instInput = setAttr(areaInput(svc.install_yaml, 10), "data-svc", "install_yaml");

  if (isInstalled) {
    [nameInput, typeInput, workdirInput, depsInput, instInput].forEach(disableInput);
  }
  if (isRunning) {
    [startInput, modeInput, stopInput, htypeInput, hurlInput, haddrInput, hintervalInput, htimeoutInput, hretriesInput, envInput].forEach(disableInput);
  }

  const grid = el("div", "form-grid");
  grid.append(
    field("名称", nameInput),
    field("类型", typeInput),
    field("工作目录", workdirInput),
    field("依赖（逗号分隔）", depsInput),
    field("启动命令", startInput),
    field("启动模式", modeInput),
    field("停止命令", stopInput),
    field("健康检查类型", htypeInput),
    field("健康检查 URL", hurlInput),
    field("健康检查地址", haddrInput),
    field("检查间隔", hintervalInput),
    field("检查超时", htimeoutInput),
    field("重试次数", hretriesInput)
  );
  const env = field("环境变量（每行 KEY=VALUE）", envInput);
  env.classList.add("span-2");
  grid.appendChild(env);
  const inst = field("安装步骤（YAML，仅此部分需手写）", instInput);
  inst.classList.add("span-2");
  grid.appendChild(inst);
  d.append(grid);
  return d;
}

function disableInput(node) {
  node.disabled = true;
}

function collectForm() {
  const f = {
    project_name: currentForm.project_name,
    project_version: currentForm.project_version,
    install_root: formVal("install_root"),
    fail_fast: currentForm.fail_fast,
    rollback_on_failure: currentForm.rollback_on_failure,
    welcome_title: currentForm.welcome_title,
    welcome_description: currentForm.welcome_description,
    welcome_guide: currentForm.welcome_guide,
    welcome_features: currentForm.welcome_features,
    services: [],
  };
  document.querySelectorAll(".service-form").forEach((d) => {
    const enInput = d.querySelector('input[data-svc="enabled"]');
    f.services.push({
      name: svcVal(d, "name"),
      type: svcVal(d, "type"),
      enabled: enInput ? enInput.checked : true,
      workdir: svcVal(d, "workdir"),
      depends_on: svcVal(d, "depends_on").split(",").map((s) => s.trim()).filter(Boolean),
      start: svcVal(d, "start"),
      start_mode: svcVal(d, "start_mode"),
      stop: svcVal(d, "stop"),
      health_type: svcVal(d, "health_type"),
      health_url: svcVal(d, "health_url"),
      health_address: svcVal(d, "health_address"),
      health_interval: svcVal(d, "health_interval"),
      health_timeout: svcVal(d, "health_timeout"),
      health_retries: parseInt(svcVal(d, "health_retries") || "0", 10),
      env: svcVal(d, "env"),
      install_yaml: svcVal(d, "install_yaml"),
    });
  });
  return f;
}

function formVal(key) { return document.querySelector(`[data-form="${key}"]`).value; }
function formChecked(key) { return document.querySelector(`[data-form="${key}"]`).checked; }
function svcVal(section, key) { return section.querySelector(`[data-svc="${key}"]`).value; }

function renderConfigModifiedBadge() {
  const b = $("#config-modified");
  if (state.config && state.config.modified) b.classList.remove("hidden");
  else b.classList.add("hidden");
}

async function resetConfig() {
  if (!window.confirm("确定重置为初始配置吗？这会丢弃当前所有修改。")) return;
  setBusy(true);
  try {
    await jpost("/api/config/reset", {});
    await refresh();
    toast("已重置为初始配置", "success");
  } catch (e) {
    toast("重置失败：" + e.message, "error");
  } finally {
    setBusy(false);
  }
}

// --- maintenance ---
function renderMaintenance() {
  document.querySelectorAll(".tab").forEach((t) => {
    t.classList.toggle("active", t.dataset.tab === state.tab);
  });
  const bar = $("#bulk-bar");
  bar.replaceChildren();
  const installed = state.services.filter((s) => s.status && s.status.installed);
  const notInstalled = state.services.filter((s) => !(s.status && s.status.installed));

  if (state.tab === "installed") {
    addBulk(bar, "一键启动", () => {
      const targets = installed.filter((s) => !s.status.running);
      bulkAction("start", targets, "启动", state.autoOpenPortal ? openPortal : null);
    });
    addBulk(bar, "一键停止", () => bulkAction("stop", installed.filter((s) => s.status.running), "停止"));
    addBulk(bar, "一键卸载", () => bulkAction("uninstall", installed, "卸载"), true);
    addBulk(bar, "进入主页", openPortal, false, "bulk-portal");
  } else {
    addBulk(bar, "一键安装", () => bulkAction("install", notInstalled.filter((s) => s.enabled), "安装"));
  }

  const cards = $("#service-cards");
  cards.replaceChildren();
  const list = state.tab === "installed" ? installed : notInstalled;
  if (!list.length) {
    cards.appendChild(el("div", "muted", state.tab === "installed" ? "暂无已安装服务" : "暂无未安装服务"));
    return;
  }
  list.forEach((svc) => cards.appendChild(cardFor(svc, state.tab === "installed")));
}

function addBulk(bar, label, fn, danger, cls) {
  const b = el("button", "btn" + (danger ? " btn-danger" : "") + (cls ? " " + cls : ""), label);
  b.addEventListener("click", fn);
  bar.appendChild(b);
}

function cardFor(svc, installed) {
  const card = el("div", "card");
  const row = el("div", "card-row");
  const key = classify(svc);
  const meta = statusMeta[key];

  const id = el("div", "card-id");
  id.append(
    el("span", "dot " + meta.dot),
    el("span", "name", svc.name),
    el("span", "type-badge", svc.type)
  );

  const bits = [];
  if (svc.status && svc.status.reason) bits.push(svc.status.reason);
  if (svc.healthcheck && svc.healthcheck.url) bits.push("健康检查 " + svc.healthcheck.url);
  if (svc.pid) bits.push("PID " + svc.pid);
  const metaLine = el("div", "card-meta grow", bits.length ? bits.join(" · ") : (installed ? "—" : "尚未安装"));

  const actions = el("div", "card-actions");
  if (installed) {
    actions.appendChild(el("span", "pill " + meta.pill, meta.label));
    if (svc.status.running) {
      actions.appendChild(btn("停止", () => singleAction("stop", svc, "停止"), "btn-ghost"));
    } else {
      actions.appendChild(btn("启动", () => singleAction("start", svc, "启动")));
    }
    actions.appendChild(btn("卸载", () => singleAction("uninstall", svc, "卸载"), "btn-danger"));
    actions.appendChild(btn("配置", () => openConfigDialog(svc), "btn-ghost"));
    actions.appendChild(btn("日志", () => toggleCardLog(card, svc), "btn-ghost"));
  } else {
    actions.appendChild(makeEnabledSwitch(svc));
    const installBtn = btn("安装", () => singleAction("install", svc, "安装"));
    installBtn.disabled = !svc.enabled;
    actions.appendChild(installBtn);
    actions.appendChild(btn("配置", () => openConfigDialog(svc), "btn-ghost"));
  }
  row.append(id, metaLine, actions);
  card.appendChild(row);
  return card;
}

function makeEnabledSwitch(svc) {
  const wrap = el("div", "switch-wrap");
  const label = el("span", "switch-label", svc.enabled ? "已启用" : "已禁用");
  const sw = el("label", "switch");
  const input = el("input");
  input.type = "checkbox";
  input.checked = svc.enabled;
  input.addEventListener("change", () => toggleEnabled(svc.name, input.checked, input));
  sw.append(input, el("span", "slider"));
  wrap.append(label, sw);
  return wrap;
}

function btn(label, fn, cls) {
  const b = el("button", "btn " + (cls || ""), label);
  b.addEventListener("click", fn);
  return b;
}

function toggleCardLog(card, svc) {
  let pre = card.querySelector(".card-log");
  if (pre) { pre.remove(); return; }
  pre = el("pre", "card-log", "加载中…");
  card.appendChild(pre);
  fetch(`/api/logs?service=${encodeURIComponent(svc.name)}&tail=200`)
    .then((r) => r.text())
    .then((t) => { pre.textContent = t || "（暂无内容）"; })
    .catch((e) => { pre.textContent = "读取失败：" + e.message; });
}

function singleAction(action, svc, verb) {
  if (action === "uninstall" && !window.confirm(`确定卸载 ${svc.name} 吗？`)) return;
  const body = { only: [svc.name] };
  if (action === "install") body.start = true;
  openProgress({
    title: `${verb}服务`,
    verb,
    services: [svc.name],
    endpoint: "/api/" + action,
    body,
    onSuccess: refresh,
  });
}

function bulkAction(action, targets, verb, after) {
  if (!targets.length) return toast("没有可操作的服务", "info");
  if (action === "uninstall" && !window.confirm(`确定卸载 ${targets.length} 个服务吗？`)) return;
  const body = { only: targets.map((s) => s.name) };
  if (action === "install") body.start = true;
  openProgress({
    title: `${verb}服务`,
    verb,
    services: orderServices(targets, action === "stop" || action === "uninstall"),
    endpoint: "/api/" + action,
    body,
    onSuccess: async () => {
      await refresh();
      if (after) await after();
    },
  });
}

// --- progress dialog ---
function openProgress(opts) {
  const dialog = $("#progress-dialog");
  $("#progress-title").textContent = opts.title;
  const list = $("#progress-list");
  list.replaceChildren();
  const rows = new Map();
  const lines = new Map();
  for (const name of opts.services) {
    const row = el("div", "progress-item");
    const head = el("div", "progress-head");
    const toggle = el("button", "p-toggle", "▸");
    head.append(
      el("span", "dot"),
      el("span", "p-name", name),
      el("span", "p-state", "排队中"),
      toggle
    );
    const barWrap = el("div", "progress-bar-wrap");
    const bar = el("div", "progress-bar");
    barWrap.appendChild(bar);
    const log = el("pre", "p-log hidden", "");
    toggle.addEventListener("click", () => log.classList.toggle("hidden"));
    row.append(head, barWrap, log);
    list.appendChild(row);
    rows.set(name, { row, head, bar, state: head.querySelector(".p-state"), log });
    lines.set(name, []);
  }
  state.progressOpen = true;
  $("#progress-done").classList.add("hidden");
  dialog.showModal();

  jpost(opts.endpoint, opts.body)
    .then(async ({ task_id }) => {
      for (;;) {
        const t = await jget("/api/tasks/" + task_id);
        applyProgress(t.progress || [], rows, lines, opts.verb);
        if (t.done) {
          $("#progress-done").classList.remove("hidden");
          if (t.report && t.report.success) {
            markRemainingDone(rows, lines, opts.verb);
            toast(opts.verb + "完成", "success");
            if (opts.onSuccess) await opts.onSuccess();
          } else {
            toast(failureReason(t.report) || opts.verb + "失败", "error");
          }
          return;
        }
        await sleep(400);
      }
    })
    .catch((e) => toast(e.message, "error"));
}

function applyProgress(events, rows, lines, verb) {
  for (const p of events) {
    const r = rows.get(p.service);
    if (!r) continue;
    if (p.phase === "running") {
      r.state.textContent = "进行中";
      r.bar.className = "progress-bar running";
      pushLine(lines, p.service, `开始${verb}…`);
    } else if (p.phase === "done") {
      r.state.textContent = "完成";
      r.bar.className = "progress-bar done";
      r.row.classList.add("done");
      pushLine(lines, p.service, `${verb}完成`);
    } else if (p.phase === "failed") {
      r.state.textContent = "失败";
      r.bar.className = "progress-bar failed";
      r.row.classList.add("failed");
      pushLine(lines, p.service, `失败：${p.reason || ""}`);
    } else if (p.phase === "skipped") {
      r.state.textContent = "跳过";
      r.bar.className = "progress-bar skipped";
      pushLine(lines, p.service, `跳过：${p.reason || ""}`);
    }
    r.log.textContent = lines.get(p.service).join("\n");
  }
}

function markRemainingDone(rows, lines, verb) {
  for (const [name, r] of rows) {
    if (r.state.textContent === "排队中" || r.state.textContent === "进行中") {
      r.state.textContent = "完成";
      r.bar.className = "progress-bar done";
      pushLine(lines, name, `${verb}完成`);
      r.log.textContent = lines.get(name).join("\n");
    }
  }
}

function pushLine(lines, name, text) {
  const arr = lines.get(name);
  if (arr[arr.length - 1] !== text) arr.push(text);
}

function failureReason(report) {
  return (report.services || []).map((s) => s.reason).filter(Boolean).join("；");
}

// --- per-service config dialog (maintenance) ---
async function openConfigDialog(svc) {
  const dialog = $("#config-dialog");
  $("#config-dialog-title").textContent = "配置 · " + svc.name;
  const body = $("#config-dialog-body");
  body.replaceChildren(el("div", "muted", "加载配置…"));
  dialog.showModal();
  let data;
  try {
    data = await jget("/api/config/form");
  } catch (e) {
    body.replaceChildren(el("div", "muted", "加载失败：" + e.message));
    return;
  }
  const svcForm = data.form.services.find((s) => s.name === svc.name);
  if (!svcForm) {
    body.replaceChildren(el("div", "muted", "找不到该服务"));
    return;
  }
  const wrap = el("div", "config-form");
  const section = el("section", "form-section");
  const d = buildServiceForm(svcForm, svc.status || {});
  d.open = true;
  section.append(d);
  const actions = el("div", "form-actions");
  const save = el("button", "btn", "保存");
  save.addEventListener("click", () => saveServiceForm(svc.name));
  actions.append(save);
  wrap.append(section, actions);
  body.replaceChildren(wrap);
}

function saveServiceForm(name) {
  const section = $("#config-dialog-body .service-form");
  const enInput = section.querySelector('input[data-svc="enabled"]');
  const f = {
    name,
    type: svcVal(section, "type"),
    enabled: enInput ? enInput.checked : true,
    workdir: svcVal(section, "workdir"),
    depends_on: svcVal(section, "depends_on").split(",").map((s) => s.trim()).filter(Boolean),
    start: svcVal(section, "start"),
    start_mode: svcVal(section, "start_mode"),
    stop: svcVal(section, "stop"),
    health_type: svcVal(section, "health_type"),
    health_url: svcVal(section, "health_url"),
    health_address: svcVal(section, "health_address"),
    health_interval: svcVal(section, "health_interval"),
    health_timeout: svcVal(section, "health_timeout"),
    health_retries: parseInt(svcVal(section, "health_retries") || "0", 10),
      env: svcVal(section, "env"),
    install_yaml: svcVal(section, "install_yaml"),
  };
  setBusy(true);
  jpost("/api/config/service/form", f)
    .then(async () => {
      await refresh();
      toast("配置已保存", "success");
      $("#config-dialog").close();
    })
    .catch((e) => toast("保存失败：" + e.message, "error"))
    .finally(() => setBusy(false));
}

// --- settings dropdown & dialogs ---
function wireSettings() {
  const menu = $("#settings-menu");
  $("#settings-btn").addEventListener("click", (e) => {
    e.stopPropagation();
    menu.classList.toggle("hidden");
  });
  document.addEventListener("click", () => menu.classList.add("hidden"));
  menu.querySelectorAll("[data-menu]").forEach((b) => {
    b.addEventListener("click", () => {
      menu.classList.add("hidden");
      const m = b.dataset.menu;
      if (m === "update") openOps("update");
      else if (m === "pack") openOps("pack");
      else if (m === "refresh") refresh();
      else if (m === "settings") openSettings();
      else if (m === "help") openHelp();
    });
  });
  document.querySelectorAll("[data-settings-tab]").forEach((b) => {
    b.addEventListener("click", () => activateSettingsTab(b.dataset.settingsTab));
  });
}

async function openSettings() {
  $("#settings-dialog").showModal();
  activateSettingsTab("common");
}

function activateSettingsTab(tab) {
  document.querySelectorAll(".settings-nav-item").forEach((b) => {
    b.classList.toggle("active", b.dataset.settingsTab === tab);
  });
  if (tab === "common") renderCommonSettings();
  else renderAboutPanel();
}

async function renderCommonSettings() {
  const panel = $("#settings-panel");
  panel.replaceChildren(el("div", "muted", "加载…"));
  let data;
  try {
    data = await jget("/api/config/form");
  } catch (e) {
    panel.replaceChildren(el("div", "muted", "加载失败：" + e.message));
    return;
  }
  currentForm = data.form;
  const wrap = el("div", "settings-form");
  wrap.append(
    switchField("失败即停（任一服务失败即中断后续安装）", currentForm.fail_fast, "fail_fast"),
    switchField("更新失败自动回滚", currentForm.rollback_on_failure, "rollback_on_failure")
  );
  const autoWrap = el("div", "form-switch");
  const autoInput = el("input");
  autoInput.type = "checkbox";
  autoInput.checked = !!state.autoOpenPortal;
  autoInput.addEventListener("change", () => {
    state.autoOpenPortal = autoInput.checked;
    localStorage.setItem("dt.auto_open_portal", autoInput.checked ? "1" : "0");
  });
  const autoSw = el("label", "switch");
  autoSw.append(autoInput, el("span", "slider"));
  autoWrap.append(el("span", "form-label", "一键启动后自动进入主页"), autoSw);
  wrap.append(autoWrap);
  const root = el("div", "form-field");
  const ir = textInput(currentForm.install_root);
  ir.disabled = true;
  root.append(el("span", "form-label", "安装目录（已安装，只读）"), ir);
  wrap.append(root);
  const actions = el("div", "form-actions");
  const save = el("button", "btn", "保存");
  save.addEventListener("click", () => saveSettings());
  actions.append(save);
  wrap.append(actions);
  panel.replaceChildren(wrap);
}

async function renderAboutPanel() {
  const panel = $("#settings-panel");
  panel.replaceChildren();
  const wrap = el("div", "about-panel");

  const brand = el("div", "about-brand");
  const logo = el("div", "about-logo");
  logo.innerHTML = '<svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="#fff" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 12 12 17 22 12"/><polyline points="2 17 12 22 22 17"/></svg>';
  brand.append(logo, el("div", "about-name", "DeployTool"));
  wrap.appendChild(brand);

  const checking = el("div", "about-checking");
  checking.append(el("span", "spinner"), el("span", "", "正在检查更新…"));
  wrap.appendChild(checking);
  panel.replaceChildren(wrap);

  const upd = await checkUpdate();
  checking.remove();

  const result = el("div", "about-result");
  if (upd.has_update) {
    const line = el("div", "about-result-line");
    line.append(
      el("span", "about-result-title", `发现新版本 ${upd.current_version}`),
      el("span", "muted", `（当前 ${upd.baseline_version || "—"}）`)
    );
    const actions = el("div", "about-result-actions");
    actions.append(btn("更新详情", openUpdateDetails, "btn-ghost"), btn("更新", doApplyUpdate));
    result.append(line, actions);
  } else {
    result.append(
      el("div", "about-result-title", "已是最新版本"),
      el("div", "muted", upd.current_version ? `当前版本 ${upd.current_version}` : "尚未配置版本号")
    );
  }
  wrap.appendChild(result);
}

function saveSettings() {
  const f = {
    project_name: currentForm.project_name,
    project_version: currentForm.project_version,
    install_root: currentForm.install_root,
    fail_fast: formChecked("fail_fast"),
    rollback_on_failure: formChecked("rollback_on_failure"),
    welcome_title: currentForm.welcome_title,
    welcome_description: currentForm.welcome_description,
    welcome_guide: currentForm.welcome_guide,
    welcome_features: currentForm.welcome_features,
    services: currentForm.services,
  };
  setBusy(true);
  jpost("/api/config/form", f)
    .then(async () => {
      await refresh();
      toast("设置已保存", "success");
      $("#settings-dialog").close();
    })
    .catch((e) => toast("保存失败：" + e.message, "error"))
    .finally(() => setBusy(false));
}

function openHelp() {
  $("#help-project-name").textContent = state.boot ? state.boot.project.name : "";
  $("#help-project-version").textContent = state.boot ? state.boot.project.version : "";
  $("#help-dialog").showModal();
}

function openOps(mode) {
  $("#ops-title").textContent = mode === "update" ? "增量更新" : "打包";
  $("#ops-update").style.display = mode === "update" ? "" : "none";
  $("#ops-pack").style.display = mode === "pack" ? "" : "none";
  const sel = $("#update-only");
  sel.replaceChildren(new Option("全部服务", ""));
  for (const svc of state.services) sel.appendChild(new Option(svc.name, svc.name));
  $("#ops-dialog").showModal();
}

function wireDialogs() {
  $("#welcome-start").addEventListener("click", () => {
    $("#welcome-dialog").close();
  });
  $("#welcome-dialog").addEventListener("close", () => {
    document.body.classList.remove("welcome-mode");
  });
  $("#start-install").addEventListener("click", async () => {
    const form = collectForm();
    const enabled = form.services.filter((s) => s.enabled);
    if (!enabled.length) return toast("请至少启用一个服务", "info");
    try {
      await jpost("/api/config/form", form);
    } catch (e) {
      return toast("配置保存失败：" + e.message, "error");
    }
    openProgress({
      title: "安装服务",
      verb: "安装",
      services: orderServices(enabled, false),
      endpoint: "/api/install",
      body: { only: enabled.map((s) => s.name), start: $("#start-after-install").checked },
      onSuccess: async () => {
        state.view = "maintenance";
        state.tab = "installed";
        showView();
        await refresh();
      },
    });
  });
  $("#progress-close").addEventListener("click", () => $("#progress-dialog").close());
  $("#progress-done").addEventListener("click", () => {
    state.progressOpen = false;
    $("#progress-dialog").close();
    refresh();
  });
  $("#config-close").addEventListener("click", () => $("#config-dialog").close());
  $("#help-close").addEventListener("click", () => $("#help-dialog").close());
  $("#settings-close").addEventListener("click", () => $("#settings-dialog").close());

  $("#update-run").addEventListener("click", async () => {
    const pkg = $("#update-package").value.trim();
    if (!pkg) return toast("请填写更新包路径", "error");
    const only = $("#update-only").value;
    $("#ops-dialog").close();
    openProgress({
      title: "更新服务",
      verb: "更新",
      services: orderServices(state.services.filter((s) => !only || s.name === only), false),
      endpoint: "/api/update",
      body: { package: pkg, only: only ? [only] : [] },
      onSuccess: refresh,
    });
  });

  $("#pack-run").addEventListener("click", async () => {
    const src = $("#pack-src").value.trim();
    const out = $("#pack-out").value.trim();
    if (!src || !out) return toast("请填写源目录和输出文件", "error");
    $("#ops-dialog").close();
    openProgress({
      title: "打包",
      verb: "打包",
      services: ["打包"],
      endpoint: "/api/pack",
      body: { src, out },
      onSuccess: refresh,
    });
  });

  document.querySelectorAll(".tab").forEach((t) => {
    t.addEventListener("click", () => { state.tab = t.dataset.tab; renderMaintenance(); });
  });

  $("#update-later").addEventListener("click", doDismissUpdate);
  $("#update-now").addEventListener("click", doApplyUpdate);
  $("#update-details-close").addEventListener("click", () => $("#update-details-dialog").close());
}

wireSettings();
wireDialogs();
init();
setInterval(() => {
  if (!state.busy && state.view === "maintenance" && !state.progressOpen) refresh();
}, 5000);
