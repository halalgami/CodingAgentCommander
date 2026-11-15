import { test, expect } from "@playwright/test";

import { boot, last, card, order } from "./sidebar-helpers.js";

const AYINA = (extra = {}) => ({ groups: [{ id: "g_aaaaaaaa", name: "Ayina", ...extra }], order: { ungrouped: ["@1"], g_aaaaaaaa: ["@2", "@3"] } });

test("new group from a card's context menu", async ({ page }) => {
  await boot(page);
  await card(page, "@2").click({ button: "right" });
  await page.getByTestId("card-menu-new").click();
  await page.getByTestId("new-group-input").fill("Ayina");
  await page.getByTestId("new-group-input").press("Enter");
  const h = page.getByTestId("group-header");
  await expect(h).toHaveCount(1);
  await expect(h).toContainText("Ayina");
  await expect(h).toContainText("(1)");
  const l = await last(page);
  expect(l.groups[0].name).toBe("Ayina");
  expect(l.order[l.groups[0].id]).toEqual(["@2"]);
});

test("context menu moves a card into an existing group", async ({ page }) => {
  await boot(page, { layout: AYINA() });
  await card(page, "@1").click({ button: "right" });
  await page.getByTestId("card-menu").getByRole("menuitem", { name: "Ayina" }).click();
  expect((await last(page)).order.g_aaaaaaaa).toEqual(["@2", "@3", "@1"]);
});

test("collapse hides cards but keeps amber", async ({ page }) => {
  await boot(page, { layout: AYINA(), stats: { "@3": { activity: "permission", status: "active" } } });
  await page.getByTestId("group-toggle").click();
  await expect(card(page, "@2")).toHaveCount(0);
  await expect(card(page, "@3")).toHaveCount(0);
  await expect(page.locator('[data-testid="group-attn"][data-attn="permission"]')).toBeVisible();
  expect((await last(page)).groups[0].collapsed).toBe(true);
});

test("delete group returns sessions to Ungrouped and kills nothing", async ({ page }) => {
  await boot(page, { layout: AYINA() });
  await page.getByTestId("group-menu").click();
  await page.getByTestId("group-delete").click();
  await expect(page.getByTestId("group-header")).toHaveCount(0);
  expect(await order(page)).toEqual(["@1", "@2", "@3"]);
  expect(await page.evaluate(() => window.__killed)).toEqual([]);
});

test("selecting a hidden session expands its group", async ({ page }) => {
  await boot(page, { layout: AYINA({ collapsed: true }) });
  await expect(card(page, "@2")).toHaveCount(0);
  await page.evaluate(() => { window.__app.sessionKey = "@2:1"; });
  await expect(card(page, "@2")).toBeVisible();
});

test("remember toggle and rename", async ({ page }) => {
  await boot(page, { layout: AYINA({ remember: true }) });
  await page.getByTestId("group-menu").click();
  await page.getByTestId("group-remember").click();
  expect((await last(page)).groups[0].remember).toBe(false);
  await page.getByTestId("group-toggle").dblclick();
  await page.getByTestId("group-rename").fill("Ayina RDP");
  await page.getByTestId("group-rename").press("Enter");
  expect((await last(page)).groups[0].name).toBe("Ayina RDP");
});

test("layout survives reload", async ({ page }) => {
  await boot(page, { layout: AYINA() });
  await expect(page.getByTestId("group-header")).toContainText("Ayina");
  expect(await order(page)).toEqual(["@1", "@2", "@3"]);
});

test("no groups looks like before: no headers, no Ungrouped label", async ({ page }) => {
  await boot(page);
  await expect(page.getByTestId("group-header")).toHaveCount(0);
  await expect(page.getByTestId("ungrouped-header")).toBeHidden();
  expect(await order(page)).toEqual(["@1", "@2", "@3"]);
});
