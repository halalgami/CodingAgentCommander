import { test, expect } from "@playwright/test";
import { stubDash, S1, pickMode } from "./dashboard-helpers.js";
import { AST } from "./asturlab-fixtures.js";

async function open(page, { ast = AST, live = [S1], windows } = {}) {
  await stubDash(page, { live, ...(windows ? { windows } : {}) });
  await page.addInitScript((ast) => { window.__ast = ast; }, ast);
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-mode-switch")).toBeVisible();
  await pickMode(page, "asturlab");
  await expect(page.getByTestId("asturlab")).toBeVisible();
}
const calls = (page, name) => page.evaluate((n) => window.__calls.filter((c) => c[0] === n), name);

test("⌘8 and the switcher open Al Asturlab", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.addInitScript((ast) => { window.__ast = ast; }, AST);
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-mode-switch")).toBeVisible();
  await page.keyboard.press("Meta+8");
  await expect(page.getByTestId("asturlab")).toBeVisible();
  await expect(page.getByTestId("dash-mode-switch")).toContainText("Al Asturlab");
  await expect(page.getByTestId("ast-tile-tokens")).toContainText("12.4M");
});

test("range tab re-queries every tile and chart with the same from/to", async ({ page }) => {
  await open(page);
  await page.evaluate(() => { window.__calls = []; });
  await page.getByTestId("ast-range-30d").click();
  await expect.poll(async () => (await calls(page, "StatsOverview")).length).toBeGreaterThan(0);
  await expect.poll(async () => (await calls(page, "Stats")).length).toBeGreaterThanOrEqual(5);
  const ov = (await calls(page, "StatsOverview"))[0];
  const stats = await calls(page, "Stats");
  const models = await calls(page, "StatsModels");
  // The calendar always shows whole weeks ending on `to`; every other query uses the range itself.
  for (const [, q] of stats) { expect(q.to).toBe(ov[2]); if (!(q.groupBy === "day" && !q.split)) expect(q.from).toBe(ov[1]); }
  for (const m of models) { expect(m[1]).toBe(ov[1]); expect(m[2]).toBe(ov[2]); }
  const days = (new Date(ov[2]) - new Date(ov[1])) / 86_400_000 + 1;
  expect(days).toBe(30);
});

test("every dollar is an estimate with its priced-at date; stale rates show the badge", async ({ page }) => {
  await open(page, { ast: { ...AST, stale: true, overview: { ...AST.overview, overview: { ...AST.overview.overview, cur: { ...AST.overview.overview.cur, stale: true } } } } });
  await expect(page.getByTestId("ast-tile-cost")).toContainText("≈ $418");
  await expect(page.getByTestId("ast-tile-cost")).toContainText("2026-10-06");
  await expect(page.getByTestId("ast-stale").first()).toBeVisible();
});

test("unpriced tokens are noted, never shown as $0", async ({ page }) => {
  const cur = { ...AST.overview.overview.cur, costUSD: 0, unpricedTokens: 1_400_000, planX: 0 };
  await open(page, { ast: { ...AST, overview: { locked: false, overview: { ...AST.overview.overview, cur } } } });
  const tile = page.getByTestId("ast-tile-cost");
  await expect(tile).toContainText("1.4M tokens unpriced");
  await expect(tile).not.toContainText("$0");
  await expect(page.getByTestId("ast-chart-models")).toContainText("tokens only");
});

test("no history: locked panel invites turning it on", async ({ page }) => {
  await open(page, { ast: null });
  await expect(page.getByTestId("ast-locked")).toBeVisible();
  await page.getByTestId("ast-locked").getByRole("button", { name: /turn on history/i }).click();
  await expect(page.getByTestId("ledger-toggle")).toBeVisible();
});

test("table toggle shows the same value as the tooltip", async ({ page }) => {
  await open(page);
  const chart = page.getByTestId("ast-chart-spend");
  await chart.locator("[data-value]").nth(1).hover();
  const tip = await chart.getByTestId("ast-tip").innerText();
  expect(tip).toContain("2026-11-11");
  await chart.getByTestId("ast-table-toggle").click();
  const table = chart.locator("table");
  await expect(table).toBeVisible();
  const value = tip.match(/[\d.,]+[kMB]?$/m)?.[0];
  await expect(table).toContainText(value);
});

test("keyboard reaches every chart's table toggle", async ({ page }) => {
  await open(page);
  const n = await page.getByTestId("ast-table-toggle").count();
  expect(n).toBeGreaterThanOrEqual(5);
  await page.getByTestId("ast-range-today").focus();
  const seen = new Set();
  for (let i = 0; i < 80 && seen.size < n; i++) {
    await page.keyboard.press("Tab");
    const id = await page.evaluate(() => document.activeElement?.dataset?.testid === "ast-table-toggle" ? document.activeElement.closest("[data-chart]")?.dataset.chart : null);
    if (id) seen.add(id);
  }
  expect(seen.size).toBe(n);
});

