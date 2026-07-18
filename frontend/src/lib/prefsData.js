// Pure prefs persistence — injected storage keeps it node-testable.
// v2: maxCols default changed 120 -> 0 (unlimited). v1 payloads are migrated
// once: a stored 120 was v1's default (setPref persisted every key, so it
// almost always means "never chose") and becomes 0; an explicit 120 picked
// under v2 persists honestly.
const KEY = "commander.prefs.v2";
const OLD_KEY = "commander.prefs.v1";

export const DEFAULTS = Object.freeze({
  fontSize: 13, scrollback: 5000, maxCols: 0,
  uiScale: 100, sidebarW: 300, rcAutoEnable: false,
});

export function loadPrefs(storage = globalThis.localStorage) {
  const out = { ...DEFAULTS };
  try {
    let saved = JSON.parse(storage.getItem(KEY) ?? "null");
    if (!saved) {
      saved = JSON.parse(storage.getItem(OLD_KEY) ?? "null");
      if (saved && saved.maxCols === 120) saved.maxCols = 0;
    }
    if (saved && typeof saved === "object") {
      for (const k of Object.keys(DEFAULTS)) {
        if (typeof saved[k] === typeof DEFAULTS[k]) out[k] = saved[k];
      }
    }
  } catch { /* corrupt -> defaults */ }
  return out;
}

export function savePrefs(obj, storage = globalThis.localStorage) {
  try { storage.setItem(KEY, JSON.stringify(obj)); } catch { /* full/blocked */ }
}
