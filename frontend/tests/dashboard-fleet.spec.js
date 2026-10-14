// frontend/tests/dashboard-fleet.spec.js
import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

const PROJ = [{ key: "k1", label: "proj", root: "/Users/me/proj", latest: { code: 40000, complexity: 800, testRatio: 0.5 } }];
const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");

async function openFleet(page, opts) {
  await stubDash(page, { projects: PROJ, ...opts });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+5");
  await expect(page.getByTestId("dash-fleet")).toBeVisible();
}
const panel = (page) => page.locator('[data-testid="fleet-panel"][data-key="@1"]');

test("⌘5 and the nav button open Fleet Ops", async ({ page }) => {
  await openFleet(page, { live: [S1] });
  await expect(page.getByTestId("dash-mode-fleet")).toHaveAttribute("aria-checked", "true");
  await expect(page.getByTestId("dash-mode-switch")).toContainText("Fleet Ops");
});

test("a working Opus session: flagship, captain, encounter, beam on Edit", async ({ page }) => {
  await openFleet(page, { live: [{ ...S1, title: "fix crash" }] });
  const p = panel(page);
  await expect(p).toHaveAttribute("data-state", "engage");
  await expect(p.getByTestId("fleet-ship")).toHaveAttribute("data-ship", "opus");
  await expect(p.getByTestId("fleet-class")).toHaveText("Flagship carrier");
  await expect(p.getByTestId("fleet-name")).toHaveText(/^ACS [A-Z][a-z]+$/);
  await expect(p.getByTestId("fleet-captain")).toHaveAttribute("title", "Captain Archmage");
  await expect(p.getByTestId("fleet-encounter")).toHaveAttribute("data-kind", "swarm");
  await expect(p.getByTestId("fleet-action")).toHaveAttribute("data-action", "beam");
  await expect(p.getByTestId("fleet-shields")).toHaveAttribute("data-pips", "3");
});

test("permission → Awaiting orders, Admiral + captain glow", async ({ page }) => {
  await openFleet(page, { live: [{ ...S1, state: "permission" }] });
  const p = panel(page);
  await expect(p).toHaveAttribute("data-state", "await");
  await expect(p.getByTestId("fleet-banner")).toHaveText(/Awaiting orders, Admiral/);
  await expect(p.getByTestId("fleet-captain")).toHaveClass(/glow/);
});

test("error → damaged hull", async ({ page }) => {
  await openFleet(page, { live: [{ ...S1, state: "error" }] });
  const p = panel(page);
  await expect(p).toHaveAttribute("data-state", "damaged");
  await expect(p.getByTestId("fleet-ship").locator("> img")).toHaveAttribute("src", /ship-opus-damaged[^/]*\.webp$/);
});

test("finished unread → jump, Sector cleared and loot", async ({ page }) => {
  const progress = { commander: { level: 3 }, projects: [{ key: "k1", rank: "Station" }], live: [{ sessionID: "s1", level: 7, title: "Adept", loot: { commits: 2, lines: 340, tests: 3 } }], locked: false };
  await openFleet(page, { live: [{ ...S1, state: "finished", unread: true }], progress });
  const p = panel(page);
  await expect(p).toHaveAttribute("data-state", "jump");
  await expect(p.getByTestId("fleet-cleared")).toContainText("Sector cleared");
  await expect(p.getByTestId("fleet-loot")).toContainText("+340 lines");
});

test("five subagents → 4 craft and a wing +1", async ({ page }) => {
  const crew = ["Explore", "Plan", "code-reviewer", "general-purpose", "general-purpose"].map((type, i) => ({ id: "a" + i, type, state: "working", depth: 1, description: "t" }));
  await openFleet(page, { live: [{ ...S1, crew }] });
  const p = panel(page);
  await expect(p.getByTestId("fleet-craft")).toHaveCount(4);
  const kinds = await p.getByTestId("fleet-craft").evaluateAll((els) => els.map((e) => e.dataset.kind));
  expect(kinds).toEqual(["scout", "pathfinder", "sentinel", "tug"]);
  await expect(p.getByTestId("fleet-wing")).toHaveText(/\+1/);
});

test("unknown model flies as a Privateer", async ({ page }) => {
  await openFleet(page, { live: [{ ...S1, tool: "Bash" }], stats: { "@1": { sessionID: "s1", activity: "tool", tool: "Bash", model: "gpt-x", status: "active", cwd: "/Users/me/proj", band: "green" } } });
  const p = panel(page);
  await expect(p.getByTestId("fleet-class")).toHaveText("Privateer");
  await expect(p.getByTestId("fleet-action")).toHaveAttribute("data-action", "salvo");
});

test("tier 2 veteran: engine glow and escort", async ({ page }) => {
  const progress = { commander: { level: 3 }, projects: [{ key: "k1", rank: "Station" }], live: [{ sessionID: "s1", level: 12, tier: 2, title: "Adept" }], locked: false };
  await openFleet(page, { live: [{ ...S1, state: "idle" }], progress });
  const p = panel(page);
  await expect(p.getByTestId("fleet-veteran")).toHaveAttribute("data-vet", "glow escort");
  await expect(p.getByTestId("fleet-escort")).toHaveCount(1);
});

test("pack captain replaces the bundled portrait", async ({ page }) => {
  await page.addInitScript(() => { window.__raidPack = { dir: "/p", name: "Mine", files: { "captain-opus": "/media/raid/captain-opus-aa" }, heroines: { opus: { name: "Layla" } }, warnings: [] }; });
  await page.route("**/media/raid/**", (r) => r.fulfill({ contentType: "image/png", body: PNG }));
  await openFleet(page, { live: [S1] });
  const cap = panel(page).getByTestId("fleet-captain");
  await expect(cap).toHaveAttribute("title", "Captain Layla");
  await expect(cap.locator("img")).toHaveAttribute("src", /\/media\/raid\/captain-opus-aa$/);
});

