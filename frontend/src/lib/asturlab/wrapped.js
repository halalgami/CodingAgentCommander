// Commander Wrapped (spec §7): the year as a short list of cards. No model is
// called; the title comes from Go's rule table.
import { fmtTokens, fmtUSD } from "./format.js";

const day = (k) => {
  const d = new Date((k || "") + "T00:00:00");
  return isNaN(d) ? "—" : d.toLocaleString("en-US", { month: "short", day: "numeric" });
};

export function cardsOf(v) {
  if (!v || v.empty || v.locked) return [{ id: "empty", label: `${v?.year ?? ""}`, big: "Nothing yet", sub: v?.locked ? "Turn on history to get your Wrapped." : "No sessions recorded this year." }];
  return [
    { id: "title", label: `Your ${v.year}`, big: v.title || "Commander", sub: "your title this year" },
    { id: "hours", label: "Hours with your agents", big: Math.round((v.activeMs || 0) / 36e5) + " h", sub: `${(v.commits || 0).toLocaleString("en-US")} commits` },
    { id: "tokens", label: "Tokens", big: fmtTokens(v.tokens), sub: `${fmtUSD(v.costUSD)} API-equivalent` },
    { id: "projects", label: "Top projects", list: (v.topProjects || []).map((p) => p.label || p.key) },
    { id: "day", label: "Biggest day", big: day(v.biggestDay?.key), sub: fmtTokens(v.biggestDay?.value) + " tokens" },
    { id: "models", label: "Models", list: (v.models || []).map((m) => m.key) },
    { id: "level", label: "Commander", big: `Level ${v.level || 0}`, sub: `${v.achievements || 0} achievements` },
  ];
}
