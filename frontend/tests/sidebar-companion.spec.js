import { test, expect } from "@playwright/test";
import { deflateSync } from "node:zlib";

// A pack shaped exactly like what LoadCompanionPack returns. Only `idle` is
// required by the format, and one variant exercises the whole resolve path.
const FIXTURE_PACK = {
  schema: 1, name: "fixture", author: "test", license: "test",
  canvas: { w: 832, h: 1216, scale: 2, anchor: "top-center" },
  alpha: true,
  slots: { idle: [{ id: "fx-idle", file: "idle.png", weight: 1 }] },
  warnings: [],
};

// A pack whose idle variant also carries a `motionId` — the derived media id
// loadPack registers for the optional animated `motion` file (companionpack.go).
const MOTION_PACK = {
  schema: 1, name: "motion", author: "t", license: "t",
  canvas: { w: 832, h: 1216, scale: 2, anchor: "top-center" },
  alpha: true,
  slots: { idle: [{ id: "fx-idle", file: "idle.png", motionId: "fx-idle-motion", weight: 1 }] },
  warnings: [],
};

async function mountPanel(page, { pack = FIXTURE_PACK, sessions = [], selected = "" } = {}) {
  await page.goto("/?nointro");
  await page.evaluate(({ pack, sessions, selected }) => {
    window.__app.companionCfg.kind = "panel";
    window.__app.companionPack = pack;
    window.__app.companionWarnings = pack ? pack.warnings : [];
    window.__app.companionState = {
      sessions, selected, running: sessions.length, finished: 0,
      finishSeq: 0, lastFinished: "",
    };
    window.__app.sessions = sessions.map((s) => ({ windowID: s.windowID, name: s.name }));
    if (selected) window.__app.sessionKey = selected;
  }, { pack, sessions, selected });
}

const sess = (windowID, over = {}) => ({
  windowID, name: windowID.toUpperCase(), status: "active",
  statusSinceMs: Date.now() - 1000, lastFinishMs: 0, errorMs: 0, ...over,
});

test("the companion renders in the sidebar dock with a fixture pack", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (e) => errors.push("" + e));
  await mountPanel(page);

  const region = page.getByTestId("sidebar-companion");
  await expect(region).toBeVisible();
  // It is inside the sidebar's dock band, not floating somewhere else.
  await expect(page.getByTestId("sidebar-dock").getByTestId("sidebar-companion")).toHaveCount(1);
  // The image is served from the NEUTRAL route. /companion/... in main.go would
  // abort the public export, so the URL shape is load-bearing (§6.5, §8.1).
  await expect(page.getByTestId("companion-art")).toHaveAttribute("src", "/media/pack/fx-idle");
  expect(errors).toEqual([]);
});

test("no pack configured shows the set-up affordance, not a figure", async ({ page }) => {
  await mountPanel(page, { pack: null });
  await expect(page.getByTestId("companion-setup")).toBeVisible();
  await expect(page.getByTestId("companion-art")).toHaveCount(0);
});

test("with bindings absent and the kind off, nothing mounts and nothing throws", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (e) => errors.push("" + e));
  page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
  await page.goto("/?nointro");
  await expect(page.getByTestId("titlebar")).toBeVisible();
  await expect(page.getByTestId("sidebar-companion")).toHaveCount(0);
  expect(await page.getByTestId("sidebar-dock").evaluate((el) => el.getBoundingClientRect().height)).toBe(0);
  expect(errors).toEqual([]);
});

test("scanlines are off by default and appear only when the pref is set", async ({ page }) => {
  await mountPanel(page);
  const frame = page.getByTestId("companion-frame");
  expect(await frame.locator("div").count()).toBeGreaterThan(0);
  const before = await frame.evaluate((el) => el.querySelectorAll("div").length);
  await page.evaluate(() => { window.__prefs.scanlines = true; });
  await expect
    .poll(async () => frame.evaluate((el) => el.querySelectorAll("div").length))
    .toBe(before + 1);
});

test("reduced motion zeroes the breath animation", async ({ page }) => {
  // The project-wide config already forces reducedMotion: "reduce".
  await mountPanel(page);
  const anim = await page.getByTestId("companion-art")
    .evaluate((el) => getComputedStyle(el).animationName);
  expect(anim).toBe("none");
});

