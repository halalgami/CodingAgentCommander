export function fmtTokens(n) {
  n = Number(n) || 0;
  const a = Math.abs(n);
  if (a >= 1e9) return (n / 1e9).toFixed(1) + "B";
  if (a >= 1e6) return (n / 1e6).toFixed(1) + "M";
  if (a >= 1e3) return (n / 1e3).toFixed(1) + "k";
  return String(Math.round(n));
}

// Every dollar is an API-equivalent estimate: the "≈" is part of the number.
export function fmtUSD(n) {
  n = Number(n) || 0;
  const opts = Math.abs(n) >= 100 ? { maximumFractionDigits: 0 } : { minimumFractionDigits: 2, maximumFractionDigits: 2 };
  return "≈ $" + n.toLocaleString("en-US", opts);
}

export function fmtDelta(cur, prev) {
  if (!Number.isFinite(cur) || !Number.isFinite(prev) || prev === 0) return "—";
  const p = Math.round(((cur - prev) / prev) * 100);
  return (p >= 0 ? "+" : "−") + Math.abs(p) + "%";
}

export const fmtHours = (ms) => ((Number(ms) || 0) / 3_600_000).toFixed(1) + " h";
