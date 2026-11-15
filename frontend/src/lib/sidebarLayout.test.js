import { test } from "node:test";
import assert from "node:assert/strict";
import {
  UNGROUPED, deriveSections, moveSession, moveGroup, createGroup, renameGroup,
  deleteGroup, setCollapsed, setRemember, sectionOf,
} from "./sidebarLayout.js";

const S = (id) => ({ windowID: id, name: id });
const L = (groups, order) => ({ groups, order });
const G = (id, extra = {}) => ({ id, name: id, ...extra });

test("ungrouped comes first and is hidden when empty; empty groups stay", () => {
  const l = L([G("g_1"), G("g_2")], { ungrouped: [], g_1: ["@1"], g_2: [] });
  const secs = deriveSections([S("@1")], l, {}, {});
  assert.deepEqual(secs.map((s) => s.key), ["g_1", "g_2"]);
  const l2 = L([G("g_1")], { ungrouped: ["@2"], g_1: ["@1"] });
  assert.deepEqual(deriveSections([S("@1"), S("@2")], l2, {}, {}).map((s) => s.key), [UNGROUPED, "g_1"]);
});

test("sessions missing from the layout append to ungrouped; dead ids drop", () => {
  const l = L([G("g_1")], { ungrouped: ["@9"], g_1: ["@1"] });
  const secs = deriveSections([S("@1"), S("@3")], l, {}, {});
  assert.deepEqual(secs[0].sessions.map((s) => s.windowID), ["@3"]);
  assert.deepEqual(secs[1].sessions.map((s) => s.windowID), ["@1"]);
  assert.equal(secs[1].count, 1);
});

test("attention: permission beats error beats finished", () => {
  const l = L([G("g_1", { collapsed: true })], { ungrouped: [], g_1: ["@1", "@2"] });
  const s = deriveSections([S("@1"), S("@2")], l, { "@2": { activity: "permission" } }, { "@1": true });
  assert.equal(s[0].attn, "permission");
  const f = deriveSections([S("@1"), S("@2")], l, {}, { "@1": true });
  assert.equal(f[0].attn, "finished");
});

test("move within a section and between sections; input untouched", () => {
  const l = L([G("g_1")], { ungrouped: ["@1", "@2", "@3"], g_1: [] });
  const before = structuredClone(l);
  const a = moveSession(l, "@3", UNGROUPED, 0);
  assert.deepEqual(a.order.ungrouped, ["@3", "@1", "@2"]);
  const b = moveSession(l, "@2", "g_1", 0);
  assert.deepEqual(b.order.ungrouped, ["@1", "@3"]);
  assert.deepEqual(b.order.g_1, ["@2"]);
  assert.deepEqual(l, before);
});

test("remembering: joining adds the folder, leaving drops it unless a sibling shares it", () => {
  const l = L([G("g_1", { remember: true, folders: [] })], { ungrouped: ["@1", "@2"], g_1: [] });
  let x = moveSession(l, "@1", "g_1", 0, "/p/a");
  assert.deepEqual(x.groups[0].folders, ["/p/a"]);
  x = moveSession(x, "@2", "g_1", 1, "/p/a");
  assert.deepEqual(x.groups[0].folders, ["/p/a"]);
  x = moveSession(x, "@1", UNGROUPED, 0, "/p/a", { "@2": "/p/a" });
  assert.deepEqual(x.groups[0].folders, ["/p/a"]);
  x = moveSession(x, "@2", UNGROUPED, 0, "/p/a", {});
  assert.deepEqual(x.groups[0].folders, []);
  const off = L([G("g_1", { remember: false, folders: [] })], { ungrouped: ["@1"], g_1: [] });
  assert.deepEqual(moveSession(off, "@1", "g_1", 0, "/p/a").groups[0].folders, []);
});

test("index clamps to the section length", () => {
  const l = L([], { ungrouped: ["@1", "@2"] });
  assert.deepEqual(moveSession(l, "@1", UNGROUPED, 99).order.ungrouped, ["@2", "@1"]);
});

test("groups: create (with a session), rename, reorder, collapse, remember, delete", () => {
  let l = L([G("g_1")], { ungrouped: ["@1", "@2"], g_1: [] });
  const r = createGroup(l, "Ayina", "@2", "/p/ayina");
  assert.match(r.id, /^g_[0-9a-f]{8}$/);
  l = r.layout;
  assert.equal(l.groups.at(-1).name, "Ayina");
  assert.equal(l.groups.at(-1).remember, true);
  assert.deepEqual(l.groups.at(-1).folders, ["/p/ayina"]);
  assert.deepEqual(l.order[r.id], ["@2"]);
  assert.deepEqual(l.order.ungrouped, ["@1"]);
  l = renameGroup(l, r.id, "Ayina RDP");
  assert.equal(l.groups.find((g) => g.id === r.id).name, "Ayina RDP");
  l = moveGroup(l, r.id, 0);
  assert.deepEqual(l.groups.map((g) => g.id), [r.id, "g_1"]);
  l = setCollapsed(l, r.id, true);
  assert.equal(l.groups[0].collapsed, true);
  l = setRemember(l, r.id, false);
  assert.equal(l.groups[0].remember, false);
  assert.equal(sectionOf(l, "@2"), r.id);
  l = deleteGroup(l, r.id);
  assert.deepEqual(l.groups.map((g) => g.id), ["g_1"]);
  assert.deepEqual(l.order.ungrouped, ["@1", "@2"]);
  assert.equal(r.id in l.order, false);
});

test("sectionOf falls back to ungrouped for unknown ids", () => {
  assert.equal(sectionOf(L([], { ungrouped: [] }), "@7"), UNGROUPED);
});

test("materialize writes the displayed order into the layout", async () => {
  const { materialize } = await import("./sidebarLayout.js");
  const l = L([G("g_1")], { ungrouped: [], g_1: ["@9", "@2"] });
  const secs = deriveSections([S("@1"), S("@2"), S("@3")], l, {}, {});
  const m = materialize(l, secs);
  assert.deepEqual(m.order.ungrouped, ["@1", "@3"]);
  assert.deepEqual(m.order.g_1, ["@2"]);
  assert.deepEqual(moveSession(m, "@3", UNGROUPED, 0).order.ungrouped, ["@3", "@1"]);
});
