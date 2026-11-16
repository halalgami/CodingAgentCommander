import { test } from "node:test";
import assert from "node:assert/strict";
import { cardsOf } from "./wrapped.js";

test("cardsOf: an empty year is one empty card", () => {
  const c = cardsOf({ year: 2019, empty: true });
  assert.equal(c.length, 1);
  assert.equal(c[0].id, "empty");
});

test("cardsOf: a full year has title, totals, top projects, biggest day, models, level", () => {
  const c = cardsOf({
    year: 2026, title: "Night Owl", tokens: 12_400_000, activeMs: 360 * 36e5, commits: 412, costUSD: 1234.5,
    topProjects: [{ label: "vat", value: 9e6 }, { label: "web", value: 3e6 }], biggestDay: { key: "2026-03-14", value: 2e6 },
    models: [{ key: "claude-opus-4-8", value: 8e6 }], level: 14, achievements: 23,
  });
  assert.deepEqual(c.map((x) => x.id), ["title", "hours", "tokens", "projects", "day", "models", "level"]);
  assert.equal(c[0].big, "Night Owl");
  assert.equal(c[1].big, "360 h");
  assert.equal(c[2].big, "12.4M");
  assert.match(c[2].sub, /≈ \$1,235 API-equivalent/);
  assert.deepEqual(c[3].list, ["vat", "web"]);
  assert.equal(c[4].big, "Mar 14");
  assert.equal(c[6].big, "Level 14");
});
