import { test } from "node:test";
import assert from "node:assert/strict";
import { layoutCode, lotSide } from "./layout.js";

const F = (path, code = 100) => ({ path, folder: path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "", code, complexity: 5 });
const view = (files, slabs = []) => ({ files, slabs, folders: [] });

test("lot side grows with lines, 1 to 3", () => {
  assert.equal(lotSide(0), 1); assert.equal(lotSide(150), 1); assert.equal(lotSide(600), 2); assert.equal(lotSide(100000), 3);
});

function checkInside(l) {
  const seen = new Set();
  for (const lot of l.lots) {
    for (let dx = 0; dx < lot.s; dx++) for (let dy = 0; dy < lot.s; dy++) {
      const k = `${lot.x + dx},${lot.y + dy}`;
      assert.ok(!seen.has(k), "overlap at " + k);
      seen.add(k);
      assert.ok(lot.x + dx >= 0 && lot.y + dy >= 0 && lot.x + dx < l.side && lot.y + dy < l.side, "outside root");
    }
  }
}

test("deterministic, non-overlapping, inside the root", () => {
  const files = [];
  for (let d = 0; d < 6; d++) for (let i = 0; i < 25; i++) files.push(F(`pkg${d}/sub${i % 3}/f${i}.go`, (i * 97) % 2000));
  const v = view(files, [{ folder: "pkg1", files: 40, code: 900 }]);
  const a = layoutCode(v), b = layoutCode(structuredClone(v));
  assert.deepEqual(a, b);
  checkInside(a);
  const placed = a.lots.filter((l) => l.kind === "file").length;
  const hidden = a.folders.reduce((n, f) => n + f.hidden, 0);
  assert.equal(placed + hidden, files.length, "every file placed or counted");
  assert.ok(a.lots.some((l) => l.kind === "slab"), "slab placed");
});

test("each file sits inside its own folder's rectangle", () => {
  const v = view([F("a/x.go"), F("a/y.go"), F("b/z.go", 3000), F("top.go")]);
  const l = layoutCode(v);
  for (const lot of l.lots.filter((x) => x.kind === "file")) {
    const fo = l.folders.find((f) => f.path === lot.ref.folder);
    assert.ok(fo, "folder rect for " + lot.ref.folder);
    assert.ok(lot.x >= fo.x && lot.y >= fo.y && lot.x + lot.s <= fo.x + fo.w && lot.y + lot.s <= fo.y + fo.h, lot.ref.path);
  }
});

// Review Focus 4.
test("a huge flat folder stays inside its rectangle", () => {
  const files = Array.from({ length: 800 }, (_, i) => F(`flat/f${String(i).padStart(4, "0")}.go`, 50 + i));
  const l = layoutCode(view(files, [{ folder: "flat", files: 2200, code: 50000 }]));
  checkInside(l);
  assert.ok(l.side <= 140, "root side " + l.side);
});

test("a file growing a little does not move files in other folders", () => {
  const files = [F("a/1.go"), F("a/2.go"), F("b/1.go"), F("b/2.go"), F("c/1.go")];
  const a = layoutCode(view(files));
  const grown = structuredClone(files); grown[0].code = 120;
  const b = layoutCode(view(grown));
  const pos = (l, p) => l.lots.find((x) => x.ref.path === p);
  for (const p of ["b/1.go", "b/2.go", "c/1.go"]) assert.deepEqual([pos(a, p).x, pos(a, p).y], [pos(b, p).x, pos(b, p).y], p);
});
