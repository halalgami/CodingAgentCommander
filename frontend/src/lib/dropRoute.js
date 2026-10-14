import { insertDropped, activeWindowID } from "./attachments.svelte.js";
import { select, toast } from "./stores.svelte.js";
import { dropTarget, raidDropStem } from "./dropTarget.js";
import { ReplaceRaidImageFrom } from "../../wailsjs/go/main/App.js";
import { setRaidPack } from "./raid/custom.svelte.js";

export { dropTarget, raidDropStem };

const ACTIVE = "wails-drop-target-active";

// handleDrop forwards the paths EXACTLY as Wails delivered them: a name with
// a newline arrives split into fragments, and only Go (InsertDroppedPaths)
// can refuse the whole drop safely. Filtering here would hide the split.
export async function handleDrop(x, y, paths) {
  // With DisableWebViewDrop the DOM drop event never fires, so Wails' own
  // cleanup never runs and the dragover highlight would stick.
  for (const el of [...document.getElementsByClassName(ACTIVE)]) el.classList.remove(ACTIVE);
  const stem = raidDropStem(document.elementFromPoint(x, y));
  if (stem) {
    try {
      setRaidPack(await ReplaceRaidImageFrom(stem, paths[0]));
      // One cell holds one image: say which file was used when several came.
      if (paths.length > 1) toast(`One image per heroine: used ${paths[0].split(/[\\/]/).pop()}`);
    } catch (e) { toast(String(e?.message || e), "error"); }
    return;
  }
  const active = activeWindowID();
  const id = dropTarget(document.elementFromPoint(x, y), active);
  if (!id) {
    toast(active ? "Drop onto the terminal or a session card" : "Start or select a session first", "error");
    return;
  }
  if (id !== active) {
    // select() swallows its own failure (it toasts), so its outcome is read
    // back from state: never paste into a window that did not become active.
    await select(id);
    if (activeWindowID() !== id) return;
  }
  await insertDropped(id, paths);
}
