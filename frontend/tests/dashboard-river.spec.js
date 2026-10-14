import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

const now = Date.now();
const timeline = { s1: [
  { session: "s1", at: now - 60000, kind: "read", label: "Read a.go" },
  { session: "s1", at: now - 30000, kind: "edit", label: "Edit a.go" },
  { session: "s1", at: now - 20000, kind: "read", label: "Grep x", agent: "a1" },
] };

async function river(page, live) {
  await stubDash(page, { live, timeline });
  await page.addInitScript(() => localStorage.setItem("commander.prefs.v2", JSON.stringify({ dashMode: "river" })));
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-river")).toBeVisible();
}

test("lane with glyphs and a working tributary", async ({ page }) => {
  await river(page, [{ ...S1, crew: [{ id: "a1", type: "Explore", state: "tool", startedMs: now - 40000, endedMs: 0, background: false, depth: 1 }] }]);
  const lane = page.locator('[data-testid="river-lane"][data-key="@1"]');
  await expect(lane.getByTestId("river-glyph")).toHaveCount(2);
  await expect(lane.locator('[data-testid="river-trib"][data-agent="a1"]')).toHaveCount(1);
  await expect(lane.getByTestId("river-merge")).toHaveCount(0);
});

test("finished agent merges back; background agent is a dashed side stream", async ({ page }) => {
  await river(page, [{ ...S1, crew: [
    { id: "a1", state: "finished", startedMs: now - 40000, endedMs: now - 10000, background: false, depth: 1 },
    { id: "a2", state: "working", startedMs: now - 30000, endedMs: 0, background: true, depth: 1 },
  ] }]);
  const lane = page.locator('[data-testid="river-lane"][data-key="@1"]');
  await expect(lane.getByTestId("river-merge")).toHaveCount(1);
  await expect(lane.locator('[data-testid="river-trib"][data-agent="a2"]')).toHaveAttribute("data-background", "true");
});

test("more than three open agents braid into +N", async ({ page }) => {
  const crew = Array.from({ length: 5 }, (_, i) => ({ id: "b" + i, state: "working", startedMs: now - 10000 * (i + 1), endedMs: 0, background: false, depth: 1 }));
  await river(page, [{ ...S1, crew }]);
  await expect(page.getByTestId("river-braid")).toContainText("+5");
});

test("a permission wait shows a counting gap and Review →", async ({ page }) => {
  await river(page, [{ ...S1, state: "permission", sinceMs: now - 134000 }]);
  await expect(page.getByTestId("river-wait")).toContainText("Waiting 2m");
  await page.getByTestId("river-review").click();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["SelectSession", "@1"]);
});

test("window toggle is remembered", async ({ page }) => {
  await river(page, [S1]);
  await page.getByTestId("river-window-5").click();
  await expect(page.getByTestId("river-window-5")).toHaveAttribute("aria-pressed", "true");
  expect(await page.evaluate(() => JSON.parse(localStorage.getItem("commander.prefs.v2")).dashRiverWindow)).toBe(5);
});
