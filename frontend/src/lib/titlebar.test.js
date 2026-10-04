import { test } from "node:test";
import assert from "node:assert/strict";
import { isBareTitlebarTarget, onTitlebarDblClick } from "./titlebar.js";

// Minimal DOM stand-in: closest() answers true when the selector list names a
// tag in the ancestor chain.
function el(tag, parent = null) {
  const e = { tag, parentElement: parent };
  e.closest = (sel) => {
    const tags = sel.split(",").map((s) => s.trim());
    for (let n = e; n; n = n.parentElement) if (tags.includes(n.tag)) return n;
    return null;
  };
  return e;
}

test("bare header calls through", () => {
  const header = el("header");
  let n = 0;
  onTitlebarDblClick({ target: header, currentTarget: header }, () => n++);
  assert.equal(n, 1);
});

test("a button inside the nav does not", () => {
  const header = el("header"), nav = el("nav", header), btn = el("button", nav);
  let n = 0;
  onTitlebarDblClick({ target: btn, currentTarget: header }, () => n++);
  assert.equal(n, 0);
});

test("the nav strip itself does not", () => {
  const header = el("header"), nav = el("nav", header);
  assert.equal(isBareTitlebarTarget(nav, header), false);
});

test("a no-drag element does not", () => {
  const prev = globalThis.getComputedStyle;
  globalThis.getComputedStyle = (e) => ({ getPropertyValue: () => (e.tag === "span" ? " no-drag" : "drag") });
  try {
    const header = el("header"), span = el("span", header);
    assert.equal(isBareTitlebarTarget(span, header), false);
    assert.equal(isBareTitlebarTarget(header, header), true);
  } finally { globalThis.getComputedStyle = prev; }
});

test("missing target is ignored", () => {
  assert.equal(isBareTitlebarTarget(null, null), false);
});
