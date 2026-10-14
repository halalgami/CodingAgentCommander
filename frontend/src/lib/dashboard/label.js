// labelOf: what a card, lane or city sign calls a session — the project as the
// headline, what it is working on below. An untitled session (or a title that
// only repeats the project) shows its current activity instead.
import { activityText } from "../activity.js";

export function labelOf(row) {
  const project = row.project || "";
  const t = (row.title || "").trim();
  const titled = t && t !== "Untitled session" && t !== project;
  return { project, task: titled ? t : activityText(row), titled: !!titled };
}
