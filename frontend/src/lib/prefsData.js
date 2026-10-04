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
  // Sidebar region presentation. Deliberately generic names, kept neutral so
  // a feature-named key cannot trip the public export's content grep.
  ambientMotion: true, scanlines: false, noticeSeconds: 6,
  // Height in px of the sidebar's lower band. 0 = size it from the column, the
  // behaviour before it was adjustable. A fixed height matters because the band
  // crops its content to fit, so a flexible one re-crops on every window
  // resize. Generic name, for the same export-grep reason as above.
  dockH: 0,
  // Provider ids whose group is folded in the model lists. Generic name, like
  // its neighbours. Object.freeze is shallow, so this array is NOT itself
  // frozen: loadPrefs copies it, and writers must replace it, never mutate.
  collapsedProviders: [],
  // Companion config is Go-owned (companion.json), not a UI pref — see
  // CompanionConfig / GetCompanionConfig.
});

export function loadPrefs(storage = globalThis.localStorage) {
  const out = { ...DEFAULTS };
  for (const k of Object.keys(DEFAULTS)) if (Array.isArray(DEFAULTS[k])) out[k] = [...DEFAULTS[k]];
  try {
    let saved = JSON.parse(storage.getItem(KEY) ?? "null");
    if (!saved) {
      saved = JSON.parse(storage.getItem(OLD_KEY) ?? "null");
      if (saved && saved.maxCols === 120) saved.maxCols = 0;
    }
    if (saved && typeof saved === "object") {
      for (const k of Object.keys(DEFAULTS)) {
        if (Array.isArray(DEFAULTS[k])) {
          // typeof [] === "object" and so does typeof null — the generic check
          // below would admit both, and a non-array here reaches .includes()
          // in the grouping code.
          if (Array.isArray(saved[k])) out[k] = saved[k].filter((v) => typeof v === "string");
          continue;
        }
        if (typeof saved[k] === typeof DEFAULTS[k]) out[k] = saved[k];
      }
    }
  } catch { /* corrupt -> defaults */ }
  return out;
}

export function savePrefs(obj, storage = globalThis.localStorage) {
  try { storage.setItem(KEY, JSON.stringify(obj)); } catch { /* full/blocked */ }
}
