import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

const ext = { id: "s9", title: "Outside job", state: "thinking", cwd: "/elsewhere", external: true, model: "sonnet", crew: [], sinceMs: Date.now() - 1000 };

test("bands order sessions by attention and move when state changes", async ({ page }) => {
  await stubDash(page, { live: [S1, ext] });
  await page.goto("/?nointro");
  const working = page.getByTestId("dash-band-working");
  await expect(working.getByTestId("dash-tile")).toHaveCount(2);
  await page.evaluate(() => window.__handlers["dash:tick"].forEach((cb) => cb({
    sessions: [{ id: "s1", title: "Fix VAT", state: "permission", tool: "Bash", target: "rm -rf build", cwd: "/Users/me/proj", crew: [], sinceMs: Date.now() - 3000 }], events: [] })));
  await expect(page.getByTestId("dash-band-needs").getByTestId("tile-action")).toHaveText("Review →");
});

test("Review → opens the terminal; Details on an external session opens nothing", async ({ page }) => {
  await stubDash(page, { live: [{ ...S1, state: "permission" }, ext] });
  await page.goto("/?nointro");
  await page.locator('[data-key="ext:s9"]').getByTestId("tile-action").click();
  await expect(page.locator('[data-key="ext:s9"]').getByTestId("tile-details")).toContainText("/elsewhere");
  await expect(page.locator('[data-key="ext:s9"]').getByTestId("external-badge")).toBeVisible();
  expect(await page.evaluate(() => window.__calls.filter((c) => c[0] === "SelectSession").length)).toBe(0);
  await page.locator('[data-key="@1"]').getByTestId("tile-action").click();
  await expect(page.getByTestId("dash-back")).toBeVisible();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["SelectSession", "@1"]);
});

test("crew strip: chips, +N and the delegating activity text", async ({ page }) => {
  const crew = Array.from({ length: 30 }, (_, i) => ({ id: "a" + i, type: "Explore", description: "Look " + i, state: "working", depth: 1, startedMs: Date.now() - 1000 }));
  await stubDash(page, { live: [{ ...S1, tool: "Agent", target: "fan out", crew, delegating: 30 }] });
  await page.goto("/?nointro");
  const tile = page.locator('[data-key="@1"]');
  await expect(tile.getByTestId("crew-chip")).toHaveCount(6);
  await expect(tile.getByTestId("crew-more")).toHaveText("+24");
  await expect(tile.getByTestId("tile-activity")).toHaveText("Waiting on 30 agents");
});

test("rank and hero chips from progress", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await expect(page.getByTestId("tile-rank")).toHaveText("Station");
  await expect(page.getByTestId("tile-hero")).toHaveText("L7 Adept");
});

test("script in a title renders as text", async ({ page }) => {
  const evil = `<img src=x onerror="window.__xss=1">`;
  await stubDash(page, { live: [{ ...S1, title: evil, crew: [{ id: "a1", description: evil, state: "working", depth: 1 }] }] });
  await page.goto("/?nointro");
  await expect(page.getByTestId("tile-title")).toHaveText(evil);
  expect(await page.evaluate(() => window.__xss)).toBeUndefined();
  expect(await page.locator('[data-testid="dash-home"] img').count()).toBe(0);
});

test("a removed session leaves the wall", async ({ page }) => {
  await stubDash(page, { live: [S1, ext] });
  await page.goto("/?nointro");
  await expect(page.locator('[data-key="ext:s9"]')).toHaveCount(1);
  await page.evaluate(() => window.__handlers["dash:tick"].forEach((cb) => cb({ sessions: [], events: [], removed: ["s9"] })));
  await expect(page.locator('[data-key="ext:s9"]')).toHaveCount(0);
});

test("cards lead with the project; an untitled session shows what it is doing", async ({ page }) => {
  await stubDash(page, { live: [{ ...S1, title: "proj" }] }); // a title that only repeats the project
  await page.goto("/?nointro");
  await expect(page.getByTestId("tile-project")).toHaveText("proj");
  await expect(page.getByTestId("tile-title")).toContainText("app.go");
  await expect(page.getByTestId("tile-title")).not.toHaveText("proj");
});
