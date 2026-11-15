// Pure sizing for the world layers (spec §6). Inputs come from world.svelte.js.
import { fmtTokens, fmtUSD } from "./format.js";

export const shareOf = (x, max) => (max > 0 ? Math.max(0, Math.min(1, (Number(x) || 0) / max)) : 0);

// spendLabel: "≈ $" only when some of it is priced; tokens otherwise.
export const spendLabel = (p) => fmtTokens(p.tokens) + " tok" + (p.cost > 0 ? " · " + fmtUSD(p.cost) : "");

// pillarOf: Al Modon economy view — a district's token pillar.
export function pillarOf(p, max) {
  if (!p || !(max > 0)) return null;
  const s = shareOf(p.tokens, max);
  return { h: Math.round(12 + 108 * s), alpha: Math.round((0.35 + 0.65 * s) * 100) / 100, label: spendLabel(p) };
}
