import { test, expect } from "@playwright/test";
import { stubDash, S1, pickMode } from "./dashboard-helpers.js";

const CITY = { locked: false, districts: [
  { key: "k1", label: "proj", rank: "Station", hours: 80, plot: 1, land: 6, roads: 1, parks: 2, cracks: 1, oldTown: false, ruins: false,
    plaques: [{ id: "first_blood", name: "First Blood", at: 1 }], buildings: [{ sessionID: "old1", title: "Refactor billing", archetype: "tower", hours: 3, seed: 5, startedAt: 1 }],
    blocks: [], oldTownCount: 0, landmarks: [] },
  { key: "k2", label: "gone-app", rank: "Outpost", hours: 2, plot: 2, land: 4, roads: 0, parks: 0, cracks: 0, oldTown: true, ruins: true,
    plaques: [], buildings: [], blocks: [], oldTownCount: 0, landmarks: [] },
] };

async function city(page, { live = [S1], cityView = CITY, motion = false } = {}) {
  if (motion) await page.emulateMedia({ reducedMotion: "no-preference" });
  await stubDash(page, { live });
  await page.addInitScript((c) => { window.__city = c; }, cityView);
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+3");
  await expect(page.getByTestId("dash-city")).toBeVisible();
}

test("⌘3 and the switcher open Al Modon with only the active districts", async ({ page }) => {
  await city(page);
  // k2 (gone-app) has no live session: hidden (owner, 2026-10-12).
  await expect(page.getByTestId("city-plate")).toHaveCount(1);
  await expect(page.getByTestId("city-plate")).toContainText("proj");
  await expect(page.getByTestId("city-plaque")).toHaveCount(0);
  await expect(page.getByTestId("city-plate")).toHaveAttribute("title", /First Blood/);
  await expect(page.getByTestId("city-sign")).toContainText("proj");
  await expect(page.getByTestId("city-sign")).toContainText("Fix VAT");
  await pickMode(page, "triage");
  await pickMode(page, "city");
  await expect(page.getByTestId("dash-city")).toBeVisible();
});

test("a waiting session shows a beacon above the blur and Review → opens it", async ({ page }) => {
  await city(page, { live: [{ ...S1, state: "permission", sinceMs: Date.now() - 5000 }] });
  const beacon = page.getByTestId("city-beacon");
  await expect(beacon).toBeVisible();
  expect(await beacon.evaluate((el) => el.closest('[data-testid="city-overlay"]') !== null)).toBe(true);
  await page.getByTestId("city-review").click();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["SelectSession", "@1"]);
});

test("session signs and hover cards are plain text", async ({ page }) => {
  const evil = `<img src=x onerror="window.__xss=1">`;
  const c = structuredClone(CITY);
  c.districts[0].buildings[0].title = evil;
  await city(page, { live: [{ ...S1, title: evil }], cityView: c });
  await expect(page.getByTestId("city-sign").first()).toContainText(evil);
  await page.evaluate(() => window.__cityHover({ sessionID: "old1" }));
  await expect(page.getByTestId("city-hover")).toContainText(evil);
  expect(await page.evaluate(() => window.__xss)).toBeUndefined();
  expect(await page.locator('[data-testid="dash-city"] img').count()).toBe(0);
});

test("history off: live sites only, with the note", async ({ page }) => {
  await city(page, { cityView: { locked: true, districts: [] } });
  await expect(page.getByTestId("city-locked")).toContainText("history");
  await expect(page.getByTestId("city-sign")).toHaveCount(1);
});

// Review Focus 4.
test("the live loop runs only while moving is allowed", async ({ page }) => {
  await city(page, { motion: true });
  const f1 = await page.evaluate(() => window.__cityFrames || 0);
  await page.waitForTimeout(400);
  const f2 = await page.evaluate(() => window.__cityFrames || 0);
  expect(f2).toBeGreaterThan(f1);
  await page.getByTestId("open-settings").click();
  await page.getByTestId("dash-motion-toggle").uncheck();
  await page.waitForTimeout(150);
  const f3 = await page.evaluate(() => window.__cityFrames);
  await page.waitForTimeout(400);
  expect(await page.evaluate(() => window.__cityFrames)).toBe(f3);
});

test("without canvas the city falls back to a list", async ({ page }) => {
  await page.addInitScript(() => { HTMLCanvasElement.prototype.getContext = () => null; });
  await city(page);
  await expect(page.getByTestId("city-list")).toContainText("proj");
});

