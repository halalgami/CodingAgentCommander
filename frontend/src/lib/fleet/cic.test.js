// frontend/src/lib/fleet/cic.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import { RINGS, R, CX, CY, sectorsOf, contactsOf, placeLabels, labelAll, labelSize, ordersOf, commsOf, statusOf, callsign } from "./cic.js";

const row = (key, projectKey, activity = "tool", extra = {}) => ({ key, id: "s-" + key, project: projectKey, projectKey, activity, sinceMs: 1000, crew: [], ...extra });

test("sectorsOf: sorted, sums to 360, floor 30°, single project is the full circle", () => {
  const rows = [row("a", "zeta"), row("b", "alpha"), ...Array.from({ length: 8 }, (_, i) => row("c" + i, "mid"))];
  const s = sectorsOf(rows);
  assert.deepEqual(s.map((x) => x.label), ["alpha", "mid", "zeta"]);
  assert.equal(s[0].start, 0);
  assert.equal(s.at(-1).end, 360);
  for (const x of s) assert.ok(x.end - x.start >= 30 - 1e-9);
  assert.ok(s[1].end - s[1].start > s[0].end - s[0].start);
  assert.deepEqual(sectorsOf([row("a", "p")]).map((x) => [x.start, x.end]), [[0, 360]]);
  assert.deepEqual(sectorsOf([]), []);
  const many = sectorsOf(Array.from({ length: 15 }, (_, i) => row("k" + i, "p" + String(i).padStart(2, "0"))));
  assert.equal(many.length, 15);
  assert.ok(Math.abs(many.at(-1).end - 360) < 1e-9);
});

test("contactsOf: ring radius by band, inside the plot, stable", () => {
  const rows = [row("w", "p", "permission"), row("t", "p", "tool"), row("q", "p", "idle")];
  const c = contactsOf(rows, sectorsOf(rows));
  const by = Object.fromEntries(c.map((x) => [x.key, x]));
  const dist = (x) => Math.hypot(x.x - CX, x.y - CY);
  assert.equal(by.w.ring, "needs");
  assert.ok(Math.abs(dist(by.w) - RINGS.needs * R) < 1e-6);
  assert.ok(Math.abs(dist(by.t) - RINGS.working * R) < 1e-6);
  assert.ok(Math.abs(dist(by.q) - RINGS.quiet * R) < 1e-6);
  assert.deepEqual(contactsOf(rows, sectorsOf(rows)), c);
  for (const x of c) assert.ok(dist(x) <= R);
});

test("placeLabels: no overlaps; boxed-in contact collapses to callsign", () => {
  const rows = Array.from({ length: 8 }, (_, i) => row("c" + i, "p", "tool"));
  const c = contactsOf(rows, sectorsOf(rows));
  const size = () => ({ w: 160, h: 40 });
  const placed = placeLabels(c, size);
  const boxes = [];
  for (const x of c) {
    const p = placed.get(x.key);
    if (!p) continue;
    const x0 = p.anchor === "start" ? x.x + p.dx : p.anchor === "end" ? x.x + p.dx - 160 : x.x + p.dx - 80;
    const y0 = x.y + p.dy - 20;
    boxes.push([x0, y0, x0 + 160, y0 + 40]);
  }
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
    const [a, b] = [boxes[i], boxes[j]];
    assert.ok(a[2] <= b[0] || b[2] <= a[0] || a[3] <= b[1] || b[3] <= a[1], "labels overlap");
  }
  const huge = placeLabels(c, () => ({ w: 900, h: 900 }));
  assert.ok([...huge.values()].every((v) => v === null));
});

test("ordersOf: selected needs contact, else the oldest needs", () => {
  const rows = [row("a", "p", "permission", { sinceMs: 5000 }), row("b", "p", "error", { sinceMs: 2000 }), row("c", "p", "tool")];
  const c = contactsOf(rows, sectorsOf(rows));
  assert.equal(ordersOf(c, null), "b");
  assert.equal(ordersOf(c, "a"), "a");
  assert.equal(ordersOf(c, "c"), "b");
  assert.equal(ordersOf(contactsOf([row("c", "p")], sectorsOf([row("c", "p")])), null), null);
});

