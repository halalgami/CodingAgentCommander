// dataviz reference palette, dark steps (Commander is dark-only). Categorical
// hues are assigned to an entity on first sight and kept for the session, so a
// filter never repaints survivors; a 9th entity folds into OTHER.
export const CATEGORICAL = ["#3987e5", "#d95926", "#199e70", "#c98500", "#d55181", "#008300", "#9085e9", "#e66767"];
export const OTHER = "#8a8a86";
export const SEQ = ["var(--surface-2)", "#184f95", "#256abf", "#3987e5", "#86b6ef"]; // level 0..4
export const MARK = "#3987e5";

const slots = new Map();
export function colorOf(key) {
  if (!slots.has(key)) slots.set(key, slots.size < CATEGORICAL.length ? CATEGORICAL[slots.size] : OTHER);
  return slots.get(key);
}
