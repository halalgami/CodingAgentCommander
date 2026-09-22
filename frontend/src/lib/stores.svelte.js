// Shared app state ($state proxies) + all Wails-calling actions. Components
// never import wailsjs directly. Every action that can fail in a plain
// browser (no Wails runtime) catches and no-ops, keeping the shell mountable.
import {
  Config, LaunchSession, ListSessions, SelectSession, PickFolder,
  KillSession, RenameSession, SessionStats, KeyStatus, SetKey, ClearKey,
  Models, AddModel, RemoveModel, SwapModel, DiscoverBedrockModels,
  DiscoverZenModels, DiscoverOllamaModels, ListProviders, AddProvider, RemoveProvider,
  EnableRemoteControl, PlanUsage,
  GetCompanionConfig,
  SetCompanionKind, LoadCompanionPack, PickCompanionPack, ClearCompanionPack,
  CompanionState,
  GetBuildInfo, LitellmRuntimeStatus, InstallLitellmRuntime,
  DependencyStatus, InstallPwsh,
} from "../../wailsjs/go/main/App.js";
import { BrowserOpenURL, EventsOn } from "../../wailsjs/runtime/runtime.js";
import { migrateHistoryOnce } from "./history.js";
import { prefs } from "./prefs.svelte.js";
import { forget } from "./termbus.js";
import { computeFirstRunOfDay } from "./dayRollover.js";

// How long a latched app-level error stays attributed to a session. Generic
// on purpose — this file has a public-override copy, and a feature-named
// constant here would trip the export's content grep on the public copy.
// Kept numerically in sync with the sidebar figure's own ERROR_TTL_MS by
// convention, not by import — the two live on opposite sides of the export
// boundary, so nothing can import across it.
export const ERROR_TTL_MS = 30000;

export const app = $state({
  models: [], catalog: [], keys: [], providers: [],
  sessions: [], stats: {}, finished: {},
  sessionKey: "", selectedModel: "", folder: "",
  drawer: null,            // null | "providers" | "models" | "settings" | "usage" | "history"
  launchConfirm: null,     // null | { folder, modelID, missing } — drives LaunchConfirmModal
  about: false,            // About modal open?
  paletteOpen: false,
  launchError: "",
  // LiteLLM first-run installer. null = closed; object = open, driving
  // LitellmRuntimeModal: { python, canInstall, running, log[], error, done }.
  litellmInstall: null,
  // Companion config (Go-owned; mirrored here for the Settings panel). `kind`
  // is authoritative and mutually exclusive: "avatar" | "panel" | "off".
  companionCfg: {
    enabled: true, kind: "off", size: 340, opacity: 1,
    jiggle: { hair: 1, bust: 1, skirt: 1 }, modelPath: "", packPath: "",
  },
  // Sidebar-companion runtime state. companionPack === null means "no pack
  // configured", which is the branch that shows the set-up affordance rather
  // than a placeholder figure.
  companionPack: null,
  companionWarnings: [],
  companionState: {
    sessions: [], selected: "", running: 0, finished: 0, finishSeq: 0, lastFinished: "",
  },
  // Latched app:error timestamp, attributed to whichever session is selected
  // when it fires (and cleared once ERROR_TTL_MS has passed — see refresh()).
  // reportError already emits app:error to the deck, so this needs no new Go
  // state and no binding regeneration.
  errorMs: 0,
  // True for the whole session iff this is the first app start of the local
  // calendar day (see dayRollover.js), set once in loadAll(). Generic name
  // and location for the same reason as errorMs above.
  firstRunOfDay: false,
  // External-tool preflight: the last DependencyStatus() snapshot (tmux, pwsh,
  // claude), and null | { running, log[], error, done } driving
  // DependenciesModal. Kept separate so the sidebar can show the tool state
  // without the modal being open.
  deps: [],
  depsModal: null,
});

export const toasts = $state([]);
let toastSeq = 0;
export function toast(msg, kind = "info") {
  const id = ++toastSeq;
  toasts.push({ id, msg, kind });
  setTimeout(() => {
    const i = toasts.findIndex((t) => t.id === id);
    if (i !== -1) toasts.splice(i, 1);
  }, 5000);
}

