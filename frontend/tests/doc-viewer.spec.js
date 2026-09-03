import { test, expect } from "@playwright/test";

// Wails bindings do not exist in a plain browser, so stub window.go's App
// before any app script runs. Go does the rendering now, so the stub returns
// HTML — which also keeps these specs honest about the contract: the frontend
// must not care how the HTML was produced.
// opts.root overrides the resolved project root Go hands back in both listing
// stubs (defaults to "/Users/me/proj", matching FILES). opts.sessionCwd
// overrides SessionStats().cwd independently of that — "" reproduces a
// session card whose stats have not polled a cwd yet, the shape of the row-
// click-does-not-open defect. opts.allSinceStartFalse makes every entry in
// the session listing sinceStart: false, the shape of the empty-since-set
// defect (the filter's fallback-to-everything path). opts.sessionCount and
// opts.historyCount widen ListSessions/ListProjects beyond the single-row
// default — the palette-crowding repro (Fix 3) needs enough rows ahead of
// the config commands to push documents out of the rendered top 12.
export async function stubDocs(page, files, opts = {}) {
  const root = opts.root ?? "/Users/me/proj";
  const sessionCwd = "sessionCwd" in opts ? opts.sessionCwd : root;
  const allSinceStartFalse = !!opts.allSinceStartFalse;
  const sessionCount = opts.sessionCount ?? 1;
  const historyCount = opts.historyCount ?? 1;
  await page.addInitScript(({ seed, root, sessionCwd, allSinceStartFalse, sessionCount, historyCount }) => {
    const store = { ...seed };
    window.__docStore = store;
    window.__opened = [];
    window.runtime = window.runtime || {};
    window.runtime.BrowserOpenURL = (u) => { window.__url = u; };
    window.go = window.go || {};
    window.go.main = window.go.main || {};
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      // Entry 0 always resolves to exactly `root`, and has the highest
      // lastOpened, so pickDocRoot's "most recently opened" fallback still
      // picks the project FILES is stubbed against even when more rows exist.
      ListProjects: async () => Array.from({ length: historyCount }, (_, i) => ({
        folder: i === 0 ? root : `${root}-alt${i}`, label: i === 0 ? "proj" : `proj-alt${i}`,
        lastModelID: "claude-opus-4-8", lastOpened: 9 - i, openCount: 1, pinned: false, missing: false,
      })),
      // One session by default so a card renders with docs-open-w1 (Task 11's
      // session entry point), and its cwd matches FILES' root everywhere
      // else. sessionCount widens this for the palette-crowding repro.
      ListSessions: async () => Array.from({ length: sessionCount }, (_, i) => ({
        windowID: `w${i + 1}`, name: `w${i + 1}`, model: "claude-opus-4-8",
      })),
      SessionStats: async () => ({
        contextTokens: 1000, estCostPerTurn: 0.01, unpriced: false, band: "green",
        turns: 1, model: "claude-opus-4-8", provider: "anthropic", uptimeSeconds: 60,
        status: "active", remoteControl: false, cwd: sessionCwd,
      }),
      Config: async () => ([{ id: "claude-opus-4-8", label: "Opus 4.8", routed: false, ready: true }]),
      ListProjectDocs: async () => ({
        entries: Object.keys(store).map((rel, i) => ({ rel, modTime: 1000 - i, size: 10 })),
        truncated: false,
        root,
      }),
      // Session-scoped listing carries the sinceStart flag (Task 9). Every doc
      // is "since this session started" except old.md, which exists solely so
      // the recency filter has something to hide -- unless allSinceStartFalse,
      // which reproduces a freshly launched session whose since-set is empty.
      ListSessionDocs: async () => ({
        entries: Object.keys(store).map((rel, i) => ({
          rel, modTime: 1000 - i, size: 10,
          sinceStart: allSinceStartFalse ? false : rel !== "old.md",
        })),
        truncated: false,
        root,
      }),
      RenderProjectDoc: async (r, rel) => {
        // Mirrors Go's own guard: an empty root is refused, not silently
        // accepted, so a component that forwards "" instead of the resolved
        // root fails here exactly as it would against the real binding.
        if (!r) throw new Error("no project folder given");
        const d = store[rel];
        if (!d) throw new Error(`refusing to open ${rel}: it is not a path inside the project`);
        return { html: d.html, css: ".chroma{}", kind: d.kind ?? "markdown", lang: "", links: d.links ?? [] };
      },
      OpenProjectDoc: async (root, rel) => { window.__opened.push(rel); },
    });
  }, { seed: files, root, sessionCwd, allSinceStartFalse, sessionCount, historyCount });
}

