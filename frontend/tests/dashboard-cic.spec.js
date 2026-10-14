// frontend/tests/dashboard-cic.spec.js
import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

const PROJ = [{ key: "k1", label: "proj", root: "/Users/me/proj" }, { key: "k2", label: "zeta", root: "/Users/me/zeta" }];
const W = (id) => ({ windowID: "@" + id, name: "w" + id });
const st = (id, cwd, model = "claude-opus-5-5") => ({ sessionID: "s" + id, model, cwd, status: "active", band: "green" });

async function openCic(page, { live, windows, stats, timeline } = {}) {
  await stubDash(page, { projects: PROJ, live, windows, stats, timeline });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+6");
  await expect(page.getByTestId("dash-cic")).toBeVisible();
}
const two = (states) => ({
  live: states.map((state, i) => ({ ...S1, id: "s" + (i + 1), state, title: "task " + (i + 1), sinceMs: Date.now() - (i + 1) * 60000 })),
  windows: states.map((_, i) => W(i + 1)),
  stats: Object.fromEntries(states.map((_, i) => ["@" + (i + 1), st(i + 1, "/Users/me/proj")])),
});

test("⌘6 opens CIC; contacts land on their attention ring; one sector for one project", async ({ page }) => {
  await openCic(page, two(["permission", "tool", "idle"]));
  await expect(page.getByTestId("dash-mode-cic")).toHaveAttribute("aria-checked", "true");
  await expect(page.getByTestId("cic-contact")).toHaveCount(3);
  await expect(page.locator('[data-testid="cic-contact"][data-key="@1"]')).toHaveAttribute("data-ring", "needs");
  await expect(page.locator('[data-testid="cic-contact"][data-key="@2"]')).toHaveAttribute("data-ring", "working");
  await expect(page.locator('[data-testid="cic-contact"][data-key="@3"]')).toHaveAttribute("data-ring", "quiet");
  await expect(page.getByTestId("cic-sector")).toHaveCount(1);
  await expect(page.locator('[data-testid="cic-divider"]')).toHaveCount(0);
  await expect(page.getByTestId("cic-status-awaiting")).toHaveText("1");
  await expect(page.getByTestId("cic-status-engaged")).toHaveText("1");
  await expect(page.getByTestId("cic-status-holding")).toHaveText("1");
});

test("two waiting: one orders box (oldest), the other a pill; selecting moves the box; no Approve", async ({ page }) => {
  await openCic(page, two(["permission", "permission"]));
  await expect(page.getByTestId("cic-orders")).toHaveCount(1);
  await expect(page.getByTestId("cic-orders")).toHaveAttribute("data-key", "@2");
  await expect(page.getByTestId("cic-pill")).toHaveCount(1);
  await expect(page.getByTestId("cic-orders").getByRole("button", { name: /approve/i })).toHaveCount(0);
  await page.locator('[data-testid="cic-contact"][data-key="@1"]').click();
  await expect(page.getByTestId("cic-orders")).toHaveAttribute("data-key", "@1");
});

test("keys: ←/→ cycle selection, Enter opens; ignored while typing in an input", async ({ page }) => {
  await openCic(page, two(["tool", "tool"]));
  await page.keyboard.press("ArrowRight");
  const sel = page.locator('[data-testid="cic-contact"][data-selected="true"]');
  await expect(sel).toHaveCount(1);
  const first = await sel.getAttribute("data-key");
  await page.keyboard.press("ArrowRight");
  await expect(sel).not.toHaveAttribute("data-key", first);
  await page.keyboard.press("Enter");
  await expect.poll(() => page.evaluate(() => window.__calls.filter((c) => c[0] === "SelectSession").length)).toBeGreaterThan(0);
});

test("keys do nothing while focus is in an input", async ({ page }) => {
  await openCic(page, two(["tool", "tool"]));
  await page.evaluate(() => { const i = document.createElement("input"); i.id = "probe"; document.body.appendChild(i); i.focus(); });
  await page.keyboard.press("ArrowRight");
  await expect(page.locator('[data-testid="cic-contact"][data-selected="true"]')).toHaveCount(0);
});

