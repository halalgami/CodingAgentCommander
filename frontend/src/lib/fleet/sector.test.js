// frontend/src/lib/fleet/sector.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import { systemsOf, SLOTS, placeSystems, stationOf, dockOf, geomOf, ordersStrip, chipText, fmtHours, headerOf, UNCHARTED } from "./sector.js";

const P = (key, hours, rank) => ({ key, label: key.toUpperCase(), hours, rank });
const row = (key, projectKey, activity, sinceMs = 0, extra = {}) => ({ key, id: "s" + key, projectKey, project: projectKey || "tmp", activity, sinceMs, unread: false, crewOut: 0, ...extra });
const PROJECTS = [{ key: "a", label: "Alpha" }, { key: "b", label: "Beta" }, { key: "n", label: "Newbie" }];

// Owner, 2026-10-14: like Al Modon, only projects with a live session are systems.
test("systemsOf: only projects with a live session, keeping their ledger hours and rank", () => {
  const prog = { locked: false, projects: [P("b", 60, "Station"), P("a", 3, "Outpost")] };
  const s = systemsOf(prog, PROJECTS, [row("@1", "b", "tool"), row("@2", "b", "idle")]);
  assert.deepEqual(s.map((x) => [x.key, x.hours, x.rank, x.rows.length]), [["b", 60, "Station", 2]]);
  assert.deepEqual(systemsOf(prog, PROJECTS, []), []);
});

test("systemsOf: live project without ledger history is an Outpost with 0 h", () => {
  const s = systemsOf({ locked: false, projects: [P("a", 3, "Outpost")] }, PROJECTS, [row("@1", "n", "tool")]);
  const n = s.find((x) => x.key === "n");
  assert.deepEqual([n.label, n.hours, n.rank, n.rows.length], ["Newbie", 0, "Outpost", 1]);
});

test("systemsOf: sessions outside any project dock at Uncharted", () => {
  const s = systemsOf({ locked: false, projects: [] }, [], [row("@1", "", "idle")]);
  assert.deepEqual(s.map((x) => [x.key, x.label, x.rank]), [[UNCHARTED, "Uncharted", "Outpost"]]);
});

test("systemsOf: ledger locked or empty → live projects as 0 h Outposts", () => {
  for (const prog of [null, { locked: true, projects: [] }, { locked: false, projects: [] }]) {
    const s = systemsOf(prog, PROJECTS, [row("@1", "a", "tool")]);
    assert.deepEqual(s.map((x) => [x.key, x.label, x.hours, x.rank]), [["a", "Alpha", 0, "Outpost"]]);
  }
});

test("systemsOf: at most one system per map slot", () => {
  const rows = Array.from({ length: 40 }, (_, i) => row("@" + i, "p" + String(i).padStart(2, "0"), "tool"));
  const s = systemsOf({ locked: false, projects: [] }, [], rows);
  assert.equal(s.length, 32);
});

test("SLOTS: 32 points inside the map, ≥ 150 apart", () => {
  assert.equal(SLOTS.length, 32);
  for (const p of SLOTS) assert.ok(p.x >= 60 && p.x <= 1540 && p.y >= 90 && p.y <= 790, JSON.stringify(p));
  let min = Infinity;
  for (let i = 0; i < 32; i++) for (let j = i + 1; j < 32; j++) min = Math.min(min, Math.hypot(SLOTS[i].x - SLOTS[j].x, SLOTS[i].y - SLOTS[j].y));
  assert.ok(min >= 150, "min spacing " + min);
});

test("placeSystems: deterministic, order-independent, distinct slots", () => {
  const sys = ["alpha", "beta", "gamma", "delta", "eps"].map((key) => ({ key }));
  const a = placeSystems(sys), b = placeSystems([...sys].reverse());
  assert.deepEqual([...a.entries()].sort(), [...b.entries()].sort());
  assert.equal(new Set([...a.values()].map((p) => p.slot)).size, 5);
});

test("placeSystems: adding a project whose home slot is free moves nobody", () => {
  const sys = ["alpha", "beta", "gamma"].map((key) => ({ key }));
  const before = placeSystems(sys);
  const used = new Set([...before.values()].map((p) => p.slot));
  const extra = Array.from({ length: 200 }, (_, i) => "new" + i).find((k) => !used.has(placeSystems([{ key: k }]).get(k).slot));
  const after = placeSystems([...sys, { key: extra }]);
  for (const s of sys) assert.deepEqual(after.get(s.key), before.get(s.key));
});

test("placeSystems: more than 32 systems never loops forever", () => {
  const m = placeSystems(Array.from({ length: 40 }, (_, i) => ({ key: "k" + i })));
  assert.equal(m.size, 32);
});

test("stationOf per rank", () => {
  assert.deepEqual(stationOf("Outpost"), { art: "station-1", scale: 0.6, halo: false });
  assert.deepEqual(stationOf("Settlement"), { art: "station-1", scale: 0.8, halo: false });
  assert.deepEqual(stationOf("Station"), { art: "station-2", scale: 1, halo: false });
  assert.deepEqual(stationOf("Citadel"), { art: "station-3", scale: 1.15, halo: false });
  assert.deepEqual(stationOf("Capital"), { art: "station-3", scale: 1.3, halo: true });
  assert.deepEqual(stationOf(""), stationOf("Outpost"));
});

