// frontend/tests/dashboard-raid.spec.js
import { test, expect } from "@playwright/test";
import { stubDash, S1, pickMode } from "./dashboard-helpers.js";

const PROJ = [{ key: "k1", label: "proj", root: "/Users/me/proj", latest: { code: 40000, complexity: 800, testRatio: 0.5 } }];
const ext = { id: "s9", title: "Outside job", state: "thinking", cwd: "/elsewhere", external: true, model: "sonnet", crew: [], sinceMs: Date.now() - 1000 };
const tick = (page, sessions, events = []) => page.evaluate(([s, e]) => window.__handlers["dash:tick"].forEach((cb) => cb({ sessions: s, events: e })), [sessions, events]);

async function openRaid(page, opts) {
  await stubDash(page, { projects: PROJ, ...opts });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+4");
  await expect(page.getByTestId("dash-raid")).toBeVisible();
}

test("⌘4 and the switcher open Raid; cards follow attention order", async ({ page }) => {
  await openRaid(page, { live: [S1, { ...ext, state: "permission" }] });
  await expect(page.getByTestId("dash-mode-raid")).toHaveAttribute("aria-checked", "true");
  const keys = await page.getByTestId("raid-card").evaluateAll((els) => els.map((e) => e.dataset.key));
  expect(keys).toEqual(["ext:s9", "@1"]);
  await pickMode(page, "triage");
  await pickMode(page, "raid");
  await expect(page.getByTestId("raid-card")).toHaveCount(2);
});

test("a working session: opus Archmage attacks a level-scaled raid boss", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, title: "Fix VAT rounding" }] });
  const card = page.locator('[data-testid="raid-card"][data-key="@1"]');
  await expect(card).toHaveAttribute("data-state", "attack");
  await expect(card.getByTestId("raid-heroine")).toHaveAttribute("data-hero", "opus");
  await expect(card.getByTestId("raid-heroine")).toHaveAttribute("data-pose", "attack");
  await expect(card.getByTestId("raid-boss")).toHaveAttribute("data-kind", "hydra");
  await expect(card.getByTestId("raid-boss-name")).toHaveText("Bug Hydra · Lv 39"); // round(4·log2(1 + 40000/100) + 800/200) = round(38.59)
  await expect(card.getByTestId("raid-move")).toHaveText("Refactor Strike · app.go");
  await expect(card.getByTestId("raid-project")).toHaveText("proj");
  await expect(card.getByTestId("raid-task")).toHaveText("Fix VAT rounding");
  await expect(card.getByTestId("raid-hero")).toHaveText("Lv 7 Adept");
});

test("permission: command window with patience, REVIEW opens the terminal", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, state: "permission", sinceMs: Date.now() - 3 * 60000 }] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-orders")).toContainText("Your orders, Commander?");
  await expect(card.getByTestId("raid-patience")).toHaveAttribute("data-stage", "sitting");
  await card.getByTestId("raid-review").click();
  await expect(page.getByTestId("dash-back")).toBeVisible();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["SelectSession", "@1"]);
});

test("error: K.O. banner and Revive opens the terminal", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, state: "error" }] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-ko")).toContainText("K.O.");
  await expect(card.getByTestId("raid-heroine")).toHaveAttribute("data-pose", "hurt");
  await card.getByTestId("raid-revive").click();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["SelectSession", "@1"]);
});

test("finished and unread: QUEST CLEAR with the loot line; read: Resting, no boss", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" }); // the animated path
  await openRaid(page, { live: [{ ...S1, state: "finished", unread: true }] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-clear")).toContainText("QUEST CLEAR");
  await expect(card.getByTestId("raid-clear")).toContainText("3 turns");
  await expect(card.getByTestId("raid-boss")).toHaveAttribute("data-boss", "dissolve");
  await tick(page, [{ ...S1, state: "finished", unread: false }]);
  await expect(card.getByTestId("raid-rest")).toHaveText("Resting");
  await expect(card.getByTestId("raid-boss")).toHaveCount(0);
});

test("hostile title and huge target stay plain clipped text", async ({ page }) => {
  const evil = '<img src=x onerror="window.__pwned=1">';
  await openRaid(page, { live: [{ ...S1, title: evil, target: "y".repeat(500) }] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-task")).toHaveText(evil);
  expect((await card.getByTestId("raid-move").textContent()).length).toBeLessThanOrEqual(80 + "Refactor Strike · ".length);
  expect(await page.evaluate(() => window.__pwned)).toBeUndefined();
});

test("external session: battle shown, no REVIEW/Revive/Open", async ({ page }) => {
  await openRaid(page, { live: [{ ...ext, state: "permission" }], windows: [] });
  const card = page.locator('[data-key="ext:s9"]');
  await expect(card.getByTestId("raid-orders")).toBeVisible();
  await expect(card.getByTestId("raid-external")).toBeVisible();
  await expect(card.getByTestId("raid-review")).toHaveCount(0);
  await expect(card.getByTestId("raid-open")).toHaveCount(0);
});

test("no project metrics: Lv 1 beast, Mercenary for an unknown model", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, cwd: "/nowhere" }], stats: { "@1": { sessionID: "s1", model: "gpt-9", cwd: "/nowhere" } } });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-boss-name")).toContainText("· Lv 1");
  await expect(card.getByTestId("raid-heroine")).toHaveAttribute("data-merc", "true");
  await expect(card).not.toContainText("NaN");
  await expect(card).not.toContainText("undefined");
});