test.describe("with motion allowed", () => {
  test.use({ contextOptions: { reducedMotion: "no-preference" } });

  test("breath runs at 0.25Hz and pauses when ambient motion is off", async ({ page }) => {
    await mountPanel(page);
    const art = page.getByTestId("companion-art");
    const s = await art.evaluate((el) => {
      const c = getComputedStyle(el);
      return { name: c.animationName, dur: c.animationDuration, play: c.animationPlayState };
    });
    expect(s.name).not.toBe("none");
    expect(s.dur).toBe("4s");          // 0.25Hz, one shared phase for breath and drift
    expect(s.play).toBe("running");

    await page.evaluate(() => { window.__prefs.ambientMotion = false; });
    await expect
      .poll(async () => art.evaluate((el) => getComputedStyle(el).animationPlayState))
      .toBe("paused");
  });

  test("idle plays the motion file when motion is allowed", async ({ page }) => {
    await mountPanel(page, { pack: MOTION_PACK });
    const art = page.getByTestId("companion-art");
    await expect(art).toHaveAttribute("src", "/media/pack/fx-idle-motion");
  });
});

test("reduced motion falls back to the still poster", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mountPanel(page, { pack: MOTION_PACK });
  const art = page.getByTestId("companion-art");
  await expect(art).toHaveAttribute("src", "/media/pack/fx-idle");
});

test("ACCEPTANCE: toggling the companion changes nothing about the terminal pane", async ({ page }) => {
  // This is the property the whole placement decision rests on (§9). The pane
  // geometry is what a pty resize would move, and the sidebar is a fixed
  // var(--sidebar-w) that the dock lives INSIDE — so if this ever regresses,
  // someone made the companion push.
  await page.goto("/?nointro");
  const paneBox = () => page.locator(".pane, [data-testid=empty-state]").first().boundingBox();
  const before = await paneBox();

  await page.evaluate(() => {
    window.__app.companionCfg.kind = "panel";
    window.__app.companionPack = {
      schema: 1, name: "fixture", canvas: { w: 832, h: 1216, scale: 2, anchor: "top-center" },
      alpha: true, slots: { idle: [{ id: "fx-idle", file: "idle.png" }] }, warnings: [],
    };
  });
  await expect(page.getByTestId("sidebar-companion")).toBeVisible();
  const during = await paneBox();

  await page.evaluate(() => { window.__app.companionCfg.kind = "off"; });
  await expect(page.getByTestId("sidebar-companion")).toHaveCount(0);
  const after = await paneBox();

  expect(during).toEqual(before);
  expect(after).toEqual(before);
  // The sidebar's own width is equally untouched: the dock lives inside it.
  const sidebarW = await page.getByTestId("sidebar").evaluate((el) => el.getBoundingClientRect().width);
  expect(sidebarW).toBe(300);
});

