// frontend/tests/dashboard-modes.spec.js
import { test, expect } from "@playwright/test";
import { stubDash, S1 } from "./dashboard-helpers.js";

async function home(page) {
  await stubDash(page, { live: [S1] });
  await page.goto("/?nointro");
  await expect(page.getByTestId("dash-mode-switch")).toBeVisible();
}

test("one switcher button names the current mode; the menu is grouped with ⌘ keys", async ({ page }) => {
  await home(page);
  const sw = page.getByTestId("dash-mode-switch");
  await expect(sw).toContainText("Triage");
  await expect(page.getByTestId("dash-mode-menu")).toBeHidden();
  await sw.click();
  const menu = page.getByTestId("dash-mode-menu");
  await expect(menu).toBeVisible();
  await expect(menu.locator(".hd")).toHaveText(["VIEWS", "WORLDS", "FLEET"]);
  await expect(menu.getByRole("menuitemradio")).toHaveCount(7);
  await expect(page.getByTestId("dash-mode-triage")).toHaveAttribute("aria-checked", "true");
  await expect(page.getByTestId("dash-mode-cic")).toContainText("⌘6");
});

test("picking an item switches mode and closes the menu; the button follows", async ({ page }) => {
  await home(page);
  await page.getByTestId("dash-mode-switch").click();
  await page.getByTestId("dash-mode-river").click();
  await expect(page.getByTestId("dash-mode-menu")).toBeHidden();
  await expect(page.getByTestId("dash-mode-switch")).toContainText("River");
  await page.keyboard.press("Meta+5");
  await expect(page.getByTestId("dash-mode-switch")).toContainText("Fleet Ops");
});

test("Escape and an outside click close the menu", async ({ page }) => {
  await home(page);
  const sw = page.getByTestId("dash-mode-switch");
  await sw.click();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("dash-mode-menu")).toBeHidden();
  await expect(sw).toBeFocused();
  await sw.click();
  await page.mouse.click(900, 500); // well clear of the menu
  await expect(page.getByTestId("dash-mode-menu")).toBeHidden();
});

test("keyboard: ↓ moves through items, Enter picks", async ({ page }) => {
  await home(page);
  await page.getByTestId("dash-mode-switch").click();
  await page.keyboard.press("ArrowDown"); // focus lands on the checked item first, then moves
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("dash-mode-switch")).toContainText("River");
});

test("leaving home with the menu open does not bring it back open", async ({ page }) => {
  await home(page);
  await page.getByTestId("dash-mode-switch").click();
  await expect(page.getByTestId("dash-mode-menu")).toBeVisible();
  await page.evaluate(() => { window.__app.sessionKey = "@1:1"; window.__app.view = "terminal"; });
  await expect(page.getByTestId("dash-back")).toBeVisible();
  await page.evaluate(() => { window.__app.view = "home"; });
  await expect(page.getByTestId("dash-mode-switch")).toBeVisible();
  await expect(page.getByTestId("dash-mode-menu")).toBeHidden();
});

// Minors sweep (Sector Map review): Tab-out left the menu open; no Home/End;
// group headers had no role.
test("Tab out of the menu closes it", async ({ page }) => {
  await home(page);
  await page.getByTestId("dash-mode-switch").click();
  await expect(page.getByTestId("dash-mode-menu")).toBeVisible();
  await page.getByTestId("dash-mode-sector").focus();
  await page.keyboard.press("Tab");
  await expect(page.getByTestId("dash-mode-menu")).toBeHidden();
});

test("Home and End jump to the first and last mode", async ({ page }) => {
  await home(page);
  await page.getByTestId("dash-mode-switch").click();
  await page.keyboard.press("End");
  await expect(page.getByTestId("dash-mode-sector")).toBeFocused();
  await page.keyboard.press("Home");
  await expect(page.getByTestId("dash-mode-triage")).toBeFocused();
});

test("menu groups are labelled groups", async ({ page }) => {
  await home(page);
  await page.getByTestId("dash-mode-switch").click();
  const groups = page.getByTestId("dash-mode-menu").locator('[role="group"]');
  await expect(groups).toHaveCount(3);
  await expect(groups.nth(2)).toHaveAttribute("aria-label", /fleet/i);
  await expect(groups.nth(2).locator('[role="menuitemradio"]')).toHaveCount(3);
});

// Minors sweep: after picking Sector or CIC from the menu, ←/→ needed a click on the map first.
test("arrow keys work on the map right after picking it from the menu", async ({ page }) => {
  await home(page);
  await page.getByTestId("dash-mode-switch").click();
  await page.getByTestId("dash-mode-sector").click();
  await expect(page.getByTestId("dash-sector")).toBeVisible();
  await page.keyboard.press("ArrowRight");
  await expect(page.locator('[data-testid="sector-system"][data-selected="true"]')).toHaveCount(1);
});

// Final review I1: WebKit blurs the focused item with no relatedTarget when the
// switcher is clicked; that must not close the menu (the click then reopened it).
test("a focusout with no target does not close the menu", async ({ page }) => {
  await home(page);
  await page.getByTestId("dash-mode-switch").click();
  await expect(page.getByTestId("dash-mode-menu")).toBeVisible();
  await page.evaluate(() => document.activeElement.dispatchEvent(new FocusEvent("focusout", { bubbles: true, relatedTarget: null })));
  await expect(page.getByTestId("dash-mode-menu")).toBeVisible();
});
