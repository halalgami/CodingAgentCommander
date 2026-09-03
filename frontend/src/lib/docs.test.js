import { test } from "node:test";
import assert from "node:assert/strict";
import {
  DOC_SANDBOX, docSrcdoc, pickDocRoot, docPaletteItems, relDocTime,
  joinDocPath, classifyDocLink, resolveDocRel,
} from "./docs.js";

// The tripwire for the whole design. Rendered markdown comes from cloned
// repositories, and this app's Go bindings live on `window` in the parent.
// allow-scripts alone collapses the layering; allow-scripts TOGETHER WITH
// allow-same-origin hands a document the bindings outright.
test("the viewer sandbox grants nothing", () => {
  assert.equal(DOC_SANDBOX, "");
  assert.ok(!DOC_SANDBOX.includes("allow-scripts"));
  assert.ok(!DOC_SANDBOX.includes("allow-same-origin"));
});

test("the frame carries a no-network CSP and no script source", () => {
  const html = docSrcdoc("<h1>hi</h1>", ".chroma { color: red }");
  assert.ok(html.includes("default-src 'none'"));
  assert.ok(html.includes("img-src data:"));
  assert.ok(html.includes("style-src 'unsafe-inline'"));
  assert.ok(!/<script/i.test(html));
  assert.ok(html.includes("<h1>hi</h1>"));
  assert.ok(html.includes(".chroma { color: red }")); // Go's highlighting CSS
});

test("the root is the selected session's cwd when there is one", () => {
  assert.equal(pickDocRoot("/p/session", [{ folder: "/p/newer", lastOpened: 20 }]), "/p/session");
});

// Go sorts ListProjects pinned-first, so "newest" cannot be entry zero.
test("with no session, the root is the newest project, pinning ignored", () => {
  const projects = [
    { folder: "/p/pinned-old", lastOpened: 5, pinned: true },
    { folder: "/p/newest", lastOpened: 30, pinned: false },
    { folder: "/p/middle", lastOpened: 10, pinned: false },
  ];
  assert.equal(pickDocRoot("", projects), "/p/newest");
  assert.equal(pickDocRoot("", []), "");
  assert.equal(pickDocRoot(null, null), "");
});

test("palette items follow the label convention and carry a relative time", () => {
  const now = 1_000_000;
  const items = docPaletteItems({
    entries: [
      { rel: "docs/PROJECT_STATE.md", modTime: now - 120, size: 10 },
      { rel: "notes.json", modTime: now - 7200, size: 10 },
    ],
    truncated: false,
  }, now);
  assert.equal(items[0].label, "Doc: docs/PROJECT_STATE.md");
  assert.equal(items[0].rel, "docs/PROJECT_STATE.md");
  assert.equal(items[0].hint, "2m ago");
  assert.equal(items[1].hint, "2h ago");
});

// Every document must stay FINDABLE: the palette's own slice(0, 12) bounds what
// is rendered, so capping here would make document 51 unsearchable — the
// opposite of what a fuzzy palette is for.
test("palette items are not capped before fuzzy matching", () => {
  const entries = Array.from({ length: 800 }, (_, i) => ({ rel: `d${i}.md`, modTime: 1, size: 1 }));
  assert.equal(docPaletteItems({ entries, truncated: true }, 10).length, 800);
  assert.equal(docPaletteItems(null, 10).length, 0);
  assert.equal(docPaletteItems({ entries: [] }, 10).length, 0);
});

test("relDocTime reads as recency, not as a timestamp", () => {
  assert.equal(relDocTime(1000, 1010), "just now");
  assert.equal(relDocTime(1000, 1000 + 120), "2m ago");
  assert.equal(relDocTime(1000, 1000 + 7200), "2h ago");
  assert.equal(relDocTime(1000, 1000 + 172800), "2d ago");
  assert.equal(relDocTime(0, 5000), "");
});

// Copy path must produce a path the OS accepts. A Windows root joined with a
// forward-slashed rel is not one.
test("joinDocPath uses the separator the root already uses", () => {
  assert.equal(joinDocPath("/Users/me/proj", "docs/a.md"), "/Users/me/proj/docs/a.md");
  assert.equal(joinDocPath("/Users/me/proj/", "docs/a.md"), "/Users/me/proj/docs/a.md");
  assert.equal(joinDocPath("C:\\Users\\me\\proj", "docs/a.md"), "C:\\Users\\me\\proj\\docs\\a.md");
  assert.equal(joinDocPath("", "a.md"), "a.md");
});

// Go returns hrefs verbatim, including a javascript: one, so the classifier is
// what makes such a link inert — and it is tested here rather than trusted.
test("links are classified, and anything unrecognised is inert", () => {
  assert.equal(classifyDocLink("https://example.com"), "external");
  assert.equal(classifyDocLink("http://example.com"), "external");
  assert.equal(classifyDocLink("./notes.md"), "doc");
  assert.equal(classifyDocLink("../docs/spec.markdown"), "doc");
  assert.equal(classifyDocLink("./main.go"), "doc");     // reading scope is any text file
  assert.equal(classifyDocLink("javascript:alert(1)"), "inert");
  assert.equal(classifyDocLink("JaVaScRiPt:alert(1)"), "inert");
  assert.equal(classifyDocLink("data:text/html,<x>"), "inert");
  assert.equal(classifyDocLink("file:///etc/passwd"), "inert");
  assert.equal(classifyDocLink("mailto:a@b.c"), "inert");
  assert.equal(classifyDocLink("#section"), "inert");
  assert.equal(classifyDocLink(""), "inert");
  assert.equal(classifyDocLink(null), "inert");
});

test("relative links resolve against the containing document", () => {
  assert.equal(resolveDocRel("docs/plans/a.md", "./b.md"), "docs/plans/b.md");
  assert.equal(resolveDocRel("docs/plans/a.md", "../specs/c.md"), "docs/specs/c.md");
  assert.equal(resolveDocRel("a.md", "sub/d.markdown"), "sub/d.markdown");
  assert.equal(resolveDocRel("docs/a.md", "b.md#heading"), "docs/b.md");
  assert.equal(resolveDocRel("docs/a.md", "b.md?v=1"), "docs/b.md");
  assert.equal(resolveDocRel("docs/a.md", "../../escape.md"), null);
  assert.equal(resolveDocRel("a.md", "/abs.md"), null);
  assert.equal(resolveDocRel("a.md", "https://example.com/x.md"), null);
});