test("ACCEPTANCE: a real mounted terminal sees no pty resize and no xterm theme/geometry change when the companion toggles", async ({ page }) => {
  // The test above proves the OUTER pane box never moves. This test goes one
  // layer deeper and proves the consequence that actually matters: with a
  // real xterm instance mounted (Terminal.svelte, not a stand-in), toggling
  // the companion fires no new ResizeObserver callback on the terminal's own
  // container — the only path that leads to a pty resize (refitSoon ->
  // refit -> fitClamped + sendSize) — and neither xterm's rendered theme
  // background nor its DOM box change. This is the specific regression the
  // sidebar placement (over an earlier behind/beside-the-terminal placement)
  // was chosen to make structurally impossible; it must be asserted, not
  // assumed from the outer-box check alone.
  //
  // The ResizeObserver on window is wrapped BEFORE navigation so it wraps the
  // real constructor Terminal.svelte calls — this is a test-only spy, not a
  // change to the component. The spec-mandated initial ResizeObserver
  // callback (fired once by the browser as soon as .observe() is called)
  // means the fire count starts at 1, not 0.
  await page.addInitScript(() => {
    const Native = window.ResizeObserver;
    window.__roFires = 0;
    window.ResizeObserver = function (cb) {
      return new Native((...entries) => { window.__roFires++; cb(...entries); });
    };
  });

  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = [{ windowID: "w1", name: "W1" }];
    window.__app.sessionKey = "w1:1"; // mounts the real Terminal.svelte, {#key}'d on this value
  });
  await expect(page.locator(".xterm")).toBeVisible();

  const snapshot = () => page.evaluate(() => {
    const term = document.querySelector(".term");
    const themed = document.querySelector(".xterm-scrollable-element"); // carries xterm's actual theme background
    const r = term.getBoundingClientRect();
    return {
      rect: { w: r.width, h: r.height },
      themeBg: themed ? themed.style.backgroundColor : null,
      roFires: window.__roFires,
    };
  });

  // Let the mandatory initial ResizeObserver callback and first fit settle
  // before taking the "before" snapshot, so it isn't mistaken for a
  // companion-caused resize later.
  await expect.poll(async () => (await snapshot()).roFires).toBeGreaterThan(0);
  const before = await snapshot();
  expect(before.themeBg).toBeTruthy(); // sanity: xterm actually applied a theme

  await page.evaluate(() => {
    window.__app.companionCfg.kind = "panel";
    window.__app.companionPack = {
      schema: 1, name: "fixture", canvas: { w: 832, h: 1216, scale: 2, anchor: "top-center" },
      alpha: true, slots: { idle: [{ id: "fx-idle", file: "idle.png" }] }, warnings: [],
    };
  });
  await expect(page.getByTestId("sidebar-companion")).toBeVisible();
  const during = await snapshot();

  await page.evaluate(() => { window.__app.companionCfg.kind = "off"; });
  await expect(page.getByTestId("sidebar-companion")).toHaveCount(0);
  const after = await snapshot();

  // No new ResizeObserver callback fired anywhere in the app -> refitSoon was
  // never scheduled -> no pty resize message was ever assembled.
  expect(during.roFires).toBe(before.roFires);
  expect(after.roFires).toBe(before.roFires);
  // xterm's own rendered theme background is byte-identical, not just close.
  expect(during.themeBg).toBe(before.themeBg);
  expect(after.themeBg).toBe(before.themeBg);
  // And the terminal's own DOM box — what a pty resize would actually move —
  // never changed either.
  expect(during.rect).toEqual(before.rect);
  expect(after.rect).toEqual(before.rect);
});

test("LEAK: switching sessions repeatedly does not grow termbus's open-pane count", async ({ page }) => {
  // termbus.js exports openPaneCount() specifically as a leak assertion (see
  // termbus.js's own comments), but until now it was only exercised by
  // termbus's unit test calling openPane()/close() directly — never against
  // Terminal.svelte's real lifecycle. Terminal.svelte is wrapped in
  // {#key app.sessionKey} in App.svelte, so it is destroyed and recreated on
  // every session switch; a missing or broken teardown (closePane() not
  // called, or called against the wrong key) would leak one registration per
  // switch and nothing else would catch it.
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.sessions = [
      { windowID: "w1", name: "W1" }, { windowID: "w2", name: "W2" }, { windowID: "w3", name: "W3" },
    ];
  });
  const baseline = await page.evaluate(() => window.__termbus.openPaneCount());
  expect(baseline).toBe(0); // nothing mounted yet

  for (let i = 0; i < 9; i++) {
    const windowID = "w" + ((i % 3) + 1);
    await page.evaluate((windowID) => {
      window.__app.sessionKey = windowID + ":" + Date.now(); // forces a fresh {#key}, same as select()
    }, windowID);
    // Exactly one pane is ever open at a time: the old Terminal's onDestroy
    // (closePane) must have already run before the new one's onMount
    // (openPane) is counted, or this would intermittently read 2.
    await expect.poll(() => page.evaluate(() => window.__termbus.openPaneCount())).toBe(1);
  }

  await page.evaluate(() => { window.__app.sessionKey = ""; }); // back to EmptyState, no pane at all
  await expect.poll(() => page.evaluate(() => window.__termbus.openPaneCount())).toBe(baseline);
});

test("ACCEPTANCE: the list still scrolls to its last item with the companion present", async ({ page }) => {
  await mountPanel(page, { sessions: Array.from({ length: 30 }, (_, i) => sess("w" + i)), selected: "w0" });
  const list = page.getByTestId("session-list");
  for (const w of [240, 480]) {
    await page.evaluate((px) => {
      document.documentElement.style.setProperty("--sidebar-w", px + "px");
    }, w);
    await expect(page.getByTestId("sidebar-companion")).toBeVisible();
    await list.evaluate((el) => { el.scrollTop = el.scrollHeight; });
    await expect(page.getByTestId("session-card").last()).toBeInViewport();
    const m = await list.evaluate((el) => ({ s: el.scrollHeight, c: el.clientHeight }));
    expect(m.s, `list must overflow at ${w}px`).toBeGreaterThan(m.c);
  }
});