export async function refresh() {
  try {
    app.sessions = await ListSessions();
    for (const s of app.sessions) app.stats[s.windowID] = await SessionStats(s.windowID);
  } catch { /* plain browser / backend gone */ }
  try { app.companionState = await CompanionState(); } catch {}
  // The latch has no timer of its own; refresh() runs on a 5s poll (App.svelte)
  // and is what actually clears an expired error out of reactive state, so a
  // card's error tint does not stay lit forever once nothing else touches it.
  if (app.errorMs && Date.now() - app.errorMs >= ERROR_TTL_MS) app.errorMs = 0;
}

// preferredModel is config's default_model, falling back to the first entry.
// Not simply models[0]: new models merge in at the end of the catalog, so an
// upgraded config's first entry is whatever it happened to list first.
function preferredModel() {
  return (app.models.find((m) => m.default) ?? app.models[0]).id;
}

// reloadModels refreshes just the picker. Discovery runs in the background at
// launch, so this can land after the user has already opened the launch panel —
// it must not re-run the dependency preflight, and it must leave a selection the
// user has already made alone unless that model has gone away.
export async function reloadModels() {
  try { app.models = await Config(); } catch { return; }
  try { app.catalog = await Models(); } catch {}
  if (app.models.length && !app.models.some((m) => m.id === app.selectedModel)) {
    app.selectedModel = preferredModel();
  }
}

export async function loadAll() {
  // Computed once per process start, straight off the injectable pure
  // function — the day-rollover key it reads/writes lives entirely in
  // dayRollover.js, this call is the only place that touches it.
  try { app.firstRunOfDay = computeFirstRunOfDay(Date.now()); } catch {}
  try {
    app.models = await Config();
    if (app.models.length && !app.selectedModel) app.selectedModel = preferredModel();
  } catch {}
  try { app.keys = await KeyStatus(); } catch {}
  try { app.catalog = await Models(); } catch {}
  try { app.providers = await ListProviders(); } catch {}
  try { app.companionCfg = await GetCompanionConfig(); } catch {}
  if (app.companionCfg.kind === "panel") await companionLoadPack();
  // Event-first delivery from the four Go push points (finish, startup, launch,
  // kill); refresh() polls the binding as a fallback.
  try { EventsOn("companion:state", (st) => { app.companionState = st; }); } catch {}
  // Latch the most recent app:error. App.svelte already toasts the same event;
  // this is an independent subscription so App.svelte's delta stays at zero.
  try { EventsOn("app:error", () => { app.errorMs = Date.now(); }); } catch {}
  try { await migrateHistoryOnce(); } catch {}
  await checkDependencies();
  await refresh();
}

// ---- sidebar-companion actions ----
// Kind is enforced exclusive in Go at overlay-creation time; a frontend-only
// check would allow two pollers to exist during the transition.
export async function companionSetKind(kind) {
  try {
    await SetCompanionKind(kind);
    app.companionCfg.kind = kind;
    if (kind === "panel") await companionLoadPack();
  } catch (e) { toast("" + e, "error"); }
}

// The ONLY error case Go raises is "no pack configured"; every other problem
// arrives as a warning on the parsed pack. So a rejection here means "show the
// set-up affordance", not "something broke".
export async function companionLoadPack() {
  try {
    const p = await LoadCompanionPack();
    app.companionPack = p;
    app.companionWarnings = p?.warnings ?? [];
  } catch {
    app.companionPack = null;
    app.companionWarnings = [];
  }
}

export async function companionPickPack() {
  try {
    const name = await PickCompanionPack();
    if (!name) return;                       // cancelled
    app.companionCfg = await GetCompanionConfig();
    await companionLoadPack();
    toast("Companion pack: " + name);
  } catch (e) { toast("" + e, "error"); }
}

export async function companionClearPack() {
  try {
    await ClearCompanionPack();
    app.companionCfg.packPath = "";
    app.companionPack = null;
    app.companionWarnings = [];
  } catch (e) { toast("" + e, "error"); }
}

// checkDependencies snapshots the external tools and, by default, opens the
// preflight modal when a required one is missing. Running it at startup is the
// point: without it the first sign of a missing tmux is a failed launch
// reporting `exec: "tmux": executable file not found in %PATH%`, which tells a
// user nothing about what to install.
export async function checkDependencies({ openIfMissing = true } = {}) {
  try { app.deps = await DependencyStatus(); } catch { return; }
  if (openIfMissing && app.deps.some((t) => t.required && !t.found)) openDependencies();
}