// Final review I1: the static layer + blur rebuild on CityView changes only.
test("live ticks redraw sites without rebuilding the static city", async ({ page }) => {
  await page.addInitScript(() => {
    window.__blurs = 0;
    const d = Object.getOwnPropertyDescriptor(CanvasRenderingContext2D.prototype, "filter");
    Object.defineProperty(CanvasRenderingContext2D.prototype, "filter", {
      get() { return d.get.call(this); },
      set(v) { if (String(v).startsWith("blur")) window.__blurs++; d.set.call(this, v); },
    });
  });
  await city(page, { motion: true });
  await page.waitForTimeout(500);
  const before = await page.evaluate(() => window.__blurs);
  for (let i = 0; i < 3; i++) {
    await page.evaluate((s) => window.__handlers["dash:tick"].forEach((cb) => cb({ sessions: [s], events: [], removed: [] })),
      { ...S1, turns: 10 + i, sinceMs: Date.now() });
    await page.waitForTimeout(700);
  }
  expect(await page.evaluate(() => window.__blurs)).toBe(before);
});

// Final review I5.
test("a height-only resize keeps every layer the same size", async ({ page }) => {
  await city(page);
  const s = page.viewportSize();
  await page.setViewportSize({ width: s.width, height: s.height - 120 });
  await page.waitForTimeout(300);
  const [st, lv] = await page.evaluate(() => [document.querySelector('[data-testid="city-static"]').height, document.querySelector('[data-testid="city-live"]').height]);
  expect(lv).toBe(st);
});

// Final review I2: what does not fit on the land is still counted on the plate.
test("a crowded district shows what it could not draw", async ({ page }) => {
  const c = structuredClone(CITY);
  c.districts[0] = { ...c.districts[0], land: 4, parks: 0, oldTownCount: 10,
    buildings: Array.from({ length: 200 }, (_, i) => ({ sessionID: "b" + i, title: "t" + i, archetype: "hall", hours: 1, seed: i, startedAt: 1 })) };
  await city(page, { cityView: c });
  // The plot grows with its contents up to 12 × 12: 144 − 4 plaza − 1 live site = 139 lots.
  await expect(page.getByTestId("city-more")).toContainText("+61 more");
  await expect(page.getByTestId("city-more")).toContainText("old town 10");
});

// Final review I6: a city refresh outside Al Modon costs nothing.
test("dash:city refetches the city only while it is shown", async ({ page }) => {
  await city(page);
  await pickMode(page, "triage");
  const n = () => page.evaluate(() => window.__calls.filter((c) => c[0] === "CityView").length);
  const before = await n();
  await page.evaluate(() => window.__handlers["dash:city"].forEach((cb) => cb()));
  await page.waitForTimeout(200);
  expect(await n()).toBe(before);
});

// Owner, 2026-10-14: zoom needed tuning — buttons, 0 to fit, scroll zooms at the cursor.
const scaleOf = async (page) => Number(await page.getByTestId("city-zoom").getAttribute("data-scale"));

test("zoom: + / − buttons, Fit and the 0 key", async ({ page }) => {
  await city(page);
  await page.waitForTimeout(300); // let the view size settle before reading the fitted scale
  const fitted = await scaleOf(page);
  await page.getByTestId("city-zoom-out").click();
  expect(await scaleOf(page)).toBeLessThan(fitted);
  await page.getByTestId("city-fit").click();
  expect(await scaleOf(page)).toBeCloseTo(fitted, 3);
  await page.getByTestId("city-zoom-out").click();
  await page.getByTestId("city-overlay").focus();
  await page.keyboard.press("0");
  expect(await scaleOf(page)).toBeCloseTo(fitted, 3);
});

test("zoom: scrolling over the city zooms instead of panning", async ({ page }) => {
  await city(page);
  await page.waitForTimeout(300);
  const fitted = await scaleOf(page);
  const box = await page.getByTestId("city-live").boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, 300);
  await expect.poll(() => scaleOf(page)).toBeLessThan(fitted);
});

// Owner, 2026-10-14: callouts covered the buildings and each other.
test("callouts of several sessions in one district never overlap", async ({ page }) => {
  const live = Array.from({ length: 4 }, (_, i) => ({ ...S1, id: "s" + (i + 1), title: "task " + (i + 1) }));
  await stubDash(page, { live, windows: live.map((_, i) => ({ windowID: "@" + (i + 1), name: "w" + i })),
    stats: Object.fromEntries(live.map((_, i) => ["@" + (i + 1), { sessionID: "s" + (i + 1), model: "claude-opus-5-5", cwd: "/Users/me/proj", status: "active", band: "green" }])) });
  await page.addInitScript((c) => { window.__city = c; }, CITY);
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+3");
  await expect(page.getByTestId("city-sign")).toHaveCount(4);
  const boxes = await page.locator('[data-testid="city-sign"], [data-testid="city-plate"]').evaluateAll((els) => els.map((e) => { const r = e.getBoundingClientRect(); return [r.left, r.top, r.right, r.bottom]; }));
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
    const [a, b] = [boxes[i], boxes[j]];
    expect(a[2] <= b[0] + 1 || b[2] <= a[0] + 1 || a[3] <= b[1] + 1 || b[3] <= a[1] + 1).toBe(true);
  }
});

