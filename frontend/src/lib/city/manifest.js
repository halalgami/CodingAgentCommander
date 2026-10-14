// City sprite pack (spec §6). Anchors (ax, ay) are the sprite's foot point as
// fractions of its size; w is its drawn width in world pixels (buildings and
// sites stay under one tile, TW 64, so neighbours do not overlap). A sprite
// marked `pending` is not generated yet: the painter draws the flat-box fallback.
const f = (name) => new URL(`../../assets/city/${name}`, import.meta.url).href;
const ARCH = ["tower", "block", "hall"];
const S = {};
for (const a of ARCH) {
  for (let st = 0; st < 4; st++) S[`site-${a}-${st}`] = { file: f(`${a}-${st + 1}.webp`), ax: 0.5, ay: 0.92, w: 56 };
  for (let v = 0; v < 4; v++) S[`building-${a}-${v}`] = { file: f(`${a}-v${v + 1}.webp`), ax: 0.5, ay: 0.92, w: 56 };
}
Object.assign(S, {
  plate: { file: f("plate.webp"), ax: 0.5, ay: 0.5, w: 64 },
  trees: { file: f("props-trees.webp"), ax: 0.5, ay: 0.9, w: 48 },
  truck: { file: f("props-truck.webp"), ax: 0.5, ay: 0.85, w: 36 },
  "block-low-0": { file: f("block-low-1.webp"), ax: 0.5, ay: 0.9, w: 60 },
  "block-low-1": { file: f("block-low-2.webp"), ax: 0.5, ay: 0.9, w: 60 },
  "oldtown-0": { file: f("oldtown-1.webp"), ax: 0.5, ay: 0.9, w: 64 },
  "oldtown-1": { file: f("oldtown-2.webp"), ax: 0.5, ay: 0.9, w: 64 },
  crane: { file: f("crane.webp"), ax: 0.3, ay: 0.95, w: 56 },
  "worker-work": { file: f("worker-work.webp"), ax: 0.5, ay: 0.95, w: 12 },
  "worker-lunch": { file: f("worker-lunch.webp"), ax: 0.5, ay: 0.95, w: 14 },
  "worker-nap": { file: f("worker-nap.webp"), ax: 0.5, ay: 0.9, w: 20 },
  foreman: { file: f("foreman.webp"), ax: 0.5, ay: 0.95, w: 16 },
  mixer: { file: f("mixer.webp"), ax: 0.5, ay: 0.85, w: 36 },
  firetruck: { file: f("firetruck.webp"), ax: 0.5, ay: 0.85, w: 40 },
  inspector: { file: f("inspector.webp"), ax: 0.5, ay: 0.95, w: 14 },
  scaffold: { file: f("scaffold.webp"), ax: 0.5, ay: 0.92, w: 72 },
  ribbon: { file: f("ribbon.webp"), ax: 0.5, ay: 0.9, w: 40 },
  park: { file: f("park.webp"), ax: 0.5, ay: 0.6, w: 64 },
  plaque: { file: f("plaque.webp"), ax: 0.5, ay: 0.9, w: 18 },
  statue: { file: f("statue.webp"), ax: 0.5, ay: 0.92, w: 28 },
  "ruins-0": { file: f("ruins-1.webp"), ax: 0.5, ay: 0.9, w: 64 },
  "ruins-1": { file: f("ruins-2.webp"), ax: 0.5, ay: 0.9, w: 64 },
});
export const SPRITES = S;

export function spriteKey(kind, archetype, stage, seed = 0) {
  if (kind === "site") return `site-${archetype}-${stage}`;
  if (kind === "building") return `building-${archetype}-${seed % 4}`;
  return kind;
}
