// frontend/tests/dashboard-raid-looks.spec.js
import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

const PROJ = [{ key: "k1", label: "proj", root: "/Users/me/proj", latest: { code: 40000, complexity: 800, testRatio: 0.5 } }];
const prog = (hero, unlocks = []) => ({ commander: { level: 3 }, projects: [{ key: "k1", rank: "Station", unlocks }], live: [{ sessionID: "s1", title: "Adept", ...hero }], locked: false });

async function openRaid(page, opts) {
  await stubDash(page, { projects: PROJ, ...opts });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+4");
  await expect(page.getByTestId("dash-raid")).toBeVisible();
}

test("tier 2, level 15 heroine: look, aura, halo and three gear sprites", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: prog({ level: 15, tier: 2 }) });
  const card = page.locator('[data-key="@1"]');
  const her = card.getByTestId("raid-heroine");
  await expect(her).toHaveAttribute("data-tier", "2");
  await expect(her).toHaveAttribute("data-look", /^[012]$/);
  await expect(card.getByTestId("raid-aura")).toHaveCount(1);
  await expect(card.getByTestId("raid-halo")).toHaveAttribute("data-kind", "halo"); // opus
  await expect(card.getByTestId("raid-gear")).toHaveCount(3);
  const kinds = await card.getByTestId("raid-gear").evaluateAll((els) => els.map((e) => e.dataset.kind));
  expect(kinds).toEqual(["familiar", "banner", "weapon"]);
});

test("no hero: no aura, halo or gear; the card still renders", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: { commander: { level: 0 }, projects: [], live: [], locked: false } });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-heroine")).toHaveAttribute("data-tier", "0");
  await expect(card.getByTestId("raid-aura")).toHaveCount(0);
  await expect(card.getByTestId("raid-halo")).toHaveCount(0);
  await expect(card.getByTestId("raid-gear")).toHaveCount(0);
});

test("attacking at tier 3: aura and gear hold still (3-effect budget)", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openRaid(page, { live: [S1], progress: prog({ level: 20, tier: 3 }) });
  const card = page.locator('[data-key="@1"]');
  await expect(card).toHaveAttribute("data-state", "attack");
  await expect(card.getByTestId("raid-aura")).toHaveAttribute("data-pulse", "false");
  for (const g of await card.getByTestId("raid-gear").all()) await expect(g).toHaveAttribute("data-drift", "false");
});

test("resting at level 8 with motion: gear drifts; still mode: nothing runs", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: prog({ level: 8, tier: 1 }) });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-gear").first()).toHaveAttribute("data-drift", "true");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(card.getByTestId("raid-gear").first()).toHaveAttribute("data-drift", "false");
  const running = await card.evaluate((el) => el.getAnimations({ subtree: true }).filter((a) => a.playState === "running").length);
  expect(running).toBe(0);
});

test("a legend image that fails to load falls back to the base pose", async ({ page }) => {
  await page.route(/opus-legend-[a-z]+[^/]*\.webp/, (r) => r.abort());
  await page.route(/opus-v[12]-[a-z]+[^/]*\.webp/, (r) => r.abort());
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: prog({ level: 20, tier: 3 }) });
  const img = page.locator('[data-key="@1"] [data-testid="raid-heroine"] > img');
  await expect(img).toHaveAttribute("src", /opus-stance[^/]*\.webp/);
  await expect(page.locator('[data-key="@1"] [data-testid="raid-heroine"] > [data-testid="raid-sil"]')).toHaveCount(0);
});

// Two windows → standard density (one session is "hero" density, where the radar is always shown).
test("radar shows on hover with the four values", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, state: "idle" }], windows: [{ windowID: "@1", name: "proj" }, { windowID: "@2", name: "p2" }],
    progress: prog({ level: 4, tier: 0, mix: { Blade: 10, Scholar: 5 } }) });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-radar")).toBeHidden();
  await card.hover();
  await expect(card.getByTestId("raid-radar")).toBeVisible();
  await expect(card.getByTestId("raid-radar")).toHaveAttribute("aria-label", "Blade 100%, Scholar 50%, Engineer 0%, Summoner 0%");
});

