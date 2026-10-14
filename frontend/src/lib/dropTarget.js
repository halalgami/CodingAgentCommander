// frontend/src/lib/dropTarget.js
// Pure drop-target lookups, split from dropRoute.js so node tests can load them.

// The card under the cursor decides, not focus (decision D3). A drop on the
// pane goes to the active session; anywhere else is not a target.
export function dropTarget(el, activeID) {
  if (!el || !el.closest) return "";
  const card = el.closest("[data-window-id]");
  if (card) return card.getAttribute("data-window-id") || "";
  if (el.closest(".pane")) return activeID || "";
  return "";
}

// A drop on a Raid heroine cell in Settings replaces that image (heroine packs §6).
export function raidDropStem(el) {
  const cell = el?.closest?.("[data-raid-stem]");
  return cell ? cell.getAttribute("data-raid-stem") || "" : "";
}
