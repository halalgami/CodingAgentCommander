// Pure logic for the doc viewer.
//
// This file imports NOTHING on purpose. It is exercised by `node --test`, and
// the generated Wails bindings are ESM whose named exports resolve at link
// time — importing them here would break every test in this file. The binding
// wrappers live in projectdocs.js. Rendering lives in Go (spec R2.2), so there
// is no renderer here at all.

// The iframe sandbox for rendered documents: the empty, maximally restrictive
// value. No allow-scripts, so nothing in a document executes; no
// allow-same-origin, so the frame keeps an opaque origin with no path to the
// parent window where this app's Go bindings live.
export const DOC_SANDBOX = "";

const DOC_CSS = `
  :root { color-scheme: dark; }
  body {
    margin: 0; padding: 16px 18px 48px;
    background: #14161a; color: #e6e6e6;
    font: 14px/1.65 -apple-system, "Segoe UI", system-ui, sans-serif;
    word-wrap: break-word;
  }
  h1, h2, h3, h4 { line-height: 1.25; margin: 1.4em 0 0.6em; }
  h1 { font-size: 1.5em; } h2 { font-size: 1.25em; } h3 { font-size: 1.1em; }
  h1, h2 { border-bottom: 1px solid #2a2e35; padding-bottom: 0.3em; }
  code, pre { font-family: ui-monospace, "JetBrains Mono", monospace; font-size: 0.9em; }
  code { background: #1d2026; padding: 0.15em 0.35em; border-radius: 3px; }
  pre { background: #1d2026; padding: 12px; border-radius: 6px; overflow-x: auto; }
  pre code { background: none; padding: 0; }
  blockquote { margin: 1em 0; padding: 0 1em; border-left: 3px solid #2a2e35; color: #a9b0ba; }
  table { border-collapse: collapse; display: block; overflow-x: auto; }
  th, td { border: 1px solid #2a2e35; padding: 6px 10px; }
  hr { border: 0; border-top: 1px solid #2a2e35; }
  input[type="checkbox"] { margin-right: 6px; }
  /* Go keeps the anchor element and drops every href, so these are visible but
     inert: underlined to show something WAS linked, in the body colour and
     with a default cursor so they do not invite a click that cannot happen.
     The drawer's link list is where the targets actually live. */
  a { color: inherit; text-decoration: underline dotted; text-underline-offset: 2px; cursor: default; }
  .chroma { background: none; }
`;

// docSrcdoc wraps Go's sanitized HTML in the document that goes into srcdoc.
// The CSP is layer two: no network of any kind, so no remote image can act as
// a read receipt and no script has a source to load from. extraCSS is the
// highlighting stylesheet Go generated from a static theme.
export function docSrcdoc(bodyHTML, extraCSS) {
  return '<!doctype html><html><head><meta charset="utf-8">' +
    '<meta http-equiv="Content-Security-Policy" ' +
    `content="default-src 'none'; img-src data:; style-src 'unsafe-inline'">` +
    `<style>${DOC_CSS}${extraCSS ?? ""}</style></head><body>${bodyHTML}</body></html>`;
}

// pickDocRoot chooses the one project whose documents are listed: the selected
// session's cwd, else the most recently opened project. Go sorts ListProjects
// pinned-first, so "newest" is computed here rather than taken from entry zero.
export function pickDocRoot(sessionCwd, projects) {
  if (sessionCwd) return sessionCwd;
  const rows = Array.isArray(projects) ? projects : [];
  let best = null;
  for (const p of rows) {
    if (!p || !p.folder) continue;
    if (!best || (p.lastOpened ?? 0) > (best.lastOpened ?? 0)) best = p;
  }
  return best ? best.folder : "";
}

// Deliberately uncapped: the palette renders only its top 12 after fuzzy
// matching, and capping here would make a document unsearchable rather than
// merely unrendered.
export function docPaletteItems(listing, nowSeconds) {
  const entries = listing && Array.isArray(listing.entries) ? listing.entries : [];
  return entries.map((e) => ({
    rel: e.rel,
    label: `Doc: ${e.rel}`,
    hint: relDocTime(e.modTime, nowSeconds),
  }));
}

export function relDocTime(unixSeconds, nowSeconds) {
  if (!unixSeconds) return "";
  const s = Math.max(0, Math.round(nowSeconds - unixSeconds));
  if (s < 60) return "just now";
  const m = Math.round(s / 60); if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60); if (h < 24) return `${h}h ago`;
  return `${Math.round(h / 24)}d ago`;
}

// joinDocPath builds a display/clipboard path in the separator the root
// already uses. Rel always arrives forward-slashed from Go.
export function joinDocPath(root, rel) {
  const r = String(root ?? "");
  const sep = r.includes("\\") && !r.includes("/") ? "\\" : "/";
  const base = r.replace(/[\\/]+$/, "");
  const tail = String(rel ?? "").split("/").join(sep);
  return base ? base + sep + tail : tail;
}

// classifyDocLink decides what a link found in a document can do. The default
// is "inert": a scheme not named here does nothing at all. "doc" means the
// viewer can open it, which under spec R2.3 is any text file — the Go guard
// re-validates and the binary sniff refuses what is not text.
export function classifyDocLink(href) {
  const h = typeof href === "string" ? href.trim() : "";
  if (!h) return "inert";
  if (/^https?:\/\//i.test(h)) return "external";
  if (/^[a-z][a-z0-9+.-]*:/i.test(h)) return "inert"; // javascript:, data:, file:, mailto:
  if (h.startsWith("#")) return "inert";
  return "doc";
}

// resolveDocRel resolves a relative link against the document containing it,
// returning a root-relative path or null. Null is a refusal; what it does
// return is still re-validated by the Go guard.
export function resolveDocRel(fromRel, href) {
  if (classifyDocLink(href) !== "doc") return null;
  const target = href.trim().split("#")[0].split("?")[0];
  if (!target || target.startsWith("/")) return null;
  const base = String(fromRel || "").split("/").slice(0, -1);
  const out = [];
  for (const seg of [...base, ...target.split("/")]) {
    if (seg === "" || seg === ".") continue;
    if (seg === "..") {
      if (!out.length) return null; // climbs out of the project
      out.pop();
      continue;
    }
    out.push(seg);
  }
  return out.length ? out.join("/") : null;
}
