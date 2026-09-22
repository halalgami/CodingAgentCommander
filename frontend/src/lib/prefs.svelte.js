// Reactive prefs. Components read `prefs.*` (tracked) and write via setPref.
import { DEFAULTS, loadPrefs, savePrefs } from "./prefsData.js";

export const prefs = $state({ ...DEFAULTS });

export function initPrefs() {
  Object.assign(prefs, loadPrefs());
}

// savePrefs stays SYNCHRONOUS, deliberately.
//
// The dock resize drag calls this on every pointermove, which looks like a case
// for debouncing — but the write is a JSON.stringify of one small flat object
// plus a localStorage.setItem, on the order of microseconds. Deferring it buys
// almost nothing and costs a guarantee: "the pref survives a reload" has to hold
// the instant the value changes, and a reload or a closed window inside the
// debounce window silently loses the drag. The e2e suite catches exactly that.
//
// The writes worth deferring in this app are the ones with real cost behind
// them — the xterm whole-theme repaint (see theme.svelte.js) and the companion
// config file write plus native window resize (see stores.svelte.js). Those are
// deferred; this is not.
export function setPref(key, value) {
  if (!(key in DEFAULTS)) return;
  prefs[key] = value;
  savePrefs({ ...prefs });
}
