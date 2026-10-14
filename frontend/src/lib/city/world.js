// World signals (spec §5): local day/night and provider-health weather.
export function dayPhase(date) {
  const h = date.getHours();
  if (h >= 7 && h < 18) return "day";
  if ((h >= 18 && h < 20) || (h >= 5 && h < 7)) return "dusk";
  return "night";
}

export function weatherOf({ rows = [], quotaPct = null, now }) {
  if (rows.some((r) => r.activity === "error" && now - (r.sinceMs || 0) < 600000)) return "rain";
  if (quotaPct != null && quotaPct >= 80) return "fog";
  return "sun";
}
