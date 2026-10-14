// frontend/src/lib/raid/cast.js
// Raid cast (spec §3): who stands on a session's arena card — the heroine
// (the model), the boss (the task), its level and tier (the project's size),
// armour (tests), the move (the tool) and summoned allies (the crew). Pure.
import { crewView } from "../dashboard/rows.js";

export function hashOf(s) {
  let h = 2166136261;
  for (const c of String(s)) { h ^= c.codePointAt(0); h = Math.imul(h, 16777619); }
  return h >>> 0;
}

export function clip(s, n) {
  const a = [...String(s ?? "")];
  return a.length > n ? a.slice(0, n - 1).join("") + "…" : a.join("");
}

export const HEROINES = [
  { id: "opus", name: "Archmage", tint: "#e0a33a" },
  { id: "sonnet", name: "Spellblade", tint: "#ef7a62" },
  { id: "haiku", name: "Ninja", tint: "#4fb477" },
  { id: "glm", name: "Gunslinger", tint: "#8f6ae0" },
  { id: "qwen", name: "Priestess", tint: "#d45fb2" },
  { id: "kimi", name: "Mecha Pilot", tint: "#36b5b0" },
];

export function heroineOf(row, pack = null) {
  const m = String(row.model || "").toLowerCase();
  const hit = HEROINES.find((h) => m.includes(h.id));
  const base = hit || HEROINES[hashOf(row.id || row.key || "") % HEROINES.length];
  const o = pack?.heroines?.[base.id] || {};
  const tint = o.tint || base.tint;
  if (hit) return { ...hit, name: o.name || hit.name, tint, mercenary: false };
  return { ...base, tint, name: "Mercenary", mercenary: true };
}

const BOSS_RULES = [
  ["hydra", ["bug", "fix", "error", "crash", "broken"]],
  ["golem", ["refactor", "legacy", "cleanup", "migrate", "rename"]],
  ["kraken", ["infra", "deploy", "ci", "build", "docker", "release"]],
  ["wraith", ["data", "db", "sql", "ledger", "schema", "query"]],
  ["sphinx", ["design", "spec", "plan", "ui", "ux", "brainstorm"]],
];
export const BOSS_NAMES = { hydra: "Bug Hydra", golem: "Legacy Golem", kraken: "Pipeline Kraken", wraith: "Data Wraith", sphinx: "Riddle Sphinx" };

export function bossOf(row) {
  const words = new Set(String(row.title || "").toLowerCase().split(/[^a-z0-9]+/).filter(Boolean));
  for (const [kind, keys] of BOSS_RULES) if (keys.some((k) => words.has(k))) return kind;
  return BOSS_RULES[hashOf(row.id || row.key || "") % BOSS_RULES.length][0];
}

export function bossLevel(latest) {
  if (!latest) return 1;
  const v = Math.round(4 * Math.log2(1 + (latest.code || 0) / 100) + (latest.complexity || 0) / 200);
  return Math.max(1, Math.min(99, v));
}

export function tierOf(latest) {
  if (!latest) return { tier: "beast", pips: 0 };
  const c = latest.code || 0;
  if (c < 2000) return { tier: "slime", pips: 0 };
  if (c < 30000) return { tier: "beast", pips: 0 };
  return { tier: "raid", pips: c < 100000 ? 2 : 3 };
}

export const armourOf = (latest) => (latest ? Math.round(3 * Math.min(1, (latest.testRatio || 0) / 0.5)) : 0);

const MOVES = {
  Edit: "Refactor Strike", MultiEdit: "Refactor Strike", NotebookEdit: "Refactor Strike", Write: "Forge",
  Read: "Scry", Grep: "Seek", Glob: "Seek", Bash: "Engineer's Blast", Agent: "Summon", Task: "Summon",
  WebFetch: "Far Sight", WebSearch: "Far Sight", TodoWrite: "Battle Plan",
};
export const moveOf = (tool) => MOVES[tool] || "Strike";

export function allyOf(a) {
  const t = String(a.type || "").toLowerCase();
  const m = String(a.model || "").toLowerCase();
  const kind = t.includes("explore") ? "fox" : t.includes("plan") ? "strategist" : t.includes("review") ? "cleric" : "knight";
  return { kind, size: m.includes("haiku") ? 0.7 : m.includes("sonnet") ? 0.85 : 1 };
}

export function partyOf(crew = []) {
  const { shown, more } = crewView(crew);
  return { allies: shown.map((a) => ({ ...a, ...allyOf(a) })), more };
}