test("commsOf: actions, waits, breaches, clears; newest first; cap 40; how long ago", () => {
  const now = 10 * 60000;
  const a = row("a", "p", "permission", { sinceMs: 9 * 60000 });
  const b = row("b", "p", "error", { sinceMs: 8 * 60000 });
  const c = row("c", "p", "finished", { sinceMs: 7 * 60000, unread: true });
  const tl = {
    "s-a": [{ at: 60000, kind: "shell", label: "npm test" }, { at: 2 * 60000, kind: "think" }, { at: 3 * 60000, kind: "agent", agent: "x" }],
    "s-b": [{ at: 4 * 60000, kind: "read", label: "app.go" }, { at: 5 * 60000, kind: "wait", label: "start" }],
  };
  const log = commsOf([a, b, c], tl, now);
  const cs = (r) => callsign(r);
  assert.deepEqual(log.map((x) => x.text), [
    `${cs(a)} · AWAITING ORDERS`, `${cs(b)} · HULL BREACH`, `${cs(c)} · SECTOR CLEARED`,
    `${cs(b)} · AWAITING ORDERS`, `${cs(b)} · SENSOR SWEEP · app.go`, `${cs(a)} · TORPEDO SALVO · npm test`,
  ]);
  assert.deepEqual(log.map((x) => x.tone), ["amber", "red", "cyan", "amber", "green", "green"]);
  // Owner, 2026-10-14: a mission clock in whole minutes showed T+00:09 on every row.
  // Each row now says how long ago it happened.
  assert.equal(log.at(-1).clock, "9m"); // the shell at 1 min, now 10 min
  assert.equal(log[0].clock, "1m");
  assert.match(cs(a), /^[A-Z]+$/);
  const lots = { "s-a": Array.from({ length: 60 }, (_, i) => ({ at: i * 1000, kind: "edit" })) };
  assert.equal(commsOf([row("a", "p", "tool")], lots, now).length, 40);
});

test("commsOf clock: now, seconds, minutes, hours", () => {
  const at = (ms) => commsOf([row("a", "p", "error", { sinceMs: 100_000_000 - ms })], {}, 100_000_000)[0].clock;
  assert.deepEqual([at(2000), at(42_000), at(3 * 60000), at(2 * 3600000 + 5 * 60000)], ["now", "42s", "3m", "2h"]);
});

test("statusOf counts bands and hostiles", () => {
  const rows = [row("a", "p", "permission"), row("b", "p", "tool"), row("c", "p", "tool"), row("d", "p", "idle")];
  assert.deepEqual(statusOf(rows), { awaiting: 1, engaged: 2, holding: 1, hostiles: 3 });
});

test("placeLabels avoids extra obstacles (encounter markers)", () => {
  const rows = [row("solo", "p", "tool")];
  const c = contactsOf(rows, sectorsOf(rows));
  const size = () => ({ w: 60, h: 20 });
  const free = placeLabels(c, size).get("solo");
  assert.deepEqual([free.dx, free.dy], [18, 0]);
  const blockRight = [[c[0].x + 10, c[0].y - 30, c[0].x + 120, c[0].y + 30]];
  const moved = placeLabels(c, size, blockRight).get("solo");
  assert.ok(moved && moved.dx < 0, "label should move off the obstacle");
});

test("narrow inner-ring cells stagger radially so glyphs never pile up", () => {
  const rows = Array.from({ length: 15 }, (_, i) => row("k" + i, "p" + String(i).padStart(2, "0"), "tool"));
  rows.push(row("w1", "p00", "permission"), row("w2", "p00", "permission"), row("w3", "p00", "permission"));
  const c = contactsOf(rows, sectorsOf(rows)).filter((x) => x.ring === "needs");
  assert.equal(c.length, 3);
  for (let i = 0; i < c.length; i++) for (let j = i + 1; j < c.length; j++) {
    assert.ok(Math.hypot(c[i].x - c[j].x, c[i].y - c[j].y) >= 18, "needs contacts too close");
  }
});

test("labelAll: full labels first, then callsign-only, never overlapping; glyph-only when nothing fits", () => {
  const rows = Array.from({ length: 8 }, (_, i) => row("c" + i, "p", "tool"));
  const c = contactsOf(rows, sectorsOf(rows));
  const res = labelAll(c, () => ({ w: 900, h: 900 }), () => ({ w: 70, h: 15 }), []);
  const kinds = [...res.values()].map((v) => (v ? v.mode : null));
  assert.ok(kinds.every((k) => k !== "full"));
  assert.ok(kinds.includes("cs"));
  const boxes = c.filter((x) => res.get(x.key)).map((x) => res.get(x.key).box);
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
    const [a, b] = [boxes[i], boxes[j]];
    assert.ok(a[2] <= b[0] || b[2] <= a[0] || a[3] <= b[1] || b[3] <= a[1], "callsigns overlap");
  }
  const none = labelAll(c, () => ({ w: 900, h: 900 }), () => ({ w: 900, h: 900 }), []);
  assert.ok([...none.values()].every((v) => v === null));
});

test("labelSize scales with font; 12 keeps the CIC metrics", () => {
  assert.deepEqual(labelSize(["abcdefghij"]), { w: 76, h: 15 });
  const s = labelSize(["abcdefghij", "x"], 16);
  assert.ok(Math.abs(s.w - 101.3) < 0.1 && Math.abs(s.h - 40) < 0.01);
});

test("placeLabels: gap and a rectangular bound replace the plot circle", () => {
  const within = (b) => b[0] >= 0 && b[1] >= 0 && b[2] <= 1600 && b[3] <= 900;
  const c = [{ key: "a", row: {}, x: 1400, y: 450 }]; // outside the CIC circle (x > 940)
  const size = () => ({ w: 80, h: 20 });
  assert.equal(placeLabels(c, size).get("a"), null);
  const p = placeLabels(c, size, [], { gap: 70, within }).get("a");
  assert.deepEqual(p, { dx: 70, dy: 0, anchor: "start" });
  const full = labelAll(c, size, size, [[1460, 430, 1500, 470]], { gap: 70, within }).get("a");
  assert.equal(full.mode, "full");
  assert.equal(full.anchor, "end"); // right side blocked by the obstacle
  assert.equal(full.dx, -70);
});