test("badges: this session's unlocks, newest first, then +N", async ({ page }) => {
  const unlocks = ["first_blood", "night_owl", "flawless", "big_brain", "speedrun"].map((id, i) => ({ id, at: i, sessionID: "s1" }))
    .concat([{ id: "marathon", at: 99, sessionID: "other" }]);
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: prog({ level: 4, tier: 0 }, unlocks) });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-badge")).toHaveCount(4);
  await expect(card.getByTestId("raid-badge").first()).toHaveAttribute("title", "Speedrun");
  await expect(card.getByTestId("raid-badge-more")).toHaveText("+1");
});

test("pouch dims when quota is at 85 %", async ({ page }) => {
  await page.addInitScript(() => { window.__usage = { five_hour: { utilization: 85 } }; });
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: prog({ level: 4, tier: 0 }) });
  await expect(page.getByTestId("raid-pouch")).toHaveAttribute("data-dim", "true");
  await expect(page.getByTestId("raid-pouch")).toHaveAttribute("title", "Quota used 85%");
});

test("pouch hidden when usage cannot be read", async ({ page }) => {
  await page.addInitScript(() => { window.__usage = null; });
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: prog({ level: 4, tier: 0 }) });
  await expect(page.getByTestId("raid-card")).toHaveCount(1);
  await expect(page.getByTestId("raid-pouch")).toHaveCount(0);
});

test("Quest Clear shows the loot line instead of turns", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, state: "finished", unread: true }], progress: prog({ level: 4, tier: 0, loot: { commits: 2, lines: 250, tests: 7 } }) });
  const loot = page.locator('[data-key="@1"]').getByTestId("raid-loot");
  await expect(loot).toContainText("+250 lines · 2 commits · 7 green tests");
  await expect(loot.locator(".gem")).toHaveCount(3);
});

test("Quest Clear without loot keeps the 6a turns line", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, state: "finished", unread: true }], progress: prog({ level: 4, tier: 0 }) });
  await expect(page.locator('[data-key="@1"]').getByTestId("raid-clear")).toContainText("3 turns");
  await expect(page.locator('[data-key="@1"]').getByTestId("raid-loot")).toHaveCount(0);
});

test("compact density: badges and pouch only — no gear, radar or halo", async ({ page }) => {
  await page.addInitScript(() => { window.__usage = { five_hour: { utilization: 10 } }; });
  const windows = Array.from({ length: 15 }, (_, i) => ({ windowID: "@" + (i + 1), name: "p" + i }));
  const unlocks = [{ id: "first_blood", at: 1, sessionID: "s1" }];
  await openRaid(page, { windows, live: [{ ...S1, state: "idle" }], progress: prog({ level: 20, tier: 3, mix: { Blade: 1 } }, unlocks) });
  const card = page.locator('[data-key="@1"]');
  await card.hover();
  await expect(card.getByTestId("raid-badge")).toHaveCount(1);
  await expect(card.getByTestId("raid-pouch")).toHaveCount(1);
  await expect(card.getByTestId("raid-gear")).toHaveCount(0);
  await expect(card.getByTestId("raid-halo")).toHaveCount(0);
  await expect(card.getByTestId("raid-radar")).toHaveCount(0);
});

// Review fix: a failing look must not be re-requested on every clock tick (the image blinked once a second).
test("a failing look image is tried once, not on every tick", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" }); // 1 s ticks
  let hits = 0;
  await page.route(/opus-legend-[a-z]+[^/]*\.webp/, (r) => { hits++; r.abort(); });
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: prog({ level: 20, tier: 3 }) });
  await expect(page.locator('[data-key="@1"] [data-testid="raid-heroine"] > img')).toBeVisible();
  await page.waitForTimeout(3500);
  expect(hits).toBe(1);
});

// Review fix: a badge earned while Raid is open appears without leaving the dashboard.
test("dash:achievement adds the badge live", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, state: "idle" }], progress: prog({ level: 4, tier: 0 }) });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-badge")).toHaveCount(0);
  await page.evaluate(() => window.__handlers["dash:achievement"].forEach((cb) => cb({ id: "first_blood", name: "First Blood", projectKey: "k1", sessionID: "s1", at: 5 })));
  await expect(card.getByTestId("raid-badge")).toHaveAttribute("title", "First Blood");
});