test("dockOf: needs oldest first, then working, then quiet; cap 4 + more", () => {
  const rows = [row("q", "a", "idle"), row("w", "a", "tool"), row("n2", "a", "permission", 2000), row("n1", "a", "permission", 1000), row("e", "a", "error", 3000), row("w2", "a", "thinking")];
  const d = dockOf(rows);
  assert.deepEqual(d.ships.map((r) => r.key), ["n1", "n2", "e", "w"]);
  assert.equal(d.more, 2);
});

test("geomOf: station centred above the dock; ships evenly spaced", () => {
  const g = geomOf({ x: 800, y: 400 }, "Station", { ships: [{ key: 1 }, { key: 2 }], more: 1 });
  assert.equal(g.station.art, "station-2");
  assert.equal(g.station.size, 96);
  assert.deepEqual(g.station.box, [752, 342, 848, 438]);
  assert.deepEqual(g.ships.map((s) => [s.x, s.y]), [[775, 458], [825, 458]]);
  assert.ok(g.moreAt.x > 821 + 20 - 1);
  assert.ok(g.dockBox[0] <= 779 - 20 && g.dockBox[2] >= g.moreAt.x);
  assert.equal(geomOf({ x: 800, y: 400 }, "Outpost", { ships: [], more: 0 }).dockBox, null);
});

test("ordersStrip + chipText", () => {
  const now = 10 * 60000;
  const rows = [row("w", "a", "tool"), row("n2", "a", "permission", now - 2 * 60000), row("n1", "a", "permission", now - 7 * 60000), row("e", "b", "error", now)];
  assert.deepEqual(ordersStrip(rows).map((r) => r.key), ["n1", "n2", "e"]);
  assert.match(chipText(rows[2], now), /^[A-Z]+ · a · awaiting 7m$/);
  assert.match(chipText(rows[3], now), / · b · hull breach$/);
  assert.match(chipText(row("f", "a", "finished", now, { unread: true }), now), / · a · mission complete$/);
});

test("fmtHours + headerOf", () => {
  assert.equal(fmtHours(4.46), "4.5");
  assert.equal(fmtHours(0), "0.0");
  assert.equal(fmtHours(263.4), "263");
  const h = headerOf([{ hours: 200 }, { hours: 63.4 }], [row("1", "a", "permission"), row("2", "a", "tool"), row("3", "a", "idle"), row("4", "a", "idle")]);
  assert.deepEqual(h, { awaiting: 1, engaged: 1, docked: 2, hours: "263", systems: 2 });
});

test("system labels never cover a waiting ship's callsign (dense maps)", async () => {
  const { layoutSystems } = await import("./sector.js");
  const { callsign } = await import("./cic.js");
  for (let seed = 0; seed < 80; seed++) {
    const systems = Array.from({ length: 12 }, (_, i) => {
      const key = `p${seed}-${i}`;
      return { key, label: ("project" + key).padEnd(12, "x"), hours: 60, rank: "Station", rows: [row("r" + key, key, "permission", 1000)] };
    });
    const { placed, labels } = layoutSystems(systems);
    // the callsign as SectorMode draws it: centred on the ship, 13-unit monospace, baseline 30 below the ship centre
    const cs = placed.flatMap((p) => p.g.ships.map((d) => {
      const w = 7.6 * (13 / 12) * callsign(d.row).length, b = d.y + 30;
      return [d.x - w / 2, b - 13, d.x + w / 2, b + 3];
    }));
    for (const p of placed) {
      const L = labels.get(p.key);
      if (!L) continue;
      for (const c of cs) {
        const overlap = !(L.box[2] <= c[0] || c[2] <= L.box[0] || L.box[3] <= c[1] || c[3] <= L.box[1]);
        assert.ok(!overlap, `seed ${seed}: label ${p.key} covers a callsign`);
      }
    }
  }
});



// Minors round 2 (Sector review): live systems beyond 32 dropped silently, and the
// ones dropped were simply the last by key — a waiting session could vanish.
test("systemsOf: past 32, waiting systems stay and the rest are counted off-map", async () => {
  const { offMapOf } = await import("./sector.js");
  const rows = Array.from({ length: 40 }, (_, i) => row("@" + i, "p" + String(i).padStart(2, "0"), "tool"));
  rows.push(row("@w", "zz-waiting", "permission"));
  const s = systemsOf({ locked: false, projects: [] }, [], rows);
  assert.equal(s.length, 32);
  assert.ok(s.some((x) => x.key === "zz-waiting"), "a waiting system dropped");
  assert.equal(offMapOf(rows), 9);
  assert.equal(offMapOf(rows.slice(0, 5)), 0);
});

// Minors round 2 (Sector review): the Capital's halo and the selection ring are
// wider than the station image, so a label could sit on their edge.
test("layoutSystems: labels clear each station's selection ring", async () => {
  const { layoutSystems } = await import("./sector.js");
  const ranks = ["Capital", "Citadel", "Station", "Settlement", "Outpost"];
  const systems = Array.from({ length: 16 }, (_, i) => ({ key: "s" + i, label: "system-" + i, hours: 400 - i, rank: ranks[i % 5], rows: [row("@" + i, "s" + i, "tool")] }));
  const { placed, labels } = layoutSystems(systems);
  const hit = (a, b) => !(a[2] <= b[0] || b[2] <= a[0] || a[3] <= b[1] || b[3] <= a[1]);
  for (const p of placed) {
    const st = p.g.station, r = st.size * 0.7;
    const ring = [st.x - r, st.y - r, st.x + r, st.y + r];
    for (const q of placed) {
      const L = labels.get(q.key);
      if (L) assert.ok(!hit(L.box, ring), `${q.key}'s label on ${p.key}'s ring`);
    }
  }
});