export const FILES = {
  "notes.md": {
    html: "<h1>Notes heading</h1><p>plain body text</p><p>link</p>",
    links: [{ text: "site", href: "https://example.com" }, { text: "next", href: "./other.md" }],
  },
  "other.md": { html: "<h1>Other heading</h1>" },
  "main.go": { html: '<pre><code><span class="k">package</span> main</code></pre>', kind: "text" },
  "inert.md": {
    html: "<h1>Inert</h1>",
    links: [{ text: "nope", href: "javascript:alert(1)" }, { text: "anchor", href: "#x" }],
  },
  // Present only so the session recency filter (Task 11) has something to
  // hide: ListSessionDocs above marks every OTHER file sinceStart.
  "old.md": { html: "<h1>Old</h1>" },
};

// A dynamic import() of a .svelte.js module does not resolve against the vite
// preview build this suite runs on (the module is bundled, not served at its
// source path), so this drives the same open through the window.__openDoc
// seam App.svelte exposes in onMount -- the established pattern for reaching
// Go-fed state from a spec (see window.__app, window.__packgen).
export async function openDirect(page, rel) {
  await page.goto("/?nointro");
  await page.waitForFunction(() => typeof window.__openDoc === "function");
  await page.evaluate((r) => window.__openDoc("/Users/me/proj", r), rel);
}

test("a document renders in the viewer", async ({ page }) => {
  await stubDocs(page, FILES);
  await openDirect(page, "notes.md");
  await expect(page.getByTestId("drawer-docview")).toBeVisible();
  await expect(page.getByTestId("docs-path")).toHaveText("/Users/me/proj/notes.md");
  await expect(page.frameLocator('[data-testid="docs-frame"]').locator("h1")).toHaveText("Notes heading");
});

// The security invariant, asserted on the attribute rather than on behaviour.
// A MISSING attribute is no sandbox at all, which is why null is checked too.
test("the frame is sandboxed with nothing granted", async ({ page }) => {
  await stubDocs(page, FILES);
  await openDirect(page, "notes.md");
  const sandbox = await page.getByTestId("docs-frame").getAttribute("sandbox");
  expect(sandbox).not.toBeNull();
  expect(sandbox).toBe("");
});

// The palette is the only way in for a real user, and its rows arrive two
// async hops after ⌘K (ListProjects -> histItems -> effect -> ListProjectDocs),
// so every case waits for the row rather than racing it.
async function openViaPalette(page, rel) {
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await expect(page.getByTestId("palette")).toBeVisible();
  await page.getByTestId("palette-input").fill(rel);
  await expect(page.getByTestId("palette").getByRole("button", { name: new RegExp("Doc: " + rel) })).toBeVisible();
  await page.keyboard.press("Enter");
}

test("a document opens from the palette", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  await expect(page.getByTestId("drawer-docview")).toBeVisible();
  await expect(page.frameLocator('[data-testid="docs-frame"]').locator("h1")).toHaveText("Notes heading");
});