export function openDependencies() {
  app.depsModal = { running: false, log: [], error: "", done: false };
}

export function closeDependencies() { app.depsModal = null; }

// runPwshInstall drives the on-demand PowerShell 7 download. Progress arrives as
// events; the subscriptions are dropped on the terminal one.
export async function runPwshInstall() {
  const s = app.depsModal;
  if (!s || s.running) return;
  s.running = true; s.error = ""; s.done = false; s.log = [];
  const offLog = EventsOn("pwsh-install:log", (line) => { s.log.push(line); });
  const offDone = EventsOn("pwsh-install:done", async () => {
    s.running = false; s.done = true; cleanup();
    // Re-snapshot rather than assume: the installer puts the managed copy on
    // PATH, so the row should now resolve to it.
    await checkDependencies({ openIfMissing: false });
    toast("PowerShell 7 installed — sessions ready", "info");
  });
  const offErr = EventsOn("pwsh-install:error", (msg) => {
    s.running = false; s.error = "" + msg; cleanup();
  });
  function cleanup() { offLog?.(); offDone?.(); offErr?.(); }
  try {
    await InstallPwsh();
  } catch (e) {
    s.running = false; s.error = "" + e; cleanup();
  }
}

export async function launch() {
  app.launchError = "";
  if (!app.folder.trim()) { app.launchError = "Pick a project folder first."; return; }
  try {
    const m = app.models.find((x) => x.id === app.selectedModel);
    const rc = !!(prefs.rcAutoEnable && m && !m.routed);
    const s = await LaunchSession(app.folder, app.selectedModel, rc);
    await refresh();
    app.sessionKey = s.windowID;
  } catch (e) {
    app.launchError = "" + e;
    maybeOfferLitellmInstall(e);
  }
}

// ---- LiteLLM first-run runtime installer ----
// Routed models (zen/bedrock) need the LiteLLM proxy, a Python package the DMG
// can't ship inline. When the backend reports it missing, offer to build an
// app-managed venv from an in-app button instead of sending the user to a shell.

function isLitellmMissing(e) {
  return ("" + e).toLowerCase().includes("litellm not found");
}

// maybeOfferLitellmInstall opens the installer when the error is the missing
// runtime; otherwise no-op so the normal error surface (toast/inline) stands.
export function maybeOfferLitellmInstall(e) {
  if (isLitellmMissing(e)) openLitellmInstaller();
}

export async function openLitellmInstaller() {
  let st = { python: "", canInstall: false };
  try { st = await LitellmRuntimeStatus(); } catch {}
  app.litellmInstall = {
    python: st.python, canInstall: st.canInstall,
    running: false, log: [], error: "", done: false,
  };
}

export function closeLitellmInstaller() { app.litellmInstall = null; }

export async function runLitellmInstall() {
  const s = app.litellmInstall;
  if (!s || s.running) return;
  s.running = true; s.error = ""; s.done = false; s.log = [];
  // Live progress arrives as events; unsubscribe on the terminal event.
  const offLog = EventsOn("litellm-install:log", (line) => { s.log.push(line); });
  const offDone = EventsOn("litellm-install:done", () => {
    s.running = false; s.done = true; cleanup();
    toast("LiteLLM runtime installed — routed models ready", "info");
  });
  const offErr = EventsOn("litellm-install:error", (msg) => {
    s.running = false; s.error = "" + msg; cleanup();
  });
  function cleanup() { offLog?.(); offDone?.(); offErr?.(); }
  try {
    await InstallLitellmRuntime();
  } catch (e) {
    s.running = false; s.error = "" + e; cleanup();
  }
}

// Launch-confirm modal: history surfaces call askLaunch to open it; the modal's
// Launch calls confirmLaunch (sets folder+model, closes modal+drawer, launches);
// Cancel calls cancelLaunch.
export function askLaunch(folder, modelID, missing = false) {
  app.launchConfirm = { folder, modelID, missing };
}
export async function confirmLaunch(folder, modelID) {
  app.folder = folder;
  if (modelID) app.selectedModel = modelID;
  app.launchConfirm = null;
  app.drawer = null;
  await launch();
}
export function cancelLaunch() {
  app.launchConfirm = null;
}