// The avatar-kind branch of this test lives in companion-overlay.spec.js
// instead of here: the public build's SettingsDrawer override drops the
// avatar radio and its controls (companion-size included), so a "set kind to
// avatar, expect companion-size visible" assertion would fail against the
// published UI once this spec ships in the mirror. companion-overlay.spec.js
// is private and deleted by the export, so the avatar assertions travel with
// it. What stays here — panel and off — passes in both trees, and the
// `companion-size` toHaveCount(0) under kind=panel still guards mutual
// exclusion from the panel side in the public build.
test("the settings kind radios switch which control group is shown", async ({ page }) => {
  await page.goto("/?nointro");
  await page.getByTestId("open-settings").click();
  await expect(page.getByTestId("drawer-settings")).toBeVisible();
  await expect(page.getByTestId("companion-kind-off")).toBeChecked();

  await page.evaluate(() => { window.__app.companionCfg.kind = "panel"; });
  await expect(page.getByTestId("companion-pick-pack")).toBeVisible();
  await expect(page.getByTestId("companion-clear-pack")).toBeVisible();
  await expect(page.getByTestId("companion-size")).toHaveCount(0);

  await expect(page.getByTestId("ambient-motion-toggle")).toBeChecked();
  await expect(page.getByTestId("scanlines-toggle")).not.toBeChecked();
});

test("pack warnings from Go render in the settings drawer", async ({ page }) => {
  await page.goto("/?nointro");
  await page.evaluate(() => {
    window.__app.companionCfg.kind = "panel";
    window.__app.companionWarnings = ["idle/a: file missing", "canvas is required"];
  });
  await page.getByTestId("open-settings").click();
  const list = page.getByTestId("companion-warnings");
  await expect(list).toBeVisible();
  await expect(list.locator("li")).toHaveCount(2);
});

// ---------------------------------------------------------------------------
// The tick pipeline, driven through the COMPONENT rather than through the pure
// modules. Every other fixture here has one variant in one slot, so pickSlot,
// createDwell, the resolver and the fade are never actually exercised end to
// end — a reactivity regression in the component (one was found during
// implementation: dwell.held() read in a template, which never re-renders)
// would break no test at all. This pack has a distinct variant per slot so a
// slot change is observable as BOTH a data-slot change and a src change.
const MULTI_SLOT_PACK = {
  schema: 1, name: "multi", author: "test", license: "test",
  canvas: { w: 832, h: 1216, scale: 2, anchor: "top-center" },
  alpha: true,
  slots: {
    idle:  [{ id: "fx-idle",  file: "idle.png",  weight: 1 }],
    done:  [{ id: "fx-done",  file: "done.png",  weight: 1 }],
    error: [{ id: "fx-error", file: "error.png", weight: 1 }],
  },
  warnings: [],
};

test("a slot change re-renders data-slot and swaps the art through a fade", async ({ page }) => {
  await mountPanel(page, {
    pack: MULTI_SLOT_PACK,
    sessions: [sess("w1")],
    selected: "w1",
  });

  const fig = page.getByTestId("sidebar-companion");
  const art = page.getByTestId("companion-art");
  await expect(fig).toBeVisible();
  // A fresh page has recorded no keystrokes, so msSinceInput is Infinity and
  // the ladder legitimately starts at `bored`, not `idle`. This pack declares
  // no bored art, so the resolver falls back to idle's — which means this line
  // also pins the fallback path.
  await expect(fig).toHaveAttribute("data-slot", "bored");
  const idleSrc = await art.getAttribute("src");
  expect(idleSrc).toContain("fx-idle");

  // Watch every opacity the art passes through, so the fade can be asserted
  // rather than inferred: a straight A->B cross-dissolve would never reach 0.
  await page.evaluate(() => {
    window.__opacities = [];
    const el = document.querySelector("[data-testid='companion-art']");
    new MutationObserver(() => window.__opacities.push(el.style.opacity))
      .observe(el, { attributes: true, attributeFilter: ["style"] });
  });

  // error is the one slot allowed to preempt the dwell, so this lands promptly.
  await page.evaluate(() => {
    const st = window.__app.companionState;
    window.__app.companionState = {
      ...st,
      sessions: st.sessions.map((s) => ({ ...s, errorMs: Date.now() })),
    };
  });

  await expect(fig).toHaveAttribute("data-slot", "error");
  await expect(art).not.toHaveAttribute("src", idleSrc);

  // The art must have been fully hidden at some point during the swap — that
  // is the fade-through-backdrop that stops two different faces ghosting
  // into each other.
  const sawZero = await page.evaluate(() => window.__opacities.includes("0"));
  expect(sawZero).toBe(true);
});