test("crowded sector: labels never overlap and the rim shows +N", async ({ page }) => {
  const states = Array(10).fill("tool");
  await openCic(page, two(states));
  const boxes = await page.locator('[data-testid="cic-label"], [data-testid="cic-cs"]').evaluateAll((els) => els.map((e) => { const r = e.getBoundingClientRect(); return [r.left, r.top, r.right, r.bottom]; }));
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
    const [a, b] = [boxes[i], boxes[j]];
    expect(a[2] <= b[0] + 1 || b[2] <= a[0] + 1 || a[3] <= b[1] + 1 || b[3] <= a[1] + 1).toBe(true);
  }
  await expect(page.getByTestId("cic-sector-more")).toHaveText(/\+\d+/);
});

test("two projects → two sectors with a divider each", async ({ page }) => {
  await openCic(page, {
    live: [{ ...S1, id: "s1" }, { ...S1, id: "s2", cwd: "/Users/me/zeta" }],
    windows: [W(1), W(2)],
    stats: { "@1": st(1, "/Users/me/proj"), "@2": st(2, "/Users/me/zeta") },
  });
  await expect(page.getByTestId("cic-sector")).toHaveCount(2);
  await expect(page.getByTestId("cic-divider")).toHaveCount(2);
});

test("comms log: tool and wait marks, newest first", async ({ page }) => {
  const now = Date.now();
  const d = two(["permission"]);
  await openCic(page, { ...d, timeline: { s1: [{ session: "s1", at: now - 5000, kind: "shell", label: "npm test" }, { session: "s1", at: now - 2000, kind: "wait", label: "start" }] } });
  const li = page.getByTestId("cic-comms").locator("li");
  await expect(li.first()).toContainText("AWAITING ORDERS");
  await expect(li.first()).toHaveAttribute("data-tone", "amber");
  await expect(page.getByTestId("cic-comms")).toContainText("TORPEDO SALVO · npm test");
});

test("subagents: up to 4 blips and +N", async ({ page }) => {
  const crew = Array.from({ length: 6 }, (_, i) => ({ id: "a" + i, type: "Explore", state: "working", depth: 1 }));
  await openCic(page, { live: [{ ...S1, crew }], windows: [W(1)], stats: { "@1": st(1, "/Users/me/proj") } });
  await expect(page.getByTestId("cic-blip")).toHaveCount(4);
  await expect(page.getByTestId("cic-blip-more")).toHaveText("+2");
});

test("still mode: the sweep does not run", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openCic(page, two(["tool"]));
  const running = await page.getByTestId("dash-cic").evaluate((el) => el.getAnimations({ subtree: true }).filter((a) => a.playState === "running").length);
  expect(running).toBe(0);
});

test("narrow window hides the status column", async ({ page }) => {
  await page.setViewportSize({ width: 820, height: 700 });
  await openCic(page, two(["tool"]));
  await expect(page.getByTestId("cic-legend")).toBeHidden();
  await expect(page.getByTestId("cic-plot")).toBeVisible();
});

test("Enter on a focused button is the button's, not CIC's", async ({ page }) => {
  await openCic(page, two(["tool", "tool"]));
  await page.locator('[data-testid="cic-contact"][data-key="@1"]').click();
  await page.evaluate(() => { const b = document.createElement("button"); b.id = "probe"; b.textContent = "probe"; b.onclick = () => (window.__probe = 1); document.body.appendChild(b); b.focus(); });
  await page.keyboard.press("Enter");
  await expect.poll(() => page.evaluate(() => window.__probe)).toBe(1);
  expect(await page.evaluate(() => window.__calls.filter((c) => c[0] === "SelectSession").length)).toBe(0);
});

test("crowded wedge: 16 projects, 6 sessions in one narrow wedge — no label overlaps at all", async ({ page }) => {
  const projects = Array.from({ length: 16 }, (_, i) => ({ key: "p" + i, label: "proj" + String(i).padStart(2, "0"), root: "/r/p" + i }));
  const cwds = [...Array(6).fill("/r/p0"), ...projects.slice(1).map((p) => p.root)];
  const live = cwds.map((cwd, i) => ({ ...S1, id: "s" + (i + 1), state: "tool", cwd, title: "task " + (i + 1) }));
  const windows = cwds.map((_, i) => W(i + 1));
  const stats = Object.fromEntries(cwds.map((cwd, i) => ["@" + (i + 1), st(i + 1, cwd)]));
  await stubDash(page, { projects, live, windows, stats });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+6");
  await expect(page.getByTestId("cic-contact")).toHaveCount(21);
  const boxes = await page.locator('[data-testid="cic-label"], [data-testid="cic-cs"]').evaluateAll((els) => els.map((e) => { const r = e.getBoundingClientRect(); return [r.left, r.top, r.right, r.bottom]; }));
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
    const [a, b] = [boxes[i], boxes[j]];
    expect(a[2] <= b[0] + 1 || b[2] <= a[0] + 1 || a[3] <= b[1] + 1 || b[3] <= a[1] + 1).toBe(true);
  }
});

