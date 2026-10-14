import { test } from "node:test";
import assert from "node:assert/strict";
import { buildScene, buildCity, placeSites, activeCity } from "./scene.js";

const city = { locked: false, districts: [
  { key: "k1", label: "app", plot: 1, land: 6, parks: 2, roads: 0, cracks: 0, buildings: [{ sessionID: "old", title: "Old work", archetype: "tower", hours: 2, seed: 7 }], blocks: [], oldTownCount: 0, plaques: [], landmarks: [] },
] };
const row = (key, cwdProject, extra = {}) => ({ key, id: key, windowID: key, project: cwdProject, projectKey: cwdProject === "app" ? "k1" : "", activity: "tool", tool: "Edit", target: "a.go", crew: [], ctxPct: 60, turns: 41, provider: "anthropic", sinceMs: 0, ...extra });

const inPlot = (p, s) => s.x >= p.x0 && s.x < p.x0 + p.land && s.y >= p.y0 && s.y < p.y0 + p.land;

// Owner, 2026-10-14: a session's tower stands on its own project's plot.
test("live sites stand on their district's plot; unknown projects get a temporary plot", () => {
  const s = buildScene(city, [row("@1", "app"), row("@2", "elsewhere")], {}, 1000);
  const a = s.sites.find((x) => x.row.key === "@1"), b = s.sites.find((x) => x.row.key === "@2");
  assert.equal(a.district, "k1"); assert.equal(a.stage, 2); assert.equal(a.floors, 2); assert.equal(a.prop, "sparks");
  assert.ok(inPlot(s.districts[0].pack, a), "site off its plot");
  assert.equal(b.district, ""); assert.ok(!inPlot(s.districts[0].pack, b), "temporary site on district 1");
});

test("items are sorted back to front and include buildings", () => {
  const s = buildScene(city, [row("@1", "app")], {}, 1000);
  for (let i = 1; i < s.items.length; i++) assert.ok(s.items[i - 1].x + s.items[i - 1].y <= s.items[i].x + s.items[i].y);
  assert.ok(s.items.some((it) => it.kind === "building" && it.ref.sessionID === "old"));
  assert.ok(Number.isFinite(s.bounds.minX) && s.bounds.maxX > s.bounds.minX);
});

test("locked city still shows live sites", () => {
  const s = buildScene({ locked: true, districts: [] }, [row("@1", "app")], {}, 1000);
  assert.equal(s.sites.length, 1); assert.equal(s.districts.length, 0);
});

test("every live row gets a site, even when its district's back rows are full", () => {
  const rows = Array.from({ length: 30 }, (_, i) => row("@" + i, "app"));
  const s = buildScene(city, rows, {}, 1000);
  assert.equal(s.sites.length, 30);
  assert.equal(new Set(s.sites.map((x) => `${x.x},${x.y}`)).size, 30, "sites overlap");
});

test("the static city depends only on how many sites each district holds", () => {
  const counts = new Map([["k1", 2]]);
  const base = buildCity(city, counts);
  assert.ok(base.items.every((it) => it.kind !== "site"));
  assert.deepEqual(buildCity(city, counts), base);
  const a = placeSites(base, [row("@1", "app")], {}, 1000), b = placeSites(base, [row("@1", "app"), row("@2", "app")], {}, 2000);
  assert.equal(a.sites.length, 1); assert.equal(b.sites.length, 2);
  assert.deepEqual(a.bounds, b.bounds, "on-plot sites are inside the static bounds");
  assert.ok(buildCity(city, new Map([["k1", 9]])).districts[0].pack.land >= base.districts[0].pack.land);
});

test("several districts sit on an even grid without touching", () => {
  const c = { locked: false, districts: ["k1", "k2", "k3", "k4"].map((key, i) => ({ ...city.districts[0], key, plot: i + 1, land: 4 + i * 6 })) };
  const packs = buildCity(c).districts.map((x) => x.pack);
  for (let i = 0; i < packs.length; i++) for (let j = i + 1; j < packs.length; j++) {
    const [a, b] = [packs[i], packs[j]];
    assert.ok(a.x0 + a.land + 1 < b.x0 - 1 || b.x0 + b.land + 1 < a.x0 - 1 || a.y0 + a.land + 1 < b.y0 - 1 || b.y0 + b.land + 1 < a.y0 - 1, `plots ${i} and ${j} touch`);
  }
});

test("only districts with a live session are shown, packed onto the first plots", () => {
  const c = { locked: false, districts: [
    { ...city.districts[0], key: "k1", plot: 7 },
    { ...city.districts[0], key: "k2", plot: 2 },
    { ...city.districts[0], key: "k3", plot: 12 },
  ] };
  const a = activeCity(c, [row("@1", "app"), { ...row("@2", "app"), projectKey: "k3" }]);
  assert.deepEqual(a.districts.map((d) => [d.key, d.plot]), [["k1", 1], ["k3", 2]]);
  assert.equal(c.districts[0].plot, 7, "input untouched");
  assert.deepEqual(activeCity(c, []).districts, []);
});

// Final review I1: with history off every project's sites sit on temporary plots;
// they must form a grid, not one long diagonal.
test("temporary plots without districts form a grid", () => {
  const rows = ["a", "b", "c", "d", "e"].map((p, i) => ({ ...row("@" + i, p), projectKey: p }));
  const s = buildScene({ locked: true, districts: [] }, rows, {}, 1000);
  assert.ok(new Set(s.sites.map((x) => x.x)).size > 1 && new Set(s.sites.map((x) => x.y)).size > 1, "one line");
});

// Final review I6: a sibling's state change re-sorts rows by attention; towers keep their cells.
test("a project's towers keep their cells whatever the row order", () => {
  const counts = new Map([["k1", 2]]);
  const base = buildCity(city, counts);
  const a = row("@1", "app"), b = row("@2", "app");
  const cell = (rows, key) => { const x = placeSites(base, rows, {}, 1).sites.find((s) => s.row.key === key); return `${x.x},${x.y}`; };
  assert.equal(cell([a, b], "@1"), cell([b, a], "@1"));
});
