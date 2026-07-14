// Pure prefs persistence — injected storage keeps it node-testable.
const KEY = "commander.prefs.v1";

export const DEFAULTS = Object.freeze({
  fontSize: 13, scrollback: 5000, maxCols: 120,
  uiScale: 100, sidebarW: 300, rcAutoEnable: false,
});

export function loadPrefs(storage = globalThis.localStorage) {
  const out = { ...DEFAULTS };
  try {
    const saved = JSON.parse(storage.getItem(KEY) ?? "null");
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
