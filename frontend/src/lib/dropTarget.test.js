import { test } from "node:test";
import assert from "node:assert/strict";
import { raidDropStem, dropTarget } from "./dropTarget.js";

const el = (attrs, parent = null) => ({
  getAttribute: (k) => attrs[k] ?? null,
  closest(sel) {
    const m = sel.match(/^\[([\w-]+)\]$/);
    if (m && attrs[m[1]] !== undefined) return this;
    if (sel === ".pane" && attrs.class === "pane") return this;
    return parent ? parent.closest(sel) : null;
  },
});

test("a drop on a heroine cell names its stem", () => {
  const cell = el({ "data-raid-stem": "opus-attack" });
  assert.equal(raidDropStem(el({}, cell)), "opus-attack");
  assert.equal(raidDropStem(el({ "data-window-id": "@1" })), "");
  assert.equal(raidDropStem(null), "");
});

test("session drops are unchanged", () => {
  assert.equal(dropTarget(el({ "data-window-id": "@2" }), "@1"), "@2");
  assert.equal(dropTarget(el({ class: "pane" }), "@1"), "@1");
});
