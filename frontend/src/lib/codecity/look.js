// CodeCity looks (spec §5): box sizes, language colours, heat, live cranes.
export const boxHeight = (cx) => Math.max(8, Math.min(120, 8 + 14 * Math.log2(1 + (cx || 0) / 10)));

export const LANG_COLOR = {
  Go: "#6cc4d9", JavaScript: "#e8d66b", TypeScript: "#5b8fd6", Svelte: "#e8794a", Python: "#5f86b8",
  Rust: "#c7835a", Java: "#c56b4f", Kotlin: "#9b7bd9", Swift: "#e86f4f", C: "#8a9bb0", "C++": "#6f86b8",
  "C#": "#77b07a", Ruby: "#c95a5a", PHP: "#8b8fc7", Shell: "#9bb07a", HTML: "#e0906b", CSS: "#7aa0d9",
  Markdown: "#b9b0a0", JSON: "#b0a080", YAML: "#a0b08a", SQL: "#d0a060",
};
export const colorOf = (f) => LANG_COLOR[f.lang] || "#a3a3a3";

const COOL = [59, 111, 182], HOT = [224, 69, 43];
export function heatColor(t) {
  const k = Math.max(0, Math.min(1, t));
  const c = COOL.map((a, i) => Math.round(a + (HOT[i] - a) * k));
  return `rgb(${c[0]}, ${c[1]}, ${c[2]})`;
}

const WINDOW = { "7d": 7 * 86400000, "30d": 30 * 86400000, all: Infinity };
export function heatOf(files, window, now) {
  const span = WINDOW[window] ?? Infinity;
  const raw = new Map();
  let max = 0;
  for (const f of files) {
    const t = f.touch || {};
    const v = Math.log(1 + (t.edits || 0) + (t.writes || 0));
    if (v <= 0 || !t.lastAt || now - t.lastAt > span) continue;
    raw.set(f.path, v);
    max = Math.max(max, v);
  }
  for (const [k, v] of raw) raw.set(k, v / max);
  return raw;
}

const busy = (s) => s === "working" || s === "tool";
export function cranesFor(rows, projectKey) {
  const byFile = new Map();
  let shown = 0, more = 0;
  const put = (path, row, agent) => {
    if (!path) return;
    const list = byFile.get(path) || [];
    if (list.length >= 3 || shown >= 12) { more++; return; }
    list.push({ row, agent });
    byFile.set(path, list);
    shown++;
  };
  for (const r of rows) {
    if (r.projectKey !== projectKey) continue;
    put(r.targetRel, r, null);
    for (const a of r.crew || []) if (busy(a.state)) put(a.targetRel, r, a);
  }
  return { byFile, shown, more };
}
