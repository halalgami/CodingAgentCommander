// Runtime accent theming. Sets CSS custom properties on :root and keeps the
// xterm theme object in sync. ANSI 16 are FIXED (hand-tuned for Claude Code
// output legibility) — only bg/fg/cursor/selection follow the app theme.
import { deriveAccent, DEFAULT_ACCENT } from "./accent.js";
import { trailing } from "../rate.js";

const KEY = "commander.accent.v1";

// Fixed, contrast-verified ANSI palette shared by every accent.
const ANSI = {
  black: "#1a1712", red: "#e5534b", green: "#57ab5a", yellow: "#c69026",
  blue: "#539bf5", magenta: "#b083f0", cyan: "#39c5cf", white: "#ada69b",
  brightBlack: "#6e6759", brightRed: "#ff6b63", brightGreen: "#6bc46d",
  brightYellow: "#daaa3f", brightBlue: "#6cb6ff", brightMagenta: "#dcbdfb",
  brightCyan: "#56d4dd", brightWhite: "#e8e3d9",
};

export const theme = $state({ h: DEFAULT_ACCENT.h, c: DEFAULT_ACCENT.c });

export const xtermTheme = $state({
  background: "#15120d",
  foreground: "#e8e3d9",
  cursor: "#e89b3c",
  selectionBackground: "#4a3517",
  ...ANSI,
});

// ONE thing is deferred here, and it is not the persistence.
//
// Terminal.svelte has an $effect that spreads xtermTheme into
// `term.options.theme` — xterm's WHOLE-theme setter, which invalidates the
// colour cache and repaints the entire viewport including scrollback. The hue
// and vividness sliders call applyAccent per input event, so dragging one
// triggered that repaint hundreds of times; on a large scrollback it is the most
// expensive thing the app does per input event.
//
// The localStorage write is NOT deferred, though it was at first: "the accent
// survives a reload" has to hold the instant the slider moves, and a reload
// inside the debounce window loses it. The e2e suite caught that. It is a tiny
// synchronous write — the repaint was always the actual cost.
//
// The CSS custom properties are likewise immediate: they are what the user is
// looking at while dragging, and the swatch has to track the slider exactly.
const settleTerminal = trailing((accentHex, selectionHex) => {
  xtermTheme.cursor = accentHex;
  xtermTheme.selectionBackground = selectionHex;
}, 120);

export function applyAccent(h, c) {
  theme.h = h; theme.c = c;
  const d = deriveAccent({ h, c });
  const root = document.documentElement.style;
  root.setProperty("--accent", d.accent);
  root.setProperty("--accent-hover", d.accentHover);
  root.setProperty("--accent-dim", d.accentDim);
  root.setProperty("--accent-faint", d.accentFaint);
  root.setProperty("--on-accent", d.onAccent);
  try { localStorage.setItem(KEY, JSON.stringify({ h, c })); } catch {}
  settleTerminal(d.accentHex, d.selectionHex);
}

// applyAccentNow settles the terminal repaint immediately, for the paths where
// no further input is coming: startup and the reset button. initTheme in
// particular must not leave the terminal on the default cursor colour for 120ms
// while the rest of the UI is already themed.
export function applyAccentNow(h, c) {
  applyAccent(h, c);
  settleTerminal.flush();
}

export function resetAccent() {
  applyAccentNow(DEFAULT_ACCENT.h, DEFAULT_ACCENT.c);
}

export function initTheme() {
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) ?? "null");
    if (saved && Number.isFinite(saved.h) && Number.isFinite(saved.c)) {
      applyAccentNow(saved.h, saved.c);
      return;
    }
  } catch {}
  resetAccent();
}
