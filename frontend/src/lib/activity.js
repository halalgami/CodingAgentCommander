// One-line description of what a session is doing, from SessionStats.
export function activityText(st) {
  if (!st || !st.activity) return "";
  const n = (k, word) => `${k} ${word}${k === 1 ? "" : "s"}`;
  if (st.delegating > 0 && st.activity === "tool" && (st.tool === "Agent" || st.tool === "Task")) {
    return `Waiting on ${n(st.delegating, "agent")}`;
  }
  if (st.crewOut > 0 && (st.activity === "finished" || st.activity === "idle")) return `${n(st.crewOut, "agent")} out`;
  switch (st.activity) {
    case "thinking": return "Thinking…";
    case "tool": return (st.target ? `${st.tool} ${st.target}` : (st.tool || "")) + (st.hotspot ? " ⚠ hotspot" : "");
    case "permission": return "Needs your approval";
    case "finished": return st.unread ? "Finished — your turn" : "Finished";
    case "idle": return "Idle";
    case "error": return "Stopped on an API error";
    default: return "";
  }
}
