import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

const CITY = { locked: false, districts: [
  { key: "k1", label: "proj", rank: "Station", hours: 8, plot: 1, land: 6, roads: 0, parks: 0, cracks: 0, ruins: false,
    plaques: [], buildings: [], blocks: [], oldTownCount: 0, landmarks: [] },
] };
const evil = `<img src=x onerror="window.__xss=1">.go`;
const CODE = { key: "k1", label: "proj", locked: false, noMetrics: false, missing: false, scannedAt: 1,
  folders: [{ path: "", code: 900 }, { path: "src", code: 800 }],
  files: [
    { path: "src/app.go", folder: "src", lang: "Go", code: 600, complexity: 80, todos: 1, hotspot: 3, flagged: true, test: false, ghost: false, new: false, touch: { edits: 9, writes: 1, reads: 3, lastAt: Date.now() } },
    { path: "src/" + evil, folder: "src", lang: "Go", code: 200, complexity: 5, todos: 0, hotspot: 0, flagged: false, test: false, ghost: false, new: false, touch: { edits: 0, writes: 0, reads: 0, lastAt: 0 } },
  ], slabs: [{ folder: "src", files: 12, code: 300 }] };

async function open(page, { code = CODE, live = [{ ...S1, cwd: "/Users/me/proj", targetRel: "src/app.go" }], motion = false } = {}) {
  if (motion) await page.emulateMedia({ reducedMotion: "no-preference" });
  await stubDash(page, { live });
  await page.addInitScript(([c, v]) => { window.__city = c; window.__code = v; }, [CITY, code]);
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+3");
  await page.getByTestId("city-plate").first().dblclick();
  await expect(page.getByTestId("dash-codecity")).toBeVisible();
}

test("double-click a district zooms in; breadcrumb and Esc go back", async ({ page }) => {
  await open(page);
  await expect(page.getByTestId("code-breadcrumb")).toContainText("proj");
  await page.getByTestId("code-breadcrumb-city").click();
  await expect(page.getByTestId("dash-city")).toBeVisible();
  await page.getByTestId("city-plate").first().dblclick();
  await expect(page.getByTestId("dash-codecity")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("dash-city")).toBeVisible();
});

// Review Focus 3.
test("file names stay plain text in the hover card", async ({ page }) => {
  await open(page);
  await page.evaluate((p) => window.__codeHover(p), "src/" + evil);
  await expect(page.getByTestId("code-hover")).toContainText(evil);
  expect(await page.evaluate(() => window.__xss)).toBeUndefined();
});

test("a live crane's sign opens its terminal", async ({ page }) => {
  await open(page);
  await expect(page.getByTestId("code-sign")).toContainText("Fix VAT");
  await page.getByTestId("code-sign").getByRole("button").click();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["SelectSession", "@1"]);
});

test("H toggles heat without rebuilding the static layer", async ({ page }) => {
  await open(page);
  await page.waitForTimeout(300);
  const before = await page.evaluate(() => window.__codeStatic);
  await page.keyboard.press("h");
  await expect(page.getByTestId("code-heat-legend")).toBeVisible();
  await expect(page.getByTestId("code-buildings")).toHaveClass(/faded/);
  await page.waitForTimeout(300);
  expect(await page.evaluate(() => window.__codeStatic)).toBe(before);
});

test("never scanned: Scan asks Go to scan", async ({ page }) => {
  await open(page, { code: { ...CODE, noMetrics: true, files: [], slabs: [], folders: [] } });
  await page.getByTestId("code-scan").click();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["ScanProject", "k1"]);
});

// Review Focus 5.
test("forgetting the project while zoomed returns to the city", async ({ page }) => {
  await open(page);
  await page.evaluate(() => { window.__city = { locked: false, districts: [] }; window.__handlers["dash:city"].forEach((cb) => cb()); });
  await expect(page.getByTestId("dash-codecity")).toHaveCount(0);
});

test("still mode: no frames", async ({ page }) => {
  await open(page);
  await page.waitForTimeout(500); // sprites load and repaint once
  const f = await page.evaluate(() => window.__codeFrames || 0);
  await page.waitForTimeout(1000);
  // Only the 1 s clock redraws; a running loop would add ~30.
  expect(await page.evaluate(() => window.__codeFrames || 0)).toBeLessThanOrEqual(f + 3);
});

// Final review I1: keys still work after clicking or dragging the map.
test("Esc goes back after a click on the map", async ({ page }) => {
  await open(page);
  const box = await page.getByTestId("dash-codecity").boundingBox();
  await page.mouse.click(box.x + box.width / 2, box.y + box.height - 40);
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("dash-city")).toBeVisible();
});

// Final review I2: steady project updates refresh at most every 2 s, and the
// city never blanks to "Loading…" while it refreshes.
test("live project updates refresh the CodeView without blanking", async ({ page }) => {
  await open(page);
  const calls = () => page.evaluate(() => window.__calls.filter((c) => c[0] === "CodeView").length);
  const first = await calls();
  for (let i = 0; i < 9; i++) {
    await page.evaluate(() => window.__handlers["dash:project"].forEach((cb) => cb({ key: "k1", label: "proj" })));
    await expect(page.getByTestId("code-loading")).toHaveCount(0);
    await page.waitForTimeout(500);
  }
  expect(await calls()).toBeGreaterThan(first);
});

