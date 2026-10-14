// frontend/src/lib/fleet/fleet.js
// Fleet Ops (phase 7a spec §4): what a session's panel shows. Built on Raid's
// cast/battle/looks so a session is the same fight in both modes. Pure.
import { heroineOf, bossOf, bossLevel, hashOf } from "../raid/cast.js";
import { ascensionOf } from "../raid/looks.js";
import { crewView } from "../dashboard/rows.js";

export const CLASSES = {
  opus: "Flagship carrier", sonnet: "Lance cruiser", haiku: "Interceptor",
  glm: "Gunship", qwen: "Science vessel", kimi: "Drone carrier",
};

const NAMES = [
  "Vigilant", "Resolute", "Dawnstar", "Valiant", "Tempest", "Aurora", "Sentinel", "Meridian",
  "Halcyon", "Intrepid", "Corsair", "Solstice", "Paragon", "Vanguard", "Nightjar", "Seraph",
  "Harbinger", "Odyssey", "Polaris", "Zephyr", "Cerulean", "Ironclad", "Starling", "Equinox",
  "Bastion", "Kestrel", "Lodestar", "Majestic", "Nomad", "Obsidian", "Pilgrim", "Radiant",
  "Sovereign", "Talon", "Umbra", "Vesper", "Wayfarer", "Xenith", "Yarrow", "Zenith",
  "Argent", "Borealis", "Cygnus", "Dauntless", "Ember", "Firebrand", "Gallant", "Heron",
  "Icarus", "Jubilee", "Kingfisher", "Lyra", "Mistral", "Nebula", "Onyx", "Peregrine",
  "Quasar", "Ranger", "Sirius", "Tenacity", "Unity", "Vortex", "Warden", "Aegis",
];

export function shipOf(row, pack = null) {
  const h = heroineOf(row, pack);
  return { id: h.id, cls: h.mercenary ? "Privateer" : CLASSES[h.id], tint: h.tint, captain: h.mercenary ? "Privateer" : h.name, privateer: h.mercenary };
}

export const shipName = (row) => "ACS " + NAMES[hashOf("ship:" + (row.id || row.key || "")) % NAMES.length];

const ENC = {
  hydra: { kind: "swarm", name: "Alien Swarm" }, golem: { kind: "derelict", name: "Derelict Megastructure" },
  kraken: { kind: "storm", name: "Ion Storm" }, wraith: { kind: "anomaly", name: "Spatial Anomaly" },
  sphinx: { kind: "gate", name: "Ancient Jump Gate" },
};
export const encounterOf = (row, latest) => ({ ...ENC[bossOf(row)], level: bossLevel(latest) });
export const ENCOUNTERS = Object.values(ENC);

const ACTIONS = {
  Read: "sweep", Grep: "sweep", Glob: "sweep", LS: "sweep",
  Edit: "beam", Write: "beam", MultiEdit: "beam", NotebookEdit: "beam",
  Bash: "salvo", WebFetch: "scan", WebSearch: "scan", Task: "launch", Agent: "launch",
};
export const actionOf = (tool) => ACTIONS[tool] || "idle";

export function craftOf(agent) {
  const t = String(agent?.type || "").toLowerCase();
  return t.includes("explore") ? "scout" : t.includes("plan") ? "pathfinder" : t.includes("review") ? "sentinel" : "tug";
}

export function wingOf(crew = []) {
  const { shown, more } = crewView(crew, 4);
  return { craft: shown.map((a) => ({ ...a, kind: craftOf(a) })), more };
}

const STATES = { charge: "engage", attack: "engage", orders: "await", ko: "damaged", clear: "jump", rest: "docked" };
export const stateOf = (s) => STATES[s] || "docked";

export function veteranOf(hero) {
  const t = ascensionOf(hero);
  return ["glow", "escort", "trim"].slice(0, t);
}

export const fuelOf = (row) => (row.ctxPct == null ? null : Math.min(1, row.ctxPct / 100));

export const spaceOf = (hour) => (hour >= 6 && hour < 12 ? "deep" : hour >= 12 && hour < 19 ? "nebula" : "planet");

// engage = ship bob + action effect (2), await = captain glow (1), jump = flash + stretch (2). Cap 3.
export function fleetMotion(state, vet, animate) {
  if (!animate) return { glowPulse: false, escortDrift: false };
  // the Babylon 5 jump (vortex out, vortex in, flash, flight) is the whole budget
  let used = state === "jump" ? 3 : state === "engage" ? 2 : state === "await" ? 1 : 0;
  const glowPulse = vet.includes("glow") && used < 3;
  if (glowPulse) used++;
  return { glowPulse, escortDrift: vet.includes("escort") && used < 3 };
}