test("a throw inside the companion leaves the rest of the deck usable", async ({ page }) => {
  // The old companion lived in its own WKWebView and could not take the app
  // down. In-deck it can, so <svelte:boundary> is load-bearing (spec 8.2) and
  // is otherwise asserted nowhere.
  await mountPanel(page, { sessions: [sess("w1")], selected: "w1" });
  await expect(page.getByTestId("sidebar-companion")).toBeVisible();

  // A pack whose slots are not iterable makes the very next tick throw.
  await page.evaluate(() => { window.__app.companionPack = { slots: 42 }; });
  await page.waitForTimeout(2000);   // MAX_TICK_FAILURES ticks at 2Hz

  await expect(page.getByTestId("sidebar")).toBeVisible();
  await expect(page.getByTestId("session-card").first()).toBeVisible();
  await page.getByTestId("open-settings").click();
  await expect(page.getByTestId("drawer-settings")).toBeVisible();
});

// --- caption --------------------------------------------------------------

// The figure reflects the SELECTED session only. With several running, one face
// tells you something is happening but not what, and the caption is the whole
// difference.
test("the caption names the session the figure is reflecting", async ({ page }) => {
  await mountPanel(page, {
    sessions: [sess("w1"), sess("w2")],
    selected: "w1",
  });
  const cap = page.getByTestId("companion-caption");
  await expect(cap).toBeVisible();
  await expect(cap).toContainText("W1");

  // Switching the selection re-points the caption, not just the art.
  await page.evaluate(() => {
    window.__app.companionState.selected = "w2";
    window.__app.sessionKey = "w2";
  });
  await expect(cap).toContainText("W2");
  await expect(cap).not.toContainText("W1");
});

test("the caption says what the figure is doing in words, not slot names", async ({ page }) => {
  // A session finished seconds ago puts the ladder in `done` through the state
  // it already reads. An earlier version called window.__termbus.noteOutput,
  // which is NOT exposed — the optional chain silently no-opped, the ladder
  // never left `idle`, and the assertions below would have held for an
  // implementation hard-coded to one word.
  await mountPanel(page, {
    sessions: [sess("w1", { lastFinishMs: Date.now() - 500 })],
    selected: "w1",
  });
  const cap = page.getByTestId("companion-caption");
  await expect(cap).toContainText("finished");
  await expect(cap).not.toContainText("done");   // the slot name must not leak
  await expect(cap).not.toContainText("undefined");
});

// --- resizable band -------------------------------------------------------

// The band CROPS its content, so a flexible height re-crops on every window
// resize — the same art sits differently in a small window than a large one.
test("the companion band has a resize handle that persists a height", async ({ page }) => {
  await mountPanel(page, { sessions: [sess("w1"), sess("w2"), sess("w3"), sess("w4"), sess("w5"), sess("w6")], selected: "w1" });
  const handle = page.getByTestId("sidebar-dock-handle");
  await expect(handle).toBeVisible();

  const before = await page.getByTestId("sidebar-dock").boundingBox();
  const box = await handle.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2, box.y - 120, { steps: 8 });
  await page.mouse.up();

  const after = await page.getByTestId("sidebar-dock").boundingBox();
  expect(after.height, "dragging the handle up must make the band taller").toBeGreaterThan(before.height);

  const stored = await page.evaluate(() => window.__prefs.dockH);
  expect(stored, "the height must persist, or it is lost on reload").toBeGreaterThan(0);
});

// A height stored from a large window must not squeeze the session list to
// nothing when the same prefs are read back in a small one.
test("a stored height is capped so the session list survives", async ({ page }) => {
  await mountPanel(page, { sessions: [sess("w1"), sess("w2")], selected: "w1" });
  await page.evaluate(() => { window.__prefs.dockH = 5000; });

  const sidebar = await page.getByTestId("sidebar").boundingBox();
  const dock = await page.getByTestId("sidebar-dock").boundingBox();
  expect(dock.height).toBeLessThanOrEqual(sidebar.height * 0.72);
  await expect(page.getByTestId("session-list")).toBeVisible();
});

