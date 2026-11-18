// Rooms state shared by the card glyph, the docked panel, the switcher and
// the dialogs. Every binding call is guarded: specs that don't stub rooms
// must still run, and rooms stay off until the user sets them up.
import * as App from "../../../wailsjs/go/main/App.js";
import { EventsOn } from "../../../wailsjs/runtime/runtime.js";

export const rooms = $state({
  status: null, list: [], hidden: [], current: "", view: null, code: null, error: "", quit: 0, busy: false,
  // UX 1.5a: the docked panel, the cwd → ProjectRoom map, open dialogs.
  panel: { open: false, pinned: false },
  projects: {},
  machines: null, // null | "list" | "add" | "join" | "reset"
  share: null,    // { cwd, label, x, y } while the Share popover is open
});

async function call(name, ...args) {
  const fn = App[name];
  if (typeof fn !== "function") throw new Error("rooms unavailable");
  return fn(...args);
}

const known = new Set(); // room ids seen this run (new-room toast)
let primed = false;     // the first load only learns what is already here
let onNewRoom = null;
export const setNewRoomHandler = (fn) => (onNewRoom = fn);

export async function loadRooms() {
  try {
    rooms.status = await call("RoomsStatus");
    const list = (await call("Rooms")) || [];
    const first = !primed;
    primed = true;
    for (const r of list) {
      if (!known.has(r.id)) {
        known.add(r.id);
        if (!first && r.needsFolder && onNewRoom) onNewRoom(r);
      }
    }
    rooms.list = list;
    rooms.hidden = (await call("HiddenRooms").catch(() => [])) || [];
    if (rooms.current && !list.some((r) => r.id === rooms.current)) rooms.current = "";
    rooms.view = rooms.current ? await call("RoomView", rooms.current) : null;
    await refreshProjects(lastCwds, true);
  } catch { /* rooms off or not stubbed */ }
}

export async function act(name, ...args) {
  rooms.error = "";
  rooms.busy = true;
  try {
    const r = await call(name, ...args);
    await loadRooms();
    return r;
  } catch (e) {
    rooms.error = String(e?.message || e);
    return null;
  } finally {
    rooms.busy = false;
  }
}

// ---- project ↔ room ----
let lastCwds = [];
export async function refreshProjects(cwds, force = false) {
  const uniq = [...new Set((cwds || []).filter(Boolean))].sort();
  if (!force && uniq.join("\n") === lastCwds.join("\n")) return;
  lastCwds = uniq;
  if (!uniq.length) { rooms.projects = {}; return; }
  try { rooms.projects = (await call("ProjectRooms", uniq)) || {}; } catch { /* not stubbed */ }
}

export const summaryOf = (id) => rooms.list.find((r) => r.id === id);
export const projectOf = (cwd) => rooms.projects[cwd];

// ---- panel ----
export async function openPanelFor(id, pin = false) {
  rooms.current = id;
  rooms.panel.open = true;
  if (pin) rooms.panel.pinned = true;
  await loadRooms();
}
export function closePanel() { rooms.panel.open = false; rooms.panel.pinned = false; }
export function togglePanel(cwd) {
  if (rooms.panel.open) return closePanel();
  const id = projectOf(cwd)?.roomId || rooms.current || rooms.list.find((r) => !r.closed)?.id || "";
  rooms.panel.open = true;
  if (id && id !== rooms.current) { rooms.current = id; loadRooms(); }
}
// The panel follows the selected session's project unless the user pinned a room.
export function followSelection(cwd) {
  if (!rooms.panel.open || rooms.panel.pinned) return;
  const id = projectOf(cwd)?.roomId;
  if (id && id !== rooms.current) { rooms.current = id; loadRooms(); }
}

export async function shareProject(cwd) {
  const r = await act("ShareProject", cwd);
  if (r?.id) { rooms.share = null; await openPanelFor(r.id); }
  return r;
}

let wired = false;
export function initRooms() {
  if (wired) return;
  wired = true;
  try {
    EventsOn("rooms:changed", () => loadRooms());
    EventsOn("rooms:quitguard", (n) => (rooms.quit = n || 1));
  } catch {}
  loadRooms();
}

export const unread = () => rooms.list.reduce((n, r) => n + (r.closed ? 0 : r.unread || 0), 0);
export const queued = () => rooms.list.reduce((n, r) => n + (r.queued || 0), 0);
