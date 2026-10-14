// frontend/tests/dashboard-sector.spec.js
import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

const PROJ = [{ key: "k1", label: "proj", root: "/Users/me/proj" }, { key: "k2", label: "zeta", root: "/Users/me/zeta" }, { key: "k3", label: "old", root: "/Users/me/old" }];
const PROG = { commander: { level: 3 }, locked: false, live: [], projects: [
  { key: "k1", label: "proj", hours: 60, rank: "Station" }, { key: "k2", label: "zeta", hours: 3, rank: "Outpost" },
  { key: "k3", label: "old", hours: 200, rank: "Citadel" }] };
const W = (id) => ({ windowID: "@" + id, name: "w" + id });
const st = (id, cwd, model = "claude-opus-5-5") => ({ sessionID: "s" + id, model, cwd, status: "active", band: "green" });
const fleet = (states, cwd = () => "/Users/me/proj") => ({
  live: states.map((state, i) => ({ ...S1, id: "s" + (i + 1), state, cwd: cwd(i), title: "task " + (i + 1), sinceMs: Date.now() - (i + 1) * 60000 })),
  windows: states.map((_, i) => W(i + 1)),
  stats: Object.fromEntries(states.map((_, i) => ["@" + (i + 1), st(i + 1, cwd(i))])),
});
const sys = (page, key) => page.locator(`[data-testid="sector-system"][data-key="${key}"]`);

async function openSector(page, opts = {}) {
  await stubDash(page, { projects: PROJ, progress: PROG, ...opts });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+7");
  await expect(page.getByTestId("dash-sector")).toBeVisible();
}

// Owner, 2026-10-14: only projects with a live session are systems (as Al Modon).
test("⌘7 opens Sector Map: one system per active project, station by rank, live ship docked", async ({ page }) => {
  await openSector(page, fleet(["tool", "idle"], (i) => (i ? "/Users/me/old" : "/Users/me/proj")));
  await expect(page.getByTestId("dash-mode-sector")).toHaveAttribute("aria-checked", "true");
  await expect(page.getByTestId("dash-mode-switch")).toContainText("Sector Map");
  await expect(page.getByTestId("sector-system")).toHaveCount(2);
  await expect(sys(page, "k2")).toHaveCount(0);
  await expect(sys(page, "k1")).toHaveAttribute("data-rank", "Station");
  await expect(sys(page, "k1").getByTestId("sector-station")).toHaveAttribute("data-art", "station-2");
  await expect(sys(page, "k3").getByTestId("sector-station")).toHaveAttribute("data-art", "station-3");
  await expect(sys(page, "k1").getByTestId("sector-ship")).toHaveCount(1);
  await expect(sys(page, "k1").getByTestId("sector-ship")).toHaveAttribute("data-state", "working");
  await expect(page.getByTestId("sector-hours")).toContainText("260 h invested across 2 active systems");
  await expect(page.getByTestId("sector-count-engaged")).toHaveText("1");
});

test("waiting session: chip in the strip, amber ring and callsign; working ship has no callsign", async ({ page }) => {
  await openSector(page, fleet(["permission", "tool"]));
  await expect(page.getByTestId("sector-chip")).toHaveCount(1);
  await expect(page.getByTestId("sector-chip")).toContainText(/· proj · awaiting \dm/);
  await expect(page.getByTestId("sector-await")).toHaveCount(1);
  await expect(page.getByTestId("sector-callsign")).toHaveCount(1);
  await expect(page.locator('[data-testid="sector-ship"][data-state="working"] [data-testid="sector-callsign"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="sector-ship"][data-state="working"] [data-testid="sector-enemy"]')).toHaveCount(1);
  await expect(page.getByTestId("dash-sector").getByRole("button", { name: /approve/i })).toHaveCount(0);
});

test("chip click opens the session", async ({ page }) => {
  await openSector(page, fleet(["permission"]));
  await page.getByTestId("sector-chip").click();
  await expect.poll(() => page.evaluate(() => window.__calls.filter((c) => c[0] === "SelectSession").length)).toBeGreaterThan(0);
});

test("error ship: damaged hull and a 'hull breach' chip", async ({ page }) => {
  await openSector(page, fleet(["error"]));
  await expect(page.getByTestId("sector-ship")).toHaveAttribute("data-damaged", "true");
  await expect(page.getByTestId("sector-chip")).toContainText("hull breach");
});

test("more than 4 ships at one station → 4 drawn and +N", async ({ page }) => {
  await openSector(page, fleet(Array(6).fill("tool")));
  await expect(sys(page, "k1").getByTestId("sector-ship")).toHaveCount(4);
  await expect(sys(page, "k1").getByTestId("sector-more")).toHaveText("+2");
});

