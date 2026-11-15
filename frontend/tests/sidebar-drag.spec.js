import { test, expect } from "@playwright/test";
import { boot, last, card, order } from "./sidebar-helpers.js";

// Real pointer down / stepped move / up — never synthetic shortcuts.
async function dragTo(page, from, to, { yFrac = 0.5, hold } = {}) {
  const a = await from.boundingBox();
  const b = await to.boundingBox();
  await page.mouse.move(a.x + 20, a.y + a.height / 2);
  await page.mouse.down();
  await page.mouse.move(a.x + 22, a.y + a.height / 2 + 6, { steps: 3 });
  await page.mouse.move(b.x + 20, b.y + b.height * yFrac, { steps: 8 });
  if (hold) await hold();
  await page.mouse.up();
}
const nameOf = (page, id) => card(page, id).getByTestId("session-name");
const TWO = { groups: [{ id: "g_aaaaaaaa", name: "Ayina" }, { id: "g_bbbbbbbb", name: "Commander" }],
  order: { ungrouped: ["@1"], g_aaaaaaaa: ["@2", "@3"], g_bbbbbbbb: ["@4"] } };
const FOUR = [["@1", "W1"], ["@2", "W2"], ["@3", "W3"], ["@4", "W4"]];

test("drag reorders within a section", async ({ page }) => {
  await boot(page);
  await dragTo(page, nameOf(page, "@3"), card(page, "@1"), { yFrac: 0.2 });
  await expect.poll(() => order(page)).toEqual(["@3", "@1", "@2"]);
  expect((await last(page)).order.ungrouped).toEqual(["@3", "@1", "@2"]);
  expect(await page.evaluate(() => window.__selected)).toEqual([]);
});

test("drag moves a card into another group", async ({ page }) => {
  await boot(page, { sessions: FOUR, layout: TWO });
  await dragTo(page, nameOf(page, "@1"), card(page, "@4"), { yFrac: 0.8 });
  await expect.poll(async () => (await last(page))?.order.g_bbbbbbbb).toEqual(["@4", "@1"]);
});

test("drop on a group header appends to that group", async ({ page }) => {
  await boot(page, { sessions: FOUR, layout: TWO });
  await dragTo(page, nameOf(page, "@1"), page.locator('[data-group-id="g_aaaaaaaa"]'));
  await expect.poll(async () => (await last(page))?.order.g_aaaaaaaa).toEqual(["@2", "@3", "@1"]);
});

test("drag a header reorders groups", async ({ page }) => {
  await boot(page, { sessions: FOUR, layout: TWO });
  await dragTo(page, page.locator('[data-group-id="g_bbbbbbbb"] [data-testid="group-toggle"]'), page.locator('[data-group-id="g_aaaaaaaa"]'), { yFrac: 0.1 });
  await expect.poll(async () => (await last(page))?.groups.map((g) => g.id)).toEqual(["g_bbbbbbbb", "g_aaaaaaaa"]);
  // The click after a drag must not toggle the group.
  expect((await last(page)).groups.every((g) => !g.collapsed)).toBe(true);
});

test("drag onto + New group creates a group", async ({ page }) => {
  await boot(page);
  await dragTo(page, nameOf(page, "@2"), page.getByTestId("new-group"));
  await page.getByTestId("new-group-input").fill("Ayina");
  await page.getByTestId("new-group-input").press("Enter");
  const l = await last(page);
  expect(l.order[l.groups[0].id]).toEqual(["@2"]);
});

test("a click still selects and does not move", async ({ page }) => {
  await boot(page);
  await nameOf(page, "@2").click();
  await expect.poll(() => page.evaluate(() => window.__selected)).toEqual(["@2"]);
  expect(await page.evaluate(() => window.__layouts.length)).toBe(0);
});

test("Esc cancels a drag", async ({ page }) => {
  await boot(page);
  await dragTo(page, nameOf(page, "@3"), card(page, "@1"), { yFrac: 0.2, hold: () => page.keyboard.press("Escape") });
  expect(await page.evaluate(() => window.__layouts.length)).toBe(0);
  expect(await order(page)).toEqual(["@1", "@2", "@3"]);
});

test("drag survives a poll", async ({ page }) => {
  await boot(page);
  await dragTo(page, nameOf(page, "@3"), card(page, "@1"), {
    yFrac: 0.2,
    hold: () => page.evaluate(() => { window.__app.sessions = window.__app.sessions.map((s) => ({ ...s })); }),
  });
  await expect.poll(async () => (await last(page))?.order.ungrouped).toEqual(["@3", "@1", "@2"]);
});

test("dropping a card on the terminal does nothing", async ({ page }) => {
  await boot(page);
  await dragTo(page, nameOf(page, "@2"), page.locator("main, .main, body").first(), { yFrac: 0.5 });
  expect(await page.evaluate(() => window.__dropped)).toEqual([]);
});

test("Alt+ArrowDown moves the focused card, across the section edge", async ({ page }) => {
  await boot(page, { sessions: FOUR, layout: TWO });
  await nameOf(page, "@1").focus();
  await page.keyboard.press("Alt+ArrowDown");
  await expect.poll(async () => (await last(page))?.order.g_aaaaaaaa).toEqual(["@1", "@2", "@3"]);
  await nameOf(page, "@3").focus();
  await page.keyboard.press("Alt+ArrowUp");
  await expect.poll(async () => (await last(page))?.order.g_aaaaaaaa).toEqual(["@1", "@3", "@2"]);
});

test("Alt+Shift+ArrowUp moves a focused group header", async ({ page }) => {
  await boot(page, { sessions: FOUR, layout: TWO });
  await page.locator('[data-group-id="g_bbbbbbbb"] [data-testid="group-toggle"]').focus();
  await page.keyboard.press("Alt+Shift+ArrowUp");
  await expect.poll(async () => (await last(page))?.groups.map((g) => g.id)).toEqual(["g_bbbbbbbb", "g_aaaaaaaa"]);
});
