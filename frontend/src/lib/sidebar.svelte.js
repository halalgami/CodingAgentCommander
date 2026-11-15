// Sidebar layout state: groups and per-section order, owned Go-side
// (internal/sidebar) and merged with live windows on every reconcile.
// Edits apply optimistically, then persist; a failed save reloads the truth.
//
// Wails runs every binding call on its own goroutine, so a SidebarLayout()
// issued after a SetSidebarLayout() can reach Go first and return the layout
// from before the edit. Hence: saves go through one queue (Go sees them in
// the order they were made), and no load is applied while any save is
// pending or was issued after the load started.
import { untrack } from "svelte";
import { SidebarLayout, SetSidebarLayout } from "../../wailsjs/go/main/App.js";
import { toast } from "./stores.svelte.js";

export const side = $state({ layout: { groups: [], order: { ungrouped: [] } }, dragging: false });

let gen = 0;
let pending = 0;
let queue = Promise.resolve();

export async function loadLayout() {
  // untrack: callers run this from an $effect; reading `dragging` must not
  // make drag-end (or anything else here) re-trigger the effect.
  const skip = untrack(() => side.dragging) || pending > 0;
  if (skip) return;
  const mine = gen;
  try {
    const l = await SidebarLayout();
    if (l && mine === gen && pending === 0 && !untrack(() => side.dragging)) side.layout = l;
  } catch { /* plain browser: keep the local layout */ }
}

export function applyLayout(next) {
  gen++;
  pending++;
  side.layout = next;
  const snapshot = $state.snapshot(next);
  queue = queue.then(async () => {
    let err = null;
    try { await SetSidebarLayout(snapshot); } catch (e) { err = e; }
    pending--;
    if (!err || (typeof window !== "undefined" && !window.go)) return; // ok, or plain browser
    toast("Couldn't save the sidebar layout: " + err, "error");
    await loadLayout();
  });
  return queue;
}