// Owner, 2026-10-14: the file name at the end of a label was cut off. Go now
// shortens paths to their tail; the log shows the whole label and wraps it.
test("commsOf: the whole tool label is shown (no 40-character clip)", () => {
  const l = "Bash cat …/feedback/feedback_never_pipe_a_gate.md | head";
  const [m] = commsOf([row("a", "p", "tool")], { "s-a": [{ at: 1000, kind: "shell", label: l }] }, 2000);
  assert.ok(m.text.endsWith(l), m.text);
});

// Minors round 2 (CIC review): labels covered the ORDERS pill, crew blips, the
// ring names and the rim sector labels — none of them were obstacles.
test("plotObstacles: pill, blips, ring labels and sector labels are all obstacles", async () => {
  const { plotObstacles, sectorLabel } = await import("./cic.js");
  const contacts = [{ key: "a", x: 500, y: 400, ring: "needs", row: { crew: [{ id: "x", state: "tool" }] } }];
  const sectors = [{ key: "p", label: "proj", start: 0, end: 360, count: 1, needs: 1 }];
  const boxes = plotObstacles(contacts, sectors, null);
  const covers = (x, y) => boxes.some(([x0, y0, x1, y1]) => x >= x0 && x <= x1 && y >= y0 && y <= y1);
  assert.ok(covers(500, 424), "ORDERS pill");
  assert.ok(covers(474, 422), "crew blip");
  assert.ok(covers(CX, CY - RINGS.quiet * R - 8), "ring label");
  const t = { x: CX, y: CY + (R - 22) }; // single sector: label at 180°
  assert.ok(covers(t.x, t.y - 4), "sector label");
  assert.equal(typeof sectorLabel, "function");
});

// Minors round 2 (CIC review): rim sector labels overlapped at 15+ projects.
test("sectorLabel: clipped to the sector's chord, hidden when there is no room", async () => {
  const { sectorLabel } = await import("./cic.js");
  const wide = sectorLabel({ label: "a-really-long-project-name", start: 0, end: 180, count: 2, needs: 1 });
  assert.equal(wide.title, "SECTOR A-REALLY-LONG-PROJECT-NAME");
  const narrow = sectorLabel({ label: "a-really-long-project-name", start: 0, end: 24, count: 1, needs: 0 });
  assert.ok(narrow.title.length < "SECTOR A-REALLY-LONG-PROJECT-NAME".length && narrow.title.endsWith("…"));
  assert.ok(narrow.w <= 2 * (R - 22) * Math.sin((24 / 2) * Math.PI / 180) + 1);
  assert.equal(sectorLabel({ label: "x", start: 0, end: 4, count: 1, needs: 0 }).title, "");
});

// Minors round 2 (CIC review): a status row with no start time was stamped
// "now" on every tick, so it stayed at the top of the log forever.
test("commsOf: status rows without a start time are left out", () => {
  const log = commsOf([row("a", "p", "error", { sinceMs: 0 }), row("b", "p", "finished", { sinceMs: 0, unread: true })], {}, 10_000);
  assert.deepEqual(log, []);
});

// Minors round 2: an event with no timestamp showed "NaNh".
test("commsOf clock: no timestamp shows no clock", () => {
  const [m] = commsOf([row("a", "p", "tool")], { "s-a": [{ kind: "edit", label: "x" }] }, 1000);
  assert.equal(m.clock, "");
});

// Final review I3: the plot marks made labels vanish ~7x more often. They now
// only push FULL labels aside; a ship can always fall back to its callsign,
// and the result no longer depends on which waiting ship holds the orders box.
test("cicLabels: the plot marks never leave more ships unlabelled than before", async () => {
  const { cicLabels } = await import("./cic.js");
  let seed = 7;
  const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
  const states = ["tool", "permission", "idle", "thinking"];
  let before = 0, after = 0;
  for (let t = 0; t < 120; t++) {
    const np = 1 + Math.floor(rnd() * 12), ns = 2 + Math.floor(rnd() * 20);
    const rows = Array.from({ length: ns }, (_, i) => row("r" + i, "p" + Math.floor(rnd() * np), states[Math.floor(rnd() * 4)],
      { title: "task " + i, model: "claude-opus-5-5", crew: rnd() < 0.3 ? [{ id: "c", state: "tool" }] : [] }));
    const sectors = sectorsOf(rows), contacts = contactsOf(rows, sectors);
    const full = (r) => labelSize([callsign(r), "FLAGSHIP CARRIER · model", String(r.title)]), cs = (r) => labelSize([callsign(r)]);
    before += [...labelAll(contacts, full, cs, []).values()].filter((v) => !v).length;
    after += [...cicLabels(contacts, sectors, [], full, cs).values()].filter((v) => !v).length;
  }
  assert.ok(after <= before, `unlabelled ships: ${after} now vs ${before} without plot marks`);
});