test("still mode: nothing animates", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openFleet(page, { live: [S1] });
  const running = await page.getByTestId("dash-fleet").evaluate((el) =>
    el.getAnimations({ subtree: true }).filter((a) => a.playState === "running").length);
  expect(running).toBe(0);
});

test("jump: two jump points play once, then the ship is back on the panel", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openFleet(page, { live: [{ ...S1, state: "finished", unread: true }] });
  await expect(panel(page).getByTestId("fleet-vortex")).toHaveCount(2);
  await expect(panel(page).locator('[data-testid="fleet-vortex"][data-phase="out"]')).toHaveCount(1);
  const ship = panel(page).getByTestId("fleet-ship");
  await page.waitForTimeout(3700);
  expect(Number(await ship.evaluate((el) => getComputedStyle(el).opacity))).toBe(1);
  const running = await panel(page).evaluate((el) => el.getAnimations({ subtree: true }).filter((a) => a.playState === "running").length);
  expect(running).toBe(0);
});

test("jump in still mode: no jump points", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openFleet(page, { live: [{ ...S1, state: "finished", unread: true }] });
  await expect(panel(page).getByTestId("fleet-cleared")).toBeVisible();
  await expect(panel(page).getByTestId("fleet-vortex")).toHaveCount(0);
});

test("jump plays once per finish: leaving Fleet Ops and coming back does not replay it", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openFleet(page, { live: [{ ...S1, state: "finished", unread: true }] });
  await expect(panel(page).getByTestId("fleet-vortex")).toHaveCount(2);
  await page.keyboard.press("Meta+6");
  await expect(page.getByTestId("dash-cic")).toBeVisible();
  await page.keyboard.press("Meta+5");
  await expect(panel(page).getByTestId("fleet-cleared")).toBeVisible();
  await expect(panel(page).getByTestId("fleet-vortex")).toHaveCount(0);
});

// Minors sweep (Fleet Ops review): a pack captain that fails to load fell back to nothing.
test("a pack captain that fails to load falls back to the built-in portrait", async ({ page }) => {
  await page.addInitScript(() => { window.__raidPack = { dir: "/p", name: "Mine", files: { "captain-opus": "/media/raid/missing-captain" }, heroines: {}, warnings: [] }; });
  await page.route("**/media/raid/missing-captain", (r) => r.fulfill({ status: 404, body: "" }));
  await openFleet(page, { live: [S1] });
  const img = panel(page).getByTestId("fleet-captain").locator("img");
  await expect(img).not.toHaveAttribute("src", /missing-captain/);
  await expect.poll(() => img.evaluate((el) => el.naturalWidth)).toBeGreaterThan(0);
});

// Minors round 2 (Fleet Ops review): the spec's HUD and state polish.
test("HUD: 48 px captain and the model label", async ({ page }) => {
  await openFleet(page, { live: [S1] });
  const cap = await panel(page).getByTestId("fleet-captain").boundingBox();
  expect(Math.round(cap.width)).toBe(48);
  await expect(panel(page).getByTestId("fleet-model")).toHaveText("claude-opus-5-5");
});

test("damaged flickers, docked engines are dim", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openFleet(page, { live: [{ ...S1, state: "error" }, { ...S1, id: "s2", state: "idle" }], windows: [{ windowID: "@1", name: "a" }, { windowID: "@2", name: "b" }],
    stats: { "@1": { sessionID: "s1", model: "claude-opus-5-5", cwd: "/Users/me/proj", status: "active" }, "@2": { sessionID: "s2", model: "claude-opus-5-5", cwd: "/Users/me/proj", status: "active" } } });
  const dmg = page.locator('[data-testid="fleet-panel"][data-state="damaged"] [data-testid="fleet-ship"]');
  expect(await dmg.evaluate((el) => getComputedStyle(el).animationName)).toContain("flicker");
  const docked = page.locator('[data-testid="fleet-panel"][data-state="docked"] [data-testid="fleet-ship"]');
  expect(Number(await docked.evaluate((el) => getComputedStyle(el).opacity))).toBeLessThan(1);
});

test("+N craft shows the fighter-trio sprite; external panels carry a chip", async ({ page }) => {
  const crew = Array.from({ length: 6 }, (_, i) => ({ id: "c" + i, type: "Explore", state: "tool", startedMs: Date.now() - 1000 }));
  await openFleet(page, { live: [{ ...S1, crew }, { ...S1, id: "x1", external: true, cwd: "/Users/me/proj" }] });
  await expect(panel(page).getByTestId("fleet-wing").locator("img")).toHaveCount(1);
  await expect(page.getByTestId("fleet-external")).toHaveCount(1);
  await expect(panel(page).getByTestId("fleet-open")).toBeVisible();
});

// Minors round 2 (Fleet Ops review): still mode with several panels in every state.
test("still mode, many panels: nothing animates", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const states = ["tool", "permission", "error", "idle", "finished"];
  await openFleet(page, {
    live: states.map((state, i) => ({ ...S1, id: "s" + (i + 1), state, unread: true })),
    windows: states.map((_, i) => ({ windowID: "@" + (i + 1), name: "w" + i })),
    stats: Object.fromEntries(states.map((_, i) => ["@" + (i + 1), { sessionID: "s" + (i + 1), model: "claude-opus-5-5", cwd: "/Users/me/proj", status: "active" }])),
  });
  await expect(page.getByTestId("fleet-panel")).toHaveCount(5);
  const running = await page.getByTestId("dash-fleet").evaluate((el) =>
    el.getAnimations({ subtree: true }).filter((a) => a.playState === "running").length);
  expect(running).toBe(0);
});
