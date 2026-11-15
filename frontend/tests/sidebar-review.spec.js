import { test, expect } from "@playwright/test";
import { boot, last, card, order } from "./sidebar-helpers.js";

const TWO = { groups: [{ id: "g_aaaaaaaa", name: "Ayina" }, { id: "g_bbbbbbbb", name: "Commander" }],
  order: { ungrouped: ["@1"], g_aaaaaaaa: ["@2", "@3"], g_bbbbbbbb: ["@4"] } };
const FOUR = [["@1", "W1"], ["@2", "W2"], ["@3", "W3"], ["@4", "W4"]];
const poll = (page) => page.evaluate(() => { window.__app.sessions = window.__app.sessions.map((s) => ({ ...s })); });
const name = (page, id) => card(page, id).getByTestId("session-name");

// Wails runs each binding on its own goroutine: a reload can reach Go before
// the save that preceded it. Model that: saves land late, loads are instant.
async function slowSaves(page) {
  await page.evaluate(() => {
    window.go.main.App.SetSidebarLayout = (l) => new Promise((r) => setTimeout(() => {
      window.__layouts.push(l); window.__layout = JSON.parse(JSON.stringify(l)); r(null);
    }, 400));
  });
}

test("a reload during an in-flight save does not snap the edit back", async ({ page }) => {
  await boot(page);
  await slowSaves(page);
  await name(page, "@1").focus();
  await page.keyboard.press("Alt+ArrowDown");
  await poll(page);
  await page.waitForTimeout(150);
  expect(await order(page)).toEqual(["@2", "@1", "@3"]);
  await page.waitForTimeout(500);
  await poll(page);
  await page.waitForTimeout(100);
  expect(await order(page)).toEqual(["@2", "@1", "@3"]);
});

test("saves reach Go in the order they were made", async ({ page }) => {
  await boot(page);
  await page.evaluate(() => {
    let n = 0;
    window.go.main.App.SetSidebarLayout = (l) => new Promise((r) => setTimeout(() => {
      window.__layouts.push(l); window.__layout = JSON.parse(JSON.stringify(l)); r(null);
    }, n++ === 0 ? 300 : 10));
  });
  await name(page, "@1").focus();
  await page.keyboard.press("Alt+ArrowDown");
  await page.keyboard.press("Alt+ArrowDown");
  await page.waitForTimeout(600);
  expect((await page.evaluate(() => window.__layout)).order.ungrouped).toEqual(["@2", "@3", "@1"]);
});

test("collapsing the group of the selected session stays collapsed", async ({ page }) => {
  await boot(page, { sessions: FOUR, layout: TWO });
  await page.evaluate(() => { window.__app.sessionKey = "@2:1"; });
  await page.locator('[data-group-id="g_aaaaaaaa"] [data-testid="group-toggle"]').click();
  await page.waitForTimeout(200);
  await expect(card(page, "@2")).toHaveCount(0);
  expect((await last(page)).groups[0].collapsed).toBe(true);
});

test("two Alt+ArrowDown presses in a row, across a section edge", async ({ page }) => {
  await boot(page, { sessions: FOUR, layout: TWO });
  await name(page, "@1").focus();
  await page.keyboard.press("Alt+ArrowDown");
  await page.keyboard.press("Alt+ArrowDown");
  await expect.poll(async () => (await last(page))?.order.g_aaaaaaaa).toEqual(["@2", "@1", "@3"]);
});

// pointerup with no click after it (released outside the window, or an engine
// that sends no click): the next real click must still work.
test("a drag with no trailing click does not eat the next click", async ({ page }) => {
  await boot(page);
  await page.evaluate(async () => {
    const el = document.querySelector('[data-window-id="@3"]');
    const b = el.getBoundingClientRect();
    const ev = (t, y) => new PointerEvent(t, { bubbles: true, button: 0, clientX: b.left + 20, clientY: y, pointerId: 1 });
    el.dispatchEvent(ev("pointerdown", b.top + 10));
    window.dispatchEvent(ev("pointermove", b.top - 40));
    window.dispatchEvent(ev("pointerup", b.top - 40));
    await new Promise((r) => setTimeout(r, 50));
  });
  await name(page, "@2").click();
  await expect.poll(() => page.evaluate(() => window.__selected)).toEqual(["@2"]);
});

test("pointercancel ends the drag and polls resume", async ({ page }) => {
  await boot(page);
  await page.evaluate(() => {
    const el = document.querySelector('[data-window-id="@3"]');
    const b = el.getBoundingClientRect();
    const ev = (t, y) => new PointerEvent(t, { bubbles: true, button: 0, clientX: b.left + 20, clientY: y, pointerId: 1 });
    el.dispatchEvent(ev("pointerdown", b.top + 10));
    window.dispatchEvent(ev("pointermove", b.top - 40));
    window.dispatchEvent(ev("pointercancel", b.top - 40));
    window.__layout = { groups: [{ id: "g_cccccccc", name: "Fresh" }], order: { ungrouped: [], g_cccccccc: ["@1"] } };
  });
  await poll(page);
  await expect(page.getByTestId("group-header")).toContainText("Fresh");
});
