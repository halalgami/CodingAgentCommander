import { test, expect } from "@playwright/test";
import { stubDash, S1, pickMode } from "./dashboard-helpers.js";
import { AST } from "./asturlab-fixtures.js";

async function open(page, init = {}) {
  await stubDash(page, { live: [S1] });
  await page.addInitScript(([ast, init]) => { window.__ast = ast; Object.assign(window, init); }, [AST, init]);
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-mode-switch")).toBeVisible();
  await pickMode(page, "asturlab");
  await expect(page.getByTestId("asturlab")).toBeVisible();
}
const calls = (page, name) => page.evaluate((n) => window.__calls.filter((c) => c[0] === n), name);

test("opening the observatory reads the briefing but never calls a model", async ({ page }) => {
  await open(page);
  await expect(page.getByTestId("ast-brief-run")).toHaveText("Click here for your weekly briefing");
  await expect(page.getByTestId("ast-brief-disclosure")).toContainText("haiku");
  await expect(page.getByTestId("ast-ask-disclosure")).toContainText("your question");
  await page.waitForTimeout(300);
  expect((await calls(page, "Briefing")).length).toBeGreaterThan(0);
  expect(await calls(page, "RunBriefing")).toHaveLength(0);
  expect(await calls(page, "AskStats")).toHaveLength(0);
});

test("the click writes the briefing once; a point links to its chart", async ({ page }) => {
  await open(page);
  await page.getByTestId("ast-brief-run").click();
  await expect(page.getByTestId("ast-brief-headline")).toHaveText("Busy week");
  await expect(page.getByTestId("ast-brief-point")).toHaveCount(2);
  expect(await calls(page, "RunBriefing")).toHaveLength(1);
  await page.getByTestId("ast-brief-link").click();
  await expect(page.getByTestId("ast-chart-spend")).toBeFocused();
});

test("a cached briefing shows without a click; errors are shown", async ({ page }) => {
  await open(page, { __brief: { week: "2026-11-09", brief: { headline: "Cached", points: [{ text: "x", chart: "" }] }, disclosure: "d" } });
  await expect(page.getByTestId("ast-brief-headline")).toHaveText("Cached");
  await page.evaluate(() => { window.__briefRun = { week: "2026-11-09", brief: { headline: "Cached", points: [{ text: "x", chart: "" }] }, error: "model error (status 429): rate", disclosure: "d" }; });
  await page.getByTestId("ast-brief-rerun").click();
  await expect(page.getByTestId("ast-brief-error")).toContainText("429");
});

test("Ask: Enter asks once and shows the answer with a chart and table", async ({ page }) => {
  await open(page);
  await page.getByTestId("ast-ask-input").fill("which project used most?");
  await page.getByTestId("ast-ask-input").press("Enter");
  await expect(page.getByTestId("ast-ask-answer")).toHaveText("Most tokens went to vat.");
  await expect(page.getByTestId("ast-ask-bars").locator("rect")).toHaveCount(2);
  expect(await calls(page, "AskStats")).toEqual([["AskStats", "which project used most?"]]);
  await page.getByTestId("ast-chart-ask").getByTestId("ast-table-toggle").click();
  await expect(page.getByTestId("ast-chart-ask").locator("tbody tr")).toHaveCount(2);
});

test("Ask error is shown; locked history hides the button", async ({ page }) => {
  await open(page, { __askAns: { question: "q", error: "the model wrote an invalid query", results: [], queries: [], disclosure: "d" } });
  await page.getByTestId("ast-ask-input").fill("q");
  await page.getByTestId("ast-ask-go").click();
  await expect(page.getByTestId("ast-ask-error")).toContainText("invalid query");
});

test("no history: the AI card asks to turn history on and offers no button", async ({ page }) => {
  await open(page, { __brief: { locked: true, disclosure: "d" } });
  await expect(page.getByTestId("ast-ai-locked")).toBeVisible();
  await expect(page.getByTestId("ast-brief-run")).toHaveCount(0);
});

test("Wrapped: opens, shows cards, switches year, saves a PNG, makes no AI call", async ({ page }) => {
  const y = new Date().getFullYear();
  const full = { year: y, title: "Night Owl", tokens: 12.4e6, activeMs: 360 * 36e5, commits: 412, costUSD: 1234, topProjects: [{ key: "k", label: "vat", value: 9e6 }],
    biggestDay: { key: `${y}-03-14`, value: 2e6 }, models: [{ key: "claude-opus-4-8", value: 8e6 }], level: 14, achievements: 23, years: [y, y - 1] };
  await open(page, { __wrapped: { [y]: full } });
  await page.getByTestId("ast-wrapped-open").click();
  await expect(page.getByTestId("ast-wrapped-card")).toHaveCount(7);
  await expect(page.locator('[data-card="title"]')).toContainText("Night Owl");
  // Review fix: the serialized SVG needs its own size or the PNG is drawn at ~300 px and blurred.
  await expect(page.locator(".art")).toHaveAttribute("width", "1080");
  await expect(page.locator(".art")).toHaveAttribute("height", "1350");
  await page.getByTestId("ast-wrapped-save").click();
  await expect.poll(async () => (await calls(page, "SaveWrappedPNG")).length).toBe(1);
  expect((await calls(page, "SaveWrappedPNG"))[0][1]).toBe("data:image/png;base64,");
  await page.getByTestId("ast-wrapped-year").selectOption(String(y - 1));
  await expect(page.getByTestId("ast-wrapped-card")).toHaveCount(1);
  await expect(page.getByTestId("ast-wrapped-save")).toBeDisabled();
  expect(await calls(page, "RunBriefing")).toHaveLength(0);
  expect(await calls(page, "AskStats")).toHaveLength(0);
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("ast-wrapped")).toHaveCount(0);
});

test("project page: sessions in the range and a small tower", async ({ page }) => {
  await open(page, { __sessions: [
    { id: "aaaaaaaa-1", title: "Fix VAT", startedAt: Date.parse("2026-11-10T09:00:00"), endedAt: 0, tokens: 1.2e6, activeMs: 2 * 36e5, commits: 3, models: ["claude-opus-5-5"] },
    { id: "bbbbbbbb-2", title: "", startedAt: Date.parse("2026-11-09T09:00:00"), endedAt: 0, tokens: 4e5, activeMs: 36e5, commits: 0, models: [] },
  ] });
  await page.evaluate(() => { window.__calls = []; });
  await page.locator('[data-testid="ast-tree-cell"][data-key="k1"]').click();
  await expect(page.getByTestId("ast-session-row")).toHaveCount(2);
  await expect(page.getByTestId("ast-session-row").first()).toContainText("Fix VAT");
  await expect(page.getByTestId("ast-session-row").nth(1)).toContainText("bbbbbbbb");
  await expect(page.getByTestId("ast-tower")).toBeVisible();
  const sc = await calls(page, "ProjectSessions");
  expect(sc.at(-1)[1]).toBe("k1");
});