// Pointer-only resize is unreachable for anyone who cannot drag, and this one
// persists real state.
test("the resize handle is keyboard operable in both directions", async ({ page }) => {
  await mountPanel(page, { sessions: [sess("w1"), sess("w2"), sess("w3"), sess("w4"), sess("w5"), sess("w6")], selected: "w1" });
  const handle = page.getByTestId("sidebar-dock-handle");
  const dock = page.getByTestId("sidebar-dock");
  await handle.focus();

  const start = (await dock.boundingBox()).height;
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  const shrunk = (await dock.boundingBox()).height;
  expect(shrunk, "ArrowDown must shrink the band").toBeLessThan(start);

  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");
  const grown = (await dock.boundingBox()).height;
  expect(grown, "ArrowUp must grow it back").toBeGreaterThan(shrunk);
});

// Nothing in the column grows once dockH is set, so the leftover space in a
// SHORT list used to collect below the band and float it mid-column. Position,
// not just size: every existing resize test asserted heights and passed
// throughout.
test("the band stays pinned to the column's bottom at any stored height", async ({ page }) => {
  await mountPanel(page, { sessions: [sess("w1")], selected: "w1" });
  const sidebar = page.getByTestId("sidebar");
  const dock = page.getByTestId("sidebar-dock");

  const gaps = [];
  for (const h of [150, 250, 350, 450]) {
    await page.evaluate((v) => { window.__prefs.dockH = v; }, h);
    const a = await sidebar.boundingBox();
    const d = await dock.boundingBox();
    expect(Math.round(d.height), `band height at dockH ${h}`).toBeCloseTo(h, -1);
    gaps.push(Math.round(a.y + a.height - (d.y + d.height)));
  }
  // The only thing under the band is the column's own bottom padding, so the
  // gap must be constant — a gap that grows with the drag is the defect.
  for (const g of gaps) expect(g, `gap below band (all: ${gaps})`).toBeLessThan(32);
  expect(Math.max(...gaps) - Math.min(...gaps), "gap must not vary with height").toBeLessThan(4);
});

// The drag reads the band's OWN bottom edge each pointermove, so an unpinned
// band moved the reference under the cursor and the height ran away.
test("dragging the handle down shrinks the band by about the drag distance", async ({ page }) => {
  await mountPanel(page, { sessions: [sess("w1")], selected: "w1" });
  await page.evaluate(() => { window.__prefs.dockH = 400; });

  const handle = page.getByTestId("sidebar-dock-handle");
  const box = await handle.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + 100, { steps: 10 });
  await page.mouse.up();

  const stored = await page.evaluate(() => window.__prefs.dockH);
  // ~300. A runaway compounds every step and bottoms out at the clamp floor.
  expect(stored, "dragging down 100px must take ~100px off the band").toBeGreaterThan(260);
  expect(stored).toBeLessThan(340);
});

// The art FILLS the band at every height — the band and the figure must never
// disagree, which is what left dead background around a stuck image. The crop
// shifting as the band is resized is the accepted cost (focusX/focusY keeps the
// face in frame); what is not acceptable is empty surface, or an overflow.
test("the frame fills the band at any band height", async ({ page }) => {
  await mountPanel(page, {
    pack: { ...FIXTURE_PACK, canvas: { w: 800, h: 1200, scale: 1, anchor: "top-center" } },
    sessions: [sess("w1")], selected: "w1",
  });
  const frame = page.getByTestId("companion-frame");

  for (const h of [200, 320, 460]) {
    await page.evaluate((v) => { window.__prefs.dockH = v; }, h);
    const box = await frame.boundingBox();
    const dock = await page.getByTestId("sidebar-dock").boundingBox();

    // Fills the width outright.
    expect(box.width, `width at dockH ${h}`).toBeGreaterThan(dock.width - 2);
    // And takes all the height the band has, less the region's own chrome
    // (border-top + padding-top + the gap above the bubble slot). That chrome
    // is a fixed cost, so the leftover must not grow with the band.
    expect(dock.height - box.height, `dead space at dockH ${h}`).toBeLessThan(40);

    // It must still never overflow the band it sits in.
    expect(box.height, `height at dockH ${h}`).toBeLessThanOrEqual(dock.height + 1);
    expect(box.width, `width at dockH ${h}`).toBeLessThanOrEqual(dock.width + 1);
  }
});

