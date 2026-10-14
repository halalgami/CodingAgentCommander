// When the dashboard may animate (spec §7). Pure.
export function motionAllowed({ pref, reduced, lowPower, visible, focused, blurredAt, now }) {
  if (!pref || reduced || lowPower || !visible) return false;
  if (!focused && now - blurredAt >= 30000) return false;
  return true;
}
