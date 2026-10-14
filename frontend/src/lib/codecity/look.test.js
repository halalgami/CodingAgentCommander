import { test } from "node:test";
import assert from "node:assert/strict";
import { boxHeight, colorOf, heatColor, heatOf, cranesFor } from "./look.js";

test("box height from complexity, clamped", () => {
  assert.equal(boxHeight(0), 8);
  assert.ok(boxHeight(10) > 20 && boxHeight(10) < 24);
  assert.equal(boxHeight(1e9), 120);
});

test("language colour, unknown grey", () => {
  assert.notEqual(colorOf({ lang: "Go" }), colorOf({ lang: "Nope" }));
  assert.equal(colorOf({ lang: "Nope" }), "#a3a3a3");
});

test("heat scale ends", () => {
  assert.equal(heatColor(0), "rgb(59, 111, 182)");
  assert.equal(heatColor(1), "rgb(224, 69, 43)");
});

test("heat window filters by last touch, intensity is all-time", () => {
  const now = 100 * 86400000;
  const files = [
    { path: "a", touch: { edits: 10, writes: 0, lastAt: now - 86400000 } },
    { path: "b", touch: { edits: 1, writes: 0, lastAt: now - 20 * 86400000 } },
    { path: "c", touch: { edits: 0, writes: 0, lastAt: 0 } },
  ];
  const w7 = heatOf(files, "7d", now), w30 = heatOf(files, "30d", now), all = heatOf(files, "all", now);
  assert.equal(w7.get("a"), 1); assert.equal(w7.has("b"), false);
  assert.ok(w30.get("b") > 0 && w30.get("b") < 1);
  assert.equal(all.has("c"), false);
});

test("cranes: lead and agents on files of this project, capped", () => {
  const row = (key, targetRel, crew = [], projectKey = "k1") => ({ key, targetRel, crew, projectKey, activity: "tool" });
  const rows = [
    row("@1", "a.go", [{ id: "x", state: "tool", targetRel: "a.go" }, { id: "y", state: "tool", targetRel: "a.go" }, { id: "z", state: "tool", targetRel: "a.go" }]),
    row("@2", "b.go", [], "other"),
    ...Array.from({ length: 12 }, (_, i) => row("@x" + i, `f${i}.go`)),
  ];
  const c = cranesFor(rows, "k1");
  assert.equal(c.byFile.get("a.go").length, 3, "≤ 3 per file");
  assert.equal(c.byFile.has("b.go"), false, "other project");
  assert.equal(c.shown, 12); assert.ok(c.more > 0);
});
