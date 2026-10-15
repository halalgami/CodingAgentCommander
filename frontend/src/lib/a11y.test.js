// Minors round 2 (model-grouping review, "palette/drawer a11y nits"): every
// component compiles with no accessibility warnings, so new ones fail here.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { compile } from "svelte/compiler";

const root = fileURLToPath(new URL("..", import.meta.url)); // frontend/src
function svelteFiles(dir) {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    return statSync(p).isDirectory() ? svelteFiles(p) : n.endsWith(".svelte") ? [p] : [];
  });
}

test("no component has an accessibility warning", () => {
  const found = [];
  for (const f of svelteFiles(root)) {
    const { warnings } = compile(readFileSync(f, "utf8"), { filename: f, generate: false });
    for (const w of warnings) if (w.code.startsWith("a11y")) found.push(`${f.slice(root.length)}:${w.start?.line} ${w.code}`);
  }
  assert.deepEqual(found, []);
});