// About panel: build info comes from Go (defaults to dev values in a plain
// browser); openURL hands links to the OS browser, never the app webview.
export async function buildInfo() {
  try { return await GetBuildInfo(); }
  catch { return { version: "dev", commit: "", buildDate: "" }; }
}
export function openURL(url) {
  try { BrowserOpenURL(url); } catch { /* no runtime in a plain browser */ }
}

export async function pickFolder() {
  try {
    const p = await PickFolder();
    if (p) app.folder = p;
  } catch (e) { app.launchError = "" + e; }
}

export async function select(windowID) {
  try {
    await SelectSession(windowID);
    app.finished[windowID] = false;
    app.sessionKey = windowID + ":" + Date.now(); // force pane reconnect
    await refresh();
  } catch (e) { toast("" + e, "error"); }
}

export async function killSession(windowID) {
  try {
    await KillSession(windowID);
    delete app.stats[windowID];
    forget(windowID);
    if (app.sessionKey.split(":")[0] === windowID) app.sessionKey = "";
    await refresh();
  } catch (e) { toast("" + e, "error"); }
}

export async function renameSession(windowID, name) {
  try {
    if (name.trim()) await RenameSession(windowID, name.trim());
    await refresh();
  } catch (e) { toast("" + e, "error"); }
}

export async function swapSession(windowID, modelID) {
  // RC survives native→native swaps (backend re-enables it); routed targets
  // can't bridge, so warn when the swap silently drops an active handoff.
  const hadRC = !!app.stats[windowID]?.remoteControl;
  const targetRouted = !!app.models.find((m) => m.id === modelID)?.routed;
  try {
    const s = await SwapModel(windowID, modelID);
    app.sessionKey = s.windowID + ":" + Date.now();
    await refresh();
    if (hadRC && targetRouted) {
      toast(`Swapped to ${modelID} — remote control dropped (routed sessions can't bridge)`);
    } else {
      toast(`Swapped to ${modelID}`);
    }
  } catch (e) { toast("" + e, "error"); maybeOfferLitellmInstall(e); }
}

// ---- config actions (drawers own their pending-input state; these throw so
// the drawer can render the error inline at the failing control) ----
export async function saveKey(env, value) {
  await SetKey(env, value.trim());
  app.keys = await KeyStatus();
  app.models = await Config();
  app.providers = await ListProviders();
}
export async function clearKey(env) {
  await ClearKey(env);
  app.keys = await KeyStatus();
  app.models = await Config();
  app.providers = await ListProviders();
}
export function providerLabel(type) {
  return { "opencode-go": "OpenCode Zen/Go", bedrock: "AWS Bedrock", "ollama-cloud": "Ollama Cloud" }[type] ?? type;
}
export async function addProvider(type, apiBase = "", region = "") {
  await AddProvider(type, apiBase, region);
  app.providers = await ListProviders();
  app.keys = await KeyStatus();
}
export async function removeProvider(type) {
  await RemoveProvider(type);
  app.providers = await ListProviders();
  app.keys = await KeyStatus();
  app.catalog = await Models();
  app.models = await Config();
}
export async function discoverZen() {
  return await DiscoverZenModels();
}
export async function discoverOllama() {
  return await DiscoverOllamaModels();
}
export async function addModel(model) {
  await AddModel(model);
  app.catalog = await Models();
  app.models = await Config();
  app.keys = await KeyStatus();
}
export async function removeModel(id) {
  await RemoveModel(id);
  app.catalog = await Models();
  app.models = await Config();
  app.keys = await KeyStatus();
}
export async function discoverBedrock(region) {
  return await DiscoverBedrockModels(region);
}
export async function fetchPlanUsage() {
  return await PlanUsage(); // throws -> drawer renders the error inline
}

export async function enableRemoteControl(windowID) {
  try {
    await EnableRemoteControl(windowID);
    toast("Remote control on — QR code in the terminal");
    await select(windowID); // put the QR on screen
  } catch (e) { toast("" + e, "error"); }
}

export function markFinished(windowID) {
  app.finished[windowID] = true;
  const s = app.sessions.find((x) => x.windowID === windowID);
  toast(`${s?.name ?? windowID} finished`);
  SessionStats(windowID).then((st) => { app.stats[windowID] = st; }).catch(() => {});
}
