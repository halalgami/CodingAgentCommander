// Reactive prefs. Components read `prefs.*` (tracked) and write via setPref.
import { DEFAULTS, loadPrefs, savePrefs } from "./prefsData.js";

export const prefs = $state({ ...DEFAULTS });

export function initPrefs() {
  Object.assign(prefs, loadPrefs());
}

export function setPref(key, value) {
  if (!(key in DEFAULTS)) return;
  prefs[key] = value;
  savePrefs({ ...prefs });
}
