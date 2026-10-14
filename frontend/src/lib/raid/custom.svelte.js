// frontend/src/lib/raid/custom.svelte.js
// The active Raid heroine pack (heroine packs spec §6). Empty = bundled art.
import { EventsOn } from "../../../wailsjs/runtime/runtime.js";
import { RaidPackInfo } from "../../../wailsjs/go/main/App.js";

export const EMPTY_PACK = { dir: "", name: "", files: {}, heroines: {}, warnings: [] };
export const raid = $state({ pack: EMPTY_PACK });

export function setRaidPack(v) {
  raid.pack = { ...EMPTY_PACK, ...(v || {}), files: v?.files || {}, heroines: v?.heroines || {}, warnings: v?.warnings || [] };
}

export async function initRaidPack() {
  try { EventsOn("raid:pack", setRaidPack); } catch {}
  try { setRaidPack(await RaidPackInfo()); } catch {}
}