test("zoomed far out, callouts become one-line chips that still open the session", async ({ page }) => {
  await city(page);
  for (let i = 0; i < 12; i++) await page.getByTestId("city-zoom-out").click();
  await expect(page.getByTestId("city-sign")).toHaveAttribute("data-mode", "chip");
  await expect(page.getByTestId("city-sign").locator("button.open")).toBeVisible();
});

// Final review I5: scrolling over a callout or plate zooms too.
test("zoom: scrolling over a callout zooms the city", async ({ page }) => {
  await city(page);
  const fitted = await scaleOf(page);
  const b = await page.getByTestId("city-sign").boundingBox();
  await page.mouse.move(b.x + b.width / 2, b.y + 4);
  await page.mouse.wheel(0, 300);
  await expect.poll(() => scaleOf(page)).toBeLessThan(fitted);
});

// Minors sweep (phase 5a review): an external session's sign was only dimmed.
test("a session Commander did not launch says so on its sign", async ({ page }) => {
  await stubDash(page, { live: [{ ...S1, external: true }], windows: [] });
  await page.addInitScript((c) => { window.__city = c; }, CITY);
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+3");
  await expect(page.getByTestId("city-sign")).toContainText("external");
});

// Minors round 2 (phase 5a review): sprites that finished decoding after Al
// Modon was left were never closed.
test("sprites that finish loading after leaving are released", async ({ page }) => {
  await page.addInitScript(() => {
    window.__made = 0; window.__closed = 0;
    const orig = window.createImageBitmap.bind(window);
    window.createImageBitmap = async (b) => {
      await new Promise((r) => setTimeout(r, 600));
      const bm = await orig(b);
      window.__made++;
      const close = bm.close.bind(bm);
      bm.close = () => { window.__closed++; close(); };
      return bm;
    };
  });
  await city(page);
  await page.keyboard.press("Meta+1"); // leave Al Modon before the sprites decode
  await expect(page.getByTestId("dash-triage")).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.__made), { timeout: 5000 }).toBeGreaterThan(0);
  await page.waitForTimeout(800);
  const [made, closed] = await page.evaluate(() => [window.__made, window.__closed]);
  expect(closed).toBe(made);
});

// Minors round 2 (phase 5a review): "focused = hasFocus() || true" made a city
// opened in a background window animate anyway.
test("a city opened while the window is in the background does not animate", async ({ page }) => {
  await page.addInitScript(() => { document.hasFocus = () => false; });
  await city(page, { motion: true });
  await page.waitForTimeout(400);
  const a = await page.evaluate(() => window.__cityFrames || 0);
  await page.waitForTimeout(1000);
  expect(await page.evaluate(() => window.__cityFrames || 0) - a).toBeLessThan(3);
});

// Minors round 2 (phase 5a review): Al Modon's keys needed a click on the map first.
test("keys work right after opening Al Modon from the menu", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.addInitScript((c) => { window.__city = c; }, CITY);
  await page.goto("/?nointro");
  await page.getByTestId("dash-mode-switch").click();
  await page.getByTestId("dash-mode-city").click();
  await expect(page.getByTestId("dash-city")).toBeVisible();
  await page.waitForTimeout(300);
  const before = Number(await page.getByTestId("city-zoom").getAttribute("data-scale"));
  await page.keyboard.press("-");
  await expect.poll(async () => Number(await page.getByTestId("city-zoom").getAttribute("data-scale"))).toBeLessThan(before);
});

// Minors round 2: a long project name pushed "· external" out of the sign.
test("the external tag stays visible after a long project name", async ({ page }) => {
  await stubDash(page, { live: [{ ...S1, external: true, cwd: "/Users/me/a-really-quite-extraordinarily-long-project-name" }], windows: [],
    projects: [{ key: "k9", label: "a-really-quite-extraordinarily-long-project-name", root: "/Users/me/a-really-quite-extraordinarily-long-project-name" }] });
  await page.addInitScript((c) => { window.__city = c; }, CITY);
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+3");
  const sign = page.getByTestId("city-sign");
  const [ext, box] = await Promise.all([sign.locator(".ext").boundingBox(), sign.boundingBox()]);
  expect(ext.x + ext.width).toBeLessThanOrEqual(box.x + box.width + 1);
});