// Owner, 2026-10-14: comms scrolled sideways (long paths) and stopped short of the panel bottom.
test("comms fill their section: no sideways scroll, full height, full text on hover", async ({ page }) => {
  const now = Date.now();
  const long = "/Users/halalgami/Documents/AlgamProjects/SomeVeryLongProjectName/internal/a/b/c/d/e/file_with_a_long_name.go";
  const timeline = { s1: Array.from({ length: 40 }, (_, i) => ({ session: "s1", at: now - i * 5000, kind: i % 2 ? "edit" : "shell", label: i % 3 ? long : "npm test" })) };
  await openCic(page, { ...two(["tool"]), timeline });
  const list = page.getByTestId("cic-comms");
  const [sw, cw, bottom, colBottom] = await list.evaluate((el) => [el.scrollWidth, el.clientWidth, el.getBoundingClientRect().bottom, el.closest("aside").getBoundingClientRect().bottom]);
  expect(sw).toBeLessThanOrEqual(cw);
  expect(colBottom - bottom).toBeLessThan(24); // reaches the bottom of the panel (padding only)
  await expect(list.locator("li").first().locator(".msg")).toHaveAttribute("title", /TORPEDO SALVO|BEAM/);
});

// Minors round 2 (CIC review): the orders box covered the plot's corner on
// narrow windows, and on very wide ones the leader ended far from the box.
test("orders box: plot never under it on a narrow window", async ({ page }) => {
  await page.setViewportSize({ width: 1290, height: 720 }); // plot area about square: the corner is tight
  await openCic(page, two(["permission", "tool"]));
  const box = await page.getByTestId("cic-orders").boundingBox();
  const bezel = await page.locator(".bezel").boundingBox();
  const r = bezel.width / 2, cx = bezel.x + r, cy = bezel.y + bezel.height / 2;
  // the box's bottom-right corner must sit outside the bezel circle
  expect(Math.hypot(box.x + box.width - cx, box.y + box.height - cy)).toBeGreaterThan(r);
});

test("orders box: the leader ends at the box on a very wide window", async ({ page }) => {
  await page.setViewportSize({ width: 2200, height: 700 });
  await openCic(page, two(["permission", "tool"]));
  const box = await page.getByTestId("cic-orders").boundingBox();
  const end = await page.getByTestId("cic-leader").evaluate((l) => {
    const svg = l.ownerSVGElement, p = svg.createSVGPoint();
    p.x = l.x2.baseVal.value; p.y = l.y2.baseVal.value;
    const s = p.matrixTransform(svg.getScreenCTM());
    return [s.x, s.y];
  });
  expect(Math.hypot(end[0] - (box.x + box.width), end[1] - (box.y + box.height))).toBeLessThan(12);
});

// Minors round 2 (CIC review): rim sector labels overlapped with many projects.
test("20 projects: rim sector labels never overlap", async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const n = 20;
  const projects = Array.from({ length: n }, (_, i) => ({ key: "p" + i, label: "client-project-number-" + i, root: "/r/p" + i }));
  await stubDash(page, {
    projects,
    live: projects.map((p, i) => ({ ...S1, id: "s" + (i + 1), state: "tool", cwd: p.root, title: "t" + i })),
    windows: projects.map((_, i) => W(i + 1)),
    stats: Object.fromEntries(projects.map((p, i) => ["@" + (i + 1), st(i + 1, p.root)])),
  });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+6");
  await expect(page.getByTestId("cic-sector")).toHaveCount(n);
  const boxes = await page.locator('[data-testid="cic-sector"] text').evaluateAll((els) => els.map((e) => { const r = e.getBoundingClientRect(); return [r.left, r.top, r.right, r.bottom, e.closest("g").dataset.key]; }));
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
    const [a, b] = [boxes[i], boxes[j]];
    if (a[4] === b[4]) continue; // a sector's own two lines
    expect(a[2] <= b[0] + 1 || b[2] <= a[0] + 1 || a[3] <= b[1] + 1 || b[3] <= a[1] + 1).toBe(true);
  }
});
