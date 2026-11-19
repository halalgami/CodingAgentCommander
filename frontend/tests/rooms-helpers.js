import { expect } from "@playwright/test";
import { stubDash } from "./dashboard-helpers.js";

// Rooms UX 1.5a harness. window.__rooms holds status, list, view, projects
// (cwd → ProjectRoom), hidden; every binding call is recorded in __calls.
export async function openRooms(page, state = {}, dash = {}) {
  await stubDash(page, dash);
  await page.addInitScript((state) => {
    const R = window.__rooms = Object.assign({
      status: { setUp: true, first: true, name: "Hussain", machine: "mini", device: "me", tailscaleUp: true },
      list: [], view: null, projects: {}, hidden: [], suggest: [], machines: [],
    }, state);
    const rec = (n, ...a) => window.__calls.push([n, ...a]);
    const changed = () => (window.__handlers["rooms:changed"] || []).forEach((f) => f());
    window.__roomsChanged = changed;
    Object.assign(window.go.main.App, {
      RoomsStatus: async () => R.status,
      Rooms: async () => R.list,
      RoomView: async (id) => (R.views && R.views[id]) || R.view,
      HiddenRooms: async () => R.hidden,
      Machines: async () => R.machines,
      ProjectRooms: async (cwds) => Object.fromEntries(cwds.map((c) => [c, R.projects[c] || { identity: "", root: c, label: c.split("/").pop(), roomId: "" }])),
      ShareProject: async (cwd) => { rec("ShareProject", cwd); const r = { id: "r9", name: "proj", folder: cwd };
        R.list = [...R.list, r]; R.projects[cwd] = { ...(R.projects[cwd] || {}), roomId: "r9" }; R.view = { ...r, threads: [], peers: [], outbox: [] }; return r; },
      SuggestFolders: async (id) => { rec("SuggestFolders", id); return R.suggest; },
      RevealFolder: async (id) => rec("RevealFolder", id),
      RoomsSetup: async (...a) => { rec("RoomsSetup", ...a); R.status = { ...R.status, setUp: a[0], joining: !a[0], first: a[0] }; },
      PairCode: async (n) => { rec("PairCode", n); return { code: "CMDR-ABC", net: n, expires: Math.floor(Date.now() / 1000) + 600 }; },
      InviteCode: async (r, n) => { rec("InviteCode", r, n); return { code: "CMDR-INV", net: n, expires: Math.floor(Date.now() / 1000) + 86400 }; },
      JoinCode: async (...a) => { rec("JoinCode", ...a); R.status = { ...R.status, join: { kind: "pair", net: "tailscale", words: ["amber", "comet", "lotus", "tiger"], status: "waiting" } }; return { net: "tailscale", words: ["amber", "comet", "lotus", "tiger"] }; },
      CancelJoin: async () => { rec("CancelJoin"); R.status = { ...R.status, join: { ...R.status.join, status: "failed", error: "cancelled" } }; },
      ConfirmCode: async (ok) => { rec("ConfirmCode", ok); R.status = { ...R.status, pending: null }; },
      CancelCode: async () => rec("CancelCode"),
      PickRoomFolder: async () => "/picked",
      PostRoomMessage: async (...a) => rec("PostRoomMessage", ...a),
      MarkRoomSeen: async (...a) => rec("MarkRoomSeen", ...a),
      SetDeviceNetwork: async (...a) => rec("SetDeviceNetwork", ...a),
      BindRoomFolder: async (...a) => rec("BindRoomFolder", ...a),
      CloseRoom: async (id) => { rec("CloseRoom", id); R.list = R.list.map((r) => (r.id === id ? { ...r, closed: true, closedBy: "mini" } : r)); },
      LeaveRoom: async (id) => rec("LeaveRoom", id),
      RemoveRoomLocal: async (id) => rec("RemoveRoomLocal", id),
      RestoreRoom: async (id) => rec("RestoreRoom", id),
      ResetRooms: async () => rec("ResetRooms"),
      QueuedCount: async () => 0,
      RenameRoom: async (...a) => { rec("RenameRoom", ...a); R.list = R.list.map((r) => (r.id === a[0] ? { ...r, name: a[1] } : r)); },
      GitHead: async (id) => { rec("GitHead", id); return { branch: "main", sha: "0123456789abcdef0123456789abcdef01234567" }; },
      RecentProjects: async () => R.recent || [],
      ProjectRoomByKey: async (k) => { rec("ProjectRoomByKey", k); await new Promise((r) => setTimeout(r, (R.delay || {})[k] || 0)); return (R.byKey || {})[k] || { root: "" }; },
      QuitAnyway: async () => rec("QuitAnyway"),
    });
  }, state);
  await page.goto("/?nointro");
  await expect(page.getByTestId("open-rooms")).toBeVisible();
}

export const calls = (page, n) => page.evaluate((n) => window.__calls.filter((c) => c[0] === n), n);

export const SHARED = { "/Users/me/proj": { identity: "rootcommit:abc", root: "/Users/me/proj", label: "proj", roomId: "r1" } };
