// frontend/src/lib/raid/looks.js
// Raid looks (6b spec §4): who she is beyond her class — a stable look, ascension,
// earned gear, how she works (radar), what she earned (badges), what she brought
// home (loot) and the account's gold (quota). Pure.
import { hashOf } from "./cast.js";

export const lookOf = (row) => hashOf("look:" + (row.id || row.key || "")) % 3;

export const ascensionOf = (hero) => Math.max(0, Math.min(3, hero?.tier ?? 0));

export function gearOf(hero) {
  const l = hero?.level ?? 0;
  return [[3, "familiar"], [8, "banner"], [15, "weapon"]].filter(([n]) => l >= n).map(([, k]) => k);
}

export function radarOf(hero) {
  const m = hero?.mix || {};
  const v = { blade: m.Blade || 0, scholar: m.Scholar || 0, engineer: m.Engineer || 0, summoner: m.Summoner || 0 };
  const top = Math.max(...Object.values(v));
  if (!top) return null;
  for (const k in v) v[k] = Math.round((v[k] / top) * 100) / 100;
  return v;
}

export const BADGES = {
  first_blood: ["First Blood", "⚔"], test_whisperer: ["Test Whisperer", "✓"], night_owl: ["Night Owl", "☾"],
  marathon: ["Marathon", "⧗"], flawless: ["Flawless", "✦"], big_brain: ["Big Brain", "◉"],
  speedrun: ["Speedrun", "»"], demolition: ["Demolition Crew", "⚒"], urban_renewal: ["Urban Renewal", "♻"],
};

export function badgesOf(row, progress) {
  if (!row.id) return { list: [], more: 0 };
  const p = progress?.projects?.find((x) => x.key === row.projectKey);
  const mine = (p?.unlocks || []).filter((u) => u.sessionID === row.id && BADGES[u.id]).sort((a, b) => b.at - a.at);
  const list = mine.slice(0, 4).map((u) => ({ id: u.id, name: BADGES[u.id][0], glyph: BADGES[u.id][1] }));
  return { list, more: Math.max(0, mine.length - 4) };
}

export function pouchOf(pct) {
  if (pct == null || Number.isNaN(pct)) return null;
  return { fill: Math.max(0, Math.round((1 - pct / 100) * 100) / 100), dim: pct >= 80 };
}

export function quotaOf(usage) {
  const xs = Object.values(usage || {}).filter((w) => w && typeof w.utilization === "number").map((w) => w.utilization);
  return xs.length ? Math.max(...xs) : null;
}

const plural = (n, one) => `${n} ${one}${n === 1 ? "" : "s"}`;

export function lootOf(hero) {
  const l = hero?.loot;
  if (!l || !(l.commits || l.lines || l.tests)) return null;
  const text = [l.lines && `+${l.lines} lines`, l.commits && plural(l.commits, "commit"), l.tests && plural(l.tests, "green test")].filter(Boolean).join(" · ");
  return { gems: Math.min(5, Math.ceil(l.lines / 100)), chest: l.commits > 0, scrolls: Math.min(3, l.tests), text };
}

// 6a spends: attack = lunge + flash + damage (3), charge = glow (1). The cap is 3 per card.
export function motionBudget(state, tier, animate) {
  if (!animate) return { auraPulse: false, gearDrift: false };
  // attack: lunge + flash + damage number; charge: the glow; clear: the boss
  // dissolve and the loot chest. At most three effects run on a card.
  let used = state === "attack" ? 3 : state === "clear" ? 2 : state === "charge" ? 1 : 0;
  const auraPulse = tier >= 1 && used < 3;
  if (auraPulse) used++;
  return { auraPulse, gearDrift: used < 3 };
}

export const wingsOf = (id) => (["opus", "sonnet", "qwen"].includes(id) ? "halo" : "wings");
