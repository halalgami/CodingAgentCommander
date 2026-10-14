import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { SPRITES, spriteKey } from "./manifest.js";

test("every non-pending sprite file exists", () => {
  for (const [k, s] of Object.entries(SPRITES)) {
    if (s.pending) continue;
    assert.ok(existsSync(fileURLToPath(s.file)), `${k}: ${s.file}`);
    assert.ok(s.w > 0 && s.ax >= 0 && s.ax <= 1 && s.ay >= 0 && s.ay <= 1, `${k} anchor`);
  }
});

test("every archetype has all four stages and four finished looks", () => {
  for (const a of ["tower", "block", "hall"]) {
    for (let st = 0; st < 4; st++) assert.ok(SPRITES[spriteKey("site", a, st)], `${a} stage ${st}`);
    for (let seed = 0; seed < 8; seed++) assert.ok(SPRITES[spriteKey("building", a, 3, seed)], `${a} look ${seed}`);
  }
});

test("the shipped pack has no pending sprites", () => {
  const pending = Object.entries(SPRITES).filter(([, s]) => s.pending).map(([k]) => k);
  assert.deepEqual(pending, []);
});
