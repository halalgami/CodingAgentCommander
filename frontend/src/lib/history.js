// Frontend access to the Go-authoritative project history + the one-time
// migration of the legacy localStorage recents. The Go store is the source of
// truth; these are thin async wrappers plus pure, node-testable migration logic.
import {
  ListProjects, PinProject, RemoveProject, RenameProject, ImportProjects,
} from "../../wailsjs/go/main/App.js";

const LEGACY_KEY = "commander.recents.v1";

export async function listProjects() { return await ListProjects(); }
export async function pinProject(folder, pinned) { return await PinProject(folder, pinned); }
export async function removeProject(folder) { return await RemoveProject(folder); }
export async function renameProject(folder, label) { return await RenameProject(folder, label); }

// Read the old {folder,modelID,ts} recents from localStorage; corrupt/absent -> [].
export function readLegacyRecents(storage = globalThis.localStorage) {
  try {
    const arr = JSON.parse(storage.getItem(LEGACY_KEY) ?? "[]");
    return Array.isArray(arr) ? arr : [];
  } catch {
    return [];
  }
}

// Collapse folder+model pairs into one entry per folder: newest ts wins the
// model, openCount = number of pairs seen for that folder.
export function foldRecents(recents) {
  const byFolder = new Map();
  for (const r of recents) {
    if (!r || !r.folder) continue;
    const cur = byFolder.get(r.folder);
    const ts = r.ts ?? 0;
    if (!cur) {
      byFolder.set(r.folder, {
        folder: r.folder, label: "", lastModelID: r.modelID ?? "",
        lastOpened: ts, openCount: 1, pinned: false,
      });
    } else {
      cur.openCount++;
      if (ts >= cur.lastOpened) { cur.lastOpened = ts; cur.lastModelID = r.modelID ?? cur.lastModelID; }
    }
  }
  return [...byFolder.values()];
}

// One-time migration: fold legacy recents into the Go store, then clear the
// legacy key. ImportProjects (importFn) is a no-op if the store already has
// data, so re-running is safe. If the import throws (backend absent / down),
// the legacy key is left intact so the next run retries.
export async function migrateHistoryOnce(storage = globalThis.localStorage, importFn = ImportProjects) {
  const legacy = readLegacyRecents(storage);
  if (legacy.length) {
    try {
      await importFn(foldRecents(legacy));
    } catch {
      return; // leave the key; retry on the next launch
    }
  }
  try { storage.removeItem?.(LEGACY_KEY); } catch { /* ignore */ }
}