// Docs must not crowd out the palette's commands: they are pushed last, and
// the palette renders only twelve rows.
test("the palette still offers its commands when many documents exist", async ({ page }) => {
  await stubDocs(page, {
    ...FILES,
    ...Object.fromEntries(Array.from({ length: 30 }, (_, i) => [`filler${i}.md`, { html: "<p>f</p>" }])),
  });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  // The row's accessible name concatenates the label and hint spans with no
  // separator text node ("Settings" + "config" -> "Settings config"), so the
  // match anchors on the label prefix rather than the exact string.
  await expect(page.getByTestId("palette").getByRole("button", { name: /^Settings\b/ })).toBeVisible();
});

// Fix 3: docs are pushed last so they never evict a command — correct — but
// that also means a busy palette (enough sessions/history rows to fill the
// rendered top 12 on their own) shows NO document ever, so nobody browsing
// ⌘K with an empty query discovers documents exist at all. 6 sessions + 2
// history rows (8 rows) plus the 4 config commands is exactly 12 -- enough to
// crowd out every doc under the old "always last" placement. The fix must
// show a few recent docs BEFORE the config rows for an empty query, while
// still leaving at least one config command (Settings) on screen.
test("the palette surfaces recent documents with an empty query, without hiding Settings", async ({ page }) => {
  await stubDocs(page, FILES, { sessionCount: 6, historyCount: 2 });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await expect(page.getByTestId("palette")).toBeVisible();
  // Two async hops (ListProjects -> histItems -> effect -> ListProjectDocs)
  // stand between ⌘K and the doc rows landing, so wait rather than race.
  await expect(page.getByTestId("palette").getByRole("button", { name: /^Doc: /}).first()).toBeVisible();
  await expect(page.getByTestId("palette").getByRole("button", { name: /^Settings\b/ })).toBeVisible();
});

// The non-empty-query path already lets fuzzy scoring rank a matching doc in
// regardless of recency; this locks that in as the query-driven counterpart
// to the empty-query fix above -- old.md is the LEAST recent of the five
// FILES entries, so it would be the last doc considered under any recency cap.
test("the palette finds a document by search even when it is not among the most recent", async ({ page }) => {
  await stubDocs(page, FILES, { sessionCount: 6, historyCount: 2 });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+KeyK");
  await page.getByTestId("palette-input").fill("old.md");
  await expect(page.getByTestId("palette").getByRole("button", { name: /^Doc: old\.md/ })).toBeVisible();
});

// Reading scope is wider than markdown (spec R2.3): a source file opens, and
// Go's highlighting classes survive into the frame.
test("a source file opens with highlighting", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "main.go");
  const frame = page.frameLocator('[data-testid="docs-frame"]');
  await expect(frame.locator("pre code span")).toHaveCount(1);
});

// Go strips hrefs, so nothing in the frame is clickable at all — this is the
// frontend half of that contract.
test("the rendered frame contains no anchors to click", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  const html = await page.frameLocator('[data-testid="docs-frame"]').locator("body").innerHTML();
  expect(html.toLowerCase()).not.toContain("href=");
  expect(html.toLowerCase()).not.toContain("<script");
});

// The link list sits inside a <details>, collapsed by default (Fix 2) so it
// no longer eats frame height uninvited. Playwright cannot click inside a
// closed <details> — its content is UA-hidden, not merely visually clipped —
// so every spec that interacts with a link opens the summary first, the same
// gesture a real user would need.
test("an external link goes to the OS browser, a relative link opens in the viewer", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  await page.getByText("Links in this document (2)").click(); // open the collapsed list
  await expect(page.getByTestId("docs-links")).toBeVisible();
  await page.getByTestId("docs-link-0").click();          // "site" -> external
  expect(await page.evaluate(() => window.__url)).toBe("https://example.com");
  await page.getByTestId("docs-link-1").click();          // "next" -> ./other.md
  await expect(page.frameLocator('[data-testid="docs-frame"]').locator("h1")).toHaveText("Other heading");
});

