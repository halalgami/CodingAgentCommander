import { test } from "node:test";
import assert from "node:assert/strict";
import { toScreen, toTile, TW, TH } from "./iso.js";

test("tile → screen → tile round-trips", () => {
  for (const [x, y] of [[0, 0], [3, 7], [-5, 2], [12.5, -3.25]]) {
    const { sx, sy } = toScreen(x, y);
    const t = toTile(sx, sy);
    assert.ok(Math.abs(t.x - x) < 1e-9 && Math.abs(t.y - y) < 1e-9, `${x},${y} → ${t.x},${t.y}`);
  }
  assert.deepEqual(toScreen(1, 0), { sx: TW / 2, sy: TH / 2 });
});
