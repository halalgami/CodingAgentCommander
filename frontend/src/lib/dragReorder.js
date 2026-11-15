// Pure drop-target geometry for sidebar drags. rects come from the DOM in
// list order: { kind: "card"|"header"|"new", key, windowID?, top, bottom }.

export const DRAG_THRESHOLD = 4;

// dropTarget answers where a drag at client y would land.
// Card drags -> { key, index } | { newGroup: true } | null; the dragged card
// itself is ignored when counting positions. Index Infinity = end of section.
// Group drags -> { groupIndex } among the groups (Ungrouped is never a target).
export function dropTarget(rects, y, dragKind, draggedID) {
  if (dragKind === "group") {
    const groups = rects.filter((r) => r.kind === "header" && r.key !== "ungrouped" && r.key !== draggedID);
    return { groupIndex: groups.filter((r) => (r.top + r.bottom) / 2 < y).length };
  }
  const list = rects.filter((r) => !(r.kind === "card" && r.windowID === draggedID));
  if (!list.length) return null;
  let r = list.find((x) => y >= x.top && y < x.bottom);
  if (!r) r = y < list[0].top ? list[0] : list.find((x) => y < x.top) ?? list[list.length - 1];
  if (r.kind === "new") return { newGroup: true };
  if (r.kind === "header") {
    // Above the first header: the very top of its section.
    if (y < r.top) return { key: r.key, index: 0 };
    return r.key === "ungrouped" ? { key: r.key, index: 0 } : { key: r.key, index: Infinity };
  }
  const index = list.filter((x) => x.kind === "card" && x.key === r.key && x.top < r.top).length;
  return { key: r.key, index: y < (r.top + r.bottom) / 2 ? index : index + 1 };
}