test("opens with zero live sessions", async ({ page }) => {
  await open(page, { live: [], windows: [] });
  await expect(page.getByTestId("ast-tile-commits")).toContainText("14");
});

test("treemap click opens the project page filtered to that project", async ({ page }) => {
  await open(page);
  await page.addInitScript(() => {});
  await page.evaluate(() => { window.__metrics = { files: { "a.go": { code: 100, complexity: 30, hotspot: 9 }, "b.go": { code: 10, complexity: 2, hotspot: 1 } },
    snapshots: [{ day: "2026-11-01", code: 1000, complexity: 200 }, { day: "2026-11-10", code: 1400, complexity: 260 }] }; window.__calls = []; });
  await page.locator('[data-testid="ast-tree-cell"][data-key="k1"]').click();
  await expect(page.getByTestId("ast-crumb")).toHaveText("Ayina");
  await expect.poll(async () => (await calls(page, "Stats")).filter(([, q]) => q.project === "k1").length).toBeGreaterThanOrEqual(5);
  expect((await calls(page, "StatsModels")).at(-1)[3]).toBe("k1");
  const pp = page.getByTestId("ast-project");
  await expect(pp).toContainText("1,400 lines");
  await expect(pp).toContainText("a.go");
});

test("a models row opens the model page; Back returns with the range kept", async ({ page }) => {
  await open(page);
  await page.getByTestId("ast-range-30d").click();
  await page.evaluate(() => { window.__calls = []; });
  await page.getByTestId("ast-model-row").first().click();
  await expect(page.getByTestId("ast-crumb")).toHaveText("claude-opus-5-5");
  await expect.poll(async () => (await calls(page, "Stats")).filter(([, q]) => q.model === "claude-opus-5-5").length).toBeGreaterThanOrEqual(5);
  await page.getByTestId("ast-back").click();
  await expect(page.getByTestId("ast-crumb")).toHaveCount(0);
  await expect(page.getByTestId("ast-range-30d")).toHaveAttribute("aria-pressed", "true");
});

test("Export days CSV calls ExportStats(days, csv, from, to)", async ({ page }) => {
  await open(page);
  await page.getByTestId("ast-export-csv").click();
  const ex = (await calls(page, "ExportStats")).at(-1);
  const ov = (await calls(page, "StatsOverview")).at(-1);
  expect(ex.slice(1, 5)).toEqual(["days", "csv", ov[1], ov[2]]);
});

test("Settings: saving a $200 Anthropic plan persists it and reloads the tiles", async ({ page }) => {
  await open(page);
  await page.getByTestId("open-settings").click();
  const s = page.getByTestId("ast-settings");
  await expect(s).toContainText("2026-10-06");
  await s.getByTestId("ast-plan-anthropic-name").fill("Max");
  await s.getByTestId("ast-plan-anthropic-usd").fill("200");
  await s.getByTestId("ast-rate-model").fill("glm-5.3");
  await s.getByTestId("ast-rate-in").fill("0.6");
  await s.getByTestId("ast-rate-out").fill("2.2");
  await s.getByTestId("ast-settings-save").click();
  const saved = (await calls(page, "SetAsturlabSettings")).at(-1)[1];
  expect(saved.plans.anthropic).toEqual({ name: "Max", monthlyUSD: 200 });
  expect(saved.overrides["glm-5.3"]).toEqual({ in: 0.6, out: 2.2, cacheRead: 0, cacheWrite: 0 });
});

test("$ mode: an unpriced model reads 'tokens unpriced', never ≈ $0.00", async ({ page }) => {
  const ast = { ...AST, rows: { ...AST.rows, "day:model": [{ key: "2026-11-11", split: "glm-5.3", value: 0, unpriced: 5000 }, { key: "2026-11-11", split: "claude-opus-5-5", value: 2.5 }] } };
  await open(page, { ast });
  await page.getByTestId("ast-chart-spend").getByRole("button", { name: "$" }).click();
  const chart = page.getByTestId("ast-chart-spend");
  await chart.getByTestId("ast-table-toggle").click();
  await expect(chart.locator("table")).toContainText("5.0k tokens unpriced");
  await expect(chart.locator("table")).not.toContainText("$0.00");
});

test("Settings: an override with no in/out rate is not saved as $0", async ({ page }) => {
  await open(page);
  await page.getByTestId("open-settings").click();
  const s = page.getByTestId("ast-settings");
  await s.getByTestId("ast-rate-model").fill("glm-5.3");
  await s.getByTestId("ast-settings-save").click();
  const saved = (await calls(page, "SetAsturlabSettings")).at(-1)?.[1];
  expect(saved?.overrides?.["glm-5.3"]).toBeUndefined();
  await expect(s).toContainText("needs an in or out rate");
});