// The collapse is the point of Fix 2: the list must not claim frame height
// until someone asks for it, and the summary must say how many links there
// are before it is opened at all.
test("the link list is collapsed until opened, and names its count", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  const summary = page.getByText("Links in this document (2)");
  await expect(summary).toBeVisible();
  await expect(page.getByTestId("docs-links")).not.toBeVisible();
  await summary.click();
  await expect(page.getByTestId("docs-links")).toBeVisible();
});

// javascript: and anchor-only links are classified inert, so they are not
// offered at all — the list must not be a way to reach what the sanitizer
// refused to keep. inert.md has zero offered links, so there is nothing to
// open here — no <details> renders at all.
test("inert links are not offered in the list", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "inert.md");
  expect(await page.getByTestId("docs-links").locator("button").count()).toBe(0);
});

test("Refresh re-reads a document changed on disk", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  await page.evaluate(() => { window.__docStore["notes.md"] = { html: "<h1>Rewritten heading</h1>" }; });
  await page.getByTestId("docs-refresh").click();
  await expect(page.frameLocator('[data-testid="docs-frame"]').locator("h1")).toHaveText("Rewritten heading");
});

test("Copy path yields a path the OS would accept", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  await page.getByTestId("docs-copy").click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("/Users/me/proj/notes.md");
});

test("Open externally hands the document to Go", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  await page.getByTestId("docs-external").click();
  expect(await page.evaluate(() => window.__opened)).toEqual(["notes.md"]);
});

// The refusal path is the one that matters most for binary and oversized
// files: readable, and with the escape hatch still on screen (spec R2.3).
test("a refused render shows the refusal and keeps the escape hatch", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  await page.evaluate(() => { delete window.__docStore["notes.md"]; });
  await page.getByTestId("docs-refresh").click();
  await expect(page.getByTestId("docs-error")).toContainText("refusing to open");
  await expect(page.getByTestId("docs-external")).toBeVisible();
});

test("Escape closes the drawer from the chrome", async ({ page }) => {
  await stubDocs(page, FILES);
  await openViaPalette(page, "notes.md");
  await page.getByTestId("docs-refresh").focus();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("drawer-docview")).not.toBeVisible();
});

// Fix 1: the iframe must flex-fill the drawer body's height so there is one
// scroll region (inside the frame), not two nested ones with ~100px of dead
// drawer below the smaller inner one. Asserted as a real measurement of the
// rendered boxes, not a CSS string — a height:60vh iframe would still pass a
// class-name check, that's the whole reason the bug shipped.
test("the document frame fills most of the drawer's height, leaving little dead space", async ({ page }) => {
  await stubDocs(page, FILES);
  await openDirect(page, "notes.md");
  const drawerBox = await page.getByTestId("drawer-docview").boundingBox();
  const frameBox = await page.getByTestId("docs-frame").boundingBox();
  // 0.6 would be satisfied by the very `height: 60vh` this fix replaced, so
  // the threshold has to be above it or the test passes on the old code.
  expect(frameBox.height).toBeGreaterThan(drawerBox.height * 0.75);
  const deadSpaceBelow = (drawerBox.y + drawerBox.height) - (frameBox.y + frameBox.height);
  expect(deadSpaceBelow).toBeLessThanOrEqual(120);
});

// 380px is cramped for a document full of tables; the document view asks the
// shell for a wide variant. Asserted as a real measurement, not a class name.
test("the document view is wider than an ordinary drawer", async ({ page }) => {
  await stubDocs(page, FILES);
  await openDirect(page, "notes.md");
  const docBox = await page.getByTestId("drawer-docview").boundingBox();
  expect(docBox.width).toBeGreaterThan(600);

  await page.getByTestId("drawer-docview-close").click();
  await page.getByTestId("open-settings").click();
  const settingsBox = await page.getByTestId("drawer-settings").boundingBox();
  expect(settingsBox.width).toBeLessThan(500); // the shell's default is untouched
});

