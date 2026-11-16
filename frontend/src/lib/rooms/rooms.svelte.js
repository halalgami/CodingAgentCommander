// Rooms drawer state. Every binding call is guarded: specs that don't stub
// rooms must still run, and rooms stay off until the user sets them up.
import * as App from "../../../wailsjs/go/main/App.js";
import { EventsOn } from "../../../wailsjs/runtime/runtime.js";

export const rooms = $state({ status: null, list: [], current: "", view: null, code: null, error: "", quit: 0, busy: false });

async function call(name, ...args) {
  const fn = App[name];
  if (typeof fn !== "function") throw new Error("rooms unavailable");
  return fn(...args);
}

export async function loadRooms() {
  try {
    rooms.status = await call("RoomsStatus");
    rooms.list = (await call("Rooms")) || [];
    if (!rooms.current && rooms.list.length) rooms.current = rooms.list[0].id;
    if (rooms.current) rooms.view = await call("RoomView", rooms.current);
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

export const unread = () => rooms.list.reduce((n, r) => n + (r.unread || 0), 0);
export const queued = () => rooms.list.reduce((n, r) => n + (r.queued || 0), 0);
