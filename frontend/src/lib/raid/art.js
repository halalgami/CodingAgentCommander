// frontend/src/lib/raid/art.js
// Raid art pack (spec §6): bundled webp only. A name in PENDING is not
// generated yet — its getter returns null and the card draws a silhouette.
const f = (name) => new URL(`../../assets/raid/${name}`, import.meta.url).href;

export const POSES = ["attack", "await", "hurt", "stance", "victory"];
export const PENDING = new Set(); // every raid image is generated (phase 6b)
const art = (name) => (PENDING.has(name) ? null : f(name));

export const bundledArt = (stem) => f(`${stem}.webp`);

// Preferred file first; Art.svelte walks the list on load errors.
// A pack that has <id>-stance takes the heroine over: only pack files, ending at
// the pack stance, so the bundled heroine never shows mid-fight (heroine packs R3).
export function heroineArt(id, pose, look = 0, tier = 0, pack = null) {
  const mine = pack?.files || {};
  const stems = [];
  if (tier >= 3) stems.push(`${id}-legend-${pose}`);
  if (look) stems.push(`${id}-v${look}-${pose}`);
  stems.push(`${id}-${pose}`);
  if (mine[`${id}-stance`]) {
    return [...new Set([...stems, `${id}-stance`].map((s) => mine[s]).filter(Boolean))];
  }
  return stems.filter((s) => !PENDING.has(`${s}.webp`)).map((s) => mine[s] || f(`${s}.webp`));
}
export const gearArt = (kind, id, pack = null) =>
  (kind === "familiar" && pack?.files?.[`gear-familiar-${id}`]) ||
  art(kind === "familiar" ? `gear-familiar-${id}.webp` : `gear-${kind}.webp`);
export const bossArt = (kind) => f(`boss-${kind}.webp`);
export const arenaArt = (name) => f(`bg-${name}.webp`);
export const arenaOf = (hour) => (hour >= 6 && hour < 19 ? "ruins" : "night");
export const allyArt = (kind, pose) => art(`ally-${kind}-${pose}.webp`);
export const slimeArt = (i) => art(`slime-${(i % 2) + 1}.webp`);
export const innArt = () => art("inn.webp");
export const chestArt = () => art("chest.webp");
export const haloArt = (kind) => art(`gear-${kind}.webp`);