// Drawer.svelte's focus and Escape behaviour is load-bearing and has shipped a
// bug twice. The wide prop must not disturb either.
test("the wide drawer still closes on Escape from its chrome", async ({ page }) => {
  await stubDocs(page, FILES);
  await openDirect(page, "notes.md");
  await page.getByTestId("docs-refresh").focus();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("drawer-docview")).not.toBeVisible();
});

// Task 11 builds the index this points at; until then, this cannot pass.
test("the document view offers a way back to the index", async ({ page }) => {
  await stubDocs(page, FILES);
  await openDirect(page, "notes.md");
  await page.getByTestId("docview-back").click();
  await expect(page.getByTestId("drawer-docs")).toBeVisible();      // the index
  await expect(page.getByTestId("drawer-docview")).not.toBeVisible();
});

test("the titlebar advertises the palette and opens it", async ({ page }) => {
  await stubDocs(page, FILES);
  await page.goto("/?nointro");
  await expect(page.getByTestId("open-palette")).toContainText("⌘K");
  await page.getByTestId("open-palette").click();
  await expect(page.getByTestId("palette")).toBeVisible();
});

test("the index lists documents and opens one", async ({ page }) => {
  await stubDocs(page, FILES);
  await page.goto("/?nointro");
  await page.evaluate(() => window.__openDocsList("/Users/me/proj", "", false));
  await expect(page.getByTestId("drawer-docs")).toBeVisible();
  await expect(page.getByTestId("docs-row-notes.md")).toBeVisible();
  await page.getByTestId("docs-row-notes.md").click();
  await expect(page.getByTestId("drawer-docview")).toBeVisible();
  await expect(page.frameLocator('[data-testid="docs-frame"]').locator("h1")).toHaveText("Notes heading");
});

test("the index searches by path", async ({ page }) => {
  await stubDocs(page, FILES);
  await page.goto("/?nointro");
  await page.evaluate(() => window.__openDocsList("/Users/me/proj", "", false));
  await page.getByTestId("docs-search").fill("other");
  await expect(page.getByTestId("docs-row-other.md")).toBeVisible();
  await expect(page.getByTestId("docs-row-notes.md")).not.toBeVisible();
});

// A filter that hides things silently is the same lie as a silent truncation
// cap, so the count of what is hidden is part of the contract.
test("the session filter hides older docs and says how many", async ({ page }) => {
  await stubDocs(page, FILES);
  await page.goto("/?nointro");
  await page.evaluate(() => window.__openDocsList("/Users/me/proj", "w1", true));
  await expect(page.getByTestId("docs-row-notes.md")).toBeVisible();     // sinceStart
  await expect(page.getByTestId("docs-row-old.md")).not.toBeVisible();   // older
  await expect(page.getByTestId("docs-filter")).toContainText("1");
  await page.getByTestId("docs-filter").click();
  await expect(page.getByTestId("docs-row-old.md")).toBeVisible();
});

// Fix 4 (cosmetic): the "changed since this session started" pip must read as
// a separate mark, not glue itself to the filename
// ("docs/TEST_1_highlighting.md•"). Measured as the real rendered gap between
// the end of the filename text and the start of the pip glyph, not a
// class-name/CSS-value check, since the source markup already has a literal
// space there — Svelte's whitespace collapsing is what erases it visually.
test("the changed-since pip reads as a separate mark from the filename", async ({ page }) => {
  await stubDocs(page, FILES);
  await page.goto("/?nointro");
  await page.evaluate(() => window.__openDocsList("/Users/me/proj", "w1", true));
  await expect(page.getByTestId("docs-row-notes.md")).toBeVisible();
  const gap = await page.evaluate(() => {
    const row = document.querySelector('[data-testid="docs-row-notes.md"]');
    const pip = row.querySelector(".pip");
    const range = document.createRange();
    range.selectNodeContents(pip.previousSibling); // the filename + trailing text node
    const textRect = range.getBoundingClientRect();
    const pipRect = pip.getBoundingClientRect();
    return pipRect.left - textRect.right;
  });
  expect(gap).toBeGreaterThan(2);
});

