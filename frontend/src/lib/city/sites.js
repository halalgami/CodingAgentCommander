// Construction-site rules (spec §4): stage, floors, cranes, workers, props, poses.
export const stageOf = (ctxPct) => (ctxPct == null ? 1 : ctxPct < 25 ? 0 : ctxPct < 50 ? 1 : ctxPct < 80 ? 2 : 3);
export const floorsOf = (turns) => Math.floor((turns || 0) / 20);

export function cranesOf(crew = []) {
  const working = crew.filter((a) => a.state === "working" || a.state === "tool").length;
  return { shown: Math.min(3, working), more: Math.max(0, working - 3) };
}

export function workersOf(events = [], now) {
  const n = events.filter((e) => !e.agent && e.kind !== "think" && e.kind !== "wait" && e.at >= now - 60000 && e.at <= now).length;
  return Math.min(12, n);
}

const TEST_RE = /\b(go test|npm (run )?test|pytest|jest|vitest|playwright test|cargo test|node --test)\b/;

export function propOf(row) {
  if (row.activity !== "tool") return null;
  switch (row.tool) {
    case "Read": case "WebFetch": case "Grep": case "Glob": return "truck";
    case "Write": return "mixer";
    case "Edit": case "MultiEdit": case "NotebookEdit": return "sparks";
    case "Bash": return TEST_RE.test(row.target || "") ? "inspector" : "drill";
  }
  return null;
}

export function poseOf(row, now) {
  const waited = now - (row.sinceMs || now);
  switch (row.activity) {
    case "permission": return waited < 120000 ? "stop" : waited < 600000 ? "lunch" : "nap";
    case "error": return "fire";
    case "finished": return row.unread ? "ribbon" : "dusk";
    case "idle": case "": return "dusk";
  }
  return "work";
}

// fireworksOf: the ribbon's burst plays just after an unread finish, not for as
// long as the session stays unread.
export function fireworksOf(row, now) {
  return row.activity === "finished" && !!row.unread && !!row.sinceMs && now - row.sinceMs < 15000;
}

export function archetypeOfRow(row) {
  const p = row.provider || "";
  if (p === "anthropic") return "tower";
  if (p.startsWith("ollama")) return "block";
  if (p) return "hall";
  return (row.model || "").startsWith("claude") ? "tower" : "hall";
}

// compacting: 0..1 progress through the 60 s compaction pose, or null.
export function compacting(row, now) {
  if (!row.compactedMs) return null;
  const p = (now - row.compactedMs) / 60000;
  return p >= 0 && p <= 1 ? p : null;
}
