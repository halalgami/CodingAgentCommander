// Shared app state ($state proxies) + all Wails-calling actions. Components
// never import wailsjs directly. Every action that can fail in a plain
// browser (no Wails runtime) catches and no-ops, keeping the shell mountable.
import {
  Config, LaunchSession, ListSessions, SelectSession, PickFolder,
  KillSession, RenameSession, SessionStats, KeyStatus, SetKey, ClearKey,
  Models, AddModel, RemoveModel, SwapModel, DiscoverBedrockModels,
  EnableRemoteControl, PlanUsage,
} from "../../wailsjs/go/main/App.js";
import { pushRecent } from "./recents.js";
import { prefs } from "./prefs.svelte.js";

export const app = $state({
  models: [], catalog: [], keys: [],
  sessions: [], stats: {}, finished: {},
  sessionKey: "", selectedModel: "", folder: "",
  drawer: null,            // null | "providers" | "models" | "settings" | "usage"
  paletteOpen: false,
  launchError: "",
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
}

export async function loadAll() {
  try {
    app.models = await Config();
    if (app.models.length && !app.selectedModel) app.selectedModel = app.models[0].id;
  } catch {}
  try { app.keys = await KeyStatus(); } catch {}
  try { app.catalog = await Models(); } catch {}
  await refresh();
}

export async function launch() {
  app.launchError = "";
  if (!app.folder.trim()) { app.launchError = "Pick a project folder first."; return; }
  try {
    const m = app.models.find((x) => x.id === app.selectedModel);
    const rc = !!(prefs.rcAutoEnable && m && !m.routed);
    const s = await LaunchSession(app.folder, app.selectedModel, rc);
    pushRecent({ folder: app.folder, modelID: app.selectedModel });
    await refresh();
    app.sessionKey = s.windowID;
  } catch (e) { app.launchError = "" + e; }
}

export async function launchInto(folder, modelID) {
  app.folder = folder;
  if (modelID) app.selectedModel = modelID;
  await launch();
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
  } catch (e) { toast("" + e, "error"); }
}

// ---- config actions (drawers own their pending-input state; these throw so
// the drawer can render the error inline at the failing control) ----
export async function saveKey(env, value) {
  await SetKey(env, value.trim());
  app.keys = await KeyStatus();
  app.models = await Config();
}
export async function clearKey(env) {
  await ClearKey(env);
  app.keys = await KeyStatus();
  app.models = await Config();
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
