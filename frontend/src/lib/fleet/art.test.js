// frontend/src/lib/fleet/art.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { fileURLToPath } from "node:url";
import { FLEET_FILES, shipArt, captainArt, craftArt, encounterArt, spaceArt, stationArt, fxArt } from "./art.js";

const DIR = fileURLToPath(new URL("../../assets/fleet/", import.meta.url));
const tail = (u) => u.split("/").pop();

test("every fleet art file exists and the pack fits 3 MB", () => {
  assert.equal(FLEET_FILES.length, 36);
  for (const f of FLEET_FILES) assert.ok(fs.existsSync(DIR + f), f + " missing");
  const total = fs.readdirSync(DIR).reduce((s, f) => s + fs.statSync(DIR + f).size, 0);
  assert.ok(total <= 3 * 1024 * 1024, `fleet pack is ${total} bytes`);
});

test("getters", () => {
  assert.equal(tail(shipArt("opus", false)), "ship-opus.webp");
  assert.deepEqual(shipArt("glm", true).map(tail), ["ship-glm-damaged.webp", "ship-glm.webp"]);
  assert.equal(tail(captainArt("kimi", null)), "captain-kimi.webp");
  assert.equal(captainArt("kimi", { files: { "captain-kimi": "/media/raid/captain-kimi-1" } }), "/media/raid/captain-kimi-1");
  assert.equal(tail(craftArt("scout")), "craft-scout.webp");
  assert.equal(tail(encounterArt("gate")), "enc-gate.webp");
  assert.equal(tail(spaceArt("nebula")), "bg-nebula.webp");
  assert.equal(tail(stationArt("station-2")), "station-2.webp");
  assert.equal(tail(spaceArt("galaxy")), "bg-galaxy.webp");
  assert.equal(tail(fxArt("vortex")), "fx-vortex.webp");
});