// Final review I5: the slab says how many files it stands for.
test("hovering a folder's slab shows how many files it summarises", async ({ page }) => {
  await open(page);
  await page.evaluate(() => window.__codeHover("slab:src"));
  await expect(page.getByTestId("code-hover")).toContainText("12 more files");
});

// Final review (CPU): nothing to animate → no animation loop, even with motion on.
test("no cranes: the live loop stays idle", async ({ page }) => {
  await open(page, { motion: true, live: [{ ...S1, cwd: "/Users/me/proj", targetRel: "" }] });
  await page.waitForTimeout(500);
  const f = await page.evaluate(() => window.__codeFrames || 0);
  await page.waitForTimeout(1000);
  expect(await page.evaluate(() => window.__codeFrames || 0)).toBeLessThanOrEqual(f + 3);
});

// Minors round 2 (phase 5b review): CodeCity zoom matches Al Modon's.
test("zoom: buttons, 0 to fit, and scroll zooms", async ({ page }) => {
  await open(page);
  const scale = async () => Number(await page.getByTestId("code-zoom").getAttribute("data-scale"));
  await page.waitForTimeout(300); // let the view size settle before reading the fitted scale
  const fitted = await scale();
  await page.getByTestId("code-zoom-out").click();
  expect(await scale()).toBeLessThan(fitted);
  await page.getByTestId("code-fit").click();
  expect(await scale()).toBeCloseTo(fitted, 3);
  const box = await page.getByTestId("code-buildings").boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, 300);
  await expect.poll(scale).toBeLessThan(fitted);
});

// Minors round 2: heat repainted every second while shown; it changes by the minute.
test("heat on: no repaint per tick", async ({ page }) => {
  await open(page, { motion: true });
  await page.keyboard.press("h");
  await expect(page.getByTestId("code-heat-legend")).toBeVisible();
  await page.waitForTimeout(500);
  const before = await page.evaluate(() => window.__codeHeat);
  expect(typeof before).toBe("number");
  await page.waitForTimeout(2500);
  expect(await page.evaluate(() => window.__codeHeat)).toBe(before);
});

// Minors round 2: a crane waiting for permission had no amber beacon.
test("a crane waiting for permission shows Needs you", async ({ page }) => {
  await open(page, { live: [{ ...S1, state: "permission", cwd: "/Users/me/proj", targetRel: "src/app.go" }] });
  await expect(page.getByTestId("code-beacon")).toContainText("Needs you");
});

// Minors round 2: the hover card kept the old counts until the mouse moved.
test("the hover card follows a refreshed CodeView", async ({ page }) => {
  await open(page);
  await page.evaluate(() => window.__codeHover("src/app.go"));
  await expect(page.getByTestId("code-hover")).toContainText("600");
  await page.evaluate(() => {
    window.__code = { ...window.__code, files: window.__code.files.map((f) => (f.path === "src/app.go" ? { ...f, code: 777 } : f)) };
    window.__handlers["dash:project"].forEach((cb) => cb({ key: "k1", label: "proj" }));
  });
  await expect(page.getByTestId("code-hover")).toContainText("777", { timeout: 5000 });
});

// Minors round 2 (phase 5b review): forgetting the project while away from the
// dashboard left CodeCity zoomed into it on return.
test("a project forgotten while away is not shown zoomed on return", async ({ page }) => {
  await open(page);
  await page.getByTestId("session-name").first().click(); // leave home
  await expect(page.getByTestId("dash-home")).toHaveCount(0);
  await page.evaluate(() => { window.__city = { locked: false, districts: [] }; });
  await page.keyboard.press("Meta+d");
  await expect(page.getByTestId("dash-home")).toBeVisible();
  await expect(page.getByTestId("dash-codecity")).toHaveCount(0);
});

// Minors round 2 (phase 5b review): the zoom-in grew from the screen centre,
// not from the district that was double-clicked.
test("zoom-in grows from the district's plate", async ({ page }) => {
  await stubDash(page, { live: [{ ...S1, cwd: "/Users/me/proj", targetRel: "src/app.go" }] });
  await page.addInitScript(([c, v]) => { window.__city = c; window.__code = v; }, [CITY, CODE]);
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+3");
  const plate = await page.getByTestId("city-plate").first().boundingBox();
  await page.getByTestId("city-plate").first().dblclick();
  const host = await page.getByTestId("dash-codecity").boundingBox();
  const origin = await page.getByTestId("dash-codecity").evaluate((el) => el.style.transformOrigin);
  const [ox, oy] = origin.split(" ").map(parseFloat);
  expect(Math.abs(host.x + ox - (plate.x + plate.width / 2))).toBeLessThan(4);
  expect(Math.abs(host.y + oy - (plate.y + plate.height / 2))).toBeLessThan(4);
});