// A real PNG, built here rather than committed as a fixture: vite preview has
// no Go backend to serve /media/pack/*, and a broken image has no intrinsic
// size, so object-fit would have nothing to measure and any assertion about
// scaling would be vacuous.
let CRC = null;
function crc32(buf) {
  if (!CRC) {
    CRC = new Int32Array(256);
    for (let n = 0; n < 256; n++) {
      let c = n;
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
      CRC[n] = c;
    }
  }
  let c = -1;
  for (let i = 0; i < buf.length; i++) c = CRC[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return (c ^ -1) >>> 0;
}

function makePNG(w, h) {
  const chunk = (type, data) => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type, "ascii"), data]);
    const crc = Buffer.alloc(4);
    crc.writeUInt32BE(crc32(td));
    return Buffer.concat([len, td, crc]);
  };
  const stride = w * 4 + 1;
  const raw = Buffer.alloc(stride * h);
  for (let y = 0; y < h; y++) {
    const row = y * stride;
    for (let x = 0; x < w; x++) {
      const o = row + 1 + x * 4;
      raw[o] = 40; raw[o + 1] = 60; raw[o + 2] = 140; raw[o + 3] = 255;
    }
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8; ihdr[9] = 6;
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", ihdr),
    chunk("IDAT", deflateSync(raw)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

// How large the picture is actually PAINTED, derived from object-fit rather
// than assumed — this is what separates contain from cover. cover satisfied
// "the frame fills the band" while painting a 1024x336 image 1570px wide with
// 26% of it on screen.
const paintedSize = () => {
  const art = document.querySelector('[data-testid="companion-art"]');
  const fr = document.querySelector('[data-testid="companion-frame"]').getBoundingClientRect();
  const fit = getComputedStyle(art).objectFit;
  const pick = fit === "cover" ? Math.max : Math.min;
  const s = pick(fr.width / art.naturalWidth, fr.height / art.naturalHeight);
  return {
    fit,
    frame: { w: Math.round(fr.width), h: Math.round(fr.height) },
    painted: { w: Math.round(art.naturalWidth * s), h: Math.round(art.naturalHeight * s) },
  };
};

test("a wide pack is shown whole, not zoomed, at any band height", async ({ page }) => {
  await page.route("**/media/pack/**", (r) =>
    r.fulfill({ status: 200, contentType: "image/png", body: makePNG(1024, 336) }));
  await mountPanel(page, {
    pack: { ...FIXTURE_PACK, canvas: { w: 1024, h: 336, scale: 1, anchor: "top-center" } },
    sessions: [sess("w1")], selected: "w1",
  });
  await page.waitForSelector('[data-testid="companion-art"]');

  for (const h of [200, 520]) {
    await page.evaluate((v) => { window.__prefs.dockH = v; }, h);
    const m = await page.evaluate(paintedSize);
    // Never painted larger than the box: that is the whole picture being visible.
    expect(m.painted.w, `painted width at dockH ${h} (${JSON.stringify(m)})`)
      .toBeLessThanOrEqual(m.frame.w + 1);
    expect(m.painted.h, `painted height at dockH ${h}`).toBeLessThanOrEqual(m.frame.h + 1);
  }
  // At a tall band a wide picture is width-bound, so it letterboxes rather than
  // cropping. Under cover this height would have exceeded the frame.
  const tall = await page.evaluate(paintedSize);
  expect(tall.painted.h, "a wide picture must letterbox in a tall band").toBeLessThan(tall.frame.h);
});

test("a portrait pack scales with the band", async ({ page }) => {
  await page.route("**/media/pack/**", (r) =>
    r.fulfill({ status: 200, contentType: "image/png", body: makePNG(832, 1216) }));
  await mountPanel(page, { sessions: [sess("w1")], selected: "w1" });
  await page.waitForSelector('[data-testid="companion-art"]');

  await page.evaluate(() => { window.__prefs.dockH = 200; });
  const small = await page.evaluate(paintedSize);
  await page.evaluate(() => { window.__prefs.dockH = 520; });
  const big = await page.evaluate(paintedSize);

  expect(big.painted.h, `dragging the band taller must scale the figure (${small.painted.h} -> ${big.painted.h})`)
    .toBeGreaterThan(small.painted.h + 50);
  expect(big.painted.w).toBeLessThanOrEqual(big.frame.w + 1);
  expect(big.painted.h).toBeLessThanOrEqual(big.frame.h + 1);
});
