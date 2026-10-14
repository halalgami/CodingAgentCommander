// frontend/src/lib/fleet/art.js
// Fleet art pack (phase 7a spec §7): bundled webp in assets/fleet.
const f = (name) => new URL(`../../assets/fleet/${name}`, import.meta.url).href;

const IDS = ["opus", "sonnet", "haiku", "glm", "qwen", "kimi"];
const CRAFT = ["scout", "pathfinder", "tug", "sentinel", "wing"];
const ENCS = ["swarm", "derelict", "storm", "anomaly", "gate"];
const SPACE = ["deep", "nebula", "planet", "galaxy"];
const STATIONS = ["station-1", "station-2", "station-3"];
const FX = ["vortex"];
export const FLEET_FILES = [
  ...IDS.flatMap((id) => [`ship-${id}.webp`, `ship-${id}-damaged.webp`, `captain-${id}.webp`]),
  ...CRAFT.map((k) => `craft-${k}.webp`), ...ENCS.map((k) => `enc-${k}.webp`), ...SPACE.map((k) => `bg-${k}.webp`),
  ...STATIONS.map((s) => `${s}.webp`), ...FX.map((k) => `fx-${k}.webp`),
];
// Encounters without a clean silhouette (storm, rift, swarm cloud, gate ring) keep their dark sky and are screen-blended.
export const BLEND = new Set(["storm", "anomaly", "swarm", "gate"]);

// Damaged first; Art.svelte walks to the healthy hull if the damaged file fails.
export const shipArt = (id, damaged) => (damaged ? [f(`ship-${id}-damaged.webp`), f(`ship-${id}.webp`)] : f(`ship-${id}.webp`));
export const captainArt = (id, pack = null) => pack?.files?.[`captain-${id}`] || f(`captain-${id}.webp`);
// captainFallback: the bundled portrait, for a pack image that fails to load.
export const captainFallback = (id) => f(`captain-${id}.webp`);
export const craftArt = (kind) => f(`craft-${kind}.webp`);
export const encounterArt = (kind) => f(`enc-${kind}.webp`);
export const spaceArt = (name) => f(`bg-${name}.webp`);
export const stationArt = (art) => f(`${art}.webp`);
export const fxArt = (name) => f(`fx-${name}.webp`);
