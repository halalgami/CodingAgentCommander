// Pure helpers for the AI card (unit-tested with node --test).
// weekLabel("2026-11-09") → "Nov 9–15"
export function weekLabel(mon) {
  const d = new Date(mon + "T00:00:00");
  if (isNaN(d)) return "";
  const e = new Date(d); e.setDate(d.getDate() + 6);
  const m = (x) => x.toLocaleString("en-US", { month: "short" });
  return m(d) === m(e) ? `${m(d)} ${d.getDate()}–${e.getDate()}` : `${m(d)} ${d.getDate()} – ${m(e)} ${e.getDate()}`;
}

// answerBars: the first result's rows → horizontal bars (top 8).
export function answerBars(result, w = 260, rowH = 18) {
  const rows = (result?.rows || []).slice(0, 8);
  const max = Math.max(1, ...rows.map((r) => r.value));
  return rows.map((r, i) => ({ y: i * rowH, w: Math.max(2, (r.value / max) * w), label: r.label || r.key, value: r.value }));
}
