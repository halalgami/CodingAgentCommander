// frontend/src/lib/raid/art.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { fileURLToPath } from "node:url";
import { heroineArt, bossArt, arenaArt, arenaOf, allyArt, slimeArt, innArt, chestArt, gearArt, haloArt, bundledArt, POSES, PENDING } from "./art.js";

const DIR = fileURLToPath(new URL("../../assets/raid/", import.meta.url));
const exists = (url) => fs.existsSync(fileURLToPath(url));

test("every shipped heroine, boss and arena file exists", () => {
  for (const id of ["opus", "sonnet", "haiku", "glm", "qwen", "kimi"]) for (const p of POSES) assert.ok(exists(heroineArt(id, p)[0]), `${id}-${p}`);
  for (const k of ["hydra", "golem", "kraken", "wraith", "sphinx"]) assert.ok(exists(bossArt(k)), k);
  for (const a of ["ruins", "night"]) assert.ok(exists(arenaArt(a)), a);
});

test("new art is null while pending and a real file once generated", () => {
  const urls = [];
  for (const k of ["fox", "strategist", "cleric", "knight"]) for (const p of ["idle", "cast"]) urls.push([`ally-${k}-${p}.webp`, allyArt(k, p)]);
  urls.push(["slime-1.webp", slimeArt(0)], ["slime-2.webp", slimeArt(1)], ["slime-1.webp", slimeArt(2)], ["inn.webp", innArt()], ["chest.webp", chestArt()]);
  for (const [name, url] of urls) {
    if (PENDING.has(name)) assert.equal(url, null, name);
    else assert.ok(url && exists(url), name);
  }
});

test("arena by local hour: ruins 06:00–18:59, night otherwise", () => {
  assert.deepEqual([0, 5, 6, 12, 18, 19, 23].map(arenaOf), ["night", "night", "ruins", "ruins", "ruins", "night", "night"]);
});

test("the raid art pack stays within 4 MB (phase 6b)", () => {
  const total = fs.readdirSync(DIR).reduce((n, f) => n + fs.statSync(DIR + f).size, 0);
  assert.ok(total <= 4 * 1024 * 1024, `raid art is ${total} bytes`);
});

test("heroineArt lists the preferred file first and look 0 last", () => {
  const files = (xs) => xs.map((u) => u.split("/").pop());
  const saved = [...PENDING];
  PENDING.clear();
  assert.deepEqual(files(heroineArt("opus", "attack")), ["opus-attack.webp"]);
  assert.deepEqual(files(heroineArt("opus", "attack", 2, 1)), ["opus-v2-attack.webp", "opus-attack.webp"]);
  assert.deepEqual(files(heroineArt("opus", "attack", 1, 3)), ["opus-legend-attack.webp", "opus-v1-attack.webp", "opus-attack.webp"]);
  PENDING.add("opus-legend-attack.webp");
  assert.deepEqual(files(heroineArt("opus", "attack", 0, 3)), ["opus-attack.webp"]);
  PENDING.clear();
  assert.match(gearArt("familiar", "glm"), /gear-familiar-glm\.webp$/);
  assert.match(gearArt("banner", "glm"), /gear-banner\.webp$/);
  assert.match(haloArt("wings"), /gear-wings\.webp$/);
  for (const n of saved) PENDING.add(n);
});

test("6b art is generated: nothing pending and every look, legend and gear file exists", () => {
  const ids = ["opus", "sonnet", "haiku", "glm", "kimi", "qwen"];
  const names = [
    ...ids.flatMap((id) => POSES.flatMap((p) => [`${id}-v1-${p}.webp`, `${id}-v2-${p}.webp`, `${id}-legend-${p}.webp`])),
    ...ids.map((id) => `gear-familiar-${id}.webp`), "gear-banner.webp", "gear-weapon.webp", "gear-halo.webp", "gear-wings.webp",
  ];
  assert.equal(PENDING.size, 0);
  assert.deepEqual(names.filter((n) => !fs.existsSync(DIR + n)), []);
});


const PK = (files, heroines = {}) => ({ files, heroines });
const tail = (u) => u.split("/").pop();

test("no pack: 6b list unchanged", () => {
  assert.deepEqual(heroineArt("opus", "hurt", 1, 3, null).map(tail), ["opus-legend-hurt.webp", "opus-v1-hurt.webp", "opus-hurt.webp"]);
});

test("partial pack (no stance): pack file replaces only its own stem", () => {
  const p = PK({ "opus-hurt": "/media/raid/opus-hurt-1" });
  assert.deepEqual(heroineArt("opus", "hurt", 1, 0, p).map(tail), ["opus-v1-hurt.webp", "opus-hurt-1"]);
  assert.deepEqual(heroineArt("opus", "attack", 0, 0, p).map(tail), ["opus-attack.webp"]);
});

test("taken over: only pack files, falling back to pack stance", () => {
  const p = PK({ "opus-stance": "/media/raid/opus-stance-1", "opus-attack": "/media/raid/opus-attack-1", "opus-v1-attack": "/media/raid/opus-v1-attack-1" });
  assert.deepEqual(heroineArt("opus", "attack", 1, 0, p).map(tail), ["opus-v1-attack-1", "opus-attack-1", "opus-stance-1"]);
  assert.deepEqual(heroineArt("opus", "hurt", 2, 3, p).map(tail), ["opus-stance-1"]);
  assert.deepEqual(heroineArt("opus", "stance", 0, 0, p).map(tail), ["opus-stance-1"]);
  assert.deepEqual(heroineArt("kimi", "hurt", 0, 0, p).map(tail), ["kimi-hurt.webp"]);
});

test("gear and bundled art", () => {
  assert.equal(tail(gearArt("familiar", "glm", PK({ "gear-familiar-glm": "/media/raid/g-1" }))), "g-1");
  assert.equal(tail(gearArt("familiar", "glm", null)), "gear-familiar-glm.webp");
  assert.equal(tail(gearArt("banner", "glm", PK({}))), "gear-banner.webp");
  assert.equal(tail(bundledArt("qwen-await")), "qwen-await.webp");
});
