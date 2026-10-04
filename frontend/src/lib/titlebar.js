// Titlebar double-click. The titlebar is a Wails drag region, which starts a
// native drag on mousedown but never zooms on double-click, so the frontend asks
// Go to do what the OS setting says. Imports nothing (node --test); App.svelte
// passes the generated binding in.

const CONTROLS = "button, a, input, select, textarea, [role=button], nav";

// True only when the double-click landed on the bare drag surface: not on a
// control, and not on anything that opted out of dragging.
export function isBareTitlebarTarget(target, boundary) {
  if (!target || typeof target.closest !== "function") return false;
  if (target.closest(CONTROLS)) return false;
  const cs = globalThis.getComputedStyle;
  if (cs) {
    for (let el = target; el; el = el.parentElement) {
      if (cs(el).getPropertyValue("--wails-draggable").trim() === "no-drag") return false;
      if (el === boundary) break;
    }
  }
  return true;
}

export function onTitlebarDblClick(e, call) {
  if (isBareTitlebarTarget(e.target, e.currentTarget)) call();
}
