// Zoom a tile/lane into the pane (spec §3): a ghost box grows from the
// element's rect to the pane's rect. Resolves at once when still.
export function zoomFrom(el, target, animate) {
  if (!animate || !el || !target || typeof el.getBoundingClientRect !== "function") return Promise.resolve();
  const a = el.getBoundingClientRect(), b = target.getBoundingClientRect();
  const ghost = document.createElement("div");
  Object.assign(ghost.style, {
    position: "fixed", left: a.left + "px", top: a.top + "px", width: a.width + "px", height: a.height + "px",
    background: "var(--surface-1)", border: "1px solid var(--accent-dim)", borderRadius: "8px", zIndex: 50, pointerEvents: "none",
  });
  document.body.appendChild(ghost);
  const anim = ghost.animate(
    [{ left: a.left + "px", top: a.top + "px", width: a.width + "px", height: a.height + "px", opacity: 1 },
     { left: b.left + "px", top: b.top + "px", width: b.width + "px", height: b.height + "px", opacity: 0.2 }],
    { duration: 200, easing: "cubic-bezier(.2,.8,.2,1)" });
  return anim.finished.catch(() => {}).finally(() => ghost.remove());
}
