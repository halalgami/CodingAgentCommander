import { test, expect } from "@playwright/test";
import { stubDash, S1, pickMode } from "./dashboard-helpers.js";

// World layers (Al Asturlab phase 2): last-7-days usage from Stats.
const WORLD = { pricedAt: "2026-10-06", rows: {
  "project|tokens": [{ key: "k1", value: 9_000_000 }, { key: "k3", value: 3_000_000 }],
  "project|cost": [{ key: "k1", value: 120.4 }, { key: "k3", value: 0, unpriced: 3_000_000 }],
  "provider|tokens": [{ key: "anthropic", value: 9_000_000 }, { key: "ollama-cloud", value: 3_000_000 }],
  "provider|cost": [{ key: "anthropic", value: 120.4 }, { key: "ollama-cloud", value: 0, unpriced: 3_000_000 }],
  "model|tokens": [{ key: "claude-opus-5-5", value: 9_000_000 }, { key: "glm-5.3", value: 3_000_000 }],
} };
const D = (key, label) => ({ key, label, rank: "Station", hours: 10, plot: 1, land: 6, roads: 1, parks: 1, cracks: 0, oldTown: false, ruins: false,
  plaques: [], buildings: [], blocks: [], oldTownCount: 0, landmarks: [] });
const CITY = { locked: false, districts: [D("k1", "proj"), D("k3", "side")] };
const S3 = { ...S1, id: "s3", title: "Side quest", cwd: "/Users/me/side" };
const PROJECTS = [{ key: "k1", label: "proj", root: "/Users/me/proj" }, { key: "k3", label: "side", root: "/Users/me/side" }];

async function city(page, { cityView = CITY, usage = { five_hour: { utilization: 40 } } } = {}) {
  await stubDash(page, { live: [S1, S3], projects: PROJECTS, windows: [{ windowID: "@1", name: "proj" }, { windowID: "@3", name: "side" }],
    stats: { "@1": { sessionID: "s1", cwd: "/Users/me/proj", model: "claude-opus-5-5", status: "active" }, "@3": { sessionID: "s3", cwd: "/Users/me/side", model: "glm-5.3", status: "active" } } });
  await page.addInitScript(({ c, w, u }) => { window.__city = c; window.__ast = w; window.__usage = u; }, { c: cityView, w: WORLD, u: usage });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+3");
  await expect(page.getByTestId("dash-city")).toBeVisible();
}

test("E toggles the economy view", async ({ page }) => {
  await city(page);
  await expect(page.getByTestId("city-pillar")).toHaveCount(0);
  await page.keyboard.press("e");
  await expect(page.getByTestId("city-pillar")).toHaveCount(2);
  await page.keyboard.press("e");
  await expect(page.getByTestId("city-pillar")).toHaveCount(0);
});

test("the busiest project has the tallest pillar; unpriced shows tokens only", async ({ page }) => {
  await city(page);
  await page.keyboard.press("e");
  const k1 = page.locator('[data-testid="city-pillar"][data-key="k1"]'), k3 = page.locator('[data-testid="city-pillar"][data-key="k3"]');
  await expect(k1).toHaveAttribute("data-h", "120");
  expect(Number(await k3.getAttribute("data-h"))).toBeLessThan(120);
  await expect(k1).toContainText("≈ $");
  await expect(k3).toContainText("3.0M tok");
  await expect(k3).not.toContainText("$");
});

test("treasury lists quota and spend per provider", async ({ page }) => {
  await city(page);
  await page.keyboard.press("e");
  const t = page.getByTestId("city-treasury");
  await expect(t).toBeVisible();
  await expect(t.locator('[data-provider="anthropic"]')).toContainText("40%");
  await expect(t.locator('[data-provider="anthropic"]')).toContainText("≈ $120");
  await expect(t.locator('[data-provider="ollama-cloud"]')).toContainText("No official quota");
  await expect(t).toContainText("priced 2026-10-06");
});

test("the economy button is keyboard reachable and says its state", async ({ page }) => {
  await city(page);
  const b = page.getByTestId("city-economy");
  await expect(b).toHaveAttribute("aria-pressed", "false");
  await b.focus();
  await page.keyboard.press("Enter");
  await expect(b).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByTestId("city-treasury")).toBeVisible();
});

test("no history: economy view invites turning it on", async ({ page }) => {
  await city(page, { cityView: { ...CITY, locked: true } });
  await page.getByTestId("city-economy").click();
  await expect(page.getByTestId("city-economy-locked")).toBeVisible();
});

// ---- Raid, Sector, Fleet ----
const PROG = { commander: { level: 3 }, locked: false, live: [], projects: [{ key: "k1", label: "proj", hours: 60, rank: "Station" }, { key: "k3", label: "side", hours: 3, rank: "Outpost" }] };
async function mode(page, key, testid, { usage } = {}) {
  await stubDash(page, { live: [S1, S3], projects: PROJECTS, progress: PROG, windows: [{ windowID: "@1", name: "proj" }, { windowID: "@3", name: "side" }],
    stats: { "@1": { sessionID: "s1", cwd: "/Users/me/proj", model: "claude-opus-5-5", status: "active" }, "@3": { sessionID: "s3", cwd: "/Users/me/side", model: "glm-5.3", status: "active" } } });
  await page.addInitScript(({ w, u }) => { window.__ast = w; window.__usage = u; }, { w: WORLD, u: usage || null });
  await page.goto("/?nointro");
  await page.keyboard.press(key);
  await expect(page.getByTestId(testid)).toBeVisible();
}

test("Raid pouch holds this week's spend and dims near quota", async ({ page }) => {
  await mode(page, "Meta+4", "dash-raid", { usage: { five_hour: { utilization: 40 } } });
  const p1 = page.locator('[data-key="@1"]').getByTestId("raid-pouch"), p3 = page.locator('[data-key="@3"]').getByTestId("raid-pouch");
  await expect(p1).toHaveAttribute("title", "≈ $120 this week · quota 40%");
  await expect(p3).toHaveAttribute("title", "3.0M tok this week · quota 40%");
  await expect(p1).toHaveAttribute("data-dim", "false");
  await page.evaluate(() => (window.__handlers["quota:sample"] || []).forEach((cb) => cb({ provider: "anthropic", maxUtil: 85 })));
  await expect(p1).toHaveAttribute("data-dim", "true");
});

test("Sector stations carry a fuel ring for this week's tokens", async ({ page }) => {
  await mode(page, "Meta+7", "dash-sector");
  await expect(page.locator('[data-testid="sector-fuel"][data-key="k1"]')).toHaveAttribute("data-frac", "1");
  await expect(page.locator('[data-testid="sector-fuel"][data-key="k3"]')).toHaveAttribute("data-frac", "0.333");
});

test("Fleet flagships glow with their model's share of the week", async ({ page }) => {
  await mode(page, "Meta+5", "dash-fleet");
  await expect(page.locator('[data-testid="fleet-panel"][data-key="@1"]').getByTestId("fleet-ship")).toHaveAttribute("data-glow", "3");
  await expect(page.locator('[data-testid="fleet-panel"][data-key="@3"]').getByTestId("fleet-ship")).toHaveAttribute("data-glow", "1");
});