test("crew are allies: kinds, background away, +N past six", async ({ page }) => {
  const crew = [
    { id: "a0", type: "Explore", model: "haiku", state: "working", depth: 1 },
    { id: "a1", type: "Plan", state: "working", depth: 1, background: true },
    ...Array.from({ length: 8 }, (_, i) => ({ id: "k" + i, type: "general-purpose", state: "working", depth: 1 })),
  ];
  await openRaid(page, { live: [{ ...S1, crew, tool: "Agent", delegating: 10 }] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-ally")).toHaveCount(6);
  await expect(card.getByTestId("raid-more")).toHaveText("+4");
  await expect(card.getByTestId("raid-ally").first()).toHaveAttribute("data-kind", "fox");
  await expect(card.locator(".ally.away")).toHaveCount(1);
  await expect(card.locator('[data-testid^="raid-minion"]')).toHaveCount(6);
});

test("hotspot target makes an elite; long work enrages; armour and raid pips", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, sinceMs: Date.now() - 11 * 60000 }],
    stats: { "@1": { sessionID: "s1", model: "opus", cwd: "/Users/me/proj", hotspot: true, contextTokens: 170000, contextWindow: 200000, turns: 3 } } });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-elite")).toHaveText("Elite · app.go");
  await expect(card.getByTestId("raid-enraged")).toBeVisible();
  await expect(card.getByTestId("raid-armour")).toHaveAttribute("data-pips", "3");
  await expect(card.getByTestId("raid-pips")).toHaveAttribute("data-pips", "2");
  await expect(card.getByTestId("raid-bag")).toHaveText("Loot 85% · Overburdened");
});

test("a small project is a slime pack, not a boss", async ({ page }) => {
  await openRaid(page, { live: [S1], projects: [{ key: "k1", label: "proj", root: "/Users/me/proj", latest: { code: 900, complexity: 10, testRatio: 0 } }] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.locator('[data-testid^="raid-slime"]')).toHaveCount(3);
  await expect(card.getByTestId("raid-armour")).toHaveCount(0);
});

test("HP drains per hit; the damage number shows once per new hit, not per tick", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" }); // the animated path
  const now = Date.now();
  const tl = { s1: [{ session: "s1", at: now - 3000, kind: "edit" }, { session: "s1", at: now - 2000, kind: "edit" }] };
  await openRaid(page, { live: [S1], timeline: tl });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-hp")).toHaveAttribute("data-hp", "86");
  await expect(card.getByTestId("raid-dmg")).toHaveCount(1);
  const id0 = await card.getByTestId("raid-dmg").evaluate((el) => (el.__mark = 1));
  await tick(page, [{ ...S1, sinceMs: now - 1000 }], []);
  expect(await card.getByTestId("raid-dmg").evaluate((el) => el.__mark)).toBe(id0); // same node: not replayed
  await tick(page, [S1], [{ session: "s1", at: Date.now(), kind: "shell" }]);
  await expect(card.getByTestId("raid-hp")).toHaveAttribute("data-hp", "79");
  expect(await card.getByTestId("raid-dmg").evaluate((el) => el.__mark)).toBeUndefined(); // new node: new hit
});

test("compaction: inn, then refreshed", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, compactedMs: Date.now() - 10000 }] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-inn")).toContainText("Resting at the inn");
  await tick(page, [{ ...S1, compactedMs: Date.now() - 90000 }]);
  await expect(card.getByTestId("raid-inn")).toHaveCount(0);
  await expect(card.getByTestId("raid-refreshed")).toBeVisible();
});

test("a missing image becomes a silhouette, not a broken icon", async ({ page }) => {
  await page.route(/opus-(v\d-|legend-)?attack[^/]*\.webp/, (r) => r.fulfill({ status: 404, body: "" })); // every look of the pose
  await openRaid(page, { live: [S1] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-heroine").locator('> [data-testid="raid-sil"]')).toHaveCount(1);
  await expect(card.getByTestId("raid-heroine").locator("> img")).toHaveCount(0);
});

test("still mode: no running animations on the raid", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openRaid(page, { live: [{ ...S1, state: "permission" }] });
  await expect(page.getByTestId("dash-home")).toHaveClass(/still/);
  const running = await page.evaluate(() => document.getAnimations().filter((a) => a.playState === "running").length);
  expect(running).toBe(0);
});

// Review fix: still mode removes animations, so one-shot effects must not be
// rendered at all (a frozen "−7" and flash), and a cleared boss is simply gone.
test("still mode: no frozen damage number or flash; a cleared boss is gone", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const now = Date.now();
  await openRaid(page, { live: [S1], timeline: { s1: [{ session: "s1", at: now - 2000, kind: "edit" }] } });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-hp")).toHaveAttribute("data-hp", "93");
  await expect(card.getByTestId("raid-dmg")).toHaveCount(0);
  await expect(card.locator(".flash")).toHaveCount(0);
  await tick(page, [{ ...S1, state: "finished", unread: true }]);
  await expect(card.getByTestId("raid-clear")).toBeVisible();
  await expect(card.getByTestId("raid-boss")).toHaveCount(0);
});

