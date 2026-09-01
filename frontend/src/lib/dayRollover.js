// Local-calendar-day rollover detection. Generic and storage-injectable on
// purpose: this file ships in the public build (it names no feature), and
// node:test has no real localStorage, so the pure function takes storage as
// an argument rather than reaching for globalThis.localStorage itself —
// same idiom as prefsData.js's loadPrefs/savePrefs.
const DAY_MARK_KEY = "cc.last-run-day";

// YYYY-M-D in the LOCAL timezone, matching how the rest of the app derives
// local hour/weekday from the same Date. Not zero-padded: this key is never
// parsed back, only compared for equality.
export function localDayStamp(nowMs) {
  const d = new Date(nowMs);
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

// True the first time this is called for a given local calendar day (across
// process restarts, via the injected storage), false on every later call the
// same day. A missing/corrupt storage read is treated as "never marked" —
// this can only produce an extra true, never a stuck false.
export function computeFirstRunOfDay(nowMs, storage = globalThis.localStorage) {
  const today = localDayStamp(nowMs);
  let prev = null;
  try { prev = storage.getItem(DAY_MARK_KEY); } catch { /* blocked/unavailable */ }
  if (prev === today) return false;
  try { storage.setItem(DAY_MARK_KEY, today); } catch { /* full/blocked: still true this call */ }
  return true;
}
