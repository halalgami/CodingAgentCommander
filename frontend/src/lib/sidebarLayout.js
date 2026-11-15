// Pure sidebar layout ops. The layout mirrors Go's internal/sidebar.Layout:
// { groups: [{ id, name, collapsed, remember, folders }], order: { key: [windowID] } }
// where key is UNGROUPED or a group id. Nothing here mutates its input; every
// op returns a new layout, which sidebar.svelte.js applies and persists.

export const UNGROUPED = "ungrouped";

function clone(l) {
  return {
    groups: (l?.groups ?? []).map((g) => ({ ...g, folders: [...(g.folders ?? [])] })),
    order: Object.fromEntries(Object.entries(l?.order ?? {}).map(([k, v]) => [k, [...v]])),
  };
}

const ATTN_RANK = { permission: 3, error: 2, finished: 1, "": 0 };

function attnOf(s, stats, finished) {
  const st = stats?.[s.windowID];
  if (st?.activity === "permission") return "permission";
  if (st?.status === "error" || st?.errored) return "error";
  if (finished?.[s.windowID]) return "finished";
  return "";
}

// deriveSections turns the raw session list plus the layout into what the
// sidebar renders. Sessions the layout does not know yet (a poll can list a
// window before the next merge) go to the end of Ungrouped; layout ids with
// no live session are skipped. Ungrouped shows first and only when non-empty;
// empty groups stay, as somewhere to drop into.
export function deriveSections(sessions, layout, stats, finished) {
  const byID = new Map(sessions.map((s) => [s.windowID, s]));
  const placed = new Set();
  const pick = (ids) => (ids ?? []).filter((id) => byID.has(id) && !placed.has(id) && placed.add(id)).map((id) => byID.get(id));
  const groups = layout?.groups ?? [];
  const order = layout?.order ?? {};
  const ung = pick(order[UNGROUPED]);
  const grouped = groups.map((g) => ({ g, list: pick(order[g.id]) }));
  for (const s of sessions) if (!placed.has(s.windowID)) { placed.add(s.windowID); ung.push(s); }
  const section = (key, group, list) => {
    let attn = "";
    for (const s of list) {
      const a = attnOf(s, stats, finished);
      if (ATTN_RANK[a] > ATTN_RANK[attn]) attn = a;
    }
    return { key, group, sessions: list, attn, count: list.length };
  };
  const out = [];
  if (ung.length) out.push(section(UNGROUPED, null, ung));
  for (const { g, list } of grouped) out.push(section(g.id, g, list));
  return out;
}

export function sectionOf(layout, windowID) {
  for (const [k, ids] of Object.entries(layout?.order ?? {})) if (ids.includes(windowID)) return k;
  return UNGROUPED;
}

// moveSession moves windowID to toKey at index (clamped). Joining a group that
// remembers folders adds cwd to it; leaving a group drops cwd from it unless
// another member still there (cwds: windowID -> cwd) shares that folder.
export function moveSession(layout, windowID, toKey, index, cwd = "", cwds = {}) {
  const l = clone(layout);
  const from = sectionOf(l, windowID);
  for (const k of Object.keys(l.order)) l.order[k] = l.order[k].filter((id) => id !== windowID);
  const list = l.order[toKey] ?? (l.order[toKey] = []);
  list.splice(Math.max(0, Math.min(index, list.length)), 0, windowID);
  if (cwd && from !== toKey) {
    const old = l.groups.find((g) => g.id === from);
    if (old && !(l.order[from] ?? []).some((id) => cwds[id] === cwd)) old.folders = old.folders.filter((f) => f !== cwd);
    const neu = l.groups.find((g) => g.id === toKey);
    if (neu?.remember && !neu.folders.includes(cwd)) neu.folders.push(cwd);
  }
  return l;
}

export function moveGroup(layout, groupID, index) {
  const l = clone(layout);
  const i = l.groups.findIndex((g) => g.id === groupID);
  if (i < 0) return l;
  const [g] = l.groups.splice(i, 1);
  l.groups.splice(Math.max(0, Math.min(index, l.groups.length)), 0, g);
  return l;
}

function newID() {
  const b = new Uint8Array(4);
  crypto.getRandomValues(b);
  return "g_" + [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
}

// createGroup appends a group (remembering folders by default) and, when a
// session is given, moves it in.
export function createGroup(layout, name, windowID, cwd = "") {
  let l = clone(layout);
  const id = newID();
  l.groups.push({ id, name, collapsed: false, remember: true, folders: [] });
  l.order[id] = [];
  if (windowID) l = moveSession(l, windowID, id, 0, cwd);
  return { layout: l, id };
}

const patchGroup = (layout, id, patch) => {
  const l = clone(layout);
  const g = l.groups.find((x) => x.id === id);
  if (g) Object.assign(g, patch);
  return l;
};

export const renameGroup = (l, id, name) => patchGroup(l, id, { name });
export const setCollapsed = (l, id, collapsed) => patchGroup(l, id, { collapsed });
export const setRemember = (l, id, remember) => patchGroup(l, id, { remember });

// deleteGroup never touches sessions: its members go to the end of Ungrouped.
export function deleteGroup(layout, id) {
  const l = clone(layout);
  l.groups = l.groups.filter((g) => g.id !== id);
  l.order[UNGROUPED] = [...(l.order[UNGROUPED] ?? []), ...(l.order[id] ?? [])];
  delete l.order[id];
  return l;
}

// materialize writes the order the sidebar is showing (sections from
// deriveSections) back into the layout, so an edit made before Go's next
// merge never drops sessions the stored layout did not list yet.
export function materialize(layout, sections) {
  const l = clone(layout);
  for (const sec of sections) l.order[sec.key] = sec.sessions.map((s) => s.windowID);
  if (!sections.some((s) => s.key === UNGROUPED)) l.order[UNGROUPED] = [];
  return l;
}