// Review fix (spec §7, R3): raid animations touch transform and opacity only.
test("raid animations animate only transform and opacity", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  const now = Date.now();
  await openRaid(page, { live: [{ ...S1, state: "thinking" }, { ...S1, id: "s2", state: "tool" }],
    windows: [{ windowID: "@1", name: "proj" }, { windowID: "@2", name: "proj" }],
    stats: { "@1": { sessionID: "s1", model: "opus", cwd: "/Users/me/proj" }, "@2": { sessionID: "s2", model: "haiku", cwd: "/Users/me/proj" } },
    timeline: { s2: [{ session: "s2", at: now - 500, kind: "edit" }] } });
  await expect(page.getByTestId("dash-home")).not.toHaveClass(/still/);
  await expect(page.getByTestId("raid-card")).toHaveCount(2);
  const props = await page.evaluate(() => [...new Set(document.getAnimations()
    .filter((a) => a.effect?.target?.closest?.('[data-testid="dash-raid"]'))
    .flatMap((a) => a.effect.getKeyframes().flatMap((k) => Object.keys(k))))]
    .filter((k) => !["offset", "easing", "composite", "computedOffset"].includes(k)));
  expect(props.length).toBeGreaterThan(0);
  expect(props.filter((p) => p !== "transform" && p !== "opacity")).toEqual([]);
});

// Minors sweep (phase 6a review): switching back to Raid replayed an old hit's damage number.
test("an old hit does not replay its damage number when Raid opens", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  const now = Date.now();
  await openRaid(page, { live: [S1], timeline: { s1: [{ session: "s1", at: now - 10_000, kind: "edit" }] } });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-hp")).toHaveAttribute("data-hp", "93");
  await expect(card.getByTestId("raid-dmg")).toHaveCount(0);
});

// Minors round 2 (phase 6a review): an enraged slime pack had no rage tint.
test("an enraged slime pack is tinted like an enraged boss", async ({ page }) => {
  await openRaid(page, { live: [{ ...S1, sinceMs: Date.now() - 11 * 60000 }], projects: [{ key: "k1", label: "proj", root: "/Users/me/proj", latest: { code: 900, complexity: 10, testRatio: 0 } }] });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-enraged")).toBeVisible();
  const f = await card.locator('[data-testid^="raid-slime"]').first().evaluate((el) => getComputedStyle(el).filter);
  expect(f).toContain("sepia");
});

// Minors round 2 (phase 6a review): compact cards (15+ sessions) hid HP and the
// elite mark, and the orders window covered the heroine.
test("compact cards keep HP and the elite mark; the orders window stays small", async ({ page }) => {
  const windows = Array.from({ length: 15 }, (_, i) => ({ windowID: "@" + (i + 1), name: "p" + i }));
  await openRaid(page, { windows, live: [{ ...S1, state: "permission" }],
    stats: { "@1": { sessionID: "s1", model: "opus", cwd: "/Users/me/proj", hotspot: true, contextTokens: 1000, contextWindow: 200000, turns: 3 } } });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-hp")).toHaveCount(1);
  await expect(card.getByTestId("raid-elite")).toHaveCount(1);
  const [win, stage] = await Promise.all([card.getByTestId("raid-orders").boundingBox(), card.locator(".stage").boundingBox()]);
  expect(win.height).toBeLessThan(stage.height * 0.45);
});

// Minors round 2: a hit arriving while Raid is open shows its damage number even
// when its transcript timestamp lags the app clock by more than 5 s.
test("a new hit with a lagging timestamp still shows its damage number", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openRaid(page, { live: [S1], timeline: {} });
  const card = page.locator('[data-key="@1"]');
  await expect(card.getByTestId("raid-dmg")).toHaveCount(0);
  await tick(page, [S1], [{ session: "s1", at: Date.now() - 20_000, kind: "edit" }]);
  await expect(card.getByTestId("raid-dmg")).toHaveCount(1);
});

// Final review: a card returning to attack (tool → thinking → tool) with no new
// hit replayed the old damage number. A hit shows for 5 s from when it was seen.
test("returning to attack without a new hit does not replay the damage number", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openRaid(page, { live: [S1], timeline: {} });
  const card = page.locator('[data-key="@1"]');
  await tick(page, [S1], [{ session: "s1", at: Date.now(), kind: "edit" }]);
  await expect(card.getByTestId("raid-dmg")).toHaveCount(1);
  await tick(page, [{ ...S1, state: "thinking" }]);
  await page.waitForTimeout(6000);
  await tick(page, [{ ...S1, state: "tool" }]);
  await page.waitForTimeout(300);
  await expect(card.getByTestId("raid-dmg")).toHaveCount(0);
});
