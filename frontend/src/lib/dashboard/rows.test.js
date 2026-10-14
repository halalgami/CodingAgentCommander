import { test } from "node:test";
import assert from "node:assert/strict";
import { buildRows, sortRows, bandOf, density, crewView } from "./rows.js";

const win = (id, sid) => ({ windowID: id, name: "w" + id });
const stat = (sid, extra = {}) => ({ sessionID: sid, model: "opus", provider: "anthropic", cwd: "/p/app", contextTokens: 50, contextWindow: 100, band: "green", turns: 3, activity: "", ...extra });

test("joins windows to live sessions and adds external ones", () => {
  const rows = buildRows({
    live: [{ id: "s1", title: "Fix VAT", state: "tool", tool: "Edit", target: "a.go", cwd: "/p/app", crew: [], sinceMs: 5 },
           { id: "s9", title: "Outside", state: "thinking", cwd: "/q", external: true, model: "sonnet", crew: [] }],
    windows: [win("@1")], stats: { "@1": stat("s1") },
    projects: [{ key: "k1", label: "app", root: "/p/app" }],
    progress: { projects: [{ key: "k1", rank: "Station" }], live: [{ sessionID: "s1", level: 7, title: "Adept" }] },
  });
  assert.equal(rows.length, 2);
  const r = rows.find((x) => x.id === "s1");
  assert.equal(r.windowID, "@1"); assert.equal(r.title, "Fix VAT"); assert.equal(r.activity, "tool");
  assert.equal(r.project, "app"); assert.equal(r.rank, "Station"); assert.equal(r.hero.level, 7); assert.equal(r.ctxPct, 50);
  const e = rows.find((x) => x.id === "s9");
  assert.equal(e.external, true); assert.equal(e.windowID, null); assert.equal(e.model, "sonnet"); assert.equal(e.project, "q");
});

test("attention order: permission, error, finished-unread, working, quiet; crew out counts as working", () => {
  const mk = (key, activity, extra = {}) => ({ key, activity, unread: false, sinceMs: 0, crewOut: 0, title: key, ...extra });
  const rows = sortRows([
    mk("quiet", "idle"), mk("work", "tool"), mk("done", "finished", { unread: true }),
    mk("err", "error"), mk("perm", "permission"), mk("out", "finished", { unread: true, crewOut: 2 }),
  ]);
  // "out" and "work" are both attention 3 with equal sinceMs; they tie-break by title.
  assert.deepEqual(rows.map((r) => r.key), ["perm", "err", "done", "out", "work", "quiet"]);
  assert.equal(bandOf(rows[0]), "needs"); assert.equal(bandOf(rows.find((r) => r.key === "out")), "working");
  assert.equal(bandOf(rows.at(-1)), "quiet");
});

test("longest waiting first within a band", () => {
  const rows = sortRows([{ key: "new", activity: "permission", sinceMs: 900 }, { key: "old", activity: "permission", sinceMs: 100 }]);
  assert.deepEqual(rows.map((r) => r.key), ["old", "new"]);
});

test("density thresholds", () => {
  assert.equal(density(1), "hero"); assert.equal(density(2), "standard");
  assert.equal(density(14), "standard"); assert.equal(density(15), "compact");
});

// Review Focus 3: a workflow burst.
test("crew view caps at 6 and rolls depth > 2 into +N", () => {
  const crew = Array.from({ length: 30 }, (_, i) => ({ id: "a" + i, depth: i === 0 ? 3 : 1 }));
  const v = crewView(crew);
  assert.equal(v.shown.length, 6); assert.equal(v.more, 24);
  assert.ok(v.shown.every((a) => a.depth <= 2));
});

test("orphan sessions (known only from their agents) are not shown", () => {
  const rows = buildRows({ live: [{ id: "o1", external: true, orphan: true, crew: [{ id: "a1" }] }], windows: [], stats: {} });
  assert.equal(rows.length, 0);
});
