import { motionAllowed } from "./motionRule.js";
export { motionAllowed };

export const motion = $state({ reduced: false, focused: true, blurredAt: 0, visible: true });

export function initMotion() {
  try {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    motion.reduced = mq.matches;
    mq.addEventListener("change", (e) => (motion.reduced = e.matches));
  } catch {}
  motion.focused = document.hasFocus();
  // Starting unfocused counts as just blurred: still only after 30 s (spec §7).
  if (!motion.focused) motion.blurredAt = Date.now();
  window.addEventListener("focus", () => (motion.focused = true));
  window.addEventListener("blur", () => { motion.focused = false; motion.blurredAt = Date.now(); });
  document.addEventListener("visibilitychange", () => (motion.visible = !document.hidden));
}
