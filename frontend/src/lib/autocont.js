// Copy for the auto-continue UI (spec 2026-10-09-auto-continue-design.md §2).
const BASE = "Auto-continue: when this model stops right after announcing a step, tell it to carry on";

export function autoTitle(m) {
  if (!m.autoContinueReady) return "Needs Claude Code 2.1.210 or newer";
  return m.autoContinueSet ? BASE : BASE + " (default)";
}

export function gaveUpText({ model, why }) {
  return why === "cap" ? `${model} auto-continued 20 times — over to you` : `${model} seems stuck — over to you`;
}
