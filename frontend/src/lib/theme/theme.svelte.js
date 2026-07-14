// Runtime accent theming. Sets CSS custom properties on :root and keeps the
// xterm theme object in sync. ANSI 16 are FIXED (hand-tuned for Claude Code
// output legibility) — only bg/fg/cursor/selection follow the app theme.
import { deriveAccent, DEFAULT_ACCENT } from "./accent.js";

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

export function applyAccent(h, c) {
  theme.h = h; theme.c = c;
  const d = deriveAccent({ h, c });
  const root = document.documentElement.style;
  root.setProperty("--accent", d.accent);
  root.setProperty("--accent-hover", d.accentHover);
  root.setProperty("--accent-dim", d.accentDim);
  root.setProperty("--accent-faint", d.accentFaint);
  root.setProperty("--on-accent", d.onAccent);
  xtermTheme.cursor = d.accentHex;
  xtermTheme.selectionBackground = d.selectionHex;
  try { localStorage.setItem(KEY, JSON.stringify({ h, c })); } catch {}
}

export function resetAccent() {
  applyAccent(DEFAULT_ACCENT.h, DEFAULT_ACCENT.c);
}

export function initTheme() {
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) ?? "null");
    if (saved && Number.isFinite(saved.h) && Number.isFinite(saved.c)) {
      applyAccent(saved.h, saved.c);
      return;
    }
  } catch {}
  resetAccent();
}
