// Ranges are local calendar days, keyed exactly like the ledger ("YYYY-MM-DD" local).
const pad = (n) => String(n).padStart(2, "0");
export const dayKey = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

function parse(key) {
  const [y, m, d] = key.split("-").map(Number);
  return new Date(y, m - 1, d, 12); // noon: immune to DST shifts
}
const addDays = (key, n) => { const d = parse(key); d.setDate(d.getDate() + n); return dayKey(d); };
export const spanDays = (from, to) => Math.round((parse(to) - parse(from)) / 86_400_000) + 1;

const BACK = { today: 0, "7d": 6, "30d": 29 };

// rangeFor: id is "today" | "7d" | "30d" | "all" | {from, to}. "all" starts at
// firstDay (the earliest ledger day) or is open ("") when unknown.
export function rangeFor(id, now, firstDay = "") {
  const to = dayKey(now);
  if (typeof id === "object" && id) return { from: id.from, to: id.to, days: spanDays(id.from, id.to) };
  if (id === "all") return { from: firstDay, to, days: firstDay ? spanDays(firstDay, to) : 0 };
  const from = addDays(to, -(BACK[id] ?? 6));
  return { from, to, days: spanDays(from, to) };
}

export function previous(r) {
  if (!r.from || !r.days) return { from: "", to: "" };
  return { from: addDays(r.from, -r.days), to: addDays(r.from, -1) };
}

export { addDays, parse as parseDay };