// Defect: a freshly launched session has nothing at or after its launch time,
// so the since-set is EMPTY and the filtered list was blank with only a small
// "N older hidden" count to explain it. The fix falls back to showing
// everything with a visible note, rather than an empty list.
test("the session filter falls back to showing everything when nothing changed since launch", async ({ page }) => {
  await stubDocs(page, FILES, { allSinceStartFalse: true });
  await page.goto("/?nointro");
  await page.evaluate(() => window.__openDocsList("/Users/me/proj", "w1", true));
  await expect(page.getByTestId("docs-filter-fallback")).toBeVisible();
  await expect(page.getByTestId("docs-filter-fallback")).toContainText(
    "Nothing changed since this session started — showing all",
  );
  // Every entry is visible, not just the ones that would have matched the
  // filter -- including old.md, which the ordinary filtered case hides.
  await expect(page.getByTestId("docs-row-notes.md")).toBeVisible();
  await expect(page.getByTestId("docs-row-old.md")).toBeVisible();

  // The manual toggle still works: turning the filter off leaves the note
  // gone (there is nothing to explain once the filter itself is off).
  await page.getByTestId("docs-filter").click();
  await expect(page.getByTestId("docs-filter-fallback")).not.toBeVisible();
  await expect(page.getByTestId("docs-row-old.md")).toBeVisible();
});

// The ordinary filtered case (a non-empty since-set) must behave exactly as
// before: filtered, with a hidden count, and no fallback note.
test("the session filter does not fall back when something changed since launch", async ({ page }) => {
  await stubDocs(page, FILES);
  await page.goto("/?nointro");
  await page.evaluate(() => window.__openDocsList("/Users/me/proj", "w1", true));
  await expect(page.getByTestId("docs-filter-fallback")).not.toBeVisible();
  await expect(page.getByTestId("docs-row-old.md")).not.toBeVisible();
});

// Defect: SessionCard passes stat?.cwd ?? "", and app.stats[windowID] may not
// have polled yet, so docsCtx.root can be "". The LIST still works (Go
// resolves the session's cwd from the registry), but a row click called
// openDoc("", rel), which Go refuses. The fix derives the root the drawer
// uses from listing.root -- the value Go itself validated -- rather than
// trusting the (possibly empty) root the frontend had on hand.
test("a row click opens even when the session's cwd has not been polled yet", async ({ page }) => {
  await stubDocs(page, FILES, { sessionCwd: "" });
  await page.goto("/?nointro");
  await page.getByTestId("docs-open-w1").click();
  await expect(page.getByTestId("drawer-docs")).toBeVisible();
  // The displayed root comes from the resolved listing, not the empty cwd.
  await expect(page.getByTestId("docs-root")).toHaveText("/Users/me/proj");
  await page.getByTestId("docs-row-notes.md").click();
  await expect(page.getByTestId("drawer-docview")).toBeVisible();
  await expect(page.frameLocator('[data-testid="docs-frame"]').locator("h1")).toHaveText("Notes heading");
});

test("a session card offers its documents", async ({ page }) => {
  await stubDocs(page, FILES);
  await page.goto("/?nointro");
  await page.getByTestId("docs-open-w1").click();
  await expect(page.getByTestId("drawer-docs")).toBeVisible();
  await expect(page.getByTestId("docs-row-notes.md")).toBeVisible();
});

test("an empty project says so", async ({ page }) => {
  await stubDocs(page, {});
  await page.goto("/?nointro");
  await page.evaluate(() => window.__openDocsList("/Users/me/proj", "", false));
  await expect(page.getByTestId("docs-empty")).toBeVisible();
});
