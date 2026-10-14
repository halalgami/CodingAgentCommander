import { test, expect } from "@playwright/test";

import { stubDash, S1, pickMode } from "./dashboard-helpers.js";

test("lands on the dashboard; switcher toggles modes", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-home")).toBeVisible();
  await pickMode(page, "river");
  await expect(page.getByTestId("dash-river")).toBeVisible();
  await pickMode(page, "triage");
  await expect(page.getByTestId("dash-triage")).toBeVisible();
});

test("⌘1/⌘2 pick a mode, ⌘D toggles home ↔ terminal, Esc never leaves the terminal", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await page.keyboard.press("Meta+2");
  await expect(page.getByTestId("dash-river")).toBeVisible();
  await page.keyboard.press("Meta+1");
  await expect(page.getByTestId("dash-triage")).toBeVisible();
  await page.getByTestId("session-name").first().click();
  await expect(page.getByTestId("dash-home")).toHaveCount(0);
  await expect(page.getByTestId("dash-back")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("dash-home")).toHaveCount(0);
  await page.keyboard.press("Meta+d");
  await expect(page.getByTestId("dash-home")).toBeVisible();
  await page.keyboard.press("Meta+d");
  await expect(page.getByTestId("dash-home")).toHaveCount(0);
});

test("← Dashboard and the sidebar entry go home", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await page.getByTestId("session-name").first().click();
  await page.getByTestId("dash-back").click();
  await expect(page.getByTestId("dash-home")).toBeVisible();
  await page.getByTestId("session-name").first().click();
  await page.getByTestId("sidebar-dashboard").click();
  await expect(page.getByTestId("dash-home")).toBeVisible();
});

// Review Focus 4: keys typed at home must not reach the hidden terminal.
test("going home moves focus out of the terminal", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await page.getByTestId("session-name").first().click();
  await page.keyboard.press("Meta+d");
  const inXterm = await page.evaluate(() => !!document.activeElement?.closest?.(".xterm"));
  expect(inXterm).toBe(false);
});

test("no sessions shows the empty state", async ({ page }) => {
  await stubDash(page, { live: [], windows: [] });
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-empty")).toBeVisible();
});

// Review fix: Tab or a drawer's focus restore must not reach the hidden xterm.
test("the terminal layer is inert while home", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await page.getByTestId("session-name").first().click();
  await expect(page.getByTestId("term-layer")).not.toHaveAttribute("inert", /.*/);
  await page.keyboard.press("Meta+d");
  await expect(page.getByTestId("term-layer")).toHaveAttribute("inert", /.*/);
});

// Spec §3: in home, 1–9 opens a session; arrows move focus between sessions.
test("number keys open a session and arrows move focus", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await page.getByTestId("dash-home").focus();
  await page.keyboard.press("ArrowDown");
  await expect(page.getByTestId("tile-action").first()).toBeFocused();
  await page.keyboard.press("1");
  await expect(page.getByTestId("dash-back")).toBeVisible();
  expect(await page.evaluate(() => window.__calls)).toContainEqual(["SelectSession", "@1"]);
});

// Minors sweep (phase 4 review): after ⌘D back, typing went nowhere until a click.
test("⌘D back to the terminal puts focus in it", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await page.getByTestId("session-name").first().click();
  await page.keyboard.press("Meta+d");
  await expect(page.getByTestId("dash-home")).toBeVisible();
  await page.keyboard.press("Meta+d");
  await expect(page.getByTestId("dash-home")).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => !!document.activeElement?.closest?.(".xterm"))).toBe(true);
});

// Final review I2: ⌘D with a drawer open must not pull focus into the hidden terminal.
test("⌘D leaves focus in an open drawer", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await page.getByTestId("session-name").first().click();
  await page.keyboard.press("Meta+d");
  await expect(page.getByTestId("dash-home")).toBeVisible();
  await page.evaluate(() => {
    const d = document.createElement("div"); d.setAttribute("role", "dialog"); d.id = "probe-dialog";
    const i = document.createElement("input"); i.id = "probe-input"; d.appendChild(i); document.body.appendChild(d); i.focus();
  });
  await page.keyboard.press("Meta+d");
  await expect(page.getByTestId("dash-home")).toHaveCount(0);
  await page.waitForTimeout(200);
  expect(await page.evaluate(() => document.activeElement?.id)).toBe("probe-input");
});

// Minors round 2 (prompt-attachments review): an error from startup (hooks could
// not be installed) was emitted before the UI listened and never shown.
test("errors held from startup are shown once the UI is listening", async ({ page }) => {
  await stubDash(page, { live: [S1] });
  await page.addInitScript(() => {
    window.go.main.App.TakeStartupErrors = async () => ["Couldn't install Commander's Claude Code hooks"];
  });
  await page.goto("/?nointro");
  await expect(page.getByTestId("toast").filter({ hasText: "Couldn't install" })).toBeVisible();
});
