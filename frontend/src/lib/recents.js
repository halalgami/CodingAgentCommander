// Recent launches (folder+model pairs) in localStorage. Pure aside from the
// injected storage, so node:test can cover it.
const KEY = "commander.recents.v1";
const MAX = 8;

export function loadRecents(storage = globalThis.localStorage) {
  try {
    const arr = JSON.parse(storage.getItem(KEY) ?? "[]");
    return Array.isArray(arr) ? arr : [];
  } catch {
    return [];
  }
}

export function pushRecent({ folder, modelID }, storage = globalThis.localStorage) {
  const rest = loadRecents(storage).filter(
    (r) => !(r.folder === folder && r.modelID === modelID),
  );
  const next = [{ folder, modelID, ts: Date.now() }, ...rest].slice(0, MAX);
  try { storage.setItem(KEY, JSON.stringify(next)); } catch { /* full/blocked */ }
  return next;
}
