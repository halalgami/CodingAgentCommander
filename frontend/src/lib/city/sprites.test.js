import { test } from "node:test";
import assert from "node:assert/strict";
import { loadSprites } from "./sprites.js";

// Review Focus 5: a failed sprite is left out (the painter boxes it); pending ones are never fetched.
test("loads non-pending sprites and drops failures", async () => {
  const asked = [];
  const load = async (file) => { asked.push(file); if (file.includes("bad")) throw new Error("decode"); return { file }; };
  const m = await loadSprites({ ok: { file: "a/ok.webp" }, bad: { file: "a/bad.webp" }, later: { file: "a/later.webp", pending: true } }, load);
  assert.deepEqual([...m.keys()], ["ok"]);
  assert.ok(!asked.includes("a/later.webp"));
});

// Minors round 2 (phase 5a review): without createImageBitmap the loader handed
// the painter a Blob, which drawImage cannot draw.
test("drawable: falls back to a decoded <img> when createImageBitmap is missing", async () => {
  const { drawable } = await import("./sprites.js");
  const made = [];
  class FakeImage { set src(v) { this._src = v; made.push(this); } async decode() { this.ok = true; } }
  const env = { Image: FakeImage, URL: { createObjectURL: () => "blob:x", revokeObjectURL: () => {} } };
  const img = await drawable({ fake: "blob" }, env);
  assert.ok(img instanceof FakeImage && img.ok && img._src === "blob:x");
  const bm = await drawable({ fake: "blob" }, { createImageBitmap: async () => "bitmap" });
  assert.equal(bm, "bitmap");
});