test("12 projects: system labels never overlap", async ({ page }) => {
  const projects = Array.from({ length: 12 }, (_, i) => ({ key: "p" + i, label: "project-number-" + i, root: "/r/p" + i }));
  const progress = { ...PROG, projects: projects.map((p, i) => ({ key: p.key, label: p.label, hours: 5 + i * 40, rank: ["Outpost", "Settlement", "Station", "Citadel", "Capital"][i % 5] })) };
  await openSector(page, { projects, progress, ...fleet(Array(12).fill("tool"), (i) => "/r/p" + i) });
  await expect(page.getByTestId("sector-system")).toHaveCount(12);
  const boxes = await page.locator('[data-testid="sector-label"], [data-testid="sector-name-only"]').evaluateAll((els) => els.map((e) => { const r = e.getBoundingClientRect(); return [r.left, r.top, r.right, r.bottom]; }));
  expect(boxes.length).toBeGreaterThan(0);
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
    const [a, b] = [boxes[i], boxes[j]];
    expect(a[2] <= b[0] + 1 || b[2] <= a[0] + 1 || a[3] <= b[1] + 1 || b[3] <= a[1] + 1).toBe(true);
  }
});

test("ledger locked: hint, active projects as Outposts", async ({ page }) => {
  await openSector(page, { progress: { locked: true, projects: [], live: [] }, ...fleet(["tool"]) });
  await expect(page.getByTestId("sector-hint")).toContainText("Enable the ledger to grow stations");
  await expect(page.getByTestId("sector-system")).toHaveCount(1);
  await expect(page.locator('[data-testid="sector-system"]:not([data-rank="Outpost"])')).toHaveCount(0);
});

test("session outside any project docks at Uncharted", async ({ page }) => {
  await openSector(page, fleet(["tool"], () => "/tmp/scratch"));
  await expect(sys(page, "uncharted").getByTestId("sector-ship")).toHaveCount(1);
  await expect(sys(page, "uncharted")).toContainText("Uncharted");
});

test("no sessions: the standard empty state, no idle systems", async ({ page }) => {
  await stubDash(page, { projects: PROJ, progress: PROG, live: [], windows: [], stats: {} });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+7");
  await expect(page.getByTestId("dash-empty")).toBeVisible();
  await expect(page.getByTestId("sector-system")).toHaveCount(0);
});

test("keys: → selects a system, Enter opens its first ship; ignored on a focused button", async ({ page }) => {
  await openSector(page, fleet(["tool"]));
  for (let i = 0; i < 3; i++) {
    await page.keyboard.press("ArrowRight");
    if (await page.locator('[data-testid="sector-system"][data-selected="true"][data-key="k1"]').count()) break;
  }
  await expect(page.locator('[data-testid="sector-system"][data-selected="true"]')).toHaveAttribute("data-key", "k1");
  await page.evaluate(() => { const b = document.createElement("button"); b.id = "probe"; b.textContent = "probe"; b.onclick = () => (window.__probe = 1); document.body.appendChild(b); b.focus(); });
  await page.keyboard.press("Enter");
  await expect.poll(() => page.evaluate(() => window.__probe)).toBe(1);
  expect(await page.evaluate(() => window.__calls.filter((c) => c[0] === "SelectSession").length)).toBe(0);
  await page.evaluate(() => { document.getElementById("probe").blur(); document.getElementById("probe").remove(); });
  await page.keyboard.press("Enter");
  await expect.poll(() => page.evaluate(() => window.__calls.filter((c) => c[0] === "SelectSession").length)).toBeGreaterThan(0);
});

test("still mode: nothing animates", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openSector(page, fleet(["permission", "tool"]));
  const running = await page.getByTestId("dash-sector").evaluate((el) => el.getAnimations({ subtree: true }).filter((a) => a.playState === "running").length);
  expect(running).toBe(0);
});

test("legend lists the station ladder, ship classes and encounters", async ({ page }) => {
  await openSector(page, fleet(["tool"]));
  const lg = page.getByTestId("sector-legend");
  for (const t of ["Outpost", "Settlement", "Station", "Citadel", "Capital", "Flagship carrier", "Interceptor", "Ion Storm"]) await expect(lg).toContainText(t);
});

test("every system carries a tooltip with its name, even when its label is dropped", async ({ page }) => {
  await openSector(page, fleet(["tool"], () => "/Users/me/old"));
  await expect(sys(page, "k3").locator("title").first()).toHaveText(/old · 200 h invested · Citadel/);
});

// Minors round 2 (Sector review): systems past the 32 slots are counted in the header.
test("more than 32 active projects: the header counts the ones off the map", async ({ page }) => {
  const n = 34;
  const projects = Array.from({ length: n }, (_, i) => ({ key: "p" + String(i).padStart(2, "0"), label: "p" + i, root: "/r/p" + i }));
  await openSector(page, { projects, progress: { locked: true, projects: [], live: [] }, ...fleet(Array(n).fill("tool"), (i) => "/r/p" + i) });
  await expect(page.getByTestId("sector-system")).toHaveCount(32);
  await expect(page.getByTestId("sector-offmap")).toHaveText(" · +2 off-map");
});
