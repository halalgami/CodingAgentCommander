import { test } from "node:test";
import assert from "node:assert/strict";
import { xOf, rowOf, layoutLane, tributaries, waitGaps, sparkline, fmtDuration } from "./river.js";

const W = 600, WIN = 600000, NOW = 1_000_000;

test("x maps time onto the lane and clamps (Review Focus 5: clock skew)", () => {
  assert.equal(xOf(NOW, NOW, WIN, W), W);
  assert.equal(xOf(NOW - WIN / 2, NOW, WIN, W), W / 2);
  assert.equal(xOf(NOW + 5000, NOW, WIN, W), W);
  assert.equal(xOf(0, NOW, WIN, W), 0);
});

test("rows: read/web/other top, edit/think middle, shell bottom", () => {
  assert.deepEqual(["read", "web", "other", "edit", "think", "agent", "shell"].map(rowOf), [0, 0, 0, 1, 1, 1, 2]);
});

test("layoutLane drops events outside the window and lead-only", () => {
  const g = layoutLane([{ at: NOW - WIN - 1, kind: "read" }, { at: NOW - 1000, kind: "edit", label: "Edit a.go" }, { at: NOW, kind: "read", agent: "a1" }], NOW, WIN, W);
  assert.equal(g.length, 1); assert.equal(g[0].row, 1); assert.equal(g[0].label, "Edit a.go");
});

test("tributaries branch and merge; >3 open agents braid", () => {
  const crew = [{ id: "a1", startedMs: NOW - 60000, endedMs: NOW - 30000, background: false }];
  const t = tributaries(crew, [{ at: NOW - 50000, kind: "read", agent: "a1" }], NOW, WIN, W);
  assert.equal(t.length, 1); assert.ok(t[0].x0 < t[0].x1); assert.equal(t[0].glyphs.length, 1);
  const open = Array.from({ length: 5 }, (_, i) => ({ id: "b" + i, startedMs: NOW - 10000 * (i + 1), endedMs: 0, background: i % 2 === 0 }));
  const b = tributaries(open, [], NOW, WIN, W);
  assert.equal(b.length, 1); assert.equal(b[0].braid, true); assert.equal(b[0].count, 5); assert.equal(b[0].x1, null);
});

test("wait gaps: past pairs and the current wait", () => {
  const gaps = waitGaps([{ at: NOW - 100000, kind: "wait", label: "start" }, { at: NOW - 40000, kind: "wait", label: "end" }],
    { activity: "permission", sinceMs: NOW - 134000 }, NOW, WIN, W);
  assert.equal(gaps.length, 2);
  assert.equal(gaps[1].label, "Waiting 2m 14s"); assert.equal(gaps[1].x1, W);
});

test("sparkline counts tool marks per bucket, ignoring think and wait", () => {
  const s = sparkline([{ at: NOW - 1, kind: "edit" }, { at: NOW - 2, kind: "think" }, { at: NOW - WIN + 1, kind: "read" }], NOW, WIN, 10);
  assert.equal(s.length, 10); assert.equal(s[9], 1); assert.equal(s[0], 1);
});

test("fmtDuration never negative", () => {
  assert.equal(fmtDuration(134000), "2m 14s"); assert.equal(fmtDuration(45000), "45s");
  assert.equal(fmtDuration(3780000), "1h 3m"); assert.equal(fmtDuration(-5000), "0s");
});

// Minors round 2 (phase 4 review): a tributary that starts "now" began at the
// lane's right edge and its 20 px curve drew past the river head.
test("tributaries: a brand-new agent leaves room for its curve", async () => {
  const { tributaries } = await import("./river.js");
  const now = 1_000_000, width = 600;
  const [t] = tributaries([{ id: "a", startedMs: now }], [], now, 600_000, width);
  assert.ok(t.x0 + 24 <= width, `x0 ${t.x0}`);
  const many = Array.from({ length: 5 }, (_, i) => ({ id: "b" + i, startedMs: now }));
  const braid = tributaries(many, [], now, 600_000, width).find((x) => x.braid);
  assert.ok(braid.x0 + 24 <= width, `braid x0 ${braid.x0}`);
});
