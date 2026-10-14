import { test } from "node:test";
import assert from "node:assert/strict";
import { sigil, projectTint, hashStr } from "./sigil.js";

test("sigil is deterministic and mirror-symmetric", () => {
  const a = sigil("6e0626a5"), b = sigil("6e0626a5");
  assert.deepEqual(a, b);
  for (let r = 0; r < 5; r++) {
    assert.equal(a.cells[r * 5], a.cells[r * 5 + 4]);
    assert.equal(a.cells[r * 5 + 1], a.cells[r * 5 + 3]);
  }
  assert.ok(a.hue >= 0 && a.hue < 360);
});

test("different sessions differ; an agent keeps its lead's hue", () => {
  assert.notDeepEqual(sigil("s-one").cells, sigil("s-two").cells);
  const lead = sigil("lead-1"), agent = sigil("agent-9", "lead-1");
  assert.equal(agent.hue, lead.hue);
  assert.notDeepEqual(agent.cells, lead.cells);
});

test("never an empty glyph; tint is an hsl colour", () => {
  for (const id of ["", "a", "zzzz", "00000000"]) assert.ok(sigil(id).cells.some(Boolean));
  assert.match(projectTint("k1"), /^hsl\(\d+ /);
  assert.equal(typeof hashStr("x"), "number");
});
